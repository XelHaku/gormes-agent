package hermes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestContextCompressorHeadroomAutoLoweredThresholdReservesHeadroomForToolsAndSystem(t *testing.T) {
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
	wantThreshold := 128_000 - toolTokens - 12_000
	if wantThreshold <= minimumContextCompressorContextLength {
		t.Fatalf("test fixture headroom too large: wantThreshold=%d floor=%d", wantThreshold, minimumContextCompressorContextLength)
	}
	if status.ThresholdTokens != wantThreshold {
		t.Fatalf("ThresholdTokens = %d, want aux context minus tool/system headroom = %d", status.ThresholdTokens, wantThreshold)
	}
	if status.ThresholdTokens >= status.AuxiliaryContextLength {
		t.Fatalf("ThresholdTokens = %d, want strictly below aux context %d", status.ThresholdTokens, status.AuxiliaryContextLength)
	}
	if status.ThresholdTokens <= minimumContextCompressorContextLength {
		t.Fatalf("ThresholdTokens = %d, want above minimum floor %d", status.ThresholdTokens, minimumContextCompressorContextLength)
	}
	if status.ToolSchemaTokens != toolTokens {
		t.Fatalf("ToolSchemaTokens = %d, want %d", status.ToolSchemaTokens, toolTokens)
	}
	if status.RequestHeadroomTokens != toolTokens+12_000 {
		t.Fatalf("RequestHeadroomTokens = %d, want tool overhead plus 12000 = %d", status.RequestHeadroomTokens, toolTokens+12_000)
	}
	if !status.HeadroomClamped {
		t.Fatalf("HeadroomClamped = false, want evidence that auxiliary threshold was lowered for request headroom")
	}
}

func TestContextCompressorHeadroomFloorsAtMinimumContext(t *testing.T) {
	tools := syntheticCompressionToolDescriptors(30, 2_000, 0)
	toolTokens := estimateToolDescriptorTokensForTest(t, tools)
	if toolTokens+12_000 <= 80_000-minimumContextCompressorContextLength {
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
	if status.ThresholdTokens != minimumContextCompressorContextLength {
		t.Fatalf("ThresholdTokens = %d, want minimum floor %d", status.ThresholdTokens, minimumContextCompressorContextLength)
	}
	if !status.HeadroomClamped {
		t.Fatalf("HeadroomClamped = false, want evidence when request headroom clamps threshold to the floor")
	}
	if status.RequestHeadroomTokens != toolTokens+12_000 {
		t.Fatalf("RequestHeadroomTokens = %d, want %d", status.RequestHeadroomTokens, toolTokens+12_000)
	}
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
