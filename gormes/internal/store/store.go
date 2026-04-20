// Package store defines the persistence seam for Gormes. Phase 1 ships
// NoopStore — a completely stateless no-op that accepts every command and
// acks immediately. Python's state.db owns conversation history in Phase 1.
//
// Phase 3 replaces NoopStore with a SQLite-backed implementation behind the
// SAME Store interface. No kernel changes are required across that swap.
package store

import (
	"context"
	"encoding/json"
	"time"
)

// Compile-time interface checks. If a future implementation drifts out of
// spec, the build fails loudly rather than at runtime.
var (
	_ Store = (*NoopStore)(nil)
	_ Store = (*SlowStore)(nil)
)

type CommandKind int

const (
	AppendUserTurn CommandKind = iota
	FinalizeAssistantTurn
)

func (c CommandKind) String() string {
	switch c {
	case AppendUserTurn:
		return "append_user_turn"
	case FinalizeAssistantTurn:
		return "finalize_assistant_turn"
	}
	return "unknown"
}

type Command struct {
	Kind    CommandKind
	Payload json.RawMessage
}

type Ack struct {
	TurnID int64 // 0 from NoopStore; populated by Phase-3 SQLite impl
}

// Store is the single persistence seam. Exec blocks until ack or ctx
// deadline; the kernel enforces a 250ms ack deadline before transitioning
// to PhaseFailed.
type Store interface {
	Exec(ctx context.Context, cmd Command) (Ack, error)
}

// NoopStore is stateless. It accepts every command and returns immediately.
// The zero value is ready to use; NewNoop is provided for symmetry.
type NoopStore struct{}

func NewNoop() *NoopStore { return &NoopStore{} }

func (*NoopStore) Exec(ctx context.Context, _ Command) (Ack, error) {
	// Honour ctx cancellation immediately — callers rely on this for the
	// kernel's uniform cancellation contract, even against the fastest Store.
	select {
	case <-ctx.Done():
		return Ack{}, ctx.Err()
	default:
		return Ack{TurnID: 0}, nil
	}
}

// SlowStore is a test helper. Each Exec sleeps `delay` before acking — used
// by the kernel's store-ack-deadline test to verify that the kernel trips
// PhaseFailed when persistence is slow.
type SlowStore struct {
	delay time.Duration
}

func NewSlow(d time.Duration) *SlowStore { return &SlowStore{delay: d} }

func (s *SlowStore) Exec(ctx context.Context, _ Command) (Ack, error) {
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return Ack{TurnID: 1}, nil
	case <-ctx.Done():
		return Ack{}, ctx.Err()
	}
}
