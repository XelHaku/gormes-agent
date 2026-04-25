package gateway

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const pairingStatusKind = "gormes-gateway-pairing"

const (
	pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	pairingCodeLength   = 8

	pairingCodeTTL          = time.Hour
	pairingRequestRateLimit = 10 * time.Minute
	pairingLockoutDuration  = time.Hour

	maxPendingPairingCodesPerPlatform = 3
	maxPairingApprovalFailures        = 5
)

// PairingPlatformState is the operator-facing per-platform pairing state.
type PairingPlatformState string

const (
	PairingPlatformStatePaired   PairingPlatformState = "paired"
	PairingPlatformStateUnpaired PairingPlatformState = "unpaired"
)

// PairingDegradedReason classifies read-only pairing-state degradation.
type PairingDegradedReason string

const (
	PairingDegradedMissing          PairingDegradedReason = "missing"
	PairingDegradedCorrupt          PairingDegradedReason = "corrupt"
	PairingDegradedPermissionDenied PairingDegradedReason = "permission_denied"
	PairingDegradedReadFailed       PairingDegradedReason = "read_failed"
	PairingDegradedRateLimited      PairingDegradedReason = "rate_limited"
	PairingDegradedMaxPending       PairingDegradedReason = "max_pending"
	PairingDegradedExpired          PairingDegradedReason = "expired"
	PairingDegradedLockedOut        PairingDegradedReason = "locked_out"
	PairingDegradedAllowlistDenied  PairingDegradedReason = "allowlist_denied"
	PairingDegradedUnresolvedUser   PairingDegradedReason = "unresolved_user"
)

// PairingCodeStatus is the state transition produced by a pairing-code request.
type PairingCodeStatus string

const (
	PairingCodeIssued          PairingCodeStatus = "issued"
	PairingCodeRateLimited     PairingCodeStatus = "rate_limited"
	PairingCodeMaxPending      PairingCodeStatus = "max_pending"
	PairingCodeLockedOut       PairingCodeStatus = "locked_out"
	PairingCodeAllowlistDenied PairingCodeStatus = "allowlist_denied"
	PairingCodeUnresolvedUser  PairingCodeStatus = "unresolved_user"
)

// PairingApprovalStatus is the state transition produced by an approval
// attempt against a code.
type PairingApprovalStatus string

const (
	PairingApprovalApproved  PairingApprovalStatus = "approved"
	PairingApprovalInvalid   PairingApprovalStatus = "invalid"
	PairingApprovalExpired   PairingApprovalStatus = "expired"
	PairingApprovalLockedOut PairingApprovalStatus = "locked_out"
)

// PairingCodeRequest carries the platform-neutral state needed to request a
// pairing code. It intentionally excludes response-copy and adapter behavior.
type PairingCodeRequest struct {
	Platform        string
	UserID          string
	UserName        string
	AllowlistDenied bool
}

// PairingCodeResult reports whether a code was issued or why policy blocked it.
type PairingCodeResult struct {
	Status    PairingCodeStatus
	Code      string
	Reason    PairingDegradedReason
	ExpiresAt time.Time
	RetryAt   time.Time
}

// PairingApprovalResult reports whether approval succeeded or why it failed.
type PairingApprovalResult struct {
	Status         PairingApprovalStatus
	Reason         PairingDegradedReason
	UserID         string
	UserName       string
	LockedOutUntil time.Time
}

// PairingPendingRecord is one pending pairing request in the read model.
type PairingPendingRecord struct {
	Platform   string    `json:"platform"`
	Code       string    `json:"code"`
	UserID     string    `json:"user_id"`
	UserName   string    `json:"user_name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	AgeSeconds int64     `json:"age_seconds"`
}

// PairingApprovedRecord is one approved user in the read model.
type PairingApprovedRecord struct {
	Platform   string    `json:"platform"`
	UserID     string    `json:"user_id"`
	UserName   string    `json:"user_name,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
}

// PairingPlatformStatus summarizes whether each platform has approved users.
type PairingPlatformStatus struct {
	Platform      string               `json:"platform"`
	State         PairingPlatformState `json:"state"`
	PendingCount  int                  `json:"pending_count"`
	ApprovedCount int                  `json:"approved_count"`
}

