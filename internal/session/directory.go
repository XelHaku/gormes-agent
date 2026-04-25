package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	metadataBucketName = "session_meta_v1"
	chatUserBucketName = "session_chat_users_v1"
)

// ErrUserBindingConflict reports an attempt to bind one canonical chat to
// multiple distinct user IDs.
var ErrUserBindingConflict = errors.New("session: chat already bound to different user_id")

// Metadata is the first durable identity layer above the raw session map.
// SessionID remains the resume handle; Source+ChatID identify the transport
// chat; UserID is the canonical participant identity that can span chats.
type Metadata struct {
	SessionID             string `json:"session_id"`
	Source                string `json:"source,omitempty"`
	ChatID                string `json:"chat_id,omitempty"`
	UserID                string `json:"user_id,omitempty"`
	ParentSessionID       string `json:"parent_session_id,omitempty"`
	LineageKind           string `json:"lineage_kind"`
	UpdatedAt             int64  `json:"updated_at"`
	ResumePending         bool   `json:"resume_pending,omitempty"`
	ResumeReason          string `json:"resume_reason,omitempty"`
	ResumeMarkedAt        int64  `json:"resume_marked_at,omitempty"`
	NonResumableReason    string `json:"non_resumable_reason,omitempty"`
	NonResumableAt        int64  `json:"non_resumable_at,omitempty"`
	ExpiryFinalized       bool   `json:"expiry_finalized,omitempty"`
	MigratedMemoryFlushed bool   `json:"migrated_memory_flushed,omitempty"`
}

func normalizeMetadata(meta Metadata) Metadata {
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.Source = strings.TrimSpace(meta.Source)
	meta.ChatID = strings.TrimSpace(meta.ChatID)
	meta.UserID = strings.TrimSpace(meta.UserID)
	meta.ParentSessionID = strings.TrimSpace(meta.ParentSessionID)
	meta.LineageKind = strings.ToLower(strings.TrimSpace(meta.LineageKind))
	meta.ResumeReason = strings.ToLower(strings.TrimSpace(meta.ResumeReason))
	meta.NonResumableReason = strings.ToLower(strings.TrimSpace(meta.NonResumableReason))
	return meta
}

func mergeMetadata(existing, incoming Metadata) (Metadata, error) {
	out := existing
	out.SessionID = incoming.SessionID
	if incoming.Source != "" {
		out.Source = incoming.Source
	}
	if incoming.ChatID != "" {
		out.ChatID = incoming.ChatID
	}
	if incoming.UserID != "" {
		out.UserID = incoming.UserID
	}
	if incoming.ParentSessionID != "" {
		if out.ParentSessionID != "" && out.ParentSessionID != incoming.ParentSessionID {
			return Metadata{}, fmt.Errorf("%w: %s parent_session_id already %s", ErrLineageConflict, incoming.SessionID, out.ParentSessionID)
		}
		out.ParentSessionID = incoming.ParentSessionID
	}
	if incoming.LineageKind != "" {
		if out.LineageKind != "" && out.LineageKind != LineageKindPrimary && out.LineageKind != incoming.LineageKind {
			return Metadata{}, fmt.Errorf("%w: %s lineage_kind already %s", ErrLineageConflict, incoming.SessionID, out.LineageKind)
		}
		out.LineageKind = incoming.LineageKind
	}
	if incoming.UpdatedAt != 0 {
		out.UpdatedAt = incoming.UpdatedAt
	}
	if incoming.ResumePending {
		out.ResumePending = true
	}
	if incoming.ResumeReason != "" {
		out.ResumeReason = incoming.ResumeReason
	}
	if incoming.ResumeMarkedAt != 0 {
		out.ResumeMarkedAt = incoming.ResumeMarkedAt
	}
	if incoming.NonResumableReason != "" {
		out.NonResumableReason = incoming.NonResumableReason
	}
	if incoming.NonResumableAt != 0 {
		out.NonResumableAt = incoming.NonResumableAt
	}
	if incoming.ExpiryFinalized {
		out.ExpiryFinalized = true
	}
	if incoming.MigratedMemoryFlushed {
		out.MigratedMemoryFlushed = true
	}
	return finalizeMetadata(out), nil
}

func chatBindingKey(source, chatID string) string {
	return source + "\x00" + chatID
}

func sortMetadata(items []Metadata) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		return items[i].SessionID < items[j].SessionID
	})
}

func decodeMetadata(raw []byte) (Metadata, error) {
	var meta Metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Metadata{}, err
	}
	applyLegacyMemoryFlushedMigration(raw, &meta)
	return finalizeMetadata(meta), nil
}

func applyLegacyMemoryFlushedMigration(raw []byte, meta *Metadata) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return
	}
	legacyRaw, ok := fields["memory_flushed"]
	if !ok {
		return
	}
	var legacyFlushed bool
	if err := json.Unmarshal(legacyRaw, &legacyFlushed); err != nil || !legacyFlushed {
		return
	}
	meta.MigratedMemoryFlushed = true
	meta.ExpiryFinalized = true
}

