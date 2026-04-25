package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const tokenLockKind = "gormes-gateway-token-lock"

// TokenLockStatus is operator-facing evidence for credential-scoped gateway
// lock decisions.
type TokenLockStatus string

const (
	TokenLockStatusAcquired               TokenLockStatus = "acquired"
	TokenLockStatusHeld                   TokenLockStatus = "lock-held"
	TokenLockStatusStaleCleared           TokenLockStatus = "stale-lock-cleared"
	TokenLockStatusCredentialHashMismatch TokenLockStatus = "credential-hash-mismatch"
	TokenLockStatusReleased               TokenLockStatus = "released"
	TokenLockStatusReleaseFailed          TokenLockStatus = "lock-release-failed"
)

var (
	ErrTokenLockHeld                   = errors.New("gateway token lock held")
	ErrTokenLockCredentialHashMismatch = errors.New("gateway token lock credential hash mismatch")
	ErrTokenLockReleaseFailed          = errors.New("gateway token lock release failed")
)

// TokenLockRequest describes the external credential identity a gateway
// process wants to reserve.
type TokenLockRequest struct {
	Platform   string
	Credential string
}

// TokenLockEvidence is safe to persist in runtime status JSON. It carries only
// platform names, paths, process identity, and non-reversible credential hashes.
type TokenLockEvidence struct {
	Status            TokenLockStatus          `json:"status,omitempty"`
	Platform          string                   `json:"platform,omitempty"`
	CredentialHash    string                   `json:"credential_hash,omitempty"`
	Path              string                   `json:"path,omitempty"`
	OwnerPID          int                      `json:"owner_pid,omitempty"`
	OwnerStartTime    int64                    `json:"owner_start_time,omitempty"`
	ProcessValidation RuntimeProcessValidation `json:"process_validation,omitempty"`
	Message           string                   `json:"message,omitempty"`
	UpdatedAt         string                   `json:"updated_at,omitempty"`
}

// TokenLockStore manages credential-scoped gateway lock records under one
// machine-local lock directory.
type TokenLockStore struct {
	dir        string
	now        func() time.Time
	pid        func() int
	startTime  func(int) (int64, bool)
	argv       func() []string
	processes  runtimeProcessTable
	removeFile func(string) error
}

// TokenScopedGatewayLock represents a lock record owned by the current
// process according to PID and process start-time evidence.
type TokenScopedGatewayLock struct {
	store  *TokenLockStore
	path   string
	record tokenLockRecord
}

type tokenLockRecord struct {
	Kind           string   `json:"kind"`
	Platform       string   `json:"platform"`
	CredentialHash string   `json:"credential_hash"`
	PID            int      `json:"pid"`
	StartTime      int64    `json:"start_time,omitempty"`
	Command        string   `json:"command,omitempty"`
	Argv           []string `json:"argv,omitempty"`
	UpdatedAt      string   `json:"updated_at"`
}

// NewTokenLockStore returns a JSON-file-backed token lock store.
func NewTokenLockStore(dir string) *TokenLockStore {
	return &TokenLockStore{
		dir:        dir,
		now:        func() time.Time { return time.Now().UTC() },
		pid:        os.Getpid,
		startTime:  procProcessStartTime,
		argv:       func() []string { return append([]string(nil), os.Args...) },
		processes:  procRuntimeProcessTable{},
		removeFile: os.Remove,
	}
}

