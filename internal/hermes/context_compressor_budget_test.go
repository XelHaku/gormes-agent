package hermes

import "testing"

func TestContextCompressorBudgetInitializesFromActiveContextWindow(t *testing.T) {
	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "model-a",
		ContextLength:      200_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.20,
	})

	status := budget.Status()
	if status.State != "ready" {
		t.Fatalf("State = %q, want ready", status.State)
	}
	if status.ThresholdTokens != 100_000 {
		t.Fatalf("ThresholdTokens = %d, want 100000", status.ThresholdTokens)
	}
	if status.TailTokenBudget != 20_000 {
		t.Fatalf("TailTokenBudget = %d, want 20000", status.TailTokenBudget)
	}
	if status.MaxSummaryTokens != 10_000 {
		t.Fatalf("MaxSummaryTokens = %d, want min(context_length*0.05, 12000) = 10000", status.MaxSummaryTokens)
	}
}

func TestContextCompressorBudgetUpdateModelContextRecalculatesBudgets(t *testing.T) {
	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "model-a",
		ContextLength:      200_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.20,
	})
	oldStatus := budget.Status()

	budget.UpdateModelContext(ContextModelContext{
		Model:         "model-b",
		ContextLength: 32_000,
	})

	status := budget.Status()
	if status.Model != "model-b" || status.ContextLength != 32_000 {
		t.Fatalf("model context = %q/%d, want model-b/32000", status.Model, status.ContextLength)
	}
	if status.ThresholdTokens != 64_000 {
		t.Fatalf("ThresholdTokens = %d, want 64000 floor", status.ThresholdTokens)
	}
	if status.TailTokenBudget != 12_800 {
		t.Fatalf("TailTokenBudget = %d, want threshold*ratio = 12800", status.TailTokenBudget)
	}
	if status.MaxSummaryTokens != 1_600 {
		t.Fatalf("MaxSummaryTokens = %d, want 1600 from new context window", status.MaxSummaryTokens)
	}
	if status.TailTokenBudget == oldStatus.TailTokenBudget || status.MaxSummaryTokens == oldStatus.MaxSummaryTokens {
		t.Fatalf("budgets were not recalculated: old=%#v new=%#v", oldStatus, status)
	}
}

func TestContextCompressorBudgetClampsSummaryRatioAndFloorsThreshold(t *testing.T) {
	lowRatio := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "low-ratio",
		ContextLength:      100_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.05,
	}).Status()
	if lowRatio.SummaryTargetRatio != 0.10 {
		t.Fatalf("low SummaryTargetRatio = %.2f, want 0.10", lowRatio.SummaryTargetRatio)
	}
	if lowRatio.ThresholdTokens != 64_000 {
		t.Fatalf("low ThresholdTokens = %d, want 64000 floor", lowRatio.ThresholdTokens)
	}
	if lowRatio.TailTokenBudget != 6_400 {
		t.Fatalf("low TailTokenBudget = %d, want 6400", lowRatio.TailTokenBudget)
	}

	highRatio := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "high-ratio",
		ContextLength:      200_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.95,
	}).Status()
	if highRatio.SummaryTargetRatio != 0.80 {
		t.Fatalf("high SummaryTargetRatio = %.2f, want 0.80", highRatio.SummaryTargetRatio)
	}
	if highRatio.TailTokenBudget != 80_000 {
		t.Fatalf("high TailTokenBudget = %d, want 80000", highRatio.TailTokenBudget)
	}
}

func TestContextCompressorBudgetUnknownModelContextDoesNotPreserveStaleBudgets(t *testing.T) {
	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "model-a",
		ContextLength:      200_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.20,
	})

	budget.UpdateModelContext(ContextModelContext{Model: "unknown-model"})

	status := budget.Status()
	if status.State != "unavailable" {
		t.Fatalf("State = %q, want unavailable for unknown context window", status.State)
	}
	if status.ContextLength != 0 || status.ThresholdTokens != 0 || status.TailTokenBudget != 0 || status.MaxSummaryTokens != 0 {
		t.Fatalf("status preserves stale budgets after unknown model switch: %#v", status)
	}
}
