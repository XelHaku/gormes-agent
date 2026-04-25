package hermes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestContextCompressorHeadroomDocumentsRemovedLegacyToolSchemaReservation(t *testing.T) {
	tools := syntheticCompressionToolDescriptors(50, 200, 120)
	toolTokens := estimateToolDescriptorTokensForTest(t, tools)

	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:                  "test-main-model",
		ContextLength:          200_000,
		ThresholdPercent:       0.70,
		SummaryTargetRatio:     0.20,
		AuxiliaryContextLength: 128_000,
		ToolDescriptors:        tools,
	})

	status := budget.Status()
	legacyThreshold := 128_000 - toolTokens - legacyContextCompressorRequestFixedHeadroomTokens
	if legacyThreshold <= minimumContextCompressorContextLength {
		t.Fatalf("test fixture legacy threshold too small: legacyThreshold=%d floor=%d", legacyThreshold, minimumContextCompressorContextLength)
	}
	if status.ThresholdTokens != 128_000 {
		t.Fatalf("ThresholdTokens = %d, want auxiliary context 128000", status.ThresholdTokens)
	}
	if status.ThresholdTokens == legacyThreshold {
		t.Fatalf("ThresholdTokens = legacy headroom threshold %d; flush-memory request headroom was reintroduced", legacyThreshold)
	}
	if status.ThresholdTokens != status.AuxiliaryContextLength {
		t.Fatalf("ThresholdTokens = %d, want equal auxiliary context %d", status.ThresholdTokens, status.AuxiliaryContextLength)
	}
	if status.ToolSchemaTokens != toolTokens {
		t.Fatalf("ToolSchemaTokens = %d, want %d", status.ToolSchemaTokens, toolTokens)
	}
	assertCompressorStatusEvidence(t, status, map[string]any{
		"threshold_source": "single_prompt_aux",
	}, []string{
		"request_headroom_tokens",
		"headroom_clamped",
	})
}

func TestContextCompressorHeadroomNoLongerFloorsAfterLegacyHeadroomSubtraction(t *testing.T) {
	tools := syntheticCompressionToolDescriptors(30, 2_000, 0)
	toolTokens := estimateToolDescriptorTokensForTest(t, tools)
	if toolTokens+legacyContextCompressorRequestFixedHeadroomTokens <= 80_000-minimumContextCompressorContextLength {
		t.Fatalf("test fixture headroom too small: toolTokens=%d", toolTokens)
	}

	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:                  "test-main-model",
		ContextLength:          200_000,
		ThresholdPercent:       0.50,
		SummaryTargetRatio:     0.20,
		AuxiliaryContextLength: 80_000,
		ToolDescriptors:        tools,
	})

	status := budget.Status()
	if status.ThresholdTokens != 80_000 {
		t.Fatalf("ThresholdTokens = %d, want auxiliary context 80000", status.ThresholdTokens)
	}
	if status.ThresholdTokens == minimumContextCompressorContextLength {
		t.Fatalf("ThresholdTokens = minimum floor %d; legacy headroom floor behavior was reintroduced", minimumContextCompressorContextLength)
	}
	assertCompressorStatusEvidence(t, status, map[string]any{
		"threshold_source": "single_prompt_aux",
	}, []string{
		"request_headroom_tokens",
		"headroom_clamped",
	})
}

func syntheticCompressionToolDescriptors(count, descriptionBytes, argDescriptionBytes int) []ToolDescriptor {
	out := make([]ToolDescriptor, 0, count)
	for i := 0; i < count; i++ {
		schema := fmt.Sprintf(
			`{"type":"object","properties":{"arg":{"type":"string","description":%q}},"required":["arg"],"additionalProperties":false}`,
			strings.Repeat("y", argDescriptionBytes),
		)
		out = append(out, ToolDescriptor{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: strings.Repeat("x", descriptionBytes),
			Schema:      json.RawMessage(schema),
		})
	}
	return out
}

func estimateToolDescriptorTokensForTest(t *testing.T, tools []ToolDescriptor) int {
	t.Helper()
	payload, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal synthetic tools: %v", err)
	}
	return (len(payload) + 3) / 4
}
