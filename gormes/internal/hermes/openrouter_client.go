package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	openRouterChatCompletionsPath = "/chat/completions"
	openRouterModelsPath          = "/models"
	defaultOpenRouterBaseURL      = "https://openrouter.ai/api/v1"
)

type openRouterClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newOpenRouterClient(baseURL, apiKey string) Client {
	return &openRouterClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    newStreamingHTTPClient(),
	}
}

type openRouterChatRequest struct {
	Model     string              `json:"model"`
	Messages  []openRouterMessage `json:"messages"`
	SessionID string              `json:"session_id,omitempty"`
	Stream    bool                `json:"stream"`
	Tools     []orToolDescriptor  `json:"tools,omitempty"`
}

type openRouterMessage struct {
	Role       string                  `json:"role"`
	Content    string                  `json:"content,omitempty"`
	ToolCalls  []openRouterMessageTool `json:"tool_calls,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
	Name       string                  `json:"name,omitempty"`
}

type openRouterMessageTool struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function openRouterToolDefinition `json:"function"`
}

type openRouterToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
}

func (c *openRouterClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	body, err := buildOpenRouterRequest(req)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+openRouterChatCompletionsPath, bytes.NewReader(raw))
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
	return newChatStream(resp.Body, ""), nil
}

func (c *openRouterClient) OpenRunEvents(context.Context, string) (RunEventStream, error) {
	return nil, ErrRunEventsNotSupported
}

func (c *openRouterClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+openRouterModelsPath, nil)
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

func buildOpenRouterRequest(req ChatRequest) (openRouterChatRequest, error) {
	messages := make([]openRouterMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		wire := openRouterMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}
		switch msg.Role {
		case "system", "user", "assistant", "tool":
		default:
			return openRouterChatRequest{}, fmt.Errorf("openrouter unsupported role %q", msg.Role)
		}
		if len(msg.ToolCalls) > 0 {
			wire.ToolCalls = make([]openRouterMessageTool, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				args, err := normalizeJSONArguments(tc.Arguments)
				if err != nil {
					return openRouterChatRequest{}, fmt.Errorf("openrouter assistant tool %q: %w", tc.Name, err)
				}
				wire.ToolCalls = append(wire.ToolCalls, openRouterMessageTool{
					ID:   tc.ID,
					Type: "function",
					Function: openRouterToolDefinition{
						Name:      tc.Name,
						Arguments: args,
					},
				})
			}
		}
		messages = append(messages, wire)
	}

	tools := make([]orToolDescriptor, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = orToolDescriptor{
			Type: "function",
			Function: struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			}{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
	}

	return openRouterChatRequest{
		Model:     req.Model,
		Messages:  messages,
		SessionID: req.SessionID,
		Stream:    true,
		Tools:     tools,
	}, nil
}
