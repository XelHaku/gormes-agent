package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// bucketName is the top-level bbolt bucket for the v1 schema. Phase 3 may
// add sessions_v2 alongside; rely on errors.Is, not bucket-name strings.
const bucketName = "sessions_v1"

// openTimeout caps how long OpenBolt waits for the file lock before
// returning ErrDBLocked. 100 ms is enough to ride out a brief overlap
// during systemd restart handoff without masking real dual-instance bugs.
const openTimeout = 100 * time.Millisecond

// BoltMap is the production Map backed by a single bbolt file.
type BoltMap struct {
	closeMu sync.Mutex
	db      *bolt.DB // nil after Close
}

// OpenBolt opens (or creates) the bbolt file at path, ensuring the parent
// directory exists with mode 0700 and the sessions_v1 bucket exists.
// Translates bbolt's internal errors into ErrDBLocked / ErrDBCorrupt where
// appropriate; other errors are surfaced wrapped.
func OpenBolt(path string) (*BoltMap, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("session: create parent dir for %s: %w", path, err)
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, classifyOpenErr(path, err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(metadataBucketName)); err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(chatUserBucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("session: create bucket in %s: %w", path, err)
	}

	return &BoltMap{db: db}, nil
}

// DB exposes the underlying *bolt.DB so other subsystems can add their
// own buckets (Phase 2.D cron_jobs bucket, future extensions). The
// caller MUST NOT close the returned handle — BoltMap's Close owns it.
func (m *BoltMap) DB() *bolt.DB {
	return m.db
}

func classifyOpenErr(path string, err error) error {
	if err != nil && (errors.Is(err, bolt.ErrTimeout) ||
		containsAny(err.Error(), "timeout", "resource temporarily unavailable")) {
		return fmt.Errorf("%w: %s", ErrDBLocked, path)
	}
	if err != nil && (errors.Is(err, bolt.ErrInvalid) ||
		containsAny(err.Error(), "invalid database", "version mismatch", "file size too small")) {
		return fmt.Errorf("%w: %s", ErrDBCorrupt, path)
	}
	return fmt.Errorf("session: open %s: %w", path, err)
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if len(n) > 0 && stringContainsFold(s, n) {
			return true
		}
	}
	return false
}

func stringContainsFold(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a, b := s[i+j], substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (m *BoltMap) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return "", errors.New("session: BoltMap is closed")
	}

	var out string
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v != nil {
			out = string(v)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("session: get %q: %w", key, err)
	}
	return out, nil
}

func (m *BoltMap) Put(ctx context.Context, key, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return errors.New("session: BoltMap is closed")
	}

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return errors.New("session: bucket missing")
		}
		if sessionID == "" {
			return b.Delete([]byte(key))
		}
		return b.Put([]byte(key), []byte(sessionID))
	})
}

// Close flushes and releases the bbolt file lock. Idempotent.
func (m *BoltMap) Close() error {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if m.db == nil {
		return nil
	}
	err := m.db.Close()
	m.db = nil
	return err
}
