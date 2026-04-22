package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type SessionIndexMirror struct {
	src  *BoltMap
	path string
	now  func() time.Time
}

const sessionIndexHeader = "# Auto-generated session index\n# This file is a read-only mirror of sessions.db for operator auditability\n"

func NewSessionIndexMirror(src *BoltMap, path string) *SessionIndexMirror {
	return &SessionIndexMirror{
		src:  src,
		path: path,
		now:  time.Now,
	}
}

func (m *SessionIndexMirror) Write() error {
	if m == nil {
		return errors.New("session: nil SessionIndexMirror")
	}
	if m.src == nil {
		return errors.New("session: nil BoltMap")
	}
	if strings.TrimSpace(m.path) == "" {
		return errors.New("session: mirror path is required")
	}

	sessions, err := m.snapshot()
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString(sessionIndexHeader)
	b.WriteString("sessions:\n")
	for _, entry := range sessions {
		b.WriteString("  ")
		b.WriteString(entry.Key)
		b.WriteString(": ")
		b.WriteString(entry.SessionID)
		b.WriteString("\n")
	}
	b.WriteString("updated_at: ")
	b.WriteString(m.now().UTC().Format(time.RFC3339))
	b.WriteString("\n")

	return writeAtomic(m.path, []byte(b.String()))
}

type sessionEntry struct {
	Key       string
	SessionID string
}

func (m *SessionIndexMirror) snapshot() ([]sessionEntry, error) {
	m.src.closeMu.Lock()
	db := m.src.db
	m.src.closeMu.Unlock()
	if db == nil {
		return nil, errors.New("session: BoltMap is closed")
	}

	var out []sessionEntry
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			out = append(out, sessionEntry{Key: string(k), SessionID: string(v)})
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("session: snapshot mirror source: %w", err)
	}
	return out, nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("session: create mirror dir for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("session: create temp mirror for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("session: write temp mirror for %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("session: chmod temp mirror for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("session: close temp mirror for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("session: rename mirror into place for %s: %w", path, err)
	}
	return nil
}
