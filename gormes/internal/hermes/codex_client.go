package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const (
	codexResponsesPath  = "/v1/responses"
	codexModelsPath     = "/v1/models"
	defaultCodexBaseURL = "https://api.openai.com"
)

type codexClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newCodexClient(baseURL, apiKey string) Client {
	return &codexClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    newStreamingHTTPClient(),
	}
}

type codexRequest struct {
	Model        string                `json:"model"`
	Instructions string                `json:"instructions,omitempty"`
	Input        []codexInputItem      `json:"input"`
	Stream       bool                  `json:"stream"`
	Tools        []codexToolDescriptor `json:"tools,omitempty"`
}

type codexInputItem struct {
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	Type      string `json:"type,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type codexToolDescriptor struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (c *codexClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	body, err := buildCodexRequest(req)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+codexResponsesPath, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(rawBody)}
	}
	return newCodexStream(resp.Body), nil
}

func (c *codexClient) OpenRunEvents(context.Context, string) (RunEventStream, error) {
	return nil, ErrRunEventsNotSupported
}

func (c *codexClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+codexModelsPath, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(body)}
	}
	return nil
}

func buildCodexRequest(req ChatRequest) (codexRequest, error) {
	instructions, input, err := translateCodexMessages(req.Messages)
	if err != nil {
		return codexRequest{}, err
	}
	tools := make([]codexToolDescriptor, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = codexToolDescriptor{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Schema,
		}
	}
	return codexRequest{
		Model:        req.Model,
		Instructions: instructions,
		Input:        input,
		Stream:       true,
		Tools:        tools,
	}, nil
}

func translateCodexMessages(messages []Message) (string, []codexInputItem, error) {
	systemParts := make([]string, 0, len(messages))
	input := make([]codexInputItem, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if strings.TrimSpace(msg.Content) != "" {
				systemParts = append(systemParts, msg.Content)
			}
		case "user":
			input = append(input, codexInputItem{Role: "user", Content: msg.Content})
		case "assistant":
			if strings.TrimSpace(msg.Content) != "" {
				input = append(input, codexInputItem{Role: "assistant", Content: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				args, err := normalizeJSONArguments(tc.Arguments)
				if err != nil {
					return "", nil, fmt.Errorf("codex assistant tool %q: %w", tc.Name, err)
				}
				input = append(input, codexInputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Name,
					Arguments: args,
				})
			}
		case "tool":
			if strings.TrimSpace(msg.ToolCallID) == "" {
				return "", nil, fmt.Errorf("codex tool result missing tool_call_id")
			}
			input = append(input, codexInputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		default:
			return "", nil, fmt.Errorf("codex unsupported role %q", msg.Role)
		}
	}

	return strings.Join(systemParts, "\n\n"), input, nil
}

func normalizeJSONArguments(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "{}", nil
	}
	if !json.Valid(trimmed) {
		return "", fmt.Errorf("invalid JSON arguments")
	}
	return string(trimmed), nil
}

type codexStream struct {
	body io.ReadCloser
	sse  *sseReader

	closed bool
	mu     sync.Mutex

	pending   []Event
	callOrder []string
	callIndex map[string]*pendingCodexToolCall
}

type pendingCodexToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func newCodexStream(body io.ReadCloser) *codexStream {
	return &codexStream{
		body:      body,
		sse:       newSSEReader(body),
		callIndex: make(map[string]*pendingCodexToolCall),
	}
}

func (s *codexStream) SessionID() string { return "" }

func (s *codexStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func (s *codexStream) Recv(ctx context.Context) (Event, error) {
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
		evType := strings.TrimSpace(f.event)
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(f.data), &base); err != nil {
			continue
		}
		if evType == "" {
			evType = strings.TrimSpace(base.Type)
		}
		raw := json.RawMessage(f.data)

		switch evType {
		case "response.output_text.delta", "response.text.delta":
			var payload struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(f.data), &payload); err != nil || payload.Delta == "" {
				continue
			}
			return Event{Kind: EventToken, Token: payload.Delta, Raw: raw}, nil
		case "response.reasoning_summary_text.delta", "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary.delta":
			var payload struct {
				Delta string `json:"delta"`
				Text  string `json:"text"`
			}
			if err := json.Unmarshal([]byte(f.data), &payload); err != nil {
				continue
			}
			reasoning := payload.Delta
			if reasoning == "" {
				reasoning = payload.Text
			}
			if reasoning == "" {
				continue
			}
			return Event{Kind: EventReasoning, Reasoning: reasoning, Raw: raw}, nil
		case "response.output_item.added", "response.output_item.done":
			var payload struct {
				Item codexResponseItem `json:"item"`
			}
			if err := json.Unmarshal([]byte(f.data), &payload); err != nil {
				continue
			}
			s.rememberCompletedToolCall(payload.Item)
			continue
		case "response.function_call_arguments.delta":
			var payload struct {
				CallID string `json:"call_id"`
				Delta  string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(f.data), &payload); err != nil || strings.TrimSpace(payload.CallID) == "" {
				continue
			}
			call := s.ensureToolCall(payload.CallID)
			call.arguments.WriteString(payload.Delta)
			continue
		case "response.function_call_arguments.done":
			var payload struct {
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(f.data), &payload); err != nil || strings.TrimSpace(payload.CallID) == "" {
				continue
			}
			call := s.ensureToolCall(payload.CallID)
			if payload.Name != "" {
				call.name = payload.Name
			}
			if payload.Arguments != "" {
				call.arguments.Reset()
				call.arguments.WriteString(payload.Arguments)
			}
			continue
		case "response.completed":
			done, ok := s.completedEvent(raw)
			if !ok {
				continue
			}
			return done, nil
		default:
			continue
		}
	}
}

type codexCompletedPayload struct {
	Response codexCompletedResponse `json:"response"`
	Usage    *codexUsage            `json:"usage,omitempty"`
}

type codexCompletedResponse struct {
	Usage  *codexUsage         `json:"usage,omitempty"`
	Output []codexResponseItem `json:"output,omitempty"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type codexResponseItem struct {
	ID        string `json:"id,omitempty"`
	Type      string `json:"type"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func (s *codexStream) completedEvent(raw json.RawMessage) (Event, bool) {
	var payload codexCompletedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Event{}, false
	}
	for _, item := range payload.Response.Output {
		s.rememberCompletedToolCall(item)
	}

	ev := Event{Kind: EventDone, FinishReason: "stop", Raw: raw}
	usage := payload.Response.Usage
	if usage == nil {
		usage = payload.Usage
	}
	if usage != nil {
		ev.TokensIn = usage.InputTokens
		ev.TokensOut = usage.OutputTokens
	}
	if len(s.callIndex) > 0 {
		ev.FinishReason = "tool_calls"
		ev.ToolCalls = s.flushToolCalls()
	}
	return ev, true
}

func (s *codexStream) ensureToolCall(callID string) *pendingCodexToolCall {
	id := strings.TrimSpace(callID)
	if id == "" {
		id = "call_0"
	}
	call, ok := s.callIndex[id]
	if !ok {
		call = &pendingCodexToolCall{id: id}
		s.callIndex[id] = call
		s.callOrder = append(s.callOrder, id)
	}
	return call
}

func (s *codexStream) rememberCompletedToolCall(item codexResponseItem) {
	if item.Type != "function_call" {
		return
	}
	id := strings.TrimSpace(item.CallID)
	if id == "" {
		id = strings.TrimSpace(item.ID)
	}
	if id == "" {
		return
	}
	call := s.ensureToolCall(id)
	if item.Name != "" {
		call.name = item.Name
	}
	if item.Arguments != "" {
		call.arguments.Reset()
		call.arguments.WriteString(item.Arguments)
	}
}

func (s *codexStream) flushToolCalls() []ToolCall {
	if len(s.callIndex) == 0 {
		return nil
	}
	order := append([]string(nil), s.callOrder...)
	sort.Strings(order)
	out := make([]ToolCall, 0, len(order))
	for _, id := range order {
		call := s.callIndex[id]
		out = append(out, ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(call.arguments.String()),
		})
	}
	s.callOrder = nil
	s.callIndex = make(map[string]*pendingCodexToolCall)
	return out
}
