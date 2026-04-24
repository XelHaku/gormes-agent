package cli

import "testing"

// Tests freeze the Go port of hermes_cli/cli_output.py (Phase 5.O).
//
// Upstream exposes five colored line helpers (print_info / print_success /
// print_warning / print_error / print_header), a prompt() helper, and a
// prompt_yes_no() helper. The Go port splits the pure formatting/parsing
// surface (what this test file covers) from the interactive I/O path, so
// the helpers stay hermetically testable without a TTY.
//
// The exact upstream prefixes and color assignments:
//   print_info    → "  <text>"     with Colors.DIM
//   print_success → "✓ <text>"    with Colors.GREEN
//   print_warning → "⚠ <text>"    with Colors.YELLOW
//   print_error   → "✗ <text>"    with Colors.RED
//   print_header  → "\n  <text>"   with Colors.YELLOW
//   prompt()      → "  <question>[ [<default>]]: " with Colors.YELLOW, then strip → default fallback
//   prompt_yes_no → hint = "Y/n" when default=True else "y/N"; empty → default; else startswith("y")

func TestFormatInfo_ColorAndPlain(t *testing.T) {
	const text = "caching backend"
	if got, want := FormatInfo(text, false), "  caching backend"; got != want {
		t.Fatalf("FormatInfo(useColor=false) = %q, want %q", got, want)
	}
	if got, want := FormatInfo(text, true), ColorDim+"  caching backend"+ColorReset; got != want {
		t.Fatalf("FormatInfo(useColor=true) = %q, want %q", got, want)
	}
}

func TestFormatSuccess_ColorAndPlain(t *testing.T) {
	const text = "registered tool"
	if got, want := FormatSuccess(text, false), "✓ registered tool"; got != want {
		t.Fatalf("FormatSuccess(useColor=false) = %q, want %q", got, want)
	}
	if got, want := FormatSuccess(text, true), ColorGreen+"✓ registered tool"+ColorReset; got != want {
		t.Fatalf("FormatSuccess(useColor=true) = %q, want %q", got, want)
	}
}

func TestFormatWarning_ColorAndPlain(t *testing.T) {
	const text = "running in degraded mode"
	if got, want := FormatWarning(text, false), "⚠ running in degraded mode"; got != want {
		t.Fatalf("FormatWarning(useColor=false) = %q, want %q", got, want)
	}
	if got, want := FormatWarning(text, true), ColorYellow+"⚠ running in degraded mode"+ColorReset; got != want {
		t.Fatalf("FormatWarning(useColor=true) = %q, want %q", got, want)
	}
}

func TestFormatError_ColorAndPlain(t *testing.T) {
	const text = "refusing to overwrite config"
	if got, want := FormatError(text, false), "✗ refusing to overwrite config"; got != want {
		t.Fatalf("FormatError(useColor=false) = %q, want %q", got, want)
	}
	if got, want := FormatError(text, true), ColorRed+"✗ refusing to overwrite config"+ColorReset; got != want {
		t.Fatalf("FormatError(useColor=true) = %q, want %q", got, want)
	}
}

func TestFormatHeader_ColorAndPlain(t *testing.T) {
	const text = "Memory backend"
	if got, want := FormatHeader(text, false), "\n  Memory backend"; got != want {
		t.Fatalf("FormatHeader(useColor=false) = %q, want %q", got, want)
	}
	if got, want := FormatHeader(text, true), ColorYellow+"\n  Memory backend"+ColorReset; got != want {
		t.Fatalf("FormatHeader(useColor=true) = %q, want %q", got, want)
	}
}

func TestFormatPromptLabel_Matrix(t *testing.T) {
	cases := []struct {
		name         string
		question     string
		defaultValue string
		useColor     bool
		want         string
	}{
		{"plain no default", "OpenAI API key", "", false, "  OpenAI API key: "},
		{"plain with default", "Model", "gpt-5.1", false, "  Model [gpt-5.1]: "},
		{"colored no default", "OpenAI API key", "", true, ColorYellow + "  OpenAI API key: " + ColorReset},
		{"colored with default", "Model", "gpt-5.1", true, ColorYellow + "  Model [gpt-5.1]: " + ColorReset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatPromptLabel(tc.question, tc.defaultValue, tc.useColor)
			if got != tc.want {
				t.Fatalf("FormatPromptLabel(%q, %q, %v) = %q, want %q",
					tc.question, tc.defaultValue, tc.useColor, got, tc.want)
			}
		})
	}
}

func TestResolvePromptAnswer_Matrix(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		defaultValue string
		want         string
	}{
		{"empty input with default", "", "gpt-5.1", "gpt-5.1"},
		{"empty input without default", "", "", ""},
		{"whitespace collapses to default", "   \t \n", "gpt-5.1", "gpt-5.1"},
		{"whitespace without default returns empty", "   ", "", ""},
		{"non-empty wins over default", "claude-4.7", "gpt-5.1", "claude-4.7"},
		{"input is trimmed", "  claude-4.7  ", "gpt-5.1", "claude-4.7"},
		{"non-empty with empty default", "claude-4.7", "", "claude-4.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePromptAnswer(tc.raw, tc.defaultValue)
			if got != tc.want {
				t.Fatalf("ResolvePromptAnswer(%q, %q) = %q, want %q",
					tc.raw, tc.defaultValue, got, tc.want)
			}
		})
	}
}

func TestFormatYesNoHint(t *testing.T) {
	if got, want := FormatYesNoHint(true), "Y/n"; got != want {
		t.Fatalf("FormatYesNoHint(true) = %q, want %q", got, want)
	}
	if got, want := FormatYesNoHint(false), "y/N"; got != want {
		t.Fatalf("FormatYesNoHint(false) = %q, want %q", got, want)
	}
}

func TestParseYesNoAnswer_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		defaultYes bool
		want       bool
	}{
		{"empty defaults to Yes", "", true, true},
		{"empty defaults to No", "", false, false},
		{"whitespace only defaults to Yes", "   ", true, true},
		{"whitespace only defaults to No", "   ", false, false},
		{"lowercase y wins over default No", "y", false, true},
		{"uppercase Y wins over default No", "Y", false, true},
		{"yes wins over default No", "yes", false, true},
		{"YES wins over default No", "YES", false, true},
		{"no overrides default Yes", "n", true, false},
		{"N overrides default Yes", "N", true, false},
		{"no overrides default Yes (full word)", "no", true, false},
		{"garbage input is not yes", "maybe", true, false},
		{"leading whitespace is trimmed before y check", "  yes", false, true},
		{"leading whitespace is trimmed before n check", "  no", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseYesNoAnswer(tc.raw, tc.defaultYes)
			if got != tc.want {
				t.Fatalf("ParseYesNoAnswer(%q, %v) = %v, want %v",
					tc.raw, tc.defaultYes, got, tc.want)
			}
		})
	}
}
