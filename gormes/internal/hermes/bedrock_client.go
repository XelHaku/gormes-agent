package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockdocument "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithy "github.com/aws/smithy-go"
)

const defaultBedrockRegion = "us-east-1"

type bedrockStreamReader interface {
	Events() <-chan bedrocktypes.ConverseStreamOutput
	Close() error
	Err() error
}

type bedrockConverseAPI interface {
	ConverseStream(ctx context.Context, input *bedrockruntime.ConverseStreamInput) (bedrockStreamReader, error)
}

type bedrockSDKClient struct {
	client *bedrockruntime.Client
}

func (c *bedrockSDKClient) ConverseStream(ctx context.Context, input *bedrockruntime.ConverseStreamInput) (bedrockStreamReader, error) {
	output, err := c.client.ConverseStream(ctx, input)
	if err != nil {
		return nil, err
	}
	return output.GetStream(), nil
}

type bedrockClient struct {
	region         string
	baseURL        string
	runtime        bedrockConverseAPI
	runtimeFactory func(context.Context, string, string) (bedrockConverseAPI, error)
}

type bedrockStream struct {
	reader       bedrockStreamReader
	closed       bool
	mu           sync.Mutex
	metadataSeen bool
	finishSeen   bool
	finish       string
	tokensIn     int
	tokensOut    int
	toolCalls    map[int]*pendingBedrockToolCall
}

type pendingBedrockToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func newBedrockClient(baseURL string) Client {
	normalized := normalizeBedrockEndpoint(baseURL)
	return &bedrockClient{
		region:         resolveBedrockRegion(normalized),
		baseURL:        normalized,
		runtimeFactory: defaultBedrockRuntimeFactory,
	}
}

func (c *bedrockClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	runtime, err := c.ensureRuntime(ctx)
	if err != nil {
		return nil, err
	}
	input, err := buildBedrockRequest(req)
	if err != nil {
		return nil, err
	}
	reader, err := runtime.ConverseStream(ctx, input)
	if err != nil {
		return nil, mapBedrockError(err)
	}
	return newBedrockStream(reader), nil
}

func (c *bedrockClient) OpenRunEvents(context.Context, string) (RunEventStream, error) {
	return nil, ErrRunEventsNotSupported
}

func (c *bedrockClient) Health(ctx context.Context) error {
	_, err := c.ensureRuntime(ctx)
	return err
}

func (c *bedrockClient) ensureRuntime(ctx context.Context) (bedrockConverseAPI, error) {
	if c.runtime != nil {
		return c.runtime, nil
	}
	factory := c.runtimeFactory
	if factory == nil {
		factory = defaultBedrockRuntimeFactory
	}
	runtime, err := factory(ctx, c.region, c.baseURL)
	if err != nil {
		return nil, err
	}
	c.runtime = runtime
	return runtime, nil
}

func defaultBedrockRuntimeFactory(ctx context.Context, region, baseURL string) (bedrockConverseAPI, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(resolveBedrockRegion(region)))
	if err != nil {
		return nil, err
	}
	if base := strings.TrimSpace(baseURL); base != "" {
		cfg.BaseEndpoint = aws.String(base)
	}
	return &bedrockSDKClient{client: bedrockruntime.NewFromConfig(cfg)}, nil
}

func defaultBedrockBaseURL(region string) string {
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", resolveBedrockRegion(region))
}

func normalizeBedrockEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" || trimmed == defaultOpenAIEndpoint {
		return defaultBedrockBaseURL("")
	}
	return trimmed
}

func resolveBedrockRegion(baseURL string) string {
	if region := bedrockRegionFromEndpoint(baseURL); region != "" {
		return region
	}
	if env := strings.TrimSpace(os.Getenv("AWS_REGION")); env != "" {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); env != "" {
		return env
	}
	return defaultBedrockRegion
}

func bedrockRegionFromEndpoint(baseURL string) string {
	if strings.TrimSpace(baseURL) == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	parts := strings.Split(host, ".")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "bedrock-runtime" {
			return parts[i+1]
		}
	}
	return ""
}

func buildBedrockRequest(req ChatRequest) (*bedrockruntime.ConverseStreamInput, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, errors.New("bedrock: model required")
	}
	system, messages, err := convertBedrockMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(model),
		Messages: messages,
	}
	if len(system) > 0 {
		input.System = system
	}
	if len(req.Tools) > 0 {
		input.ToolConfig, err = buildBedrockToolConfig(req.Tools)
		if err != nil {
			return nil, err
		}
	}
	return input, nil
}

