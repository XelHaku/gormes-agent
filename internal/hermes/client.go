// Package hermes owns the outbound chat-stream client contracts used by the
// kernel. It ships transport adapters for Hermes-compatible servers and
// provider-native APIs, and it is the ONLY Gormes package that opens HTTP
// connections.
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
	"strings"
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
	Model           string
	MaxTokens       int
	Temperature     *float64
	Messages        []Message
	SessionID       string
	Stream          bool
	ReasoningEffort *ReasoningEffort
	Tools           []ToolDescriptor // omitempty at wire time via the Marshal path in http_client
}

type ReasoningEffort string

const (
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
)

type ReasoningEffortSource string

const (
	ReasoningEffortSourceConfigDefault ReasoningEffortSource = "config_default"
	ReasoningEffortSourceTurnOverride  ReasoningEffortSource = "turn_override"
)

type ReasoningEffortState string

const (
	ReasoningEffortStateDefault     ReasoningEffortState = "default"
	ReasoningEffortStateDisabled    ReasoningEffortState = "disabled"
	ReasoningEffortStateOverride    ReasoningEffortState = "override"
	ReasoningEffortStateInvalid     ReasoningEffortState = "invalid"
	ReasoningEffortStateUnsupported ReasoningEffortState = "unsupported"
)

type ReasoningEffortEvidence struct {
	State     ReasoningEffortState
	Source    ReasoningEffortSource
	Requested string
	Effort    ReasoningEffort
	Supported bool
	Forwarded bool
	Reason    string
}

func NormalizeReasoningEffort(effort ReasoningEffort) (ReasoningEffort, bool) {
	normalized := ReasoningEffort(strings.ToLower(strings.TrimSpace(string(effort))))
	switch normalized {
	case ReasoningEffortNone,
		ReasoningEffortMinimal,
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh:
		return normalized, true
	default:
		return "", false
	}
}

func ResolveReasoningEffort(raw string, source ReasoningEffortSource, status ProviderStatus) ReasoningEffortEvidence {
	if source == "" {
		source = ReasoningEffortSourceConfigDefault
	}
	requested := strings.ToLower(strings.TrimSpace(raw))
	supported := ProviderSupportsReasoningEffort(status)
	if requested == "" {
		return ReasoningEffortEvidence{
			State:     ReasoningEffortStateDefault,
			Source:    source,
			Supported: supported,
			Reason:    "no reasoning_effort supplied; provider default applies",
		}
	}

	effort, ok := NormalizeReasoningEffort(ReasoningEffort(requested))
	if !ok {
		return ReasoningEffortEvidence{
			State:     ReasoningEffortStateInvalid,
			Source:    source,
			Requested: requested,
			Supported: supported,
			Reason:    "invalid reasoning_effort " + requested + "; valid values are none, minimal, low, medium, high, xhigh",
		}
	}

	state := ReasoningEffortStateOverride
	reason := "reasoning_effort " + string(effort) + " will be sent on this request"
	if effort == ReasoningEffortNone {
		state = ReasoningEffortStateDisabled
		reason = "reasoning disabled for this request"
	}
	if !supported {
		normalized := normalizeProviderStatus(status)
		return ReasoningEffortEvidence{
			State:     ReasoningEffortStateUnsupported,
			Source:    source,
			Requested: requested,
			Effort:    effort,
			Supported: false,
			Forwarded: false,
			Reason:    "provider runtime " + normalized.Runtime + " does not serialize reasoning_effort",
		}
	}
	return ReasoningEffortEvidence{
		State:     state,
		Source:    source,
		Requested: requested,
		Effort:    effort,
		Supported: true,
		Forwarded: true,
		Reason:    reason,
	}
}

func ProviderSupportsReasoningEffort(status ProviderStatus) bool {
	normalized := normalizeProviderStatus(status)
	return normalized.Runtime == "chat_completions"
}

// ToolDescriptor mirrors tools.ToolDescriptor so hermes stays
// dependency-free of the tools package. Serialised shape is
// OpenAI's {"type":"function","function":{...}} wrapper — the
// kernel populates Tools by calling tools.Registry.Descriptors()
// and converting them.
type ToolDescriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// MarshalJSON for ToolDescriptor wraps in OpenAI's function envelope.
func (d ToolDescriptor) MarshalJSON() ([]byte, error) {
	inner := struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}{Name: d.Name, Description: d.Description, Parameters: sanitizeToolSchema(d.Schema)}
	wrap := struct {
		Type     string `json:"type"`
		Function any    `json:"function"`
	}{Type: "function", Function: inner}
	return json.Marshal(wrap)
}

type Message struct {
	Role             string               `json:"role"`
	Content          string               `json:"content"`
	ContentParts     []MessageContentPart `json:"content_parts,omitempty"`
	CacheControl     *CacheControl        `json:"cache_control,omitempty"`
	Reasoning        *ReasoningContent    `json:"reasoning,omitempty"`
	ReasoningContent *string              `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall           `json:"tool_calls,omitempty"`   // set only on assistant messages that requested tools
	ToolCallID       string               `json:"tool_call_id,omitempty"` // set only on "tool" role messages replying to a call
	Name             string               `json:"name,omitempty"`         // set only on "tool" role messages; echoes the tool name
}

type MessageContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// CacheControl carries provider-specific prompt-caching hints on content
// blocks. Providers that do not support cache markers ignore it.
type CacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

// ReasoningContent carries provider-native reasoning echoes that must be
// replayed alongside assistant turns for providers that require them.
type ReasoningContent struct {
	Text            string `json:"text,omitempty"`
	Signature       string `json:"signature,omitempty"`
	RedactedContent string `json:"redacted_content,omitempty"`
}

// ToolCall is one function-call request made by the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Event struct {
	Kind         EventKind
	Token        string
	Reasoning    string
	FinishReason string
	TokensIn     int
	TokensOut    int
	ToolCalls    []ToolCall // populated only on EventDone with FinishReason=="tool_calls"
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
