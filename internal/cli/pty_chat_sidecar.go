package cli

import (
	"sync"
	"sync/atomic"
)

// DefaultPtyChatSidecarQueueSize matches Hermes' upstream WsPublisherTransport
// drop threshold so the dashboard sidecar feed has the same back-pressure
// envelope as the upstream Python reference implementation.
const DefaultPtyChatSidecarQueueSize = 256

// PtyChatSidecarSink is the structured-event transport seam used by
// PtyChatSidecar. Implementations push one event at a time and may return an
// error to mark the transport dead; callers must never block the producer.
type PtyChatSidecarSink interface {
	Publish(event map[string]any) error
	Close() error
}

// PtyChatSidecarConfig wires a sink and queue size for a sidecar publisher.
// Sink is required; QueueSize falls back to DefaultPtyChatSidecarQueueSize.
type PtyChatSidecarConfig struct {
	Sink      PtyChatSidecarSink
	QueueSize int
}

// PtyChatSidecar is a best-effort structured-event publisher that runs
// alongside the PTY transport. It is deliberately decoupled from PtyAdapter:
// PTY bytes never flow through this type and a sink failure here must not
// reach the PTY producer.
type PtyChatSidecar struct {
	sink    PtyChatSidecarSink
	queue   chan map[string]any
	done    chan struct{}
	wg      sync.WaitGroup
	healthy atomic.Bool
	closed  atomic.Bool
	once    sync.Once
}

// NewPtyChatSidecar starts the worker goroutine that drains queued events
// into the sink. A nil sink yields a permanently-unhealthy publisher so
// callers can treat "no sidecar configured" identically to a dead transport.
func NewPtyChatSidecar(cfg PtyChatSidecarConfig) *PtyChatSidecar {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = DefaultPtyChatSidecarQueueSize
	}

	s := &PtyChatSidecar{
		sink:  cfg.Sink,
		queue: make(chan map[string]any, queueSize),
		done:  make(chan struct{}),
	}
	if cfg.Sink == nil {
		s.healthy.Store(false)
		s.closed.Store(true)
		close(s.done)
		return s
	}

	s.healthy.Store(true)
	s.wg.Add(1)
	go s.drain()
	return s
}

// Publish enqueues a structured event. It returns false on a closed or
// unhealthy sidecar, or on a full queue (best-effort drop). It never blocks
// the caller.
func (s *PtyChatSidecar) Publish(event map[string]any) bool {
	if s == nil || s.closed.Load() || !s.healthy.Load() {
		return false
	}
	select {
	case s.queue <- cloneSidecarEvent(event):
		return true
	default:
		return false
	}
}

// Healthy reports whether the worker is alive and the sink has not signaled
// a transport-fatal error.
func (s *PtyChatSidecar) Healthy() bool {
	if s == nil {
		return false
	}
	return s.healthy.Load() && !s.closed.Load()
}

// Close stops the worker. It is idempotent so callers can defer it from any
// path that constructs the sidecar.
func (s *PtyChatSidecar) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		s.closed.Store(true)
		s.healthy.Store(false)
		close(s.done)
		s.wg.Wait()
		if s.sink != nil {
			err = s.sink.Close()
		}
	})
	return err
}

func (s *PtyChatSidecar) drain() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			return
		case event := <-s.queue:
			if err := s.sink.Publish(event); err != nil {
				s.healthy.Store(false)
				return
			}
		}
	}
}

func cloneSidecarEvent(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
