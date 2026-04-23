package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockdocument "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithy "github.com/aws/smithy-go"
)

type fakeBedrockRuntime struct {
	stream bedrockStreamReader
	err    error
}

func (f *fakeBedrockRuntime) ConverseStream(context.Context, *bedrockruntime.ConverseStreamInput) (bedrockStreamReader, error) {
	return f.stream, f.err
}

type fakeBedrockStream struct {
	events chan bedrocktypes.ConverseStreamOutput
	err    error
}

func newFakeBedrockStream(events ...bedrocktypes.ConverseStreamOutput) *fakeBedrockStream {
	ch := make(chan bedrocktypes.ConverseStreamOutput, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return &fakeBedrockStream{events: ch}
}

func (s *fakeBedrockStream) Events() <-chan bedrocktypes.ConverseStreamOutput { return s.events }
func (s *fakeBedrockStream) Close() error                                     { return nil }
func (s *fakeBedrockStream) Err() error                                       { return s.err }

func TestNewClient_BedrockUsesNativeClient(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")

	client := NewClient("bedrock", defaultOpenAIEndpoint, "")
	if _, ok := client.(*bedrockClient); !ok {
		t.Fatalf("client type = %T, want *bedrockClient", client)
	}
	if got := EffectiveEndpoint("bedrock", defaultOpenAIEndpoint); got != "https://bedrock-runtime.us-west-2.amazonaws.com" {
		t.Fatalf("EffectiveEndpoint() = %q, want Bedrock regional runtime URL", got)
	}
}

func TestBuildBedrockRequest_MapsConversationAndTools(t *testing.T) {
	input, err := buildBedrockRequest(ChatRequest{
		Model: "us.anthropic.claude-sonnet-4-6",
		Messages: []Message{
			{Role: "system", Content: "system guidance"},
			{Role: "user", Content: "look up weather"},
			{
				Role:    "assistant",
				Content: "Calling a tool",
				ToolCalls: []ToolCall{{
					ID:        "toolu_1",
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"location":"Monterrey"}`),
				}},
			},
			{Role: "tool", ToolCallID: "toolu_1", Name: "get_weather", Content: "72F and sunny"},
		},
		Tools: []ToolDescriptor{{
			Name:        "get_weather",
			Description: "Returns the weather",
			Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		}},
	})
	if err != nil {
		t.Fatalf("buildBedrockRequest() error = %v", err)
	}

	if got := aws.ToString(input.ModelId); got != "us.anthropic.claude-sonnet-4-6" {
		t.Fatalf("ModelId = %q, want us.anthropic.claude-sonnet-4-6", got)
	}
	if len(input.System) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(input.System))
	}
	systemBlock, ok := input.System[0].(*bedrocktypes.SystemContentBlockMemberText)
	if !ok || systemBlock.Value != "system guidance" {
		t.Fatalf("system block = %#v, want cached text block", input.System[0])
	}
	if len(input.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3 after tool-result continuation mapping", len(input.Messages))
	}

	user := input.Messages[0]
	if user.Role != bedrocktypes.ConversationRoleUser {
		t.Fatalf("user role = %q, want %q", user.Role, bedrocktypes.ConversationRoleUser)
	}
	userText, ok := user.Content[0].(*bedrocktypes.ContentBlockMemberText)
	if !ok || userText.Value != "look up weather" {
		t.Fatalf("user content = %#v, want text block", user.Content[0])
	}

	assistant := input.Messages[1]
	if assistant.Role != bedrocktypes.ConversationRoleAssistant {
		t.Fatalf("assistant role = %q, want %q", assistant.Role, bedrocktypes.ConversationRoleAssistant)
	}
	if len(assistant.Content) != 2 {
		t.Fatalf("assistant blocks len = %d, want 2", len(assistant.Content))
	}
	assistantToolUse, ok := assistant.Content[1].(*bedrocktypes.ContentBlockMemberToolUse)
	if !ok {
		t.Fatalf("assistant tool use = %#v, want tool use block", assistant.Content[1])
	}
	if got := aws.ToString(assistantToolUse.Value.ToolUseId); got != "toolu_1" {
		t.Fatalf("tool_use_id = %q, want toolu_1", got)
	}
	if got := aws.ToString(assistantToolUse.Value.Name); got != "get_weather" {
		t.Fatalf("tool_use name = %q, want get_weather", got)
	}
	assistantInput := decodeBedrockDocument(t, assistantToolUse.Value.Input)
	if assistantInput["location"] != "Monterrey" {
		t.Fatalf("assistant tool input = %#v, want Monterrey", assistantInput)
	}

	toolResult := input.Messages[2]
	if toolResult.Role != bedrocktypes.ConversationRoleUser {
		t.Fatalf("tool result role = %q, want %q", toolResult.Role, bedrocktypes.ConversationRoleUser)
	}
	resultBlock, ok := toolResult.Content[0].(*bedrocktypes.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("tool result block = %#v, want tool result", toolResult.Content[0])
	}
	if got := aws.ToString(resultBlock.Value.ToolUseId); got != "toolu_1" {
		t.Fatalf("tool result tool_use_id = %q, want toolu_1", got)
	}
	resultText, ok := resultBlock.Value.Content[0].(*bedrocktypes.ToolResultContentBlockMemberText)
	if !ok || resultText.Value != "72F and sunny" {
		t.Fatalf("tool result content = %#v, want text block", resultBlock.Value.Content[0])
	}

	if input.ToolConfig == nil || len(input.ToolConfig.Tools) != 1 {
		t.Fatalf("tool config = %#v, want one tool", input.ToolConfig)
	}
	spec, ok := input.ToolConfig.Tools[0].(*bedrocktypes.ToolMemberToolSpec)
	if !ok {
		t.Fatalf("tool spec = %#v, want ToolMemberToolSpec", input.ToolConfig.Tools[0])
	}
	if got := aws.ToString(spec.Value.Name); got != "get_weather" {
		t.Fatalf("tool name = %q, want get_weather", got)
	}
	schemaDoc, ok := spec.Value.InputSchema.(*bedrocktypes.ToolInputSchemaMemberJson)
	if !ok {
		t.Fatalf("tool schema = %#v, want json schema", spec.Value.InputSchema)
	}
	schema := decodeBedrockDocument(t, schemaDoc.Value)
	if schema["type"] != "object" {
		t.Fatalf("schema = %#v, want type object", schema)
	}
}

func TestBedrockStream_AccumulatesToolUseDeltasAndMapsStopReason(t *testing.T) {
	stream := newBedrockStream(newFakeBedrockStream(
		&bedrocktypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: bedrocktypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta: &bedrocktypes.ContentBlockDeltaMemberReasoningContent{
					Value: &bedrocktypes.ReasoningContentBlockDeltaMemberText{Value: "Need a tool."},
				},
			},
		},
		&bedrocktypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: bedrocktypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(1),
				Delta:             &bedrocktypes.ContentBlockDeltaMemberText{Value: "Checking weather. "},
			},
		},
		&bedrocktypes.ConverseStreamOutputMemberContentBlockStart{
			Value: bedrocktypes.ContentBlockStartEvent{
				ContentBlockIndex: aws.Int32(2),
				Start: &bedrocktypes.ContentBlockStartMemberToolUse{
					Value: bedrocktypes.ToolUseBlockStart{
						Name:      aws.String("get_weather"),
						ToolUseId: aws.String("toolu_1"),
					},
				},
			},
		},
		&bedrocktypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: bedrocktypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(2),
				Delta: &bedrocktypes.ContentBlockDeltaMemberToolUse{
					Value: bedrocktypes.ToolUseBlockDelta{Input: aws.String("{\"location\":\"Monterrey\"}")},
				},
			},
		},
		&bedrocktypes.ConverseStreamOutputMemberMessageStop{
			Value: bedrocktypes.MessageStopEvent{StopReason: bedrocktypes.StopReasonToolUse},
		},
		&bedrocktypes.ConverseStreamOutputMemberMetadata{
			Value: bedrocktypes.ConverseStreamMetadataEvent{
				Usage: &bedrocktypes.TokenUsage{
					InputTokens:  aws.Int32(11),
					OutputTokens: aws.Int32(23),
					TotalTokens:  aws.Int32(34),
				},
			},
		},
	))

	var got []Event
	for {
		event, err := stream.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
		got = append(got, event)
	}

	if len(got) != 3 {
		t.Fatalf("event count = %d, want 3 (reasoning, token, done)", len(got))
	}
	if got[0].Kind != EventReasoning || got[0].Reasoning != "Need a tool." {
		t.Fatalf("got[0] = %+v, want reasoning delta", got[0])
	}
	if got[1].Kind != EventToken || got[1].Token != "Checking weather. " {
		t.Fatalf("got[1] = %+v, want token delta", got[1])
	}
	final := got[2]
	if final.Kind != EventDone {
		t.Fatalf("final kind = %v, want EventDone", final.Kind)
	}
	if final.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want tool_calls", final.FinishReason)
	}
	if final.TokensIn != 11 || final.TokensOut != 23 {
		t.Fatalf("usage = %d/%d, want 11/23", final.TokensIn, final.TokensOut)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "get_weather" {
		t.Fatalf("tool call = %+v, want toolu_1/get_weather", tc)
	}
	if strings.TrimSpace(string(tc.Arguments)) != `{"location":"Monterrey"}` {
		t.Fatalf("tool args = %s, want Monterrey payload", tc.Arguments)
	}
}

func TestBedrockOpenStream_MapsRateLimitErrors(t *testing.T) {
	client := &bedrockClient{
		region: "us-east-1",
		runtime: &fakeBedrockRuntime{
			err: &smithy.OperationError{
				ServiceID:     "Bedrock Runtime",
				OperationName: "ConverseStream",
				Err: &smithy.GenericAPIError{
					Code:    "ThrottlingException",
					Message: "slow down",
					Fault:   smithy.FaultClient,
				},
			},
		},
	}

	_, err := client.OpenStream(context.Background(), ChatRequest{
		Model:    "us.anthropic.claude-sonnet-4-6",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("OpenStream() err = nil, want throttling error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpErr.Status != 429 {
		t.Fatalf("status = %d, want 429", httpErr.Status)
	}
	if !strings.Contains(httpErr.Body, "slow down") {
		t.Fatalf("body = %q, want slow down", httpErr.Body)
	}
	if got := Classify(err); got != ClassRetryable {
		t.Fatalf("Classify(err) = %q, want %q", got, ClassRetryable)
	}
}

func decodeBedrockDocument(t *testing.T, doc bedrockdocument.Interface) map[string]any {
	t.Helper()
	raw, err := doc.MarshalSmithyDocument()
	if err != nil {
		t.Fatalf("MarshalSmithyDocument() error = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return out
}
