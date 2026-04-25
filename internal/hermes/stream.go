package hermes

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"
)

type chatStream struct {
	body      io.ReadCloser
	sse       *sseReader
	sessionID string
	closed    bool
	mu        sync.Mutex
	// pending holds extra events decoded from a single chunk (e.g. a chunk
	// that carries both `reasoning` and `content` in one delta). Drained FIFO
	// before the next SSE frame is read.
	pending []Event

	// Pending tool-call accumulator, keyed by upstream index field.
	// Populated across partial tool_calls deltas; flushed on finish_reason=="tool_calls".
	pendingCalls map[int]*pendingToolCall
	tools        []ToolDescriptor
}

type pendingToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func newChatStream(body io.ReadCloser, sessionID string, tools []ToolDescriptor) *chatStream {
	return &chatStream{
		body:         body,
		sse:          newSSEReader(body),
		sessionID:    sessionID,
		pendingCalls: make(map[int]*pendingToolCall),
		tools:        SanitizeToolDescriptors(tools),
	}
}

func (s *chatStream) SessionID() string { return s.sessionID }

// Close is idempotent. Safe to call multiple times; first call releases the
// HTTP body and subsequent calls are no-ops.
func (s *chatStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

type orChunkDelta struct {
	Content   string            `json:"content"`
	Reasoning string            `json:"reasoning"`
	ToolCalls []orChunkToolCall `json:"tool_calls,omitempty"`
}

type orChunkToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type orChunkChoice struct {
	Delta        orChunkDelta `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

type orChunkUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type orChunk struct {
	Choices []orChunkChoice `json:"choices"`
	Usage   *orChunkUsage   `json:"usage,omitempty"`
}

// Recv returns the next semantically meaningful Event. Skips malformed frames
// silently so one bad chunk doesn't kill an otherwise-healthy stream. Returns
// io.EOF on normal end-of-stream (including a mid-stream TCP drop that
// closes the scanner), or ctx.Err() if the caller's context is cancelled.
//
// A single SSE chunk may carry more than one logical event (e.g. reasoning +
// content in the same delta). Recv emits them one at a time, reasoning first,
// in FIFO order before advancing the SSE scanner.
func (s *chatStream) Recv(ctx context.Context) (Event, error) {
	for {
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		default:
		}
		if len(s.pending) > 0 {
			ev := s.pending[0]
			s.pending = s.pending[1:]
			return ev, nil
		}
		f, err := s.sse.Next(ctx)
		if err != nil {
			if err == io.EOF && len(s.pendingCalls) > 0 {
				return s.flushPendingCallsOnEOF()
			}
			return Event{}, err
		}
		if strings.TrimSpace(f.data) == "[DONE]" {
			return Event{}, io.EOF
		}
		var chunk orChunk
		if jerr := json.Unmarshal([]byte(f.data), &chunk); jerr != nil {
			continue // skip malformed frame, keep stream alive
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		c := chunk.Choices[0]
		raw := json.RawMessage(f.data)

		// Accumulate tool-call deltas across chunks. Multiple deltas per stream
		// carry partial tool_calls data keyed by .index; we buffer until
		// finish_reason=="tool_calls" arrives, then emit as a single EventDone.
		for _, tc := range c.Delta.ToolCalls {
			p, ok := s.pendingCalls[tc.Index]
			if !ok {
				p = &pendingToolCall{}
				s.pendingCalls[tc.Index] = p
			}
			if tc.ID != "" {
				p.id = tc.ID
			}
			if tc.Function.Name != "" {
				p.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				p.arguments.WriteString(tc.Function.Arguments)
			}
		}

		var events []Event
		if c.Delta.Reasoning != "" {
			events = append(events, Event{Kind: EventReasoning, Reasoning: c.Delta.Reasoning, Raw: raw})
		}
		if c.Delta.Content != "" {
			events = append(events, Event{Kind: EventToken, Token: c.Delta.Content, Raw: raw})
		}
		if c.FinishReason != "" {
			ev := Event{Kind: EventDone, FinishReason: c.FinishReason, Raw: raw}
			if chunk.Usage != nil {
				ev.TokensIn = chunk.Usage.PromptTokens
				ev.TokensOut = chunk.Usage.CompletionTokens
			}
			if c.FinishReason == "tool_calls" && len(s.pendingCalls) > 0 {
				toolCalls, err := RepairToolCalls(flushPending(s.pendingCalls), s.tools)
				s.pendingCalls = make(map[int]*pendingToolCall) // reset for possible reuse
				if err != nil {
					return Event{}, err
				}
				ev.ToolCalls = toolCalls
			}
			events = append(events, ev)
		}
		if len(events) == 0 {
			continue
		}
		head := events[0]
		if len(events) > 1 {
			s.pending = append(s.pending, events[1:]...)
		}
		return head, nil
	}
}

func (s *chatStream) flushPendingCallsOnEOF() (Event, error) {
	toolCalls, err := RepairToolCalls(flushPending(s.pendingCalls), s.tools)
	s.pendingCalls = make(map[int]*pendingToolCall)
	if err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind:         EventDone,
		FinishReason: "tool_calls",
		ToolCalls:    toolCalls,
	}
	return ev, nil
}

// flushPending converts the accumulator map into a sorted, finalised ToolCall slice.
func flushPending(m map[int]*pendingToolCall) []ToolCall {
	indexes := make([]int, 0, len(m))
	for idx := range m {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		p := m[idx]
		out = append(out, ToolCall{
			ID:        p.id,
			Name:      p.name,
			Arguments: json.RawMessage(p.arguments.String()),
		})
	}
	return out
}
