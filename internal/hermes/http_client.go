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
	"time"
)

const defaultChatCompletionsPath = "/v1/chat/completions"
const defaultHealthPath = "/health"

type httpClient struct {
	baseURL  string
	apiKey   string
	provider string
	http     *http.Client
}

// NewHTTPClient returns a Client that talks HTTP+SSE to a Hermes-compatible
// api_server. baseURL example: "http://127.0.0.1:8642".
// The returned client streams without a global timeout so long turns
// (minutes, with tool use) are not truncated; see per-phase timeouts inside.
func NewHTTPClient(baseURL, apiKey string) Client {
	return NewHTTPClientWithProvider(baseURL, apiKey, "")
}

// NewHTTPClientWithProvider returns an OpenAI-compatible HTTP client with a
// provider identity hint for providers whose replay rules differ from the
// generic Chat Completions shape.
func NewHTTPClientWithProvider(baseURL, apiKey, provider string) Client {
	// Clone the default transport and enforce the header-phase budget via
	// ResponseHeaderTimeout. This caps time-to-first-byte WITHOUT affecting
	// the streaming body read afterwards — unlike wrapping the request
	// context, which would cancel body reads mid-stream.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	return &httpClient{
		baseURL:  baseURL,
		apiKey:   apiKey,
		provider: strings.TrimSpace(provider),
		http:     &http.Client{Timeout: 0, Transport: transport},
	}
}

func (c *httpClient) ProviderStatus() ProviderStatus {
	return openAICompatibleProviderStatus(c.provider, c.baseURL)
}

func (c *httpClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+defaultHealthPath, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return newHTTPError(resp.StatusCode, string(body), resp.Header)
	}
	return nil
}

type orMessage struct {
	Role             string       `json:"role"`
	Content          string       `json:"content"`
	ReasoningContent *string      `json:"reasoning_content,omitempty"`
	ToolCalls        []orToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string       `json:"tool_call_id,omitempty"`
	Name             string       `json:"name,omitempty"`
}

type orToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function orToolFunction `json:"function"`
}

type orToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type orToolDescriptor struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type orChatRequest struct {
	Model    string             `json:"model"`
	Messages []orMessage        `json:"messages"`
	Stream   bool               `json:"stream"`
	Tools    []orToolDescriptor `json:"tools,omitempty"`
}

func (c *httpClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	msgs := makeOpenAICompatibleMessages(req.Messages, c.provider, req.Model, c.baseURL)
	tools := make([]orToolDescriptor, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = orToolDescriptor{
			Type: "function",
			Function: struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			}{Name: t.Name, Description: t.Description, Parameters: t.Schema},
		}
	}
	body, err := json.Marshal(orChatRequest{Model: req.Model, Messages: msgs, Stream: true, Tools: tools})
	if err != nil {
		return nil, err
	}

	// Header-phase budget enforced by Transport.ResponseHeaderTimeout (5s).
	// The request ctx governs the full response lifetime including body reads —
	// do NOT cancel it after Do returns or streaming breaks.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+defaultChatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if req.SessionID != "" {
		httpReq.Header.Set("X-Hermes-Session-Id", req.SessionID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newHTTPError(resp.StatusCode, string(raw), resp.Header)
	}
	// The body stays open for streaming; chatStream owns the Close.
	return newChatStream(resp.Body, resp.Header.Get("X-Hermes-Session-Id")), nil
}

func makeOpenAICompatibleMessages(messages []Message, provider, model, baseURL string) []orMessage {
	out := make([]orMessage, 0, len(messages))
	for _, msg := range messages {
		wire := orMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}
		if msg.Role == "assistant" {
			wire.ReasoningContent = openAICompatibleReasoningContent(msg, provider, model, baseURL)
		}
		if len(msg.ToolCalls) > 0 {
			wire.ToolCalls = make([]orToolCall, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				args := string(call.Arguments)
				if args == "" {
					args = "{}"
				}
				wire.ToolCalls = append(wire.ToolCalls, orToolCall{
					ID:   call.ID,
					Type: "function",
					Function: orToolFunction{
						Name:      call.Name,
						Arguments: args,
					},
				})
			}
		}
		out = append(out, wire)
	}
	return out
}

func openAICompatibleReasoningContent(msg Message, provider, model, baseURL string) *string {
	if len(msg.ToolCalls) == 0 || !openAICompatibleRequiresReasoningEcho(provider, model, baseURL) {
		return nil
	}
	if msg.ReasoningContent != nil {
		return msg.ReasoningContent
	}
	if msg.Reasoning != nil && msg.Reasoning.Text != "" {
		text := msg.Reasoning.Text
		return &text
	}
	empty := ""
	return &empty
}

func openAICompatibleRequiresReasoningEcho(provider, model, baseURL string) bool {
	return openAICompatibleNeedsDeepSeekToolReasoning(provider, model, baseURL) ||
		openAICompatibleNeedsKimiToolReasoning(provider, baseURL)
}

func openAICompatibleNeedsDeepSeekToolReasoning(provider, model, baseURL string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	return provider == "deepseek" ||
		strings.Contains(model, "deepseek") ||
		baseURLHostMatches(baseURL, "api.deepseek.com")
}

func openAICompatibleNeedsKimiToolReasoning(provider, baseURL string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	return provider == "kimi-coding" ||
		provider == "kimi-coding-cn" ||
		baseURLHostMatches(baseURL, "api.kimi.com") ||
		baseURLHostMatches(baseURL, "moonshot.ai") ||
		baseURLHostMatches(baseURL, "moonshot.cn")
}

func baseURLHostMatches(rawBaseURL, domain string) bool {
	host := baseURLHostname(rawBaseURL)
	if host == "" {
		return false
	}
	domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func baseURLHostname(rawBaseURL string) string {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	if rawBaseURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawBaseURL)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + rawBaseURL)
		if err != nil {
			return ""
		}
	}
	return strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
}

// OpenRunEvents subscribes to SSE stream for a run's events.
// 404 returns ErrRunEventsNotSupported for non-Hermes servers.
func (c *httpClient) OpenRunEvents(ctx context.Context, runID string) (RunEventStream, error) {
	// Header-phase budget enforced by Transport.ResponseHeaderTimeout (5s).
	// The request ctx governs the full response lifetime including body reads —
	// do NOT cancel it after Do returns or streaming breaks.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/runs/%s/events", c.baseURL, runID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		_ = resp.Body.Close()
		return nil, ErrRunEventsNotSupported
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newHTTPError(resp.StatusCode, string(raw), resp.Header)
	}
	return newRunEventStream(resp.Body), nil
}
