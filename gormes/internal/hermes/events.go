package hermes

import (
	"context"
	"encoding/json"
	"io"
	"sync"
)

type runEventStream struct {
	body   io.ReadCloser
	sse    *sseReader
	closed bool
	mu     sync.Mutex
}

func newRunEventStream(body io.ReadCloser) *runEventStream {
	return &runEventStream{body: body, sse: newSSEReader(body)}
}

// Close is idempotent.
func (s *runEventStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

type toolStartedPayload struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type toolCompletedPayload struct {
	Name    string `json:"name"`
	Preview string `json:"result_preview,omitempty"`
}

type reasoningPayload struct {
	Text string `json:"text"`
}

// Recv returns the next RunEvent. Unknown event names map to RunEventUnknown
// so Gormes is forward-compatible with new Hermes event kinds.
func (s *runEventStream) Recv(ctx context.Context) (RunEvent, error) {
	for {
		select {
		case <-ctx.Done():
			return RunEvent{}, ctx.Err()
		default:
		}
		f, err := s.sse.Next(ctx)
		if err != nil {
			return RunEvent{}, err
		}
		switch f.event {
		case "tool.started":
			var p toolStartedPayload
			_ = json.Unmarshal([]byte(f.data), &p)
			return RunEvent{
				Type:     RunEventToolStarted,
				ToolName: p.Name,
				Preview:  previewArgs(p.Args),
				Raw:      json.RawMessage(f.data),
			}, nil
		case "tool.completed":
			var p toolCompletedPayload
			_ = json.Unmarshal([]byte(f.data), &p)
			return RunEvent{
				Type:     RunEventToolCompleted,
				ToolName: p.Name,
				Preview:  p.Preview,
				Raw:      json.RawMessage(f.data),
			}, nil
		case "reasoning.available":
			var p reasoningPayload
			_ = json.Unmarshal([]byte(f.data), &p)
			return RunEvent{
				Type:      RunEventReasoningAvailable,
				Reasoning: p.Text,
				Raw:       json.RawMessage(f.data),
			}, nil
		default:
			return RunEvent{Type: RunEventUnknown, Raw: json.RawMessage(f.data)}, nil
		}
	}
}

// previewArgs serialises arbitrary tool args JSON and truncates to 60 chars.
func previewArgs(args any) string {
	if args == nil {
		return ""
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	s := string(raw)
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
