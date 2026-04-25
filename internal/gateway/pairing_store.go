package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const pairingStatusKind = "gormes-gateway-pairing"

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
)

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

// PairingDegradedEvidence records why the read model could not be trusted.
type PairingDegradedEvidence struct {
	Reason  PairingDegradedReason `json:"reason"`
	Path    string                `json:"path"`
	Message string                `json:"message"`
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
		status.Platforms = append(status.Platforms, PairingPlatformStatus{
			Platform:      platformName,
			State:         platformState,
			PendingCount:  len(platform.Pending),
			ApprovedCount: len(platform.Approved),
		})

		for code, record := range platform.Pending {
			age := int64(s.now().UTC().Sub(record.CreatedAt.UTC()) / time.Second)
			if age < 0 {
				age = 0
			}
			status.Pending = append(status.Pending, PairingPendingRecord{
				Platform:   platformName,
				Code:       code,
				UserID:     record.UserID,
				UserName:   record.UserName,
				CreatedAt:  record.CreatedAt.UTC(),
				AgeSeconds: age,
			})
		}
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
	return status
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
		Kind:      pairingStatusKind,
		Version:   1,
		Platforms: map[string]pairingPlatformFile{},
		UpdatedAt: now.UTC(),
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
	Kind      string                         `json:"kind"`
	Version   int                            `json:"version"`
	Platforms map[string]pairingPlatformFile `json:"platforms"`
	UpdatedAt time.Time                      `json:"updated_at"`
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