// PairingDegradedEvidence records read-model degradation and pairing-policy
// attempts operators need to see in status output.
type PairingDegradedEvidence struct {
	Reason   PairingDegradedReason `json:"reason"`
	Path     string                `json:"path"`
	Message  string                `json:"message"`
	Platform string                `json:"platform,omitempty"`
	UserID   string                `json:"user_id,omitempty"`
	Code     string                `json:"code,omitempty"`
	At       time.Time             `json:"at,omitempty"`
	Until    time.Time             `json:"until,omitempty"`
	Count    int                   `json:"count,omitempty"`
}

// PairingStatus is the deterministic, operator-facing pairing readout.
type PairingStatus struct {
	Kind      string                    `json:"kind"`
	Version   int                       `json:"version"`
	Platforms []PairingPlatformStatus   `json:"platforms"`
	Pending   []PairingPendingRecord    `json:"pending"`
	Approved  []PairingApprovedRecord   `json:"approved"`
	Degraded  []PairingDegradedEvidence `json:"degraded,omitempty"`
	UpdatedAt time.Time                 `json:"updated_at,omitempty"`
}

// PairingStore persists gateway pairing state as one atomic JSON read model.
type PairingStore struct {
	path      string
	now       func() time.Time
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
	mu        sync.Mutex
}

// DefaultPairingStorePath returns the XDG data path for the pairing read model.
func DefaultPairingStorePath() string {
	return filepath.Join(xdgDataHomeForPairing(), "gormes", "pairing.json")
}

// NewXDGPairingStore returns a pairing store under the XDG data root.
func NewXDGPairingStore() *PairingStore {
	return NewPairingStore(DefaultPairingStorePath())
}

// NewPairingStore returns a JSON-backed pairing store at path.
func NewPairingStore(path string) *PairingStore {
	return &PairingStore{
		path:      path,
		now:       func() time.Time { return time.Now().UTC() },
		readFile:  os.ReadFile,
		writeFile: atomicWritePairingFile,
	}
}

// PairingCodeRequestFromInbound extracts the identity used by pairing policy
// from a gateway event. Telegram private-chat events may fall back to chat.id
// when from_user is unavailable; group/channel events do not.
func PairingCodeRequestFromInbound(ev InboundEvent, allowlistDenied bool) PairingCodeRequest {
	userID := ev.PairingUserID()
	userName := strings.TrimSpace(ev.UserName)
	if userName == "" && userID != "" && userID == strings.TrimSpace(ev.ChatID) && ev.IsDirectMessage() {
		userName = strings.TrimSpace(ev.ChatName)
	}
	return PairingCodeRequest{
		Platform:        ev.Platform,
		UserID:          userID,
		UserName:        userName,
		AllowlistDenied: allowlistDenied,
	}
}