// TokenCredentialHash returns the non-reversible credential scope hash used in
// lock filenames and status evidence.
func TokenCredentialHash(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

// LockPath returns the lock path for platform plus credential identity.
func (s *TokenLockStore) LockPath(platform, credential string) string {
	return filepath.Join(s.lockDir(), sanitizeTokenLockPlatform(platform)+"-"+TokenCredentialHash(credential)+".lock")
}

// Acquire claims a platform/credential lock or returns evidence explaining why
// the current process could not safely acquire it.
func (s *TokenLockStore) Acquire(ctx context.Context, req TokenLockRequest) (*TokenScopedGatewayLock, TokenLockEvidence, error) {
	if s == nil {
		s = NewTokenLockStore("")
	}
	if err := ctx.Err(); err != nil {
		return nil, TokenLockEvidence{}, err
	}

	platform := sanitizeTokenLockPlatform(req.Platform)
	hash := TokenCredentialHash(req.Credential)
	path := filepath.Join(s.lockDir(), platform+"-"+hash+".lock")
	record := s.currentRecord(platform, hash)
	evidenceStatus := TokenLockStatusAcquired
	var staleValidation RuntimeProcessValidation

	for attempt := 0; attempt < 2; attempt++ {
		existing, err := readTokenLockRecord(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			lock, evidence, err := s.createLock(ctx, path, record, evidenceStatus, staleValidation)
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return lock, evidence, err
		case err != nil:
			return nil, s.evidence(record, path, TokenLockStatusHeld, RuntimeProcessValidation{}, err.Error()), err
		}

		if existing.Platform != platform || existing.CredentialHash != hash {
			evidence := s.evidence(record, path, TokenLockStatusCredentialHashMismatch, RuntimeProcessValidation{}, "lock record identity does not match requested platform and credential hash")
			evidence.OwnerPID = existing.PID
			evidence.OwnerStartTime = existing.StartTime
			return nil, evidence, ErrTokenLockCredentialHashMismatch
		}

		if s.ownsRecord(existing) {
			if err := writeTokenLockRecordAtomic(path, record); err != nil {
				return nil, s.evidence(record, path, TokenLockStatusHeld, RuntimeProcessValidation{}, err.Error()), err
			}
			lock := &TokenScopedGatewayLock{store: s, path: path, record: record}
			return lock, s.evidence(record, path, TokenLockStatusAcquired, RuntimeProcessValidation{}, ""), nil
		}

		validation := s.validateTokenLockOwner(existing)
		if !tokenLockValidationProvesGone(validation) {
			evidence := s.evidence(record, path, TokenLockStatusHeld, validation, "credential lock is held by a live or unverified process")
			evidence.OwnerPID = existing.PID
			evidence.OwnerStartTime = existing.StartTime
			return nil, evidence, fmt.Errorf("%w: %s", ErrTokenLockHeld, path)
		}

		if err := s.remove(path); err != nil {
			evidence := s.evidence(record, path, TokenLockStatusHeld, validation, "stale credential lock could not be cleared: "+err.Error())
			evidence.OwnerPID = existing.PID
			evidence.OwnerStartTime = existing.StartTime
			return nil, evidence, fmt.Errorf("%w: %v", ErrTokenLockHeld, err)
		}
		evidenceStatus = TokenLockStatusStaleCleared
		staleValidation = validation
	}

	evidence := s.evidence(record, path, TokenLockStatusHeld, staleValidation, "credential lock changed while acquiring")
	return nil, evidence, fmt.Errorf("%w: %s", ErrTokenLockHeld, path)
}

// Path returns the filesystem path of the acquired lock.
func (l *TokenScopedGatewayLock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// CredentialHash returns the lock's credential hash.
func (l *TokenScopedGatewayLock) CredentialHash() string {
	if l == nil {
		return ""
	}
	return l.record.CredentialHash
}

// Release removes this lock only when the on-disk record still belongs to the
// current process identity.
func (l *TokenScopedGatewayLock) Release(ctx context.Context) (TokenLockEvidence, error) {
	if l == nil || l.store == nil || l.path == "" {
		return TokenLockEvidence{}, nil
	}
	if err := ctx.Err(); err != nil {
		return TokenLockEvidence{}, err
	}

	record := l.store.currentRecord(l.record.Platform, l.record.CredentialHash)
	evidence := l.store.evidence(record, l.path, TokenLockStatusReleased, RuntimeProcessValidation{}, "")
	existing, err := readTokenLockRecord(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return evidence, nil
	}
	if err != nil {
		evidence.Status = TokenLockStatusReleaseFailed
		evidence.Message = err.Error()
		return evidence, fmt.Errorf("%w: %v", ErrTokenLockReleaseFailed, err)
	}
	if existing.Platform != l.record.Platform ||
		existing.CredentialHash != l.record.CredentialHash ||
		existing.PID != record.PID ||
		existing.StartTime != record.StartTime {
		evidence.OwnerPID = existing.PID
		evidence.OwnerStartTime = existing.StartTime
		evidence.Message = "lock is no longer owned by this process"
		return evidence, nil
	}
	if err := l.store.remove(l.path); err != nil {
		evidence.Status = TokenLockStatusReleaseFailed
		evidence.OwnerPID = existing.PID
		evidence.OwnerStartTime = existing.StartTime
		evidence.Message = err.Error()
		return evidence, fmt.Errorf("%w: %v", ErrTokenLockReleaseFailed, err)
	}
	return evidence, nil
}

func (s *TokenLockStore) lockDir() string {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return filepath.Join(".", "gateway-locks")
	}
	return s.dir
}

func (s *TokenLockStore) currentRecord(platform, credentialHash string) tokenLockRecord {
	pid := s.pid()
	startTime, _ := s.startTime(pid)
	argv := append([]string(nil), s.argv()...)
	return tokenLockRecord{
		Kind:           tokenLockKind,
		Platform:       platform,
		CredentialHash: credentialHash,
		PID:            pid,
		StartTime:      startTime,
		Command:        strings.Join(argv, " "),
		Argv:           argv,
		UpdatedAt:      s.now().UTC().Format(time.RFC3339Nano),
	}
}

func (s *TokenLockStore) ownsRecord(record tokenLockRecord) bool {
	current := s.currentRecord(record.Platform, record.CredentialHash)
	return record.PID > 0 &&
		record.PID == current.PID &&
		record.StartTime != 0 &&
		record.StartTime == current.StartTime
}

func (s *TokenLockStore) createLock(ctx context.Context, path string, record tokenLockRecord, status TokenLockStatus, validation RuntimeProcessValidation) (*TokenScopedGatewayLock, TokenLockEvidence, error) {
	if err := ctx.Err(); err != nil {
		return nil, TokenLockEvidence{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, s.evidence(record, path, TokenLockStatusHeld, validation, err.Error()), err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, s.evidence(record, path, TokenLockStatusHeld, validation, err.Error()), err
	}
	raw = append(raw, '\n')

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, TokenLockEvidence{}, os.ErrExist
		}
		return nil, s.evidence(record, path, TokenLockStatusHeld, validation, err.Error()), err
	}
	tmpClosed := false
	defer func() {
		if !tmpClosed {
			_ = file.Close()
			_ = s.remove(path)
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return nil, s.evidence(record, path, TokenLockStatusHeld, validation, err.Error()), err
	}
	if err := file.Close(); err != nil {
		return nil, s.evidence(record, path, TokenLockStatusHeld, validation, err.Error()), err
	}
	tmpClosed = true
	lock := &TokenScopedGatewayLock{store: s, path: path, record: record}
	return lock, s.evidence(record, path, status, validation, ""), nil
}

func (s *TokenLockStore) validateTokenLockOwner(record tokenLockRecord) RuntimeProcessValidation {
	checkedAt := s.now().UTC().Format(time.RFC3339Nano)
	validation := RuntimeProcessValidation{
		PID:               record.PID,
		ExpectedStartTime: record.StartTime,
		Command:           record.Command,
		CheckedAt:         checkedAt,
	}
	if record.PID <= 0 {
		validation.Status = RuntimeProcessValidationStalePID
		validation.Message = "token lock PID is missing or invalid"
		return validation
	}

	processes := s.processes
	if processes == nil {
		processes = procRuntimeProcessTable{}
	}
	process, err := processes.LookupRuntimeProcess(record.PID)
	if err != nil {
		switch {
		case errors.Is(err, errRuntimeProcessPermissionDenied):
			validation.Status = RuntimeProcessValidationPermissionDenied
			validation.Message = "process lookup was denied"
		case errors.Is(err, errRuntimeProcessNotFound):
			validation.Status = RuntimeProcessValidationStalePID
			validation.Message = "process is not running"
		default:
			validation.Status = RuntimeProcessValidationStalePID
			validation.Message = err.Error()
		}
		return validation
	}

	validation.ActualStartTime = process.StartTime
	if record.StartTime == 0 || process.StartTime == 0 {
		validation.Status = RuntimeProcessValidationLive
		validation.Live = true
		validation.Message = "process exists but start time could not be validated"
		return validation
	}
	if process.StartTime != record.StartTime {
		validation.Status = RuntimeProcessValidationPIDReused
		validation.Message = "process start time does not match token lock"
		return validation
	}
	if process.Stopped {
		validation.Status = RuntimeProcessValidationStopped
		validation.Message = "process is stopped"
		return validation
	}
	validation.Status = RuntimeProcessValidationLive
	validation.Live = true
	if validation.Command == "" {
		validation.Command = process.Command
	}
	return validation
}

func tokenLockValidationProvesGone(validation RuntimeProcessValidation) bool {
	if validation.Live {
		return false
	}
	switch validation.Status {
	case RuntimeProcessValidationStalePID, RuntimeProcessValidationPIDReused, RuntimeProcessValidationStopped:
		return true
	default:
		return false
	}
}

func (s *TokenLockStore) evidence(record tokenLockRecord, path string, status TokenLockStatus, validation RuntimeProcessValidation, message string) TokenLockEvidence {
	evidence := TokenLockEvidence{
		Status:            status,
		Platform:          record.Platform,
		CredentialHash:    record.CredentialHash,
		Path:              path,
		OwnerPID:          record.PID,
		OwnerStartTime:    record.StartTime,
		ProcessValidation: validation,
		Message:           message,
		UpdatedAt:         s.now().UTC().Format(time.RFC3339Nano),
	}
	if evidence.ProcessValidation.Status == "" {
		evidence.ProcessValidation = RuntimeProcessValidation{}
	}
	return evidence
}

func (s *TokenLockStore) remove(path string) error {
	if s != nil && s.removeFile != nil {
		return s.removeFile(path)
	}
	return os.Remove(path)
}

func readTokenLockRecord(path string) (tokenLockRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return tokenLockRecord{}, err
	}
	if len(raw) == 0 {
		return tokenLockRecord{}, os.ErrNotExist
	}
	var record tokenLockRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return tokenLockRecord{}, fmt.Errorf("decode token lock record: %w", err)
	}
	return record, nil
}

func writeTokenLockRecordAtomic(path string, record tokenLockRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".token-lock-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func sanitizeTokenLockPlatform(platform string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range platform {
		allowed := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if allowed {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "unknown"
	}
	return out
}
