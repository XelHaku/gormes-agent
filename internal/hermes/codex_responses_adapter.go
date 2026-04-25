package hermes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const defaultCodexResponsesInstructions = "You are Gormes."

var codexResponsesToolCallLeakPattern = regexp.MustCompile(`(?i)(?:^|[\s>|])to=functions\.[A-Za-z_][\w.]*`)

type codexResponsesPayload struct {
	Model             string               `json:"model"`
	Instructions      string               `json:"instructions"`
	Input             []any                `json:"input"`
	Tools             []codexResponsesTool `json:"tools,omitempty"`
	Store             bool                 `json:"store"`
	MaxOutputTokens   int                  `json:"max_output_tokens,omitempty"`
	ToolChoice        string               `json:"tool_choice,omitempty"`
	ParallelToolCalls bool                 `json:"parallel_tool_calls,omitempty"`
}

type codexResponsesMessageItem struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type codexResponsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type codexResponsesFunctionCallItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type codexResponsesFunctionCallOutputItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type codexResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Strict      bool            `json:"strict"`
	Parameters  json.RawMessage `json:"parameters"`
}

type codexResponsesResponse struct {
	Status     string                     `json:"status"`
	Output     []codexResponsesOutputItem `json:"output"`
	OutputText string                     `json:"output_text,omitempty"`
	Usage      codexResponsesUsage        `json:"usage"`
}

type codexResponsesOutputItem struct {
	Type             string                        `json:"type"`
	ID               string                        `json:"id,omitempty"`
	Status           string                        `json:"status,omitempty"`
	Role             string                        `json:"role,omitempty"`
	Content          []codexResponsesOutputContent `json:"content,omitempty"`
	Summary          []codexResponsesOutputContent `json:"summary,omitempty"`
	EncryptedContent string                        `json:"encrypted_content,omitempty"`
	CallID           string                        `json:"call_id,omitempty"`
	Name             string                        `json:"name,omitempty"`
	Arguments        json.RawMessage               `json:"arguments,omitempty"`
	Input            json.RawMessage               `json:"input,omitempty"`
}

type codexResponsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type codexResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type codexResponsesNormalized struct {
	Message      Message
	Events       []Event
	Usage        codexResponsesUsage
	FinishReason string
	Diagnostics  []string
}

func buildCodexResponsesPayload(req ChatRequest) (codexResponsesPayload, error) {
	input := make([]any, 0, len(req.Messages))
	instructions := ""
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
			if instructions != "" {
				instructions += "\n\n"
			}
			instructions += msg.Content
		case "user", "assistant":
			if len(msg.ContentParts) > 0 {
				input = append(input, codexResponsesMessageItem{
					Role:    msg.Role,
					Content: codexResponsesContentParts(msg.Role, msg.ContentParts),
				})
			} else if msg.Content != "" || msg.Role == "user" {
				input = append(input, codexResponsesMessageItem{Role: msg.Role, Content: msg.Content})
			}
			if msg.Role == "assistant" {
				for _, call := range msg.ToolCalls {
					name := strings.TrimSpace(call.Name)
					if name == "" {
						continue
					}
					args := codexResponsesArguments(call.Arguments)
					callID, _ := splitCodexResponsesToolID(call.ID)
					if callID == "" {
						callID = deterministicCodexResponsesCallID(name, args, len(input))
					}
					input = append(input, codexResponsesFunctionCallItem{
						Type:      "function_call",
						CallID:    callID,
						Name:      name,
						Arguments: args,
					})
				}
			}
		case "tool":
			callID, _ := splitCodexResponsesToolID(msg.ToolCallID)
			if callID == "" {
				callID = strings.TrimSpace(msg.ToolCallID)
			}
			if callID == "" {
				continue
			}
			input = append(input, codexResponsesFunctionCallOutputItem{
				Type:   "function_call_output",
				CallID: callID,
				Output: msg.Content,
			})
		}
	}
	if instructions == "" {
		instructions = defaultCodexResponsesInstructions
	}

	payload := codexResponsesPayload{
		Model:             req.Model,
		Instructions:      instructions,
		Input:             input,
		Tools:             codexResponsesTools(req.Tools),
		Store:             false,
		MaxOutputTokens:   req.MaxTokens,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
	}
	return payload, nil
}

func normalizeCodexResponsesResponse(response codexResponsesResponse) (codexResponsesNormalized, error) {
	return normalizeCodexResponsesResponseWithTools(response, nil)
}