// GeneratePairingCode applies the Hermes-compatible pairing-code policy and
// persists pending state plus operator-visible policy evidence.
func (s *PairingStore) GeneratePairingCode(ctx context.Context, request PairingCodeRequest) (PairingCodeResult, error) {
	if s == nil || s.path == "" {
		return PairingCodeResult{}, nil
	}
	if err := ctx.Err(); err != nil {
		return PairingCodeResult{}, err
	}

	request.Platform = strings.TrimSpace(request.Platform)
	request.UserID = strings.TrimSpace(request.UserID)
	request.UserName = strings.TrimSpace(request.UserName)
	if request.Platform == "" {
		return PairingCodeResult{}, fmt.Errorf("generate pairing code: platform is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return PairingCodeResult{}, err
	}
	now := s.now().UTC()
	s.cleanupExpiredPairingCodesLocked(&state, request.Platform, now)

	if request.UserID == "" {
		s.recordPairingEvidenceLocked(&state, PairingDegradedUnresolvedUser, request.Platform, "", "", "pairing request has no resolvable user identity", time.Time{}, 0, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingCodeResult{}, err
		}
		return PairingCodeResult{Status: PairingCodeUnresolvedUser, Reason: PairingDegradedUnresolvedUser}, nil
	}

	if request.AllowlistDenied {
		s.recordPairingEvidenceLocked(&state, PairingDegradedAllowlistDenied, request.Platform, request.UserID, "", "pairing request was denied by allowlist policy", time.Time{}, 0, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingCodeResult{}, err
		}
		return PairingCodeResult{Status: PairingCodeAllowlistDenied, Reason: PairingDegradedAllowlistDenied}, nil
	}

	if until, ok := s.activePairingLockoutLocked(&state, request.Platform, now); ok {
		s.recordPairingEvidenceLocked(&state, PairingDegradedLockedOut, request.Platform, request.UserID, "", "pairing approval failures locked this platform", until, 0, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingCodeResult{}, err
		}
		return PairingCodeResult{
			Status:  PairingCodeLockedOut,
			Reason:  PairingDegradedLockedOut,
			RetryAt: until,
		}, nil
	}

	if retryAt, ok := s.activePairingRateLimitLocked(&state, request.Platform, request.UserID, now); ok {
		s.recordPairingEvidenceLocked(&state, PairingDegradedRateLimited, request.Platform, request.UserID, "", "pairing code request was rate limited", retryAt, 0, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingCodeResult{}, err
		}
		return PairingCodeResult{
			Status:  PairingCodeRateLimited,
			Reason:  PairingDegradedRateLimited,
			RetryAt: retryAt,
		}, nil
	}

	platform := s.ensurePairingPlatformLocked(&state, request.Platform)
	if len(platform.Pending) >= maxPendingPairingCodesPerPlatform {
		s.recordPairingEvidenceLocked(&state, PairingDegradedMaxPending, request.Platform, request.UserID, "", "platform has the maximum number of pending pairing codes", time.Time{}, len(platform.Pending), now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingCodeResult{}, err
		}
		return PairingCodeResult{Status: PairingCodeMaxPending, Reason: PairingDegradedMaxPending}, nil
	}

	code, err := generatePairingCode()
	if err != nil {
		return PairingCodeResult{}, err
	}
	for attempts := 0; platform.Pending[code].UserID != "" && attempts < 16; attempts++ {
		code, err = generatePairingCode()
		if err != nil {
			return PairingCodeResult{}, err
		}
	}
	if platform.Pending[code].UserID != "" {
		return PairingCodeResult{}, errors.New("generate pairing code: exhausted collision retries")
	}

	platform.Pending[code] = pairingPendingFileRecord{
		UserID:    request.UserID,
		UserName:  request.UserName,
		CreatedAt: now,
	}
	state.Platforms[request.Platform] = platform
	if state.RateLimits == nil {
		state.RateLimits = map[string]time.Time{}
	}
	state.RateLimits[pairingRateLimitKey(request.Platform, request.UserID)] = now
	state.UpdatedAt = now
	if err := s.writeStateLocked(ctx, state); err != nil {
		return PairingCodeResult{}, err
	}
	return PairingCodeResult{
		Status:    PairingCodeIssued,
		Code:      code,
		ExpiresAt: now.Add(pairingCodeTTL),
	}, nil
}

// ApprovePairingCode approves a pending code or records failed approval
// evidence. Codes are normalized case-insensitively like upstream Hermes.
func (s *PairingStore) ApprovePairingCode(ctx context.Context, platformName, code string) (PairingApprovalResult, error) {
	if s == nil || s.path == "" {
		return PairingApprovalResult{}, nil
	}
	if err := ctx.Err(); err != nil {
		return PairingApprovalResult{}, err
	}
	platformName = strings.TrimSpace(platformName)
	code = strings.ToUpper(strings.TrimSpace(code))
	if platformName == "" {
		return PairingApprovalResult{}, fmt.Errorf("approve pairing code: platform is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return PairingApprovalResult{}, err
	}
	now := s.now().UTC()
	expired := s.cleanupExpiredPairingCodesLocked(&state, platformName, now)
	if _, ok := expired[code]; ok {
		lockedUntil := s.recordFailedPairingApprovalLocked(&state, platformName, code, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingApprovalResult{}, err
		}
		if !lockedUntil.IsZero() {
			return PairingApprovalResult{Status: PairingApprovalLockedOut, Reason: PairingDegradedLockedOut, LockedOutUntil: lockedUntil}, nil
		}
		return PairingApprovalResult{Status: PairingApprovalExpired, Reason: PairingDegradedExpired}, nil
	}

	platform := s.ensurePairingPlatformLocked(&state, platformName)
	entry, ok := platform.Pending[code]
	if !ok {
		lockedUntil := s.recordFailedPairingApprovalLocked(&state, platformName, code, now)
		state.UpdatedAt = now
		if err := s.writeStateLocked(ctx, state); err != nil {
			return PairingApprovalResult{}, err
		}
		if !lockedUntil.IsZero() {
			return PairingApprovalResult{Status: PairingApprovalLockedOut, Reason: PairingDegradedLockedOut, LockedOutUntil: lockedUntil}, nil
		}
		return PairingApprovalResult{Status: PairingApprovalInvalid}, nil
	}

	delete(platform.Pending, code)
	if platform.Approved == nil {
		platform.Approved = map[string]pairingApprovedFileRecord{}
	}
	platform.Approved[entry.UserID] = pairingApprovedFileRecord{
		UserName:   entry.UserName,
		ApprovedAt: now,
	}
	state.Platforms[platformName] = platform
	state.UpdatedAt = now
	if err := s.writeStateLocked(ctx, state); err != nil {
		return PairingApprovalResult{}, err
	}
	return PairingApprovalResult{
		Status:   PairingApprovalApproved,
		UserID:   entry.UserID,
		UserName: entry.UserName,
	}, nil
}

// RecordPendingPairing persists a caller-supplied pending pairing record.
func (s *PairingStore) RecordPendingPairing(ctx context.Context, record PairingPendingRecord) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record.Platform = strings.TrimSpace(record.Platform)
	record.Code = strings.TrimSpace(record.Code)
	record.UserID = strings.TrimSpace(record.UserID)
	if record.Platform == "" || record.Code == "" || record.UserID == "" {
		return fmt.Errorf("record pending pairing: platform, code, and user_id are required")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = s.now()
	}
	record.CreatedAt = record.CreatedAt.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}
	platform := state.Platforms[record.Platform]
	if platform.Pending == nil {
		platform.Pending = map[string]pairingPendingFileRecord{}
	}
	if platform.Approved == nil {
		platform.Approved = map[string]pairingApprovedFileRecord{}
	}
	platform.Pending[record.Code] = pairingPendingFileRecord{
		UserID:    record.UserID,
		UserName:  record.UserName,
		CreatedAt: record.CreatedAt,
	}
	state.Platforms[record.Platform] = platform
	state.UpdatedAt = s.now().UTC()
	return s.writeStateLocked(ctx, state)
}

