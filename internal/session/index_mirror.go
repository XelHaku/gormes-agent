package session

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

type SessionIndexMirror struct {
	src  *BoltMap
	path string
	now  func() time.Time
}

type SessionIndexMirrorRefresher struct {
	mirror          *SessionIndexMirror
	ticker          *time.Ticker
	stop            chan struct{}
	stopOnce        sync.Once
	wg              sync.WaitGroup
	log             *slog.Logger
	lastFingerprint string
	lastMu          sync.Mutex
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
	return writeAtomic(m.path, m.render(sessions))
}

type sessionEntry struct {
	Key       string
	SessionID string
}

type sessionSnapshot struct {
	Sessions []sessionEntry
	Lineage  []LineageAuditEntry
}

func (m *SessionIndexMirror) snapshot() (sessionSnapshot, error) {
	m.src.closeMu.Lock()
	db := m.src.db
	m.src.closeMu.Unlock()
	if db == nil {
		return sessionSnapshot{}, errors.New("session: BoltMap is closed")
	}

	var out sessionSnapshot
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				out.Sessions = append(out.Sessions, sessionEntry{Key: string(k), SessionID: string(v)})
				return nil
			}); err != nil {
				return err
			}
		}

		mb := tx.Bucket([]byte(metadataBucketName))
		if mb == nil {
			return nil
		}
		var metas []Metadata
		if err := mb.ForEach(func(_, raw []byte) error {
			meta, err := decodeMetadata(raw)
			if err != nil {
				return fmt.Errorf("session: decode metadata during mirror snapshot: %w", err)
			}
			metas = append(metas, meta)
			return nil
		}); err != nil {
			return err
		}
		out.Lineage = buildLineageAudit(metas)
		return nil
	})
	if err != nil {
		return sessionSnapshot{}, fmt.Errorf("session: snapshot mirror source: %w", err)
	}
	return out, nil
}

func (m *SessionIndexMirror) render(snapshot sessionSnapshot) []byte {
	var b strings.Builder
	b.WriteString(sessionIndexHeader)
	b.WriteString("sessions:\n")
	for _, entry := range snapshot.Sessions {
		b.WriteString("  ")
		b.WriteString(entry.Key)
		b.WriteString(": ")
		b.WriteString(entry.SessionID)
		b.WriteString("\n")
	}
	if len(snapshot.Lineage) > 0 {
		b.WriteString("lineage:\n")
		for _, entry := range snapshot.Lineage {
			meta := entry.Metadata
			b.WriteString("  ")
			b.WriteString(meta.SessionID)
			b.WriteString(":\n")
			if meta.ParentSessionID != "" {
				b.WriteString("    parent_session_id: ")
				b.WriteString(meta.ParentSessionID)
				b.WriteString("\n")
			}
			b.WriteString("    lineage_kind: ")
			b.WriteString(effectiveLineageKind(meta))
			b.WriteString("\n")
			if len(entry.Children) > 0 {
				b.WriteString("    children:\n")
				for _, child := range entry.Children {
					b.WriteString("      - ")
					b.WriteString(child)
					b.WriteString("\n")
				}
			}
			b.WriteString("    lineage_status: ")
			b.WriteString(entry.Status)
			b.WriteString("\n")
			if meta.Source != "" {
				b.WriteString("    source: ")
				b.WriteString(meta.Source)
				b.WriteString("\n")
			}
			if meta.ChatID != "" {
				b.WriteString("    chat_id: ")
				b.WriteString(meta.ChatID)
				b.WriteString("\n")
			}
			if meta.UserID != "" {
				b.WriteString("    user_id: ")
				b.WriteString(meta.UserID)
				b.WriteString("\n")
			}
			if meta.UpdatedAt != 0 {
				b.WriteString("    metadata_updated_at: ")
				b.WriteString(fmt.Sprintf("%d", meta.UpdatedAt))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("updated_at: ")
	b.WriteString(m.now().UTC().Format(time.RFC3339))
	b.WriteString("\n")
	return []byte(b.String())
}

func (m *SessionIndexMirror) StartRefresh(interval time.Duration, log *slog.Logger) *SessionIndexMirrorRefresher {
	if m == nil {
		return nil
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}

	r := &SessionIndexMirrorRefresher{
		mirror: m,
		ticker: time.NewTicker(interval),
		stop:   make(chan struct{}, 1),
		log:    log,
	}
	r.wg.Add(1)
	go r.loop()
	return r
}

func (r *SessionIndexMirrorRefresher) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stop)
		r.ticker.Stop()
		r.wg.Wait()
	})
}

func (r *SessionIndexMirrorRefresher) loop() {
	defer r.wg.Done()
	r.sync()
	for {
		select {
		case <-r.ticker.C:
			r.sync()
		case <-r.stop:
			return
		}
	}
}

func (r *SessionIndexMirrorRefresher) sync() {
	sessions, err := r.mirror.snapshot()
	if err != nil {
		r.log.Warn("session index mirror refresh failed", "err", err)
		return
	}

	fingerprint := fingerprintSnapshot(sessions)
	r.lastMu.Lock()
	same := fingerprint == r.lastFingerprint
	r.lastMu.Unlock()
	if same {
		if _, err := os.Stat(r.mirror.path); err == nil {
			return
		}
	}

	if err := writeAtomic(r.mirror.path, r.mirror.render(sessions)); err != nil {
		r.log.Warn("session index mirror write failed", "path", r.mirror.path, "err", err)
		return
	}

	r.lastMu.Lock()
	r.lastFingerprint = fingerprint
	r.lastMu.Unlock()
}

func fingerprintSnapshot(snapshot sessionSnapshot) string {
	var b strings.Builder
	for _, entry := range snapshot.Sessions {
		b.WriteString(entry.Key)
		b.WriteByte(0)
		b.WriteString(entry.SessionID)
		b.WriteByte(0)
	}
	for _, entry := range snapshot.Lineage {
		meta := entry.Metadata
		b.WriteString(meta.SessionID)
		b.WriteByte(0)
		b.WriteString(meta.Source)
		b.WriteByte(0)
		b.WriteString(meta.ChatID)
		b.WriteByte(0)
		b.WriteString(meta.UserID)
		b.WriteByte(0)
		b.WriteString(meta.ParentSessionID)
		b.WriteByte(0)
		b.WriteString(effectiveLineageKind(meta))
		b.WriteByte(0)
		b.WriteString(entry.Status)
		b.WriteByte(0)
		for _, child := range entry.Children {
			b.WriteString(child)
			b.WriteByte(0)
		}
		b.WriteByte(0)
	}
	return b.String()
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
