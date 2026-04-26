package tuigateway

import "testing"

func seconds(v float64) *float64 {
	return &v
}

// TestFormatToolSummary_WebSearchPlural mirrors upstream
// hermes-agent/tui_gateway/server.py:_tool_summary's data.web count for
// web_search tool results.
func TestFormatToolSummary_WebSearchPlural(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("web_search", []byte(`{"data":{"web":[{},{},{}]}}`), seconds(1.2))
	const want = "Did 3 searches in 1.2s"
	if !ok {
		t.Fatalf("FormatToolSummary(web_search) ok = false; want true")
	}
	if got != want {
		t.Errorf("FormatToolSummary(web_search) = %q; want %q", got, want)
	}
}

// TestFormatToolSummary_WebSearchSingular proves the singular search label
// when upstream would count exactly one web result.
func TestFormatToolSummary_WebSearchSingular(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("web_search", []byte(`{"data":{"web":[{}]}}`), nil)
	const want = "Did 1 search"
	if !ok {
		t.Fatalf("FormatToolSummary(web_search) ok = false; want true")
	}
	if got != want {
		t.Errorf("FormatToolSummary(web_search) = %q; want %q", got, want)
	}
}

// TestFormatToolSummary_WebExtractPlural mirrors upstream's top-level
// results count for web_extract tool results.
func TestFormatToolSummary_WebExtractPlural(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("web_extract", []byte(`{"results":[{},{}]}`), seconds(0.5))
	const want = "Extracted 2 pages in 0.5s"
	if !ok {
		t.Fatalf("FormatToolSummary(web_extract) ok = false; want true")
	}
	if got != want {
		t.Errorf("FormatToolSummary(web_extract) = %q; want %q", got, want)
	}
}

// TestFormatToolSummary_UnknownToolWithDuration covers the upstream fallback:
// a tool with no specialised summary still emits Completed when duration is
// present.
func TestFormatToolSummary_UnknownToolWithDuration(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("custom_tool", []byte(`{"anything":true}`), seconds(0.3))
	const want = "Completed in 0.3s"
	if !ok {
		t.Fatalf("FormatToolSummary(custom_tool) ok = false; want true")
	}
	if got != want {
		t.Errorf("FormatToolSummary(custom_tool) = %q; want %q", got, want)
	}
}

// TestFormatToolSummary_UnknownToolNoDuration confirms the pure helper leaves
// emission to callers when there is neither a recognised tool shape nor a
// duration to report.
func TestFormatToolSummary_UnknownToolNoDuration(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("custom_tool", []byte(`{"anything":true}`), nil)
	if ok {
		t.Fatalf("FormatToolSummary(custom_tool) ok = true; want false")
	}
	if got != "" {
		t.Errorf("FormatToolSummary(custom_tool) = %q; want empty string", got)
	}
}

// TestFormatToolSummary_InvalidJSONFallsBackToCompleted treats malformed
// result JSON as no data, matching upstream's json.loads exception path.
func TestFormatToolSummary_InvalidJSONFallsBackToCompleted(t *testing.T) {
	t.Parallel()

	got, ok := FormatToolSummary("web_search", []byte(`{"data":`), seconds(0.7))
	const want = "Completed in 0.7s"
	if !ok {
		t.Fatalf("FormatToolSummary(web_search invalid JSON) ok = false; want true")
	}
	if got != want {
		t.Errorf("FormatToolSummary(web_search invalid JSON) = %q; want %q", got, want)
	}
}