func normalizeCodexResponsesResponseWithTools(response codexResponsesResponse, tools []ToolDescriptor) (codexResponsesNormalized, error) {
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if status == "failed" || status == "cancelled" {
		return codexResponsesNormalized{}, fmt.Errorf("codex responses status %q", response.Status)
	}

	output := response.Output
	var diagnostics []string
	if len(output) == 0 {
		if strings.TrimSpace(response.OutputText) == "" {
			return codexResponsesNormalized{}, errors.New("responses API returned no output items")
		}
		diagnostics = append(diagnostics, "repaired empty response.output from streamed output_text")
		output = []codexResponsesOutputItem{{
			Type:   "message",
			Role:   "assistant",
			Status: "completed",
			Content: []codexResponsesOutputContent{{
				Type: "output_text",
				Text: strings.TrimSpace(response.OutputText),
			}},
		}}
	}

	var textParts []string
	var reasoningParts []string
	var toolCalls []ToolCall
	events := make([]Event, 0, len(output)+1)
	incomplete := status == "queued" || status == "in_progress" || status == "incomplete"

	for _, item := range output {
		itemStatus := strings.ToLower(strings.TrimSpace(item.Status))
		if itemStatus == "queued" || itemStatus == "in_progress" || itemStatus == "incomplete" {
			incomplete = true
		}
		switch item.Type {
		case "reasoning":
			text := codexResponsesReasoningText(item)
			if text == "" {
				continue
			}
			reasoningParts = append(reasoningParts, text)
			events = append(events, Event{Kind: EventReasoning, Reasoning: text})
		case "message":
			text := codexResponsesMessageText(item)
			if text != "" {
				textParts = append(textParts, text)
			}
		case "function_call", "custom_tool_call":
			if itemStatus == "queued" || itemStatus == "in_progress" || itemStatus == "incomplete" {
				continue
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			args := codexResponsesOutputArguments(item)
			callID, _ := splitCodexResponsesToolID(item.CallID)
			if callID == "" {
				callID, _ = splitCodexResponsesToolID(item.ID)
			}
			if callID == "" {
				callID = deterministicCodexResponsesCallID(name, args, len(toolCalls))
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        callID,
				Name:      name,
				Arguments: json.RawMessage(args),
			})
		default:
			return codexResponsesNormalized{}, fmt.Errorf("unsupported Codex Responses output item %q", item.Type)
		}
	}
	if len(toolCalls) > 0 && tools != nil {
		repaired, err := RepairToolCalls(toolCalls, tools)
		if err != nil {
			return codexResponsesNormalized{}, err
		}
		toolCalls = repaired
	}

	content := strings.TrimSpace(strings.Join(textParts, "\n"))
	if content == "" && strings.TrimSpace(response.OutputText) != "" {
		content = strings.TrimSpace(response.OutputText)
	}
	if content != "" && len(toolCalls) == 0 && codexResponsesToolCallLeakPattern.MatchString(content) {
		diagnostics = append(diagnostics, "rejected leaked tool-call text from assistant content")
		content = ""
		incomplete = true
	}
	if content != "" {
		events = append(events, Event{Kind: EventToken, Token: content})
	}

	reasoning := strings.TrimSpace(strings.Join(reasoningParts, "\n\n"))
	message := Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
	if reasoning != "" {
		message.Reasoning = &ReasoningContent{Text: reasoning}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	} else if incomplete {
		finishReason = "incomplete"
	}
	events = append(events, Event{
		Kind:         EventDone,
		FinishReason: finishReason,
		TokensIn:     response.Usage.InputTokens,
		TokensOut:    response.Usage.OutputTokens,
		ToolCalls:    toolCalls,
	})

	return codexResponsesNormalized{
		Message:      message,
		Events:       events,
		Usage:        response.Usage,
		FinishReason: finishReason,
		Diagnostics:  diagnostics,
	}, nil
}

func codexResponsesContentParts(role string, parts []MessageContentPart) []codexResponsesContentPart {
	out := make([]codexResponsesContentPart, 0, len(parts))
	textType := "input_text"
	if strings.ToLower(strings.TrimSpace(role)) == "assistant" {
		textType = "output_text"
	}
	for _, part := range parts {
		partType := strings.ToLower(strings.TrimSpace(part.Type))
		switch partType {
		case "text", "input_text", "output_text":
			if part.Text == "" {
				continue
			}
			out = append(out, codexResponsesContentPart{Type: textType, Text: part.Text})
		case "image_url", "input_image":
			if part.ImageURL == "" {
				continue
			}
			image := codexResponsesContentPart{Type: "input_image", ImageURL: part.ImageURL}
			if strings.TrimSpace(part.Detail) != "" {
				image.Detail = strings.TrimSpace(part.Detail)
			}
			out = append(out, image)
		}
	}
	return out
}

func codexResponsesTools(tools []ToolDescriptor) []codexResponsesTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]codexResponsesTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Schema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, codexResponsesTool{
			Type:        "function",
			Name:        name,
			Description: tool.Description,
			Strict:      false,
			Parameters:  params,
		})
	}
	return out
}

func codexResponsesArguments(raw json.RawMessage) string {
	args := strings.TrimSpace(string(raw))
	if args == "" {
		return "{}"
	}
	return args
}

func codexResponsesMessageText(item codexResponsesOutputItem) string {
	parts := make([]string, 0, len(item.Content))
	for _, part := range item.Content {
		switch part.Type {
		case "output_text", "text":
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func codexResponsesReasoningText(item codexResponsesOutputItem) string {
	parts := make([]string, 0, len(item.Summary))
	for _, part := range item.Summary {
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func codexResponsesOutputArguments(item codexResponsesOutputItem) string {
	raw := item.Arguments
	if item.Type == "custom_tool_call" && len(raw) == 0 {
		raw = item.Input
	}
	args := strings.TrimSpace(string(raw))
	if args == "" {
		return "{}"
	}
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err == nil {
		args = strings.TrimSpace(decoded)
		if args == "" {
			return "{}"
		}
	}
	return args
}

func deterministicCodexResponsesCallID(name, arguments string, index int) string {
	sum := sha256.Sum256([]byte(name + ":" + arguments + ":" + strconv.Itoa(index)))
	return "call_" + hex.EncodeToString(sum[:])[:12]
}

func splitCodexResponsesToolID(raw string) (callID string, responseItemID string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ""
	}
	if before, after, ok := strings.Cut(value, "|"); ok {
		return strings.TrimSpace(before), strings.TrimSpace(after)
	}
	if strings.HasPrefix(value, "fc_") {
		return "", value
	}
	return value, ""
}
