package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const geminiModelsPath = "/models"

type geminiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newGeminiClient(baseURL, apiKey string) Client {
	return &geminiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    newStreamingHTTPClient(),
	}
}

type geminiRequest struct {
	SystemInstruction *geminiContent   `json:"system_instruction,omitempty"`
	Contents          []geminiContent  `json:"contents"`
	Tools             []geminiToolList `json:"tools,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Response any    `json:"response,omitempty"`
}

type geminiToolList struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (c *geminiClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	body, err := buildGeminiRequest(req)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiStreamURL(c.baseURL, req.Model), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("x-goog-api-key", c.apiKey)
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
	return newGeminiStream(resp.Body), nil
}

func (c *geminiClient) OpenRunEvents(context.Context, string) (RunEventStream, error) {
	return nil, ErrRunEventsNotSupported
}

func (c *geminiClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+geminiModelsPath, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("x-goog-api-key", c.apiKey)
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

func buildGeminiRequest(req ChatRequest) (geminiRequest, error) {
	systemInstruction, contents, err := translateGeminiMessages(req.Messages)
	if err != nil {
		return geminiRequest{}, err
	}

	out := geminiRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
	}
	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDeclaration, len(req.Tools))
		for i, tool := range req.Tools {
			decls[i] = geminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Schema,
			}
		}
		out.Tools = []geminiToolList{{FunctionDeclarations: decls}}
	}
	return out, nil
}

func translateGeminiMessages(messages []Message) (*geminiContent, []geminiContent, error) {
	systemParts := make([]geminiPart, 0, len(messages))
	out := make([]geminiContent, 0, len(messages))

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		switch msg.Role {
		case "system":
			if strings.TrimSpace(msg.Content) != "" {
				systemParts = append(systemParts, geminiPart{Text: msg.Content})
			}
		case "user":
			out = append(out, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})
		case "assistant":
			parts := make([]geminiPart, 0, 1+len(msg.ToolCalls))
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, geminiPart{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				args, err := parseGeminiFunctionArgs(tc.Arguments)
				if err != nil {
					return nil, nil, fmt.Errorf("gemini assistant tool %q: %w", tc.Name, err)
				}
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: args,
					},
				})
			}
			if len(parts) == 0 {
				continue
			}
			out = append(out, geminiContent{Role: "model", Parts: parts})
		case "tool":
			parts := make([]geminiPart, 0, 1)
			for ; i < len(messages) && messages[i].Role == "tool"; i++ {
				toolMsg := messages[i]
				if strings.TrimSpace(toolMsg.ToolCallID) == "" {
					return nil, nil, fmt.Errorf("gemini tool result missing tool_call_id")
				}
				parts = append(parts, geminiPart{
					FunctionResponse: &geminiFunctionResponse{
						ID:       toolMsg.ToolCallID,
						Name:     toolMsg.Name,
						Response: parseGeminiFunctionResponse(toolMsg.Content),
					},
				})
			}
			i--
			out = append(out, geminiContent{Role: "user", Parts: parts})
		default:
			return nil, nil, fmt.Errorf("gemini unsupported role %q", msg.Role)
		}
	}

	if len(systemParts) == 0 {
		return nil, out, nil
	}
	return &geminiContent{Parts: systemParts}, out, nil
}

func parseGeminiFunctionArgs(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal(trimmed, &args); err != nil {
		return nil, err
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

func parseGeminiFunctionResponse(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	if json.Valid([]byte(trimmed)) {
		var decoded any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]any{"content": raw}
}

func geminiStreamURL(baseURL, model string) string {
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")
	return fmt.Sprintf("%s%s/%s:streamGenerateContent?alt=sse", strings.TrimRight(baseURL, "/"), geminiModelsPath, url.PathEscape(model))
}

type geminiStream struct {
	body    io.ReadCloser
	sse     *sseReader
	closed  bool
	mu      sync.Mutex
	pending []Event
}

type geminiToolCallIn struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiStreamPart struct {
	Text         string            `json:"text,omitempty"`
	FunctionCall *geminiToolCallIn `json:"functionCall,omitempty"`
}

type geminiStreamCandidate struct {
	Content struct {
		Role  string             `json:"role"`
		Parts []geminiStreamPart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

type geminiChunk struct {
	Candidates    []geminiStreamCandidate `json:"candidates"`
	UsageMetadata *geminiUsageMetadata    `json:"usageMetadata,omitempty"`
}

func newGeminiStream(body io.ReadCloser) *geminiStream {
	return &geminiStream{
		body: body,
		sse:  newSSEReader(body),
	}
}

func (s *geminiStream) SessionID() string { return "" }

func (s *geminiStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func (s *geminiStream) Recv(ctx context.Context) (Event, error) {
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

		var chunk geminiChunk
		if err := json.Unmarshal([]byte(f.data), &chunk); err != nil {
			continue
		}
		if len(chunk.Candidates) == 0 {
			continue
		}

		candidate := chunk.Candidates[0]
		raw := json.RawMessage(f.data)
		events := make([]Event, 0, len(candidate.Content.Parts)+1)
		toolCalls := make([]ToolCall, 0)

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				events = append(events, Event{Kind: EventToken, Token: part.Text, Raw: raw})
			}
			if part.FunctionCall != nil {
				args := bytes.TrimSpace(part.FunctionCall.Args)
				if len(args) == 0 {
					args = []byte("{}")
				}
				toolCalls = append(toolCalls, ToolCall{
					ID:        part.FunctionCall.ID,
					Name:      part.FunctionCall.Name,
					Arguments: json.RawMessage(args),
				})
			}
		}

		finishReason := normalizeGeminiFinishReason(candidate.FinishReason)
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		}
		if finishReason != "" {
			done := Event{Kind: EventDone, FinishReason: finishReason, Raw: raw, ToolCalls: toolCalls}
			if chunk.UsageMetadata != nil {
				done.TokensIn = chunk.UsageMetadata.PromptTokenCount
				done.TokensOut = chunk.UsageMetadata.CandidatesTokenCount
			}
			events = append(events, done)
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

func normalizeGeminiFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "":
		return ""
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return strings.ToLower(strings.TrimSpace(reason))
	}
}
