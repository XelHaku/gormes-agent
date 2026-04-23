package hermes

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"
)

type anthropicStream struct {
	body        io.ReadCloser
	sse         *sseReader
	closed      bool
	mu          sync.Mutex
	pending     []Event
	inputTokens int
	toolCalls   map[int]*anthropicPendingToolCall
}

type anthropicPendingToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type anthropicMessageStart struct {
	Message struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
}

type anthropicContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content_block"`
}

type anthropicContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthropicMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicErrorPayload struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func newAnthropicStream(body io.ReadCloser) *anthropicStream {
	return &anthropicStream{
		body:      body,
		sse:       newSSEReader(body),
		toolCalls: make(map[int]*anthropicPendingToolCall),
	}
}

func (s *anthropicStream) SessionID() string { return "" }

func (s *anthropicStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func (s *anthropicStream) Recv(ctx context.Context) (Event, error) {
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
		frame, err := s.sse.Next(ctx)
		if err != nil {
			return Event{}, err
		}
		eventType := anthropicFrameType(frame)
		raw := json.RawMessage(frame.data)
		switch eventType {
		case "message_start":
			var start anthropicMessageStart
			if err := json.Unmarshal(raw, &start); err != nil {
				continue
			}
			s.inputTokens = start.Message.Usage.InputTokens
		case "content_block_start":
			var start anthropicContentBlockStart
			if err := json.Unmarshal(raw, &start); err != nil {
				continue
			}
			if start.ContentBlock.Type != "tool_use" {
				continue
			}
			call := &anthropicPendingToolCall{id: start.ContentBlock.ID, name: start.ContentBlock.Name}
			if len(start.ContentBlock.Input) > 0 && string(start.ContentBlock.Input) != "null" && string(start.ContentBlock.Input) != "{}" {
				call.arguments.Write(start.ContentBlock.Input)
			}
			s.toolCalls[start.Index] = call
		case "content_block_delta":
			var delta anthropicContentBlockDelta
			if err := json.Unmarshal(raw, &delta); err != nil {
				continue
			}
			switch delta.Delta.Type {
			case "text_delta":
				return Event{Kind: EventToken, Token: delta.Delta.Text, Raw: raw}, nil
			case "thinking_delta":
				return Event{Kind: EventReasoning, Reasoning: delta.Delta.Thinking, Raw: raw}, nil
			case "input_json_delta":
				call, ok := s.toolCalls[delta.Index]
				if !ok {
					call = &anthropicPendingToolCall{}
					s.toolCalls[delta.Index] = call
				}
				call.arguments.WriteString(delta.Delta.PartialJSON)
			}
		case "message_delta":
			var delta anthropicMessageDelta
			if err := json.Unmarshal(raw, &delta); err != nil {
				continue
			}
			done := Event{
				Kind:         EventDone,
				FinishReason: mapAnthropicStopReason(delta.Delta.StopReason),
				TokensIn:     s.inputTokens,
				TokensOut:    delta.Usage.OutputTokens,
				Raw:          raw,
			}
			if done.FinishReason == "tool_calls" && len(s.toolCalls) > 0 {
				done.ToolCalls = flushAnthropicToolCalls(s.toolCalls)
				s.toolCalls = make(map[int]*anthropicPendingToolCall)
			}
			return done, nil
		case "error":
			var payload anthropicErrorPayload
			if err := json.Unmarshal(raw, &payload); err != nil {
				return Event{}, err
			}
			return Event{}, &HTTPError{Status: anthropicErrorStatus(payload.Error.Type), Body: payload.Error.Message}
		case "message_stop":
			continue
		default:
			continue
		}
	}
}

func anthropicFrameType(frame *sseFrame) string {
	if frame == nil {
		return ""
	}
	if frame.event != "" {
		return frame.event
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(frame.data), &envelope); err == nil && envelope.Type != "" {
		return envelope.Type
	}
	return ""
}

func flushAnthropicToolCalls(m map[int]*anthropicPendingToolCall) []ToolCall {
	indexes := make([]int, 0, len(m))
	for idx := range m {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		call := m[idx]
		args := call.arguments.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out = append(out, ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(args),
		})
	}
	return out
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens", "model_context_window_exceeded":
		return "length"
	case "refusal":
		return "content_filter"
	default:
		if reason == "" {
			return "stop"
		}
		return reason
	}
}

func anthropicErrorStatus(kind string) int {
	switch kind {
	case "rate_limit_error":
		return 429
	case "authentication_error":
		return 401
	case "permission_error":
		return 403
	case "invalid_request_error":
		return 400
	case "overloaded_error":
		return 529
	default:
		return 500
	}
}
