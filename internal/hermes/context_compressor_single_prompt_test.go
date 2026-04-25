package hermes

import (
	"encoding/json"
	"testing"
)

const legacyContextCompressorRequestFixedHeadroomTokens = 12_000

func TestContextCompressorSinglePromptAuxiliaryThresholdUsesAuxContextDirectly(t *testing.T) {
	tools := syntheticCompressionToolDescriptors(50, 200, 120)
	legacyHeadroomThreshold := 128_000 - estimateToolDescriptorTokensForTest(t, tools) - legacyContextCompressorRequestFixedHeadroomTokens
	if legacyHeadroomThreshold <= minimumContextCompressorContextLength {
		t.Fatalf("test fixture legacy threshold too small: got %d", legacyHeadroomThreshold)
	}

	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:                  "test-main-model",
		ContextLength:          200_000,
		ThresholdPercent:       0.70,
		SummaryTargetRatio:     0.20,
		AuxiliaryContextLength: 128_000,
		ToolDescriptors:        tools,
	})

	status := budget.Status()
	if status.ThresholdTokens != 128_000 {
		t.Fatalf("ThresholdTokens = %d, want auxiliary context 128000", status.ThresholdTokens)
	}
	if status.ThresholdTokens == legacyHeadroomThreshold {
		t.Fatalf("ThresholdTokens = legacy headroom threshold %d; flush-memory request headroom was reintroduced", legacyHeadroomThreshold)
	}
	if status.TailTokenBudget != 25_600 {
		t.Fatalf("TailTokenBudget = %d, want threshold*summary_target_ratio = 25600", status.TailTokenBudget)
	}
	if status.MaxSummaryTokens != 10_000 {
		t.Fatalf("MaxSummaryTokens = %d, want min(context_length*0.05, 12000) = 10000", status.MaxSummaryTokens)
	}
	assertCompressorStatusEvidence(t, status, map[string]any{
		"threshold_source": "single_prompt_aux",
	}, []string{
		"request_headroom_tokens",
		"headroom_clamped",
	})
}

func TestContextCompressorSinglePromptProviderCapPreservesResolverEvidence(t *testing.T) {
	tools := syntheticCompressionToolDescriptors(8, 180, 90)
	resolver := NewModelContextResolver(ModelContextLookupFunc(func(query ModelContextQuery) (int, bool, error) {
		if query.Provider == "openai-codex" && query.Model == "gpt-5.5" {
			return 272_000, true, nil
		}
		return 0, false, nil
	}))

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

	status := got.Budget.Status()
	if status.ThresholdTokens != 272_000 {
		t.Fatalf("ThresholdTokens = %d, want provider-resolved auxiliary cap 272000", status.ThresholdTokens)
	}
	if status.AuxiliaryContextSource != ModelContextSourceProviderCap {
		t.Fatalf("AuxiliaryContextSource = %q, want %q", status.AuxiliaryContextSource, ModelContextSourceProviderCap)
	}
	assertCompressorStatusEvidence(t, status, map[string]any{
		"auxiliary_context_source": string(ModelContextSourceProviderCap),
		"threshold_source":         "provider_cap",
	}, []string{
		"request_headroom_tokens",
		"headroom_clamped",
	})
}

func TestContextCompressorSinglePromptStatusReportsUnavailableThresholdEvidence(t *testing.T) {
	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:                       "missing-aux",
		AuxiliaryContextLookupError: "provider metadata unavailable",
	})

	status := budget.Status()
	if status.State != "unavailable" {
		t.Fatalf("State = %q, want unavailable", status.State)
	}
	assertCompressorStatusEvidence(t, status, map[string]any{
		"threshold_source": "unavailable",
	}, []string{
		"request_headroom_tokens",
		"headroom_clamped",
	})
}

func assertCompressorStatusEvidence(t *testing.T, status ContextCompressorBudgetStatus, want map[string]any, absent []string) {
	t.Helper()

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("unmarshal status JSON %s: %v", payload, err)
	}
	for key, wantValue := range want {
		got, ok := fields[key]
		if !ok {
			t.Fatalf("status JSON missing %q in %s", key, payload)
		}
		if got != wantValue {
			t.Fatalf("status JSON %q = %#v, want %#v in %s", key, got, wantValue, payload)
		}
	}
	for _, key := range absent {
		if got, ok := fields[key]; ok {
			t.Fatalf("status JSON reported legacy %q = %#v in %s", key, got, payload)
		}
	}
}
