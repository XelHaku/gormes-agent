package cli

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestPtyChatSidecarPublishesStructuredEventsSeparately is the contract anchor:
// the sidecar accepts structured (map-shaped) events from the publisher and
// hands them to its sink without ever taking PTY bytes through the same path.
func TestPtyChatSidecarPublishesStructuredEventsSeparately(t *testing.T) {
	sink := newRecordingSidecarSink()
	sidecar := NewPtyChatSidecar(PtyChatSidecarConfig{Sink: sink, QueueSize: 4})
	t.Cleanup(func() { _ = sidecar.Close() })

	ok := sidecar.Publish(map[string]any{
		"jsonrpc": "2.0",
		"method":  "event",
		"params":  map[string]any{"type": "tool.start", "name": "repo_search"},
	})
	if !ok {
		t.Fatalf("Publish returned false on healthy sidecar")
	}

	got := sink.waitForEvent(t, time.Second)
	if got == nil {
		t.Fatalf("sink received no event")
	}
	if got["method"] != "event" {
		t.Fatalf("event %+v missing method=event", got)
	}
	if !sidecar.Healthy() {
		t.Fatalf("Healthy() = false after a clean publish")
	}
}

// TestPtyChatSidecarBecomesUnhealthyAfterSinkFailure pins the failure
// signaling: a sink write error must mark the sidecar unhealthy so callers
// can surface "sidecar unavailable" without ever blocking the producer.
func TestPtyChatSidecarBecomesUnhealthyAfterSinkFailure(t *testing.T) {
	sink := newRecordingSidecarSink()
	sink.setError(errors.New("ws closed"))
	sidecar := NewPtyChatSidecar(PtyChatSidecarConfig{Sink: sink, QueueSize: 4})
	t.Cleanup(func() { _ = sidecar.Close() })

	if !sidecar.Publish(map[string]any{"type": "tool.start"}) {
		t.Fatalf("Publish returned false before sink failure was observed")
	}

	deadline := time.Now().Add(time.Second)
	for sidecar.Healthy() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sidecar.Healthy() {
		t.Fatalf("sidecar still reports Healthy after sink error %q", sink.err())
	}

	if got := sidecar.Publish(map[string]any{"type": "tool.end"}); got {
		t.Fatalf("Publish on unhealthy sidecar = true, want false (best-effort drop)")
	}
}

// TestPtyChatSidecarPublishFailureDoesNotBlockPtyAdapter is the keystone test
// for the "sidecar publish failures do not kill the PTY session" acceptance:
// even if every publish enters a broken sink, an independent PTY adapter
// must still accept reads, writes and resizes without sharing state with the
// sidecar.
func TestPtyChatSidecarPublishFailureDoesNotBlockPtyAdapter(t *testing.T) {
	pty := &recordingPtySession{}
	bridge := NewPtyAdapterForSession(pty)

	sink := newRecordingSidecarSink()
	sink.setError(errors.New("publisher dead"))
	sidecar := NewPtyChatSidecar(PtyChatSidecarConfig{Sink: sink, QueueSize: 1})
	t.Cleanup(func() { _ = sidecar.Close() })

	for i := 0; i < 8; i++ {
		sidecar.Publish(map[string]any{"type": "tool.tick", "i": i})
	}

	deadline := time.Now().Add(time.Second)
	for sidecar.Healthy() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sidecar.Healthy() {
		t.Fatalf("sidecar still healthy after deliberate sink errors")
	}

	if err := bridge.Write([]byte("ls\n")); err != nil {
		t.Fatalf("bridge.Write after sidecar failure err = %v, want nil", err)
	}
	if err := bridge.Resize(120, 40); err != nil {
		t.Fatalf("bridge.Resize after sidecar failure err = %v, want nil", err)
	}
	if len(pty.writes) != 1 || string(pty.writes[0]) != "ls\n" {
		t.Fatalf("PTY writes = %q, want exactly the post-failure write", pty.writes)
	}
	if len(pty.resizes) != 1 || pty.resizes[0] != (PtySize{Cols: 120, Rows: 40}) {
		t.Fatalf("PTY resizes = %+v, want exactly the post-failure resize", pty.resizes)
	}
}

