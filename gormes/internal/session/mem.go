package session

import (
	"context"
	"sync"
)

// MemMap is an in-memory Map for unit tests. Zero persistence across
// process restarts. Safe for concurrent use.
type MemMap struct {
	mu sync.Mutex
	m  map[string]string

	meta      map[string]Metadata
	chatUsers map[string]string
}

// NewMemMap constructs an empty MemMap.
func NewMemMap() *MemMap {
	return &MemMap{
		m:         make(map[string]string),
		meta:      make(map[string]Metadata),
		chatUsers: make(map[string]string),
	}
}

func (m *MemMap) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key], nil
}

func (m *MemMap) Put(ctx context.Context, key, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if sessionID == "" {
		delete(m.m, key)
	} else {
		m.m[key] = sessionID
	}
	return nil
}

// Close is a no-op. Always returns nil.
func (*MemMap) Close() error { return nil }
