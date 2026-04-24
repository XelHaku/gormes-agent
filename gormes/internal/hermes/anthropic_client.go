package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthropicContentBlock struct {
	Type         string                  `json:"type"`
	Text         string                  `json:"text,omitempty"`
	ID           string                  `json:"id,omitempty"`
	Name         string                  `json:"name,omitempty"`
	Input        any                     `json:"input,omitempty"`
	Source       *anthropicContentSource `json:"source,omitempty"`
	ToolUseID    string                  `json:"tool_use_id,omitempty"`
	Content      string                  `json:"content,omitempty"`
	CacheControl *CacheControl           `json:"cache_control,omitempty"`
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

func newAnthropicClient(baseURL, apiKey string) Client {
	return NewAnthropicClient(baseURL, apiKey)
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
		return nil, newHTTPError(resp, raw)
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
		return newHTTPError(resp, raw)
	}
	return nil
}

func (c *anthropicClient) applyAuth(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	req.Header.Set("x-api-key", c.apiKey)
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
		systemBlocks []anthropicContentBlock
		systemText   []string
		out          []anthropicMessage
	)
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		switch msg.Role {
		case "system":
			if len(msg.Parts) > 0 {
				return nil, nil, fmt.Errorf("anthropic system messages do not support multimodal content")
			}
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
			if msg.CacheControl != nil {
				systemBlocks = append(systemBlocks, textBlock(msg.Content, msg.CacheControl))
				continue
			}
			systemText = append(systemText, msg.Content)
		case "user":
			blocks, err := anthropicMessageBlocks(msg)
			if err != nil {
				return nil, nil, err
			}
			if msg.CacheControl != nil && len(blocks) == 1 && blocks[0].Type == "text" {
				blocks[0].CacheControl = msg.CacheControl
			}
			out = append(out, anthropicMessage{Role: "user", Content: blocks})
		case "assistant":
			blocks, err := assistantContentBlocks(msg)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: blocks})
		case "tool":
			var blocks []anthropicContentBlock
			for ; i < len(messages) && messages[i].Role == "tool"; i++ {
				toolMsg := messages[i]
				block, err := anthropicToolResultBlock(toolMsg)
				if err != nil {
					return nil, nil, err
				}
				blocks = append(blocks, block)
			}
			i--
			out = append(out, anthropicMessage{Role: "user", Content: blocks})
		default:
			return nil, nil, fmt.Errorf("anthropic unsupported role %q", msg.Role)
		}
	}
	if len(systemBlocks) > 0 {
		return systemBlocks, out, nil
	}
	if len(systemText) > 0 {
		return strings.Join(systemText, "\n\n"), out, nil
	}
	return nil, out, nil
}

func assistantContentBlocks(msg Message) ([]anthropicContentBlock, error) {
	blocks, err := anthropicMessageBlocks(msg)
	if err != nil {
		return nil, err
	}
	for _, block := range blocks {
		if block.Type == "image" {
			return nil, fmt.Errorf("anthropic assistant messages do not support image content")
		}
	}
	for _, tc := range msg.ToolCalls {
		input, err := parseAnthropicToolInput(tc.Arguments)
		if err != nil {
			return nil, fmt.Errorf("anthropic assistant tool %q: %w", tc.Name, err)
		}
		blocks = append(blocks, anthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, textBlock("(empty)", nil))
	}
	return blocks, nil
}

func anthropicToolResultBlock(msg Message) (anthropicContentBlock, error) {
	if strings.TrimSpace(msg.ToolCallID) == "" {
		return anthropicContentBlock{}, fmt.Errorf("anthropic tool result missing tool_call_id")
	}
	content := msg.Content
	if content == "" {
		content = "(no output)"
	}
	return anthropicContentBlock{
		Type:         "tool_result",
		ToolUseID:    msg.ToolCallID,
		Content:      content,
		CacheControl: msg.CacheControl,
	}, nil
}

func parseAnthropicToolInput(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal(trimmed, &input); err != nil {
		return nil, err
	}
	if input == nil {
		return map[string]any{}, nil
	}
	return input, nil
}

func textBlock(text string, cache *CacheControl) anthropicContentBlock {
	return anthropicContentBlock{Type: "text", Text: text, CacheControl: cache}
}
