package hermes

import (
	"encoding/json"
	"fmt"
	"strings"
)

const defaultBedrockMaxTokens = 4096

type bedrockConversePayload struct {
	Messages        []bedrockMessage       `json:"messages"`
	System          []bedrockContentBlock  `json:"system,omitempty"`
	InferenceConfig bedrockInferenceConfig `json:"inferenceConfig"`
	ToolConfig      *bedrockToolConfig     `json:"toolConfig,omitempty"`
}

type bedrockInferenceConfig struct {
	MaxTokens   int      `json:"maxTokens"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockContentBlock struct {
	Text             string                   `json:"text,omitempty"`
	ReasoningContent *bedrockReasoningContent `json:"reasoningContent,omitempty"`
	ToolUse          *bedrockToolUse          `json:"toolUse,omitempty"`
	ToolResult       *bedrockToolResult       `json:"toolResult,omitempty"`
	CachePoint       *bedrockCachePoint       `json:"cachePoint,omitempty"`
}

type bedrockReasoningContent struct {
	ReasoningText   *bedrockReasoningText `json:"reasoningText,omitempty"`
	RedactedContent string                `json:"redactedContent,omitempty"`
}

type bedrockReasoningText struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

type bedrockToolUse struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type bedrockToolResult struct {
	ToolUseID string                     `json:"toolUseId"`
	Content   []bedrockToolResultContent `json:"content"`
	Status    string                     `json:"status,omitempty"`
}

type bedrockToolResultContent struct {
	Text string `json:"text,omitempty"`
}

type bedrockCachePoint struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type bedrockToolConfig struct {
	Tools []bedrockTool `json:"tools"`
}

type bedrockTool struct {
	ToolSpec bedrockToolSpec `json:"toolSpec"`
}

type bedrockToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema bedrockToolInputSchema `json:"inputSchema"`
}

type bedrockToolInputSchema struct {
	JSON json.RawMessage `json:"json"`
}

func buildBedrockConversePayload(req ChatRequest) (bedrockConversePayload, error) {
	system, messages, err := convertBedrockMessages(req.Messages)
	if err != nil {
		return bedrockConversePayload{}, err
	}
	tools, err := convertBedrockTools(req.Tools)
	if err != nil {
		return bedrockConversePayload{}, err
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultBedrockMaxTokens
	}

	payload := bedrockConversePayload{
		Messages: messages,
		System:   system,
		InferenceConfig: bedrockInferenceConfig{
			MaxTokens:   maxTokens,
			Temperature: req.Temperature,
		},
	}
	if len(tools) > 0 {
		payload.ToolConfig = &bedrockToolConfig{Tools: tools}
	}
	return payload, nil
}

func convertBedrockMessages(messages []Message) ([]bedrockContentBlock, []bedrockMessage, error) {
	var (
		system []bedrockContentBlock
		out    []bedrockMessage
	)
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if strings.TrimSpace(msg.Content) != "" {
				system = append(system, bedrockContentBlock{Text: msg.Content})
			}
			system = appendBedrockCachePoint(system, msg.CacheControl)
		case "assistant":
			blocks, err := bedrockAssistantContentBlocks(msg)
			if err != nil {
				return nil, nil, err
			}
			out = appendOrMergeBedrockMessage(out, "assistant", blocks)
		case "tool":
			blocks := []bedrockContentBlock{{
				ToolResult: &bedrockToolResult{
					ToolUseID: msg.ToolCallID,
					Content: []bedrockToolResultContent{{
						Text: nonEmptyBedrockText(msg.Content),
					}},
				},
			}}
			blocks = appendBedrockCachePoint(blocks, msg.CacheControl)
			out = appendOrMergeBedrockMessage(out, "user", blocks)
		default:
			blocks := []bedrockContentBlock{{Text: nonEmptyBedrockText(msg.Content)}}
			blocks = appendBedrockCachePoint(blocks, msg.CacheControl)
			out = appendOrMergeBedrockMessage(out, "user", blocks)
		}
	}
	if len(out) == 0 {
		out = append(out, bedrockMessage{Role: "user", Content: []bedrockContentBlock{{Text: " "}}})
	}
	if out[0].Role != "user" {
		out = append([]bedrockMessage{{Role: "user", Content: []bedrockContentBlock{{Text: " "}}}}, out...)
	}
	if out[len(out)-1].Role != "user" {
		out = append(out, bedrockMessage{Role: "user", Content: []bedrockContentBlock{{Text: " "}}})
	}
	return system, out, nil
}

func bedrockAssistantContentBlocks(msg Message) ([]bedrockContentBlock, error) {
	blocks := make([]bedrockContentBlock, 0, 1+len(msg.ToolCalls))
	if strings.TrimSpace(msg.Content) != "" {
		blocks = append(blocks, bedrockContentBlock{Text: msg.Content})
	}
	if msg.Reasoning != nil {
		if block, ok := bedrockReasoningBlock(msg.Reasoning); ok {
			blocks = append(blocks, block)
		}
	}
	for _, tc := range msg.ToolCalls {
		input := json.RawMessage(`{}`)
		if len(tc.Arguments) > 0 {
			if !json.Valid(tc.Arguments) {
				return nil, fmt.Errorf("bedrock tool call %q arguments are invalid JSON", tc.ID)
			}
			input = append(json.RawMessage(nil), tc.Arguments...)
		}
		blocks = append(blocks, bedrockContentBlock{
			ToolUse: &bedrockToolUse{
				ToolUseID: tc.ID,
				Name:      tc.Name,
				Input:     input,
			},
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, bedrockContentBlock{Text: " "})
	}
	return blocks, nil
}

func bedrockReasoningBlock(reasoning *ReasoningContent) (bedrockContentBlock, bool) {
	if reasoning.Text != "" || reasoning.Signature != "" {
		return bedrockContentBlock{
			ReasoningContent: &bedrockReasoningContent{
				ReasoningText: &bedrockReasoningText{
					Text:      nonEmptyBedrockText(reasoning.Text),
					Signature: reasoning.Signature,
				},
			},
		}, true
	}
	if reasoning.RedactedContent != "" {
		return bedrockContentBlock{
			ReasoningContent: &bedrockReasoningContent{
				RedactedContent: reasoning.RedactedContent,
			},
		}, true
	}
	return bedrockContentBlock{}, false
}

func appendOrMergeBedrockMessage(out []bedrockMessage, role string, blocks []bedrockContentBlock) []bedrockMessage {
	if len(out) > 0 && out[len(out)-1].Role == role {
		out[len(out)-1].Content = append(out[len(out)-1].Content, blocks...)
		return out
	}
	return append(out, bedrockMessage{Role: role, Content: blocks})
}

func appendBedrockCachePoint(blocks []bedrockContentBlock, cache *CacheControl) []bedrockContentBlock {
	if cache == nil {
		return blocks
	}
	cacheType := cache.Type
	if cacheType == "" || cacheType == "ephemeral" {
		cacheType = "default"
	}
	return append(blocks, bedrockContentBlock{
		CachePoint: &bedrockCachePoint{
			Type: cacheType,
			TTL:  cache.TTL,
		},
	})
}

func convertBedrockTools(tools []ToolDescriptor) ([]bedrockTool, error) {
	descriptors := SanitizeToolDescriptors(tools)
	out := make([]bedrockTool, 0, len(descriptors))
	for _, tool := range descriptors {
		schema := tool.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		if !json.Valid(schema) {
			return nil, fmt.Errorf("bedrock tool %q schema is invalid JSON", tool.Name)
		}
		out = append(out, bedrockTool{
			ToolSpec: bedrockToolSpec{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: bedrockToolInputSchema{
					JSON: append(json.RawMessage(nil), schema...),
				},
			},
		})
	}
	return out, nil
}

func nonEmptyBedrockText(text string) string {
	if strings.TrimSpace(text) == "" {
		return " "
	}
	return text
}