func (m *BoltMap) PutMetadata(ctx context.Context, meta Metadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return errors.New("session: BoltMap is closed")
	}

	meta = normalizeMetadata(meta)
	if err := validateMetadataIdentity(meta); err != nil {
		return err
	}

	return db.Update(func(tx *bolt.Tx) error {
		mb := tx.Bucket([]byte(metadataBucketName))
		cb := tx.Bucket([]byte(chatUserBucketName))
		if mb == nil || cb == nil {
			return errors.New("session: metadata buckets missing")
		}

		if raw := mb.Get([]byte(meta.SessionID)); raw != nil {
			existing, err := decodeMetadata(raw)
			if err != nil {
				return fmt.Errorf("session: decode metadata for %q: %w", meta.SessionID, err)
			}
			meta, err = mergeMetadata(existing, meta)
			if err != nil {
				return err
			}
		}
		meta = finalizeMetadata(meta)
		if err := validateMetadata(meta); err != nil {
			return err
		}
		if err := detectLineageLoop(meta.SessionID, meta.ParentSessionID, func(id string) (Metadata, bool, error) {
			raw := mb.Get([]byte(id))
			if raw == nil {
				return Metadata{}, false, nil
			}
			meta, err := decodeMetadata(raw)
			if err != nil {
				return Metadata{}, false, fmt.Errorf("session: decode lineage parent %q: %w", id, err)
			}
			return meta, true, nil
		}); err != nil {
			return err
		}

		if meta.Source != "" && meta.ChatID != "" {
			key := chatBindingKey(meta.Source, meta.ChatID)
			if raw := cb.Get([]byte(key)); len(raw) > 0 {
				bound := strings.TrimSpace(string(raw))
				if meta.UserID == "" {
					meta.UserID = bound
				} else if bound != meta.UserID {
					return fmt.Errorf("%w: %s/%s bound to %s", ErrUserBindingConflict, meta.Source, meta.ChatID, bound)
				}
			}
			if meta.UserID != "" {
				if err := cb.Put([]byte(key), []byte(meta.UserID)); err != nil {
					return fmt.Errorf("session: persist chat binding: %w", err)
				}
			}
		}

		if meta.UpdatedAt == 0 {
			meta.UpdatedAt = time.Now().Unix()
		}
		raw, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("session: encode metadata for %q: %w", meta.SessionID, err)
		}
		return mb.Put([]byte(meta.SessionID), raw)
	})
}

func (m *BoltMap) GetMetadata(ctx context.Context, sessionID string) (Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, false, err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return Metadata{}, false, errors.New("session: BoltMap is closed")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Metadata{}, false, nil
	}

	var (
		meta Metadata
		ok   bool
	)
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		if b == nil {
			return errors.New("session: metadata bucket missing")
		}
		raw := b.Get([]byte(sessionID))
		if raw == nil {
			return nil
		}
		decoded, err := decodeMetadata(raw)
		if err != nil {
			return fmt.Errorf("session: decode metadata for %q: %w", sessionID, err)
		}
		meta = decoded
		ok = true
		return nil
	})
	if err != nil {
		return Metadata{}, false, err
	}
	return meta, ok, nil
}

func (m *BoltMap) ClearResumePending(ctx context.Context, sessionID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return false, errors.New("session: BoltMap is closed")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil
	}

	var cleared bool
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		if b == nil {
			return errors.New("session: metadata bucket missing")
		}
		raw := b.Get([]byte(sessionID))
		if raw == nil {
			return nil
		}
		meta, err := decodeMetadata(raw)
		if err != nil {
			return fmt.Errorf("session: decode metadata for %q: %w", sessionID, err)
		}
		if !meta.ResumePending && meta.ResumeReason == "" && meta.ResumeMarkedAt == 0 {
			return nil
		}
		meta.ResumePending = false
		meta.ResumeReason = ""
		meta.ResumeMarkedAt = 0
		encoded, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("session: encode metadata for %q: %w", sessionID, err)
		}
		if err := b.Put([]byte(sessionID), encoded); err != nil {
			return err
		}
		cleared = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return cleared, nil
}

func (m *BoltMap) ResolveUserID(ctx context.Context, source, chatID string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return "", false, errors.New("session: BoltMap is closed")
	}

	source = strings.TrimSpace(source)
	chatID = strings.TrimSpace(chatID)
	if source == "" || chatID == "" {
		return "", false, nil
	}

	var out string
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(chatUserBucketName))
		if b == nil {
			return errors.New("session: chat binding bucket missing")
		}
		raw := b.Get([]byte(chatBindingKey(source, chatID)))
		if raw != nil {
			out = strings.TrimSpace(string(raw))
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return out, out != "", nil
}