// RecordApprovedPairing persists a caller-supplied approved pairing record.
func (s *PairingStore) RecordApprovedPairing(ctx context.Context, record PairingApprovedRecord) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record.Platform = strings.TrimSpace(record.Platform)
	record.UserID = strings.TrimSpace(record.UserID)
	if record.Platform == "" || record.UserID == "" {
		return fmt.Errorf("record approved pairing: platform and user_id are required")
	}
	if record.ApprovedAt.IsZero() {
		record.ApprovedAt = s.now()
	}
	record.ApprovedAt = record.ApprovedAt.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}
	platform := state.Platforms[record.Platform]
	if platform.Pending == nil {
		platform.Pending = map[string]pairingPendingFileRecord{}
	}
	if platform.Approved == nil {
		platform.Approved = map[string]pairingApprovedFileRecord{}
	}
	platform.Approved[record.UserID] = pairingApprovedFileRecord{
		UserName:   record.UserName,
		ApprovedAt: record.ApprovedAt,
	}
	state.Platforms[record.Platform] = platform
	state.UpdatedAt = s.now().UTC()
	return s.writeStateLocked(ctx, state)
}

// ReadPairingStatus reads the pairing read model. Missing, corrupt, and
// permission-denied files return an empty model plus structured degradation.
func (s *PairingStore) ReadPairingStatus(ctx context.Context) (PairingStatus, error) {
	if s == nil || s.path == "" {
		return emptyPairingStatus("", nil), nil
	}
	if err := ctx.Err(); err != nil {
		return PairingStatus{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, degraded, err := s.readStateForStatusLocked()
	if err != nil {
		return PairingStatus{}, err
	}
	if degraded != nil {
		return emptyPairingStatus(s.path, degraded), nil
	}
	return s.statusFromState(state), nil
}

func (s *PairingStore) statusFromState(state pairingFile) PairingStatus {
	now := s.now().UTC()
	status := PairingStatus{
		Kind:      pairingStatusKind,
		Version:   1,
		UpdatedAt: state.UpdatedAt,
	}

	platforms := make([]string, 0, len(state.Platforms))
	for platform := range state.Platforms {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)

	for _, platformName := range platforms {
		platform := state.Platforms[platformName]
		platformState := PairingPlatformStateUnpaired
		if len(platform.Approved) > 0 {
			platformState = PairingPlatformStatePaired
		}

		activePendingCount := 0
		for code, record := range platform.Pending {
			createdAt := record.CreatedAt.UTC()
			if now.Sub(createdAt) > pairingCodeTTL {
				status.Degraded = append(status.Degraded, PairingDegradedEvidence{
					Reason:   PairingDegradedExpired,
					Path:     s.path,
					Message:  "pairing code expired before approval",
					Platform: platformName,
					UserID:   record.UserID,
					Code:     code,
					At:       now,
				})
				continue
			}
			activePendingCount++
			age := int64(now.Sub(createdAt) / time.Second)
			if age < 0 {
				age = 0
			}
			status.Pending = append(status.Pending, PairingPendingRecord{
				Platform:   platformName,
				Code:       code,
				UserID:     record.UserID,
				UserName:   record.UserName,
				CreatedAt:  createdAt,
				AgeSeconds: age,
			})
		}
		status.Platforms = append(status.Platforms, PairingPlatformStatus{
			Platform:      platformName,
			State:         platformState,
			PendingCount:  activePendingCount,
			ApprovedCount: len(platform.Approved),
		})
		for userID, record := range platform.Approved {
			status.Approved = append(status.Approved, PairingApprovedRecord{
				Platform:   platformName,
				UserID:     userID,
				UserName:   record.UserName,
				ApprovedAt: record.ApprovedAt.UTC(),
			})
		}
	}

	sort.SliceStable(status.Pending, func(i, j int) bool {
		left, right := status.Pending[i], status.Pending[j]
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.Code < right.Code
	})
	sort.SliceStable(status.Approved, func(i, j int) bool {
		left, right := status.Approved[i], status.Approved[j]
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		return left.ApprovedAt.Before(right.ApprovedAt)
	})
	for _, evidence := range state.Evidence {
		status.Degraded = append(status.Degraded, PairingDegradedEvidence{
			Reason:   evidence.Reason,
			Path:     s.path,
			Message:  evidence.Message,
			Platform: evidence.Platform,
			UserID:   evidence.UserID,
			Code:     evidence.Code,
			At:       evidence.At.UTC(),
			Until:    evidence.Until.UTC(),
			Count:    evidence.Count,
		})
	}
	sort.SliceStable(status.Degraded, func(i, j int) bool {
		left, right := status.Degraded[i], status.Degraded[j]
		if !left.At.Equal(right.At) {
			return left.At.Before(right.At)
		}
		if left.Reason != right.Reason {
			return left.Reason < right.Reason
		}
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		return left.Code < right.Code
	})
	return status
}

func (s *PairingStore) ensurePairingPlatformLocked(state *pairingFile, platformName string) pairingPlatformFile {
	if state.Platforms == nil {
		state.Platforms = map[string]pairingPlatformFile{}
	}
	platform := state.Platforms[platformName]
	if platform.Pending == nil {
		platform.Pending = map[string]pairingPendingFileRecord{}
	}
	if platform.Approved == nil {
		platform.Approved = map[string]pairingApprovedFileRecord{}
	}
	state.Platforms[platformName] = platform
	return platform
}

func (s *PairingStore) cleanupExpiredPairingCodesLocked(state *pairingFile, platformName string, now time.Time) map[string]pairingPendingFileRecord {
	platform := s.ensurePairingPlatformLocked(state, platformName)
	expired := map[string]pairingPendingFileRecord{}
	for code, record := range platform.Pending {
		if now.Sub(record.CreatedAt.UTC()) > pairingCodeTTL {
			expired[code] = record
			delete(platform.Pending, code)
			s.recordPairingEvidenceLocked(state, PairingDegradedExpired, platformName, record.UserID, code, "pairing code expired before approval", time.Time{}, 0, now)
		}
	}
	if len(expired) > 0 {
		state.Platforms[platformName] = platform
		state.UpdatedAt = now
	}
	return expired
}

func (s *PairingStore) activePairingRateLimitLocked(state *pairingFile, platformName, userID string, now time.Time) (time.Time, bool) {
	if state.RateLimits == nil {
		state.RateLimits = map[string]time.Time{}
		return time.Time{}, false
	}
	key := pairingRateLimitKey(platformName, userID)
	lastRequest, ok := state.RateLimits[key]
	if !ok {
		return time.Time{}, false
	}
	retryAt := lastRequest.UTC().Add(pairingRequestRateLimit)
	if now.Before(retryAt) {
		return retryAt, true
	}
	delete(state.RateLimits, key)
	return time.Time{}, false
}

func (s *PairingStore) activePairingLockoutLocked(state *pairingFile, platformName string, now time.Time) (time.Time, bool) {
	if state.Lockouts == nil {
		state.Lockouts = map[string]time.Time{}
		return time.Time{}, false
	}
	until, ok := state.Lockouts[platformName]
	if !ok {
		return time.Time{}, false
	}
	until = until.UTC()
	if now.Before(until) {
		return until, true
	}
	delete(state.Lockouts, platformName)
	return time.Time{}, false
}

func (s *PairingStore) recordFailedPairingApprovalLocked(state *pairingFile, platformName, code string, now time.Time) time.Time {
	if state.Failures == nil {
		state.Failures = map[string]int{}
	}
	failures := state.Failures[platformName] + 1
	if failures < maxPairingApprovalFailures {
		state.Failures[platformName] = failures
		return time.Time{}
	}
	delete(state.Failures, platformName)
	if state.Lockouts == nil {
		state.Lockouts = map[string]time.Time{}
	}
	until := now.Add(pairingLockoutDuration)
	state.Lockouts[platformName] = until
	s.recordPairingEvidenceLocked(state, PairingDegradedLockedOut, platformName, "", code, "platform locked after repeated invalid pairing approvals", until, maxPairingApprovalFailures, now)
	return until
}

func (s *PairingStore) recordPairingEvidenceLocked(state *pairingFile, reason PairingDegradedReason, platformName, userID, code, message string, until time.Time, count int, now time.Time) {
	state.Evidence = append(state.Evidence, pairingEvidenceFileRecord{
		Reason:   reason,
		Platform: strings.TrimSpace(platformName),
		UserID:   strings.TrimSpace(userID),
		Code:     strings.TrimSpace(code),
		Message:  message,
		At:       now.UTC(),
		Until:    until.UTC(),
		Count:    count,
	})
}

func (s *PairingStore) readStateForStatusLocked() (pairingFile, []PairingDegradedEvidence, error) {
	raw, err := s.readFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return pairingFile{}, []PairingDegradedEvidence{{
			Reason:  PairingDegradedMissing,
			Path:    s.path,
			Message: "pairing state is missing",
		}}, nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return pairingFile{}, []PairingDegradedEvidence{{
			Reason:  PairingDegradedPermissionDenied,
			Path:    s.path,
			Message: "permission denied reading pairing state",
		}}, nil
	}
	if err != nil {
		return pairingFile{}, []PairingDegradedEvidence{{
			Reason:  PairingDegradedReadFailed,
			Path:    s.path,
			Message: fmt.Sprintf("read pairing state: %v", err),
		}}, nil
	}

	state, err := decodePairingState(raw)
	if err != nil {
		return pairingFile{}, []PairingDegradedEvidence{{
			Reason:  PairingDegradedCorrupt,
			Path:    s.path,
			Message: fmt.Sprintf("decode pairing state: %v", err),
		}}, nil
	}
	return state, nil, nil
}

func (s *PairingStore) readStateLocked() (pairingFile, error) {
	raw, err := s.readFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return newPairingFile(s.now()), nil
	}
	if err != nil {
		return pairingFile{}, fmt.Errorf("read pairing state: %w", err)
	}
	state, err := decodePairingState(raw)
	if err != nil {
		return pairingFile{}, fmt.Errorf("decode pairing state: %w", err)
	}
	return state, nil
}

