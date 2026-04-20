package store

import (
	"context"
	"sync"
)

// Compile-time interface check.
var _ Store = (*RecordingStore)(nil)

// RecordingStore is a test double that captures every Command passed to
// Exec. Safe for concurrent use. Use Commands() to read a snapshot.
type RecordingStore struct {
	mu   sync.Mutex
	cmds []Command
}

// NewRecording constructs an empty RecordingStore.
func NewRecording() *RecordingStore { return &RecordingStore{} }

func (r *RecordingStore) Exec(ctx context.Context, cmd Command) (Ack, error) {
	if err := ctx.Err(); err != nil {
		return Ack{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
	return Ack{}, nil
}

// Commands returns a snapshot slice — safe to iterate while other goroutines
// continue recording.
func (r *RecordingStore) Commands() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Command, len(r.cmds))
	copy(out, r.cmds)
	return out
}
