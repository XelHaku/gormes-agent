package hermes

import (
	"errors"
	"testing"
)

func TestContextCompressorProviderCapForwardsProviderIdentityBeforeBudgeting(t *testing.T) {
	var queries []ModelContextQuery
	resolver := NewModelContextResolver(ModelContextLookupFunc(func(query ModelContextQuery) (int, bool, error) {
		queries = append(queries, query)
		if query.Provider == "openai-codex" && query.Model == "gpt-5.5" {
			return 272_000, true, nil
		}
		return 0, false, nil
	}))
	tools := syntheticCompressionToolDescriptors(8, 180, 90)

	got := NewContextCompressorAuxiliaryBudget(ContextCompressorAuxiliaryBudgetConfig{
		Model:                     "gpt-5.5",
		ContextLength:             1_050_000,
		ThresholdPercent:          0.85,
		SummaryTargetRatio:        0.20,
		ToolDescriptors:           tools,
		AuxiliaryProvider:         "openai-codex",
		AuxiliaryModel:            "gpt-5.5",
		AuxiliaryRawContextLength: 1_050_000,
		ContextResolver:           resolver,
	})

	if len(queries) != 1 {
		t.Fatalf("resolver queries = %d, want exactly one auxiliary lookup", len(queries))
	}
	query := queries[0]
	if query.Provider != "openai-codex" || query.Model != "gpt-5.5" || query.ModelInfo.ContextWindow != 1_050_000 {
		t.Fatalf("resolver query = %#v, want provider/model/raw auxiliary context forwarded", query)
	}
	if got.AuxiliaryContext.ContextLength != 272_000 {
		t.Fatalf("AuxiliaryContext.ContextLength = %d, want provider cap 272000", got.AuxiliaryContext.ContextLength)
	}
	if got.AuxiliaryContext.Source != ModelContextSourceProviderCap {
		t.Fatalf("AuxiliaryContext.Source = %q, want %q", got.AuxiliaryContext.Source, ModelContextSourceProviderCap)
	}

	status := got.Budget.Status()
	wantThreshold := 272_000 - status.ToolSchemaTokens - contextCompressorRequestFixedHeadroomTokens
	if status.AuxiliaryContextLength != 272_000 {
		t.Fatalf("budget AuxiliaryContextLength = %d, want provider-resolved 272000", status.AuxiliaryContextLength)
	}
	if status.AuxiliaryContextSource != ModelContextSourceProviderCap {
		t.Fatalf("budget AuxiliaryContextSource = %q, want %q", status.AuxiliaryContextSource, ModelContextSourceProviderCap)
	}
	if status.ThresholdTokens != wantThreshold {
		t.Fatalf("ThresholdTokens = %d, want provider cap minus tool/system headroom = %d", status.ThresholdTokens, wantThreshold)
	}
	if status.ThresholdTokens >= 272_000 {
		t.Fatalf("ThresholdTokens = %d, want below provider-resolved auxiliary cap", status.ThresholdTokens)
	}
	if !status.HeadroomClamped {
		t.Fatalf("HeadroomClamped = false, want provider-cap threshold to reserve auxiliary request headroom")
	}
}

func TestContextCompressorProviderCapDoesNotBorrowCodexCapForOpenAIOrEmptyProvider(t *testing.T) {
	resolver := NewModelContextResolver(ModelContextLookupFunc(func(query ModelContextQuery) (int, bool, error) {
		if query.Provider == "" {
			return 0, false, errors.New("missing auxiliary provider")
		}
		if query.Provider == "openai-codex" && query.Model == "gpt-5.5" {
			return 272_000, true, nil
		}
		return 0, false, nil
	}))

	openAI := NewContextCompressorAuxiliaryBudget(ContextCompressorAuxiliaryBudgetConfig{
		Model:                     "gpt-5.5",
		ContextLength:             1_050_000,
		ThresholdPercent:          0.85,
		SummaryTargetRatio:        0.20,
		AuxiliaryProvider:         "openai",
		AuxiliaryModel:            "gpt-5.5",
		AuxiliaryRawContextLength: 1_050_000,
		ContextResolver:           resolver,
	})
	openAIStatus := openAI.Budget.Status()
	if openAI.AuxiliaryContext.ContextLength != 1_050_000 {
		t.Fatalf("openai AuxiliaryContext.ContextLength = %d, want raw-model fallback 1050000", openAI.AuxiliaryContext.ContextLength)
	}
	if openAIStatus.AuxiliaryContextSource != ModelContextSourceModelsDev {
		t.Fatalf("openai AuxiliaryContextSource = %q, want %q", openAIStatus.AuxiliaryContextSource, ModelContextSourceModelsDev)
	}
	if openAIStatus.AuxiliaryContextLength == 272_000 || openAIStatus.HeadroomClamped {
		t.Fatalf("openai budget borrowed Codex cap or clamped unexpectedly: %#v", openAIStatus)
	}

	missing := NewContextCompressorAuxiliaryBudget(ContextCompressorAuxiliaryBudgetConfig{
		Model:              "gpt-5.5",
		ContextLength:      1_050_000,
		ThresholdPercent:   0.85,
		SummaryTargetRatio: 0.20,
		AuxiliaryModel:     "gpt-5.5",
		ContextResolver:    resolver,
	})
	missingStatus := missing.Budget.Status()
	if missing.AuxiliaryContext.ContextLength != 0 {
		t.Fatalf("missing-provider AuxiliaryContext.ContextLength = %d, want unavailable context", missing.AuxiliaryContext.ContextLength)
	}
	if missingStatus.AuxiliaryContextSource != ModelContextSourceUnknown {
		t.Fatalf("missing-provider AuxiliaryContextSource = %q, want %q", missingStatus.AuxiliaryContextSource, ModelContextSourceUnknown)
	}
	if missingStatus.AuxiliaryContextLookupError != "missing auxiliary provider" {
		t.Fatalf("missing-provider lookup error = %q, want evidence", missingStatus.AuxiliaryContextLookupError)
	}
}
