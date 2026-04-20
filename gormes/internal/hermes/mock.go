package hermes

import (
	"context"
	"io"
	"sync"
)

// Compile-time interface checks. If any mock drifts out of spec, the build
// fails loudly — we do NOT want the kernel tests passing against an incomplete
// mock only to have the real HTTP client surface a missing method at runtime.
var (
	_ Client         = (*MockClient)(nil)
	_ Stream         = (*MockStream)(nil)
	_ RunEventStream = (*MockRunEventStream)(nil)
)

// MockClient is a test harness for the kernel. Tests queue event sequences
// via Script / ScriptRunEvents; each OpenStream / OpenRunEvents call dequeues
// the next scripted sequence and returns a Stream / RunEventStream backed by it.
type MockClient struct {
	mu          sync.Mutex
	streams     []*MockStream
	runStreams  []*MockRunEventStream
	healthErr   error
	requests    []ChatRequest
}

func NewMockClient() *MockClient { return &MockClient{} }

// SetHealth makes Health return err for the life of this MockClient.
func (m *MockClient) SetHealth(err error) { m.healthErr = err }

// Script queues a Stream emitting the given Events for the next OpenStream call.
// sessionID is what Stream.SessionID() returns.
func (m *MockClient) Script(events []Event, sessionID string) *MockStream {
	s := &MockStream{events: events, sessionID: sessionID}
	m.mu.Lock()
	m.streams = append(m.streams, s)
	m.mu.Unlock()
	return s
}

// ScriptRunEvents queues a RunEventStream for the next OpenRunEvents call.
// If no run-events are ever scripted, OpenRunEvents returns
// ErrRunEventsNotSupported (matches the real-server 404 path).
func (m *MockClient) ScriptRunEvents(events []RunEvent) *MockRunEventStream {
	s := &MockRunEventStream{events: events}
	m.mu.Lock()
	m.runStreams = append(m.runStreams, s)
	m.mu.Unlock()
	return s
}

func (m *MockClient) Health(ctx context.Context) error {
	return m.healthErr
}

func (m *MockClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if len(m.streams) == 0 {
		// Return an already-exhausted stream so callers see a clean EOF.
		return &MockStream{}, nil
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

// Requests returns a snapshot of every ChatRequest passed to OpenStream
// since this MockClient was constructed. Safe to call from any goroutine.
func (m *MockClient) Requests() []ChatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChatRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func (m *MockClient) OpenRunEvents(ctx context.Context, _ string) (RunEventStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.runStreams) == 0 {
		return nil, ErrRunEventsNotSupported
	}
	s := m.runStreams[0]
	m.runStreams = m.runStreams[1:]
	return s, nil
}

// MockStream emits a pre-scripted slice of Events, one per Recv, then io.EOF.
type MockStream struct {
	events    []Event
	sessionID string
	pos       int
	closed    bool
	mu        sync.Mutex
}

func (s *MockStream) SessionID() string { return s.sessionID }

func (s *MockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *MockStream) Recv(ctx context.Context) (Event, error) {
	// Check cancellation FIRST — caller expects prompt ctx.Err on cancel even
	// if more scripted events remain.
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	default:
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Event{}, io.EOF
	}
	if s.pos >= len(s.events) {
		s.mu.Unlock()
		return Event{}, io.EOF
	}
	e := s.events[s.pos]
	s.pos++
	s.mu.Unlock()
	return e, nil
}

// MockRunEventStream is the run-events analogue of MockStream.
type MockRunEventStream struct {
	events []RunEvent
	pos    int
	closed bool
	mu     sync.Mutex
}

func (s *MockRunEventStream) Recv(ctx context.Context) (RunEvent, error) {
	select {
	case <-ctx.Done():
		return RunEvent{}, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.pos >= len(s.events) {
		return RunEvent{}, io.EOF
	}
	e := s.events[s.pos]
	s.pos++
	return e, nil
}

func (s *MockRunEventStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
