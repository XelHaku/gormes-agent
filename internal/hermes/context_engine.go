package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
)

const ContextStatusToolName = "context_status"

var (
	ErrUnknownContextTool  = errors.New("hermes: unknown context engine tool")
	ErrCompressionDisabled = errors.New("hermes: context compression disabled")
)

type ContextEngine interface {
	Name() string
	UpdateFromResponse(ContextUsage)
	ShouldCompress(promptTokens int) bool
	Compress(ctx context.Context, messages []Message, req CompressionRequest) ([]Message, CompressionReport, error)
	ShouldCompressPreflight(messages []Message) bool
	HasContentToCompress(messages []Message) bool
	OnSessionStart(ctx context.Context, sessionID string, meta ContextSessionMeta) error
	OnSessionEnd(ctx context.Context, sessionID string, messages []Message) error
	OnSessionReset()
	ToolDescriptors() []ToolDescriptor
	HandleToolCall(ctx context.Context, name string, args json.RawMessage, opts ContextToolCallOptions) (json.RawMessage, error)
	Status() ContextStatus
	UpdateModelContext(ContextModelContext)
}

type ContextUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ContextModelContext struct {
	Model                       string
	ContextLength               int
	ThresholdPercent            float64
	ThresholdTokens             int
	AuxiliaryContextLength      int
	AuxiliaryContextSource      ModelContextSource
	AuxiliaryContextLookupError string
	ToolDescriptors             []ToolDescriptor
	BaseURL                     string
	Provider                    string
}

type CompressionRequest struct {
	CurrentTokens int
	FocusTopic    string
}

type CompressionReport struct {
	State          string `json:"state"`
	BeforeMessages int    `json:"before_messages"`
	AfterMessages  int    `json:"after_messages"`
	CurrentTokens  int    `json:"current_tokens,omitempty"`
	FocusTopic     string `json:"focus_topic,omitempty"`
}

type ContextSessionMeta struct {
	Model         string
	ContextLength int
	Platform      string
}

type ContextToolCallOptions struct {
	Messages []Message
}

type ContextStatus struct {
	Engine               string                   `json:"engine"`
	Model                string                   `json:"model"`
	ContextLength        int                      `json:"context_length"`
	ThresholdTokens      int                      `json:"threshold_tokens"`
	ThresholdPercent     float64                  `json:"threshold_percent"`
	LastPromptTokens     int                      `json:"last_prompt_tokens"`
	LastCompletionTokens int                      `json:"last_completion_tokens"`
	LastTotalTokens      int                      `json:"last_total_tokens"`
	UsagePercent         float64                  `json:"usage_percent"`
	CompressionCount     int                      `json:"compression_count"`
	Budget               ContextBudgetStatus      `json:"budget"`
	Compression          ContextCompressionStatus `json:"compression"`
	Tools                ContextToolStatus        `json:"tools"`
	Replay               ContextReplayStatus      `json:"replay"`
}

type ContextBudgetStatus struct {
	State           string `json:"state"`
	RemainingTokens int    `json:"remaining_tokens"`
	Pressure        bool   `json:"pressure"`
}