func convertBedrockMessages(messages []Message) ([]bedrocktypes.SystemContentBlock, []bedrocktypes.Message, error) {
	var (
		system []bedrocktypes.SystemContentBlock
		out    []bedrocktypes.Message
	)
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				system = append(system, &bedrocktypes.SystemContentBlockMemberText{Value: msg.Content})
			}
		case "user":
			out = append(out, bedrocktypes.Message{
				Role:    bedrocktypes.ConversationRoleUser,
				Content: []bedrocktypes.ContentBlock{&bedrocktypes.ContentBlockMemberText{Value: textOrFallback(msg.Content, "(empty)")}},
			})
		case "assistant":
			content, err := bedrockAssistantContent(msg)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, bedrocktypes.Message{
				Role:    bedrocktypes.ConversationRoleAssistant,
				Content: content,
			})
		case "tool":
			out = appendBedrockToolResult(out, msg)
		default:
			return nil, nil, fmt.Errorf("bedrock: unsupported message role %q", msg.Role)
		}
	}
	return system, out, nil
}

func bedrockAssistantContent(msg Message) ([]bedrocktypes.ContentBlock, error) {
	blocks := make([]bedrocktypes.ContentBlock, 0, 1+len(msg.ToolCalls))
	if msg.Content != "" {
		blocks = append(blocks, &bedrocktypes.ContentBlockMemberText{Value: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		input := map[string]any{}
		if len(tc.Arguments) > 0 {
			if err := json.Unmarshal(tc.Arguments, &input); err != nil {
				return nil, err
			}
		}
		blocks = append(blocks, &bedrocktypes.ContentBlockMemberToolUse{
			Value: bedrocktypes.ToolUseBlock{
				Input:     bedrockdocument.NewLazyDocument(input),
				Name:      aws.String(tc.Name),
				ToolUseId: aws.String(tc.ID),
			},
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, &bedrocktypes.ContentBlockMemberText{Value: "(empty)"})
	}
	return blocks, nil
}

func appendBedrockToolResult(out []bedrocktypes.Message, msg Message) []bedrocktypes.Message {
	block := &bedrocktypes.ContentBlockMemberToolResult{
		Value: bedrocktypes.ToolResultBlock{
			Content: []bedrocktypes.ToolResultContentBlock{
				&bedrocktypes.ToolResultContentBlockMemberText{Value: textOrFallback(msg.Content, "(no output)")},
			},
			Status:    bedrocktypes.ToolResultStatusSuccess,
			ToolUseId: aws.String(msg.ToolCallID),
		},
	}
	if len(out) > 0 && out[len(out)-1].Role == bedrocktypes.ConversationRoleUser && startsWithBedrockToolResult(out[len(out)-1].Content) {
		out[len(out)-1].Content = append(out[len(out)-1].Content, block)
		return out
	}
	return append(out, bedrocktypes.Message{
		Role:    bedrocktypes.ConversationRoleUser,
		Content: []bedrocktypes.ContentBlock{block},
	})
}

func startsWithBedrockToolResult(blocks []bedrocktypes.ContentBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	_, ok := blocks[0].(*bedrocktypes.ContentBlockMemberToolResult)
	return ok
}

func buildBedrockToolConfig(tools []ToolDescriptor) (*bedrocktypes.ToolConfiguration, error) {
	out := make([]bedrocktypes.Tool, 0, len(tools))
	for _, tool := range tools {
		schema := map[string]any{}
		if len(tool.Schema) > 0 {
			if err := json.Unmarshal(tool.Schema, &schema); err != nil {
				return nil, err
			}
		}
		spec := bedrocktypes.ToolSpecification{
			InputSchema: &bedrocktypes.ToolInputSchemaMemberJson{Value: bedrockdocument.NewLazyDocument(schema)},
			Name:        aws.String(tool.Name),
		}
		if tool.Description != "" {
			spec.Description = aws.String(tool.Description)
		}
		out = append(out, &bedrocktypes.ToolMemberToolSpec{Value: spec})
	}
	return &bedrocktypes.ToolConfiguration{Tools: out}, nil
}

func textOrFallback(text, fallback string) string {
	if text != "" {
		return text
	}
	return fallback
}

func newBedrockStream(reader bedrockStreamReader) *bedrockStream {
	return &bedrockStream{
		reader:    reader,
		toolCalls: make(map[int]*pendingBedrockToolCall),
	}
}

func (s *bedrockStream) SessionID() string { return "" }

func (s *bedrockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.reader.Close()
}

func (s *bedrockStream) Recv(ctx context.Context) (Event, error) {
	for {
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case event, ok := <-s.reader.Events():
			if !ok {
				if done, ok := s.buildDoneEvent(); ok {
					return done, nil
				}
				if err := s.reader.Err(); err != nil {
					return Event{}, mapBedrockError(err)
				}
				return Event{}, io.EOF
			}
			if mapped, ok := s.handleBedrockEvent(event); ok {
				return mapped, nil
			}
		}
	}
}

func (s *bedrockStream) handleBedrockEvent(event bedrocktypes.ConverseStreamOutput) (Event, bool) {
	switch v := event.(type) {
	case *bedrocktypes.ConverseStreamOutputMemberContentBlockStart:
		s.handleBedrockContentStart(v.Value)
	case *bedrocktypes.ConverseStreamOutputMemberContentBlockDelta:
		return s.handleBedrockContentDelta(v.Value)
	case *bedrocktypes.ConverseStreamOutputMemberMessageStop:
		s.finishSeen = true
		s.finish = mapBedrockStopReason(string(v.Value.StopReason))
		return s.buildDoneEvent()
	case *bedrocktypes.ConverseStreamOutputMemberMetadata:
		if v.Value.Usage != nil {
			s.tokensIn = int(aws.ToInt32(v.Value.Usage.InputTokens))
			s.tokensOut = int(aws.ToInt32(v.Value.Usage.OutputTokens))
		}
		s.metadataSeen = true
		return s.buildDoneEvent()
	}
	return Event{}, false
}

func (s *bedrockStream) handleBedrockContentStart(event bedrocktypes.ContentBlockStartEvent) {
	index := int(aws.ToInt32(event.ContentBlockIndex))
	switch start := event.Start.(type) {
	case *bedrocktypes.ContentBlockStartMemberToolUse:
		s.toolCalls[index] = &pendingBedrockToolCall{
			id:   aws.ToString(start.Value.ToolUseId),
			name: aws.ToString(start.Value.Name),
		}
	}
}

func (s *bedrockStream) handleBedrockContentDelta(event bedrocktypes.ContentBlockDeltaEvent) (Event, bool) {
	index := int(aws.ToInt32(event.ContentBlockIndex))
	switch delta := event.Delta.(type) {
	case *bedrocktypes.ContentBlockDeltaMemberReasoningContent:
		if text := bedrockReasoningText(delta.Value); text != "" {
			return Event{Kind: EventReasoning, Reasoning: text}, true
		}
	case *bedrocktypes.ContentBlockDeltaMemberText:
		if delta.Value != "" {
			return Event{Kind: EventToken, Token: delta.Value}, true
		}
	case *bedrocktypes.ContentBlockDeltaMemberToolUse:
		call, ok := s.toolCalls[index]
		if !ok {
			call = &pendingBedrockToolCall{}
			s.toolCalls[index] = call
		}
		if delta.Value.Input != nil {
			call.arguments.WriteString(aws.ToString(delta.Value.Input))
		}
	}
	return Event{}, false
}

func (s *bedrockStream) buildDoneEvent() (Event, bool) {
	if !s.finishSeen || !s.metadataSeen {
		return Event{}, false
	}
	done := Event{
		Kind:         EventDone,
		FinishReason: s.finish,
		TokensIn:     s.tokensIn,
		TokensOut:    s.tokensOut,
	}
	if done.FinishReason == "tool_calls" && len(s.toolCalls) > 0 {
		done.ToolCalls = flushBedrockToolCalls(s.toolCalls)
		s.toolCalls = make(map[int]*pendingBedrockToolCall)
	}
	s.finishSeen = false
	s.metadataSeen = false
	s.finish = ""
	s.tokensIn = 0
	s.tokensOut = 0
	return done, true
}

func flushBedrockToolCalls(m map[int]*pendingBedrockToolCall) []ToolCall {
	indexes := make([]int, 0, len(m))
	for idx := range m {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		call := m[idx]
		args := call.arguments.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out = append(out, ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(args),
		})
	}
	return out
}

func bedrockReasoningText(reasoning bedrocktypes.ReasoningContentBlockDelta) string {
	switch value := reasoning.(type) {
	case *bedrocktypes.ReasoningContentBlockDeltaMemberText:
		return value.Value
	default:
		return ""
	}
}

func mapBedrockStopReason(reason string) string {
	switch reason {
	case string(bedrocktypes.StopReasonEndTurn), string(bedrocktypes.StopReasonStopSequence):
		return "stop"
	case string(bedrocktypes.StopReasonToolUse):
		return "tool_calls"
	case string(bedrocktypes.StopReasonMaxTokens), string(bedrocktypes.StopReasonModelContextWindowExceeded):
		return "length"
	case string(bedrocktypes.StopReasonGuardrailIntervened), string(bedrocktypes.StopReasonContentFiltered):
		return "content_filter"
	default:
		if reason == "" {
			return "stop"
		}
		return reason
	}
}

func mapBedrockError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return &HTTPError{
			Status: bedrockErrorStatus(apiErr.ErrorCode()),
			Body:   apiErr.ErrorMessage(),
		}
	}
	return err
}

func bedrockErrorStatus(code string) int {
	switch code {
	case "AccessDeniedException", "UnrecognizedClientException":
		return 403
	case "ResourceNotFoundException":
		return 404
	case "ThrottlingException":
		return 429
	case "ValidationException":
		return 400
	case "InternalServerException", "ModelNotReadyException", "ModelStreamErrorException", "ModelTimeoutException", "ServiceUnavailableException":
		return 503
	default:
		return 500
	}
}
