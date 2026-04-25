package hermes

import "encoding/json"

const (
	defaultContextCompressorThresholdPercent    = 0.50
	defaultContextCompressorSummaryTargetRatio  = 0.20
	minContextCompressorSummaryTargetRatio      = 0.10
	maxContextCompressorSummaryTargetRatio      = 0.80
	minimumContextCompressorContextLength       = 64_000
	contextCompressorSummaryTokensCeiling       = 12_000
	contextCompressorRequestFixedHeadroomTokens = 12_000
)

type ContextCompressorBudgetConfig struct {
	Model                  string
	ContextLength          int
	ThresholdPercent       float64
	SummaryTargetRatio     float64
	AuxiliaryContextLength int
	ToolDescriptors        []ToolDescriptor
}

type ContextCompressorBudgetStatus struct {
	State                  string  `json:"state"`
	Model                  string  `json:"model,omitempty"`
	ContextLength          int     `json:"context_length,omitempty"`
	AuxiliaryContextLength int     `json:"auxiliary_context_length,omitempty"`
	ThresholdPercent       float64 `json:"threshold_percent"`
	ThresholdTokens        int     `json:"threshold_tokens,omitempty"`
	RawThresholdTokens     int     `json:"raw_threshold_tokens,omitempty"`
	SummaryTargetRatio     float64 `json:"summary_target_ratio"`
	TailTokenBudget        int     `json:"tail_token_budget,omitempty"`
	MaxSummaryTokens       int     `json:"max_summary_tokens,omitempty"`
	ToolSchemaTokens       int     `json:"tool_schema_tokens,omitempty"`
	RequestHeadroomTokens  int     `json:"request_headroom_tokens,omitempty"`
	HeadroomClamped        bool    `json:"headroom_clamped,omitempty"`
}

type ContextCompressorBudget struct {
	model                  string
	contextLength          int
	auxiliaryContextLength int
	thresholdPercent       float64
	summaryTargetRatio     float64
	toolDescriptors        []ToolDescriptor
	thresholdTokens        int
	rawThresholdTokens     int
	tailTokenBudget        int
	maxSummaryTokens       int
	toolSchemaTokens       int
	requestHeadroomTokens  int
	headroomClamped        bool
}

func NewContextCompressorBudget(config ContextCompressorBudgetConfig) *ContextCompressorBudget {
	budget := &ContextCompressorBudget{
		model:                  config.Model,
		contextLength:          config.ContextLength,
		auxiliaryContextLength: config.AuxiliaryContextLength,
		thresholdPercent:       normalizeContextCompressorThresholdPercent(config.ThresholdPercent),
		summaryTargetRatio:     normalizeContextCompressorSummaryTargetRatio(config.SummaryTargetRatio),
		toolDescriptors:        cloneToolDescriptors(config.ToolDescriptors),
	}
	budget.recalculate()
	return budget
}

func (b *ContextCompressorBudget) UpdateModelContext(update ContextModelContext) {
	previousModel := b.model
	modelChanged := update.Model != "" && update.Model != previousModel
	if update.Model != "" {
		b.model = update.Model
	}
	if update.ThresholdPercent > 0 {
		b.thresholdPercent = update.ThresholdPercent
	} else if b.thresholdPercent <= 0 {
		b.thresholdPercent = defaultContextCompressorThresholdPercent
	}
	if update.AuxiliaryContextLength > 0 {
		b.auxiliaryContextLength = update.AuxiliaryContextLength
	}
	if update.ToolDescriptors != nil {
		b.toolDescriptors = cloneToolDescriptors(update.ToolDescriptors)
	}

	switch {
	case update.ContextLength > 0:
		b.contextLength = update.ContextLength
		b.recalculate()
	case modelChanged:
		b.contextLength = 0
		b.clearDerivedBudgets()
	default:
		b.recalculate()
	}
}