type ContextCompressionStatus struct {
	Enabled         bool   `json:"enabled"`
	ShouldCompress  bool   `json:"should_compress"`
	CooldownSeconds int    `json:"cooldown_seconds"`
	DisabledReason  string `json:"disabled_reason,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

type ContextToolStatus struct {
	StatusTool        string             `json:"status_tool"`
	UnknownToolErrors []ContextToolError `json:"unknown_tool_errors,omitempty"`
}

type ContextToolError struct {
	Type    string `json:"type"`
	Tool    string `json:"tool"`
	Message string `json:"message"`
}

func (e ContextToolError) Error() string { return e.Message }

type ContextReplayStatus struct {
	Gaps []ContextReplayGap `json:"gaps,omitempty"`
}

type ContextReplayGap struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type DisabledContextEngine struct {
	mu     sync.Mutex
	status ContextStatus
}

var _ ContextEngine = (*DisabledContextEngine)(nil)

func NewDisabledContextEngine(reason string) *DisabledContextEngine {
	if reason == "" {
		reason = "context compression disabled"
	}
	return &DisabledContextEngine{
		status: ContextStatus{
			Engine:           "disabled",
			ThresholdPercent: 0.75,
			Compression: ContextCompressionStatus{
				Enabled:        false,
				ShouldCompress: false,
				DisabledReason: reason,
			},
			Tools: ContextToolStatus{
				StatusTool: ContextStatusToolName,
			},
		},
	}
}

func (e *DisabledContextEngine) Name() string { return "disabled" }

func (e *DisabledContextEngine) UpdateFromResponse(usage ContextUsage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.LastPromptTokens = usage.PromptTokens
	e.status.LastCompletionTokens = usage.CompletionTokens
	if usage.TotalTokens > 0 {
		e.status.LastTotalTokens = usage.TotalTokens
	} else {
		e.status.LastTotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	e.refreshLocked()
}

func (e *DisabledContextEngine) ShouldCompress(int) bool { return false }

func (e *DisabledContextEngine) Compress(_ context.Context, messages []Message, req CompressionRequest) ([]Message, CompressionReport, error) {
	out := append([]Message(nil), messages...)
	return out, CompressionReport{
		State:          "disabled",
		BeforeMessages: len(messages),
		AfterMessages:  len(messages),
		CurrentTokens:  req.CurrentTokens,
		FocusTopic:     req.FocusTopic,
	}, ErrCompressionDisabled
}

func (e *DisabledContextEngine) ShouldCompressPreflight([]Message) bool { return false }

func (e *DisabledContextEngine) HasContentToCompress([]Message) bool { return false }

func (e *DisabledContextEngine) OnSessionStart(context.Context, string, ContextSessionMeta) error {
	return nil
}

func (e *DisabledContextEngine) OnSessionEnd(context.Context, string, []Message) error {
	return nil
}

func (e *DisabledContextEngine) OnSessionReset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.LastPromptTokens = 0
	e.status.LastCompletionTokens = 0
	e.status.LastTotalTokens = 0
	e.status.CompressionCount = 0
	e.status.Tools.UnknownToolErrors = nil
	e.refreshLocked()
}

func (e *DisabledContextEngine) ToolDescriptors() []ToolDescriptor {
	return []ToolDescriptor{ContextStatusToolDescriptor()}
}

func ContextStatusToolDescriptor() ToolDescriptor {
	return ToolDescriptor{
		Name:        ContextStatusToolName,
		Description: "Reports context-window budget, compression state, and context-engine degraded modes.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (e *DisabledContextEngine) HandleToolCall(_ context.Context, name string, _ json.RawMessage, _ ContextToolCallOptions) (json.RawMessage, error) {
	if name == ContextStatusToolName {
		e.mu.Lock()
		e.refreshLocked()
		status := e.status
		e.mu.Unlock()
		payload, err := json.Marshal(status)
		return payload, err
	}

	toolErr := unknownContextToolError(name)
	e.mu.Lock()
	e.status.Tools.UnknownToolErrors = append(e.status.Tools.UnknownToolErrors, toolErr)
	e.refreshLocked()
	e.mu.Unlock()
	payload, err := json.Marshal(struct {
		Error ContextToolError `json:"error"`
	}{Error: toolErr})
	if err != nil {
		return nil, err
	}
	return payload, fmt.Errorf("%w: %s", ErrUnknownContextTool, name)
}

func (e *DisabledContextEngine) Status() ContextStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refreshLocked()
	return e.status
}

func (e *DisabledContextEngine) UpdateModelContext(update ContextModelContext) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if update.Model != "" {
		e.status.Model = update.Model
	}
	if update.ContextLength > 0 {
		e.status.ContextLength = update.ContextLength
	}
	if update.ThresholdPercent > 0 {
		e.status.ThresholdPercent = update.ThresholdPercent
	} else if e.status.ThresholdPercent <= 0 {
		e.status.ThresholdPercent = 0.75
	}
	if update.ThresholdTokens > 0 {
		e.status.ThresholdTokens = update.ThresholdTokens
	} else if e.status.ContextLength > 0 {
		e.status.ThresholdTokens = int(float64(e.status.ContextLength) * e.status.ThresholdPercent)
	}
	e.refreshLocked()
}

func (e *DisabledContextEngine) SetCompressionCooldown(seconds int, lastError string) {
	if seconds < 0 {
		seconds = 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.Compression.CooldownSeconds = seconds
	e.status.Compression.LastError = lastError
	e.refreshLocked()
}

func (e *DisabledContextEngine) RecordReplayGap(gap ContextReplayGap) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.Replay.Gaps = append(e.status.Replay.Gaps, gap)
	e.refreshLocked()
}

func (e *DisabledContextEngine) refreshLocked() {
	if e.status.ContextLength > 0 {
		usage := float64(e.status.LastPromptTokens) / float64(e.status.ContextLength) * 100
		e.status.UsagePercent = math.Min(100, roundPercent(usage))
	} else {
		e.status.UsagePercent = 0
	}
	e.status.Budget = classifyContextBudget(e.status.LastPromptTokens, e.status.ThresholdTokens, e.status.ContextLength)
	e.status.Compression.Enabled = false
	e.status.Compression.ShouldCompress = false
	e.status.Tools.StatusTool = ContextStatusToolName
}

func classifyContextBudget(promptTokens, thresholdTokens, contextLength int) ContextBudgetStatus {
	if thresholdTokens <= 0 || contextLength <= 0 {
		return ContextBudgetStatus{State: "unknown", RemainingTokens: 0, Pressure: false}
	}
	remaining := thresholdTokens - promptTokens
	if remaining < 0 {
		remaining = 0
	}
	state := "ok"
	pressure := false
	if promptTokens >= contextLength {
		state = "over_window"
		pressure = true
	} else if promptTokens >= thresholdTokens {
		state = "over_threshold"
		pressure = true
	} else if promptTokens >= int(float64(thresholdTokens)*0.90) {
		state = "pressure"
		pressure = true
	}
	return ContextBudgetStatus{State: state, RemainingTokens: remaining, Pressure: pressure}
}

func unknownContextToolError(name string) ContextToolError {
	return ContextToolError{
		Type:    "unknown_context_tool",
		Tool:    name,
		Message: "Unknown context engine tool: " + name,
	}
}

func roundPercent(v float64) float64 {
	return math.Round(v*100) / 100
}
