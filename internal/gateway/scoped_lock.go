package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LockProcessProbe reports whether a PID is currently running and returns an
// opaque start token that identifies that particular run. Implementations
// must return (tok, false, nil) — not an error — when the PID does not
// exist. The token is treated as an opaque string; callers compare tokens
// byte-for-byte to detect PID reuse.
type LockProcessProbe func(pid int) (startToken string, running bool, err error)

// LockStoreConfig configures a LockStore. Zero-valued fields fall back to
// process-derived defaults so production callers can pass LockStoreConfig{}.
type LockStoreConfig struct {
	BaseDir      string
	PID          int
	StartToken   string
	Now          func() time.Time
	ProcessProbe LockProcessProbe
}

// LockStore owns the XDG-backed scoped-lock directory. It ports the
// upstream Hermes acquire_scoped_lock/release_scoped_lock contract so two
// profiles cannot connect with the same credential at the same time, but
// falls back to the caller cleanly when the previous owner's process is
// gone or its start token no longer matches.
type LockStore struct {
	baseDir    string
	pid        int
	startToken string
	now        func() time.Time
	probe      LockProcessProbe

	mu sync.Mutex
}

// ScopedLock is an acquired lock handle. Release must be called when the
// caller is done. The zero value is not usable.
type ScopedLock struct {
	store    *LockStore
	path     string
	platform string
}

// lockFile is the on-disk JSON payload. Field names are stable so future
// operator tooling (gormes gateway status) can display them.
type lockFile struct {
	Platform   string `json:"platform"`
	PID        int    `json:"pid"`
	StartToken string `json:"start_token"`
	AcquiredAt string `json:"acquired_at"`
}

// NewLockStore constructs a LockStore from cfg. Missing fields are filled
// from the current process.
func NewLockStore(cfg LockStoreConfig) *LockStore {
	ls := &LockStore{
		baseDir:    cfg.BaseDir,
		pid:        cfg.PID,
		startToken: cfg.StartToken,
		now:        cfg.Now,
		probe:      cfg.ProcessProbe,
	}
	if ls.pid == 0 {
		ls.pid = os.Getpid()
	}
	if ls.now == nil {
		ls.now = time.Now
	}
	if ls.probe == nil {
		ls.probe = defaultLockProcessProbe
	}
	if ls.startToken == "" {
		if tok, _, err := ls.probe(ls.pid); err == nil {
			ls.startToken = tok
		}
	}
	return ls
}

// HashCredential returns the deterministic SHA-256 hex digest used as the
// filename component for (platform, credential). The raw credential never
// leaves this call.
func HashCredential(platform, credential string) string {
	h := sha256.New()
	h.Write([]byte(platform))
	h.Write([]byte{0})
	h.Write([]byte(credential))
	return hex.EncodeToString(h.Sum(nil))
}

// ScopedLockDir returns the default XDG-compatible directory used to hold
// gateway scoped-lock files: $XDG_STATE_HOME/gormes/gateway/locks with a
// $HOME/.local/state fallback.
func ScopedLockDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "gormes", "gateway", "locks")
}

func (s *LockStore) lockPath(platform, credential string) string {
	return filepath.Join(s.baseDir, platform+"-"+HashCredential(platform, credential)+".lock")
}

// TryAcquire attempts to take the scoped lock for (platform, credential).
// Returns (lock, true, nil) on success, (nil, false, nil) when another live
// process owns the lock, and (nil, false, err) on filesystem errors.
func (s *LockStore) TryAcquire(platform, credential string) (*ScopedLock, bool, error) {
	if platform == "" {
		return nil, false, errors.New("gateway: scoped lock platform must be non-empty")
	}
	if credential == "" {
		return nil, false, errors.New("gateway: scoped lock credential must be non-empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return nil, false, fmt.Errorf("gateway: scoped lock mkdir: %w", err)
	}
	path := s.lockPath(platform, credential)

	live, err := s.existingLockIsLive(path)
	if err != nil {
		return nil, false, err
	}
	if live {
		return nil, false, nil
	}

	payload := lockFile{
		Platform:   platform,
		PID:        s.pid,
		StartToken: s.startToken,
		AcquiredAt: s.now().UTC().Format(time.RFC3339Nano),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("gateway: scoped lock encode: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return nil, false, fmt.Errorf("gateway: scoped lock write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, false, fmt.Errorf("gateway: scoped lock rename: %w", err)
	}
	return &ScopedLock{store: s, path: path, platform: platform}, true, nil
}

// existingLockIsLive reports whether the file at path records a scoped
// lock whose owner process is still running with a matching start token.
// A missing file, unreadable JSON, or dead/reused owner all return false
// so the caller treats the slot as available for takeover.
func (s *LockStore) existingLockIsLive(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("gateway: scoped lock read: %w", err)
	}
	var existing lockFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return false, nil
	}
	tok, running, err := s.probe(existing.PID)
	if err != nil {
		return false, fmt.Errorf("gateway: scoped lock probe pid=%d: %w", existing.PID, err)
	}
	return running && tok == existing.StartToken, nil
}

// Release removes the on-disk lock file if and only if it still records
// this acquirer's PID+start_token. Idempotent: calling Release more than
// once is safe.
func (l *ScopedLock) Release() error {
	if l == nil || l.store == nil {
		return nil
	}
	l.store.mu.Lock()
	defer l.store.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("gateway: scoped lock release read: %w", err)
	}
	var existing lockFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil
	}
	if existing.PID != l.store.pid || existing.StartToken != l.store.startToken {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("gateway: scoped lock release remove: %w", err)
	}
	return nil
}

// defaultLockProcessProbe reads /proc/<pid>/stat on Linux and returns field 22
// (starttime, in clock ticks since boot) as the opaque start token. A PID
// whose /proc entry has disappeared is reported as not running.
func defaultLockProcessProbe(pid int) (string, bool, error) {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	// The comm field (field 2) is wrapped in parens and may contain
	// whitespace; find the LAST ')' to resume field parsing beyond it.
	idx := bytes.LastIndexByte(data, ')')
	if idx < 0 || idx+2 >= len(data) {
		return "", false, fmt.Errorf("gateway: malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(string(data[idx+2:]))
	// After stripping "<pid> (<comm>) ", the remaining slice starts at
	// field 3 (state). Field 22 (starttime) is therefore fields[19].
	const starttimeIdx = 19
	if len(fields) <= starttimeIdx {
		return "", false, fmt.Errorf("gateway: short /proc/%d/stat", pid)
	}
	return fields[starttimeIdx], true, nil
}
