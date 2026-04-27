package hermes

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompressionContentLength_PlainString(t *testing.T) {
	content := strings.Repeat("界", 500)

	if got := compressionContentLength(content); got != 500 {
		t.Fatalf("compressionContentLength(500-rune string) = %d, want 500", got)
	}

	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:              "model-a",
		ContextLength:      200_000,
		ThresholdPercent:   0.50,
		SummaryTargetRatio: 0.20,
	})
	status := budget.Status()
	if status.TailTokenBudget != 20_000 {
		t.Fatalf("TailTokenBudget = %d, want 20000", status.TailTokenBudget)
	}
}

func TestCompressionContentLength_MultimodalTextAndImage(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,abc"}},
		map[string]any{"type": "text", "text": "world"},
	}

	if got := compressionContentLength(content); got != 10 {
		t.Fatalf("compressionContentLength(multimodal content) = %d, want text chars only 10", got)
	}
}

func TestCompressionContentLength_BareStringsInList(t *testing.T) {
	content := []any{"hello", map[string]any{"text": "extra"}}

	if got := compressionContentLength(content); got != 10 {
		t.Fatalf("compressionContentLength(bare-string list content) = %d, want 10", got)
	}
}

func TestCompressionContentLength_UnknownListItemFallback(t *testing.T) {
	first := struct {
		Name string
	}{Name: "fallback"}
	second := 42
	content := []any{
		first,
		second,
	}
	want := utf8.RuneCountInString(fmt.Sprint(first)) + utf8.RuneCountInString(fmt.Sprint(second))

	if got := compressionContentLength(content); got != want {
		t.Fatalf("compressionContentLength(unknown list items) = %d, want fmt.Sprint length %d", got, want)
	}
}

func TestCompressionContentLength_ImageOnlyBlockZeroText(t *testing.T) {
	content := []any{
		map[string]any{"type": "image_url", "image_url": "https://example.test/image.png"},
		map[string]any{"type": "input_image", "image_url": map[string]any{"url": "https://example.test/image-2.png"}},
	}

	if got := compressionContentLength(content); got != 0 {
		t.Fatalf("compressionContentLength(image-only blocks) = %d, want 0 text chars", got)
	}
}