func (b *ContextCompressorBudget) Status() ContextCompressorBudgetStatus {
	if b.contextLength <= 0 {
		return ContextCompressorBudgetStatus{
			State:                  "unavailable",
			Model:                  b.model,
			AuxiliaryContextLength: b.auxiliaryContextLength,
			ThresholdPercent:       b.thresholdPercent,
			SummaryTargetRatio:     b.summaryTargetRatio,
			ToolSchemaTokens:       b.toolSchemaTokens,
		}
	}
	return ContextCompressorBudgetStatus{
		State:                  "ready",
		Model:                  b.model,
		ContextLength:          b.contextLength,
		AuxiliaryContextLength: b.auxiliaryContextLength,
		ThresholdPercent:       b.thresholdPercent,
		ThresholdTokens:        b.thresholdTokens,
		RawThresholdTokens:     b.rawThresholdTokens,
		SummaryTargetRatio:     b.summaryTargetRatio,
		TailTokenBudget:        b.tailTokenBudget,
		MaxSummaryTokens:       b.maxSummaryTokens,
		ToolSchemaTokens:       b.toolSchemaTokens,
		RequestHeadroomTokens:  b.requestHeadroomTokens,
		HeadroomClamped:        b.headroomClamped,
	}
}

func (b *ContextCompressorBudget) recalculate() {
	b.toolSchemaTokens = estimateToolDescriptorTokensRough(b.toolDescriptors)
	if b.contextLength <= 0 {
		b.clearDerivedBudgets()
		return
	}
	thresholdTokens := int(float64(b.contextLength) * b.thresholdPercent)
	if thresholdTokens < minimumContextCompressorContextLength {
		thresholdTokens = minimumContextCompressorContextLength
	}
	b.rawThresholdTokens = thresholdTokens
	b.requestHeadroomTokens = 0
	b.headroomClamped = false
	if b.auxiliaryContextLength > 0 && thresholdTokens > b.auxiliaryContextLength {
		b.requestHeadroomTokens = b.toolSchemaTokens + contextCompressorRequestFixedHeadroomTokens
		thresholdTokens = b.auxiliaryContextLength - b.requestHeadroomTokens
		if thresholdTokens < minimumContextCompressorContextLength {
			thresholdTokens = minimumContextCompressorContextLength
		}
		b.headroomClamped = true
	}
	b.thresholdTokens = thresholdTokens
	b.tailTokenBudget = int(float64(b.thresholdTokens) * b.summaryTargetRatio)
	b.maxSummaryTokens = minInt(
		int(float64(b.contextLength)*0.05),
		contextCompressorSummaryTokensCeiling,
	)
}

func (b *ContextCompressorBudget) clearDerivedBudgets() {
	b.thresholdTokens = 0
	b.rawThresholdTokens = 0
	b.tailTokenBudget = 0
	b.maxSummaryTokens = 0
	b.requestHeadroomTokens = 0
	b.headroomClamped = false
}

func normalizeContextCompressorThresholdPercent(value float64) float64 {
	if value > 0 {
		return value
	}
	return defaultContextCompressorThresholdPercent
}

func normalizeContextCompressorSummaryTargetRatio(value float64) float64 {
	if value <= 0 {
		return defaultContextCompressorSummaryTargetRatio
	}
	if value < minContextCompressorSummaryTargetRatio {
		return minContextCompressorSummaryTargetRatio
	}
	if value > maxContextCompressorSummaryTargetRatio {
		return maxContextCompressorSummaryTargetRatio
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cloneToolDescriptors(in []ToolDescriptor) []ToolDescriptor {
	if in == nil {
		return nil
	}
	out := make([]ToolDescriptor, len(in))
	for i, descriptor := range in {
		out[i] = ToolDescriptor{
			Name:        descriptor.Name,
			Description: descriptor.Description,
			Schema:      append(json.RawMessage(nil), descriptor.Schema...),
		}
	}
	return out
}

func estimateToolDescriptorTokensRough(descriptors []ToolDescriptor) int {
	if len(descriptors) == 0 {
		return 0
	}
	payload, err := json.Marshal(descriptors)
	if err != nil {
		return 0
	}
	return (len(payload) + 3) / 4
}
