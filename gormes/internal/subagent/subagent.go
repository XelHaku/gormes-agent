// gormes/internal/subagent/subagent.go
package subagent

import (
	"context"
	"sync"
	"sync/atomic"
)

// Subagent represents a single child execution. Construction is the
// SubagentManager's responsibility; consumers interact via Events() and
// WaitForResult() only.
type Subagent struct {
	ID       string
	ParentID string
	Depth    int

	cfg           SubagentConfig
	ctx           context.Context
	cancel        context.CancelFunc // cancels the composed child ctx
	timeoutCancel context.CancelFunc // optional; nil if cfg.Timeout == 0
	callerStop    func() bool        // optional; disarms Spawn caller bridge

	publicEvents chan SubagentEvent // closed by lifecycle goroutine after runner returns
	done         chan struct{}      // closed after result is set

	interruptMsg atomic.Value // string; written by Manager.Interrupt before sa.cancel()

	mu           sync.RWMutex
	result       *SubagentResult
	cancelReason string
}

// Events returns a receive-only channel that receives every SubagentEvent
// emitted by the runner, in order. The channel is closed exactly once when
// the runner has returned and all events have been forwarded.
func (s *Subagent) Events() <-chan SubagentEvent { return s.publicEvents }

// WaitForResult blocks until the subagent finishes (returning the result) or
// the supplied ctx is cancelled (returning ctx.Err()). The caller's ctx
// cancellation does NOT cancel the subagent — use Manager.Interrupt for that.
func (s *Subagent) WaitForResult(ctx context.Context) (*SubagentResult, error) {
	select {
	case <-s.done:
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// setResult is called exactly once by the lifecycle goroutine before close(s.done).
func (s *Subagent) setResult(r *SubagentResult) {
	s.mu.Lock()
	s.result = r
	s.mu.Unlock()
}

func (s *Subagent) setCancelReason(reason string) {
	if reason == "" {
		return
	}
	s.mu.Lock()
	if s.cancelReason == "" {
		s.cancelReason = reason
	}
	s.mu.Unlock()
}

func (s *Subagent) getCancelReason() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cancelReason
}