// TestPtyChatSidecarFullQueueDropsAndStaysOpen verifies the publisher's
// best-effort guarantee: when the worker is slow, full-queue Publish() drops
// the new event and returns false but does not mark the sidecar unhealthy.
func TestPtyChatSidecarFullQueueDropsAndStaysOpen(t *testing.T) {
	sink := newRecordingSidecarSink()
	sink.block()
	sidecar := NewPtyChatSidecar(PtyChatSidecarConfig{Sink: sink, QueueSize: 1})
	t.Cleanup(func() {
		sink.unblock()
		_ = sidecar.Close()
	})

	dropped := 0
	for i := 0; i < 50; i++ {
		if !sidecar.Publish(map[string]any{"type": "tool.tick", "i": i}) {
			dropped++
		}
	}

	if dropped == 0 {
		t.Fatalf("expected at least one Publish to drop on a wedged sink, got none")
	}
	if !sidecar.Healthy() {
		t.Fatalf("Healthy() = false after queue-full drops; queue full is best-effort, not a transport error")
	}
}

// TestPtyChatSidecarRejectsStructuredEventsAfterClose pins that Close() is
// terminal: subsequent Publish() calls must return false and never reach the
// sink, so the sidecar can be torn down without racing the producer.
func TestPtyChatSidecarRejectsStructuredEventsAfterClose(t *testing.T) {
	sink := newRecordingSidecarSink()
	sidecar := NewPtyChatSidecar(PtyChatSidecarConfig{Sink: sink, QueueSize: 4})

	if err := sidecar.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}
	if sidecar.Publish(map[string]any{"type": "tool.start"}) {
		t.Fatalf("Publish after Close returned true; want false")
	}
	if sidecar.Healthy() {
		t.Fatalf("Healthy() = true after Close; want false")
	}
	// Idempotent close — a defer Close in the public path must not panic.
	if err := sidecar.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}

	if events := sink.snapshot(); len(events) != 0 {
		t.Fatalf("sink received events after Close: %+v", events)
	}
}

type recordingSidecarSink struct {
	mu       sync.Mutex
	events   []map[string]any
	cond     *sync.Cond
	failure  error
	blocking bool
	gate     chan struct{}
}

func newRecordingSidecarSink() *recordingSidecarSink {
	s := &recordingSidecarSink{gate: make(chan struct{})}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *recordingSidecarSink) Publish(event map[string]any) error {
	s.mu.Lock()
	if s.blocking {
		gate := s.gate
		s.mu.Unlock()
		<-gate
		s.mu.Lock()
	}
	defer s.mu.Unlock()

	if s.failure != nil {
		return s.failure
	}

	clone := make(map[string]any, len(event))
	for k, v := range event {
		clone[k] = v
	}
	s.events = append(s.events, clone)
	s.cond.Broadcast()
	return nil
}

func (s *recordingSidecarSink) Close() error { return nil }

func (s *recordingSidecarSink) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = err
}

func (s *recordingSidecarSink) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failure
}

func (s *recordingSidecarSink) block() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocking = true
}

func (s *recordingSidecarSink) unblock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocking {
		s.blocking = false
		close(s.gate)
		s.gate = make(chan struct{})
	}
}

func (s *recordingSidecarSink) snapshot() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, len(s.events))
	copy(out, s.events)
	return out
}

func (s *recordingSidecarSink) waitForEvent(t *testing.T, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.events) == 0 {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil
		}
		done := make(chan struct{})
		go func() {
			time.Sleep(remaining)
			s.mu.Lock()
			s.cond.Broadcast()
			s.mu.Unlock()
			close(done)
		}()
		s.cond.Wait()
		select {
		case <-done:
		default:
		}
	}
	return s.events[0]
}