func (m *BoltMap) ListMetadataByUserID(ctx context.Context, userID string) ([]Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return nil, errors.New("session: BoltMap is closed")
	}

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}

	var items []Metadata
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		if b == nil {
			return errors.New("session: metadata bucket missing")
		}
		return b.ForEach(func(_, raw []byte) error {
			meta, err := decodeMetadata(raw)
			if err != nil {
				return fmt.Errorf("session: decode metadata during list: %w", err)
			}
			if meta.UserID == userID {
				items = append(items, meta)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sortMetadata(items)
	return items, nil
}

func (m *BoltMap) listAllMetadata(ctx context.Context) ([]Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.closeMu.Lock()
	db := m.db
	m.closeMu.Unlock()
	if db == nil {
		return nil, errors.New("session: BoltMap is closed")
	}

	var items []Metadata
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		if b == nil {
			return errors.New("session: metadata bucket missing")
		}
		return b.ForEach(func(_, raw []byte) error {
			meta, err := decodeMetadata(raw)
			if err != nil {
				return fmt.Errorf("session: decode metadata during list: %w", err)
			}
			items = append(items, meta)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sortMetadata(items)
	return items, nil
}

func (m *BoltMap) ResolveLineageTip(ctx context.Context, sessionID string) (LineageResolution, error) {
	items, err := m.listAllMetadata(ctx)
	if err != nil {
		return LineageResolution{}, err
	}
	return resolveLineageTipFromMetadata(strings.TrimSpace(sessionID), items), nil
}

func (m *MemMap) PutMetadata(ctx context.Context, meta Metadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	meta = normalizeMetadata(meta)
	if err := validateMetadataIdentity(meta); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.meta[meta.SessionID]; ok {
		var err error
		meta, err = mergeMetadata(existing, meta)
		if err != nil {
			return err
		}
	}
	meta = finalizeMetadata(meta)
	if err := validateMetadata(meta); err != nil {
		return err
	}
	if err := detectLineageLoop(meta.SessionID, meta.ParentSessionID, func(id string) (Metadata, bool, error) {
		meta, ok := m.meta[id]
		if !ok {
			return Metadata{}, false, nil
		}
		return finalizeMetadata(meta), true, nil
	}); err != nil {
		return err
	}
	if meta.Source != "" && meta.ChatID != "" {
		key := chatBindingKey(meta.Source, meta.ChatID)
		if bound, ok := m.chatUsers[key]; ok {
			if meta.UserID == "" {
				meta.UserID = bound
			} else if bound != meta.UserID {
				return fmt.Errorf("%w: %s/%s bound to %s", ErrUserBindingConflict, meta.Source, meta.ChatID, bound)
			}
		}
		if meta.UserID != "" {
			m.chatUsers[key] = meta.UserID
		}
	}
	if meta.UpdatedAt == 0 {
		meta.UpdatedAt = time.Now().Unix()
	}
	m.meta[meta.SessionID] = meta
	return nil
}

func (m *MemMap) GetMetadata(ctx context.Context, sessionID string) (Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, false, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Metadata{}, false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.meta[sessionID]
	return meta, ok, nil
}

func (m *MemMap) ClearResumePending(ctx context.Context, sessionID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.meta[sessionID]
	if !ok {
		return false, nil
	}
	if !meta.ResumePending && meta.ResumeReason == "" && meta.ResumeMarkedAt == 0 {
		return false, nil
	}
	meta.ResumePending = false
	meta.ResumeReason = ""
	meta.ResumeMarkedAt = 0
	m.meta[sessionID] = meta
	return true, nil
}

func (m *MemMap) ResolveUserID(ctx context.Context, source, chatID string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	source = strings.TrimSpace(source)
	chatID = strings.TrimSpace(chatID)
	if source == "" || chatID == "" {
		return "", false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	userID, ok := m.chatUsers[chatBindingKey(source, chatID)]
	return userID, ok, nil
}

func (m *MemMap) ListMetadataByUserID(ctx context.Context, userID string) ([]Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Metadata, 0, len(m.meta))
	for _, meta := range m.meta {
		if meta.UserID == userID {
			items = append(items, meta)
		}
	}
	sortMetadata(items)
	return items, nil
}

func (m *MemMap) listAllMetadata(ctx context.Context) ([]Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Metadata, 0, len(m.meta))
	for _, meta := range m.meta {
		items = append(items, finalizeMetadata(meta))
	}
	sortMetadata(items)
	return items, nil
}

func (m *MemMap) ResolveLineageTip(ctx context.Context, sessionID string) (LineageResolution, error) {
	items, err := m.listAllMetadata(ctx)
	if err != nil {
		return LineageResolution{}, err
	}
	return resolveLineageTipFromMetadata(strings.TrimSpace(sessionID), items), nil
}
