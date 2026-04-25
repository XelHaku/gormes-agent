package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicVersion             = "2023-06-01"
	defaultAnthropicMessagesPath = "/v1/messages"
	defaultAnthropicModelsPath   = "/v1/models"
	defaultAnthropicMaxTokens    = 1024
)

type anthropicClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	System    any                `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTextBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type anthropicToolUseBlock struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

type anthropicToolResultBlock struct {
	Type         string        `json:"type"`
	ToolUseID    string        `json:"tool_use_id"`
	Content      string        `json:"content"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// NewAnthropicClient returns a Client that talks directly to Anthropic's
// Messages API over HTTP+SSE.
func NewAnthropicClient(baseURL, apiKey string) Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	return &anthropicClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 0, Transport: transport},
	}
}

func (c *anthropicClient) ProviderStatus() ProviderStatus {
	return anthropicProviderStatus()
}

func (c *anthropicClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	payload, err := buildAnthropicRequest(req)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+defaultAnthropicMessagesPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newHTTPError(resp.StatusCode, string(raw), resp.Header)
	}
	return newAnthropicStream(resp.Body), nil
}

func (c *anthropicClient) OpenRunEvents(context.Context, string) (RunEventStream, error) {
	return nil, ErrRunEventsNotSupported
}

func (c *anthropicClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+defaultAnthropicModelsPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return newHTTPError(resp.StatusCode, string(raw), resp.Header)
	}
	return nil
}

func (c *anthropicClient) applyAuth(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	if strings.HasPrefix(c.apiKey, "sk-ant-api") {
		req.Header.Set("x-api-key", c.apiKey)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func buildAnthropicRequest(req ChatRequest) (anthropicRequest, error) {
	system, messages, err := convertAnthropicMessages(req.Messages)
	if err != nil {
		return anthropicRequest{}, err
	}
	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Schema,
		})
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}
	return anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    true,
		System:    system,
		Messages:  messages,
		Tools:     tools,
	}, nil
}

func convertAnthropicMessages(messages []Message) (any, []anthropicMessage, error) {
	var (
		systemBlocks []anthropicTextBlock
		systemText   []string
		out          []anthropicMessage
	)
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.CacheControl != nil {
				systemBlocks = append(systemBlocks, textBlock(msg.Content, msg.CacheControl))
				continue
			}
			systemText = append(systemText, msg.Content)
		case "assistant":
			content, err := assistantContentBlocks(msg)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: content})
		case "tool":
			out = appendAnthropicToolResult(out, msg)
		default:
			if msg.CacheControl != nil {
				out = append(out, anthropicMessage{
					Role:    msg.Role,
					Content: []any{textBlock(msg.Content, msg.CacheControl)},
				})
				continue
			}
			out = append(out, anthropicMessage{Role: msg.Role, Content: msg.Content})
		}
	}
	if len(systemBlocks) > 0 {
		return toAnySlice(systemBlocks), out, nil
	}
	if len(systemText) > 0 {
		return strings.Join(systemText, "\n"), out, nil
	}
	return nil, out, nil
}

func assistantContentBlocks(msg Message) ([]any, error) {
	blocks := make([]any, 0, 1+len(msg.ToolCalls))
	if msg.Content != "" {
		blocks = append(blocks, anthropicTextBlock{Type: "text", Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		input := map[string]any{}
		if len(tc.Arguments) > 0 {
			if err := json.Unmarshal(tc.Arguments, &input); err != nil {
				return nil, err
			}
		}
		blocks = append(blocks, anthropicToolUseBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropicTextBlock{Type: "text", Text: "(empty)"})
	}
	return blocks, nil
}

func appendAnthropicToolResult(out []anthropicMessage, msg Message) []anthropicMessage {
	block := anthropicToolResultBlock{
		Type:         "tool_result",
		ToolUseID:    msg.ToolCallID,
		Content:      msg.Content,
		CacheControl: msg.CacheControl,
	}
	if block.Content == "" {
		block.Content = "(no output)"
	}
	if len(out) > 0 && out[len(out)-1].Role == "user" {
		if blocks, ok := out[len(out)-1].Content.([]any); ok && startsWithAnthropicToolResult(blocks) {
			out[len(out)-1].Content = append(blocks, block)
			return out
		}
	}
	return append(out, anthropicMessage{Role: "user", Content: []any{block}})
}

func startsWithAnthropicToolResult(blocks []any) bool {
	if len(blocks) == 0 {
		return false
	}
	first, ok := blocks[0].(anthropicToolResultBlock)
	return ok && first.Type == "tool_result"
}

func textBlock(text string, cache *CacheControl) anthropicTextBlock {
	return anthropicTextBlock{Type: "text", Text: text, CacheControl: cache}
}

func toAnySlice[T any](in []T) []any {
	out := make([]any, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}