func (s *PairingStore) writeStateLocked(ctx context.Context, state pairingFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state.Kind = pairingStatusKind
	state.Version = 1
	if state.Platforms == nil {
		state.Platforms = map[string]pairingPlatformFile{}
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pairing state: %w", err)
	}
	raw = append(raw, '\n')
	if err := s.writeFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("replace pairing state: %w", err)
	}
	return nil
}

func decodePairingState(raw []byte) (pairingFile, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return pairingFile{}, errors.New("empty pairing state")
	}
	var state pairingFile
	if err := json.Unmarshal(raw, &state); err != nil {
		return pairingFile{}, err
	}
	if state.Kind == "" {
		state.Kind = pairingStatusKind
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Platforms == nil {
		state.Platforms = map[string]pairingPlatformFile{}
	}
	if state.RateLimits == nil {
		state.RateLimits = map[string]time.Time{}
	}
	if state.Failures == nil {
		state.Failures = map[string]int{}
	}
	if state.Lockouts == nil {
		state.Lockouts = map[string]time.Time{}
	}
	for name, platform := range state.Platforms {
		if platform.Pending == nil {
			platform.Pending = map[string]pairingPendingFileRecord{}
		}
		if platform.Approved == nil {
			platform.Approved = map[string]pairingApprovedFileRecord{}
		}
		state.Platforms[name] = platform
	}
	return state, nil
}

