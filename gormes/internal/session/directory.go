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
	SessionID string `json:"session_id"`
	Source    string `json:"source,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

func normalizeMetadata(meta Metadata) Metadata {
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.Source = strings.TrimSpace(meta.Source)
	meta.ChatID = strings.TrimSpace(meta.ChatID)
	meta.UserID = strings.TrimSpace(meta.UserID)
	return meta
}

func validateMetadata(meta Metadata) error {
	if meta.SessionID == "" {
		return errors.New("session: metadata session_id is required")
	}
	if (meta.Source == "") != (meta.ChatID == "") {
		return errors.New("session: metadata source and chat_id must both be set or both be empty")
	}
	if meta.UserID != "" && (meta.Source == "" || meta.ChatID == "") {
		return errors.New("session: metadata user_id requires source and chat_id")
	}
	return nil
}

func mergeMetadata(existing, incoming Metadata) Metadata {
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
	if incoming.UpdatedAt != 0 {
		out.UpdatedAt = incoming.UpdatedAt
	}
	return out
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
	return normalizeMetadata(meta), nil
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
	if err := validateMetadata(meta); err != nil {
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
			meta = mergeMetadata(existing, meta)
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

func (m *MemMap) PutMetadata(ctx context.Context, meta Metadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	meta = normalizeMetadata(meta)
	if err := validateMetadata(meta); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.meta[meta.SessionID]; ok {
		meta = mergeMetadata(existing, meta)
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
