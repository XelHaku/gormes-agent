// Package hermes speaks HTTP+SSE to Python's api_server on port 8642.
// It is the ONLY Gormes package that opens HTTP connections.
//
// Task 5 (this file) declares the interfaces and types.
// Task 6 implements NewHTTPClient / OpenStream / Health.
// Task 7 implements OpenRunEvents.
// Task 8 implements MockClient for tests.
package hermes

import (
	"context"
	"encoding/json"
	"errors"
)

// Client is the single outbound HTTP surface of Gormes.
type Client interface {
	OpenStream(ctx context.Context, req ChatRequest) (Stream, error)
	OpenRunEvents(ctx context.Context, runID string) (RunEventStream, error)
	Health(ctx context.Context) error
}

// Stream is a pull-based SSE consumer — callers Recv() one Event at a time.
// Pull-based is deliberate: the kernel paces intake so a fast provider cannot
// firehose the render pipeline.
type Stream interface {
	Recv(ctx context.Context) (Event, error)
	SessionID() string
	Close() error
}

type RunEventStream interface {
	Recv(ctx context.Context) (RunEvent, error)
	Close() error
}

type ChatRequest struct {
	Model     string
	Messages  []Message
	SessionID string
	Stream    bool
}

type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

type Event struct {
	Kind         EventKind
	Token        string
	Reasoning    string
	FinishReason string
	TokensIn     int
	TokensOut    int
	Raw          json.RawMessage
}

type EventKind int

const (
	EventToken EventKind = iota
	EventReasoning
	EventDone
)

type RunEvent struct {
	Type      RunEventType
	ToolName  string
	Preview   string
	Reasoning string
	Raw       json.RawMessage
}

type RunEventType int

const (
	RunEventToolStarted RunEventType = iota
	RunEventToolCompleted
	RunEventReasoningAvailable
	RunEventUnknown
)

// ErrRunEventsNotSupported is returned by OpenRunEvents when the server
// responds 404 — which is the case for non-Hermes OpenAI-compatible servers
// (LM Studio, Open WebUI) that don't implement /v1/runs.
var ErrRunEventsNotSupported = errors.New("hermes: /v1/runs not supported by this server")
