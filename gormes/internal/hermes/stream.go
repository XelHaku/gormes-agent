package hermes

import (
	"context"
	"encoding/json"
	"io"
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
}

func newChatStream(body io.ReadCloser, sessionID string) *chatStream {
	return &chatStream{
		body:      body,
		sse:       newSSEReader(body),
		sessionID: sessionID,
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
	Content   string `json:"content"`
	Reasoning string `json:"reasoning"`
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
