// Package session persists (platform, chat_id) -> session_id mappings so
// Gormes binaries can resume the canonical Python-server transcript across
// restarts. See gormes/docs/superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md.
//
// Two implementations:
//   - BoltMap: bbolt-backed (production). See bolt.go.
//   - MemMap:  in-memory (tests). See mem.go.
//
// Both implement Map. Callers should treat a non-existent key as "no prior
// session" (Get returns ("", nil)) and use Put(key, "") to clear a mapping.
package session

import (
	"context"
	"errors"
	"strconv"
)

// Map persists session_id handles. Safe for concurrent use.
type Map interface {
	// Get returns the session_id for key, or ("", nil) if absent.
	// Honors ctx cancellation at the boundary only: bbolt I/O is not
	// mid-flight interruptible.
	Get(ctx context.Context, key string) (sessionID string, err error)

	// Put writes sessionID for key. Put(key, "") deletes the key and is
	// a no-op if the key was already absent.
	Put(ctx context.Context, key string, sessionID string) error

	// Close releases underlying resources. Idempotent.
	Close() error
}

// ErrDBLocked is returned by OpenBolt when another process holds the bbolt
// file lock. Caller should exit 1 with a clear message — retrying is
// pointless because dual-instance is a config bug.
var ErrDBLocked = errors.New("session: database locked by another process")

// ErrDBCorrupt is returned by OpenBolt when the bbolt file's magic bytes
// are wrong or the header is malformed. Caller should exit 1 and instruct
// the user to delete the file.
var ErrDBCorrupt = errors.New("session: database appears corrupted")

// TUIKey returns the canonical map key for the TUI binary.
func TUIKey() string { return "tui:default" }

// TelegramKey returns the canonical map key for a Telegram chat.
func TelegramKey(chatID int64) string {
	return "telegram:" + strconv.FormatInt(chatID, 10)
}
