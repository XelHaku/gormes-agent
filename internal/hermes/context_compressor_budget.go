package hermes

const (
	defaultContextCompressorThresholdPercent   = 0.50
	defaultContextCompressorSummaryTargetRatio = 0.20
	minContextCompressorSummaryTargetRatio     = 0.10
	maxContextCompressorSummaryTargetRatio     = 0.80
	minimumContextCompressorContextLength      = 64_000
	contextCompressorSummaryTokensCeiling      = 12_000
)

type ContextCompressorBudgetConfig struct {
	Model              string
	ContextLength      int
	ThresholdPercent   float64
	SummaryTargetRatio float64
}

type ContextCompressorBudgetStatus struct {
	State              string  `json:"state"`
	Model              string  `json:"model,omitempty"`
	ContextLength      int     `json:"context_length,omitempty"`
	ThresholdPercent   float64 `json:"threshold_percent"`
	ThresholdTokens    int     `json:"threshold_tokens,omitempty"`
	SummaryTargetRatio float64 `json:"summary_target_ratio"`
	TailTokenBudget    int     `json:"tail_token_budget,omitempty"`
	MaxSummaryTokens   int     `json:"max_summary_tokens,omitempty"`
}

type ContextCompressorBudget struct {
	model              string
	contextLength      int
	thresholdPercent   float64
	summaryTargetRatio float64
	thresholdTokens    int
	tailTokenBudget    int
	maxSummaryTokens   int
}

func NewContextCompressorBudget(config ContextCompressorBudgetConfig) *ContextCompressorBudget {
	budget := &ContextCompressorBudget{
		model:              config.Model,
		contextLength:      config.ContextLength,
		thresholdPercent:   normalizeContextCompressorThresholdPercent(config.ThresholdPercent),
		summaryTargetRatio: normalizeContextCompressorSummaryTargetRatio(config.SummaryTargetRatio),
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
			State:              "unavailable",
			Model:              b.model,
			ThresholdPercent:   b.thresholdPercent,
			SummaryTargetRatio: b.summaryTargetRatio,
		}
	}
	return ContextCompressorBudgetStatus{
		State:              "ready",
		Model:              b.model,
		ContextLength:      b.contextLength,
		ThresholdPercent:   b.thresholdPercent,
		ThresholdTokens:    b.thresholdTokens,
		SummaryTargetRatio: b.summaryTargetRatio,
		TailTokenBudget:    b.tailTokenBudget,
		MaxSummaryTokens:   b.maxSummaryTokens,
	}
}

func (b *ContextCompressorBudget) recalculate() {
	if b.contextLength <= 0 {
		b.clearDerivedBudgets()
		return
	}
	thresholdTokens := int(float64(b.contextLength) * b.thresholdPercent)
	if thresholdTokens < minimumContextCompressorContextLength {
		thresholdTokens = minimumContextCompressorContextLength
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
	b.tailTokenBudget = 0
	b.maxSummaryTokens = 0
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