func newPairingFile(now time.Time) pairingFile {
	return pairingFile{
		Kind:       pairingStatusKind,
		Version:    1,
		Platforms:  map[string]pairingPlatformFile{},
		RateLimits: map[string]time.Time{},
		Failures:   map[string]int{},
		Lockouts:   map[string]time.Time{},
		UpdatedAt:  now.UTC(),
	}
}

func emptyPairingStatus(path string, degraded []PairingDegradedEvidence) PairingStatus {
	return PairingStatus{
		Kind:     pairingStatusKind,
		Version:  1,
		Degraded: degraded,
	}
}

func atomicWritePairingFile(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pairing state dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".pairing-*.tmp")
	if err != nil {
		return fmt.Errorf("create pairing temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	_ = tmp.Chmod(mode)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write pairing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync pairing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close pairing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename pairing state: %w", err)
	}
	_ = os.Chmod(path, mode)
	return nil
}

func xdgDataHomeForPairing() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

type pairingFile struct {
	Kind       string                         `json:"kind"`
	Version    int                            `json:"version"`
	Platforms  map[string]pairingPlatformFile `json:"platforms"`
	RateLimits map[string]time.Time           `json:"rate_limits,omitempty"`
	Failures   map[string]int                 `json:"failures,omitempty"`
	Lockouts   map[string]time.Time           `json:"lockouts,omitempty"`
	Evidence   []pairingEvidenceFileRecord    `json:"evidence,omitempty"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

type pairingPlatformFile struct {
	Pending  map[string]pairingPendingFileRecord  `json:"pending,omitempty"`
	Approved map[string]pairingApprovedFileRecord `json:"approved,omitempty"`
}

type pairingPendingFileRecord struct {
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type pairingApprovedFileRecord struct {
	UserName   string    `json:"user_name,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
}

type pairingEvidenceFileRecord struct {
	Reason   PairingDegradedReason `json:"reason"`
	Platform string                `json:"platform,omitempty"`
	UserID   string                `json:"user_id,omitempty"`
	Code     string                `json:"code,omitempty"`
	Message  string                `json:"message"`
	At       time.Time             `json:"at"`
	Until    time.Time             `json:"until,omitempty"`
	Count    int                   `json:"count,omitempty"`
}

func pairingRateLimitKey(platformName, userID string) string {
	return platformName + ":" + userID
}

func generatePairingCode() (string, error) {
	var b strings.Builder
	b.Grow(pairingCodeLength)
	max := big.NewInt(int64(len(pairingCodeAlphabet)))
	for i := 0; i < pairingCodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate pairing code: %w", err)
		}
		b.WriteByte(pairingCodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}
