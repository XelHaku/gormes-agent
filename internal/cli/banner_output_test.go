package cli

import (
	"strings"
	"testing"
)

func TestContextLengthFormattingMatchesUpstreamGolden(t *testing.T) {
	tests := []struct {
		tokens int
		want   string
	}{
		{tokens: 999, want: "999"},
		{tokens: 1000, want: "1K"},
		{tokens: 1049, want: "1K"},
		{tokens: 1050, want: "1.1K"},
		{tokens: 128000, want: "128K"},
		{tokens: 1048576, want: "1M"},
		{tokens: 1099999, want: "1.1M"},
		{tokens: 1999000, want: "2M"},
	}

	for _, tt := range tests {
		if got := FormatContextLength(tt.tokens); got != tt.want {
			t.Fatalf("FormatContextLength(%d) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestToolsetNameDisplayMatchesUpstreamGolden(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "", want: "unknown"},
		{name: "homeassistant_tools", want: "homeassistant"},
		{name: "honcho_tools", want: "honcho"},
		{name: "web_tools", want: "web"},
		{name: "browser", want: "browser"},
		{name: "file", want: "file"},
		{name: "terminal", want: "terminal"},
	}

	for _, tt := range tests {
		if got := DisplayToolsetName(tt.name); got != tt.want {
			t.Fatalf("DisplayToolsetName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBannerVersionLabelsMatchUpstreamGolden(t *testing.T) {
	base := BannerVersion{
		AgentName:   "Hermes Agent",
		Version:     "0.11.0",
		ReleaseDate: "2026.4.23",
	}
	tests := []struct {
		name  string
		state *BannerGitState
		want  string
	}{
		{
			name: "without git state",
			want: "Hermes Agent v0.11.0 (2026.4.23)",
		},
		{
			name:  "on upstream main",
			state: &BannerGitState{Upstream: "b2f477a3", Local: "b2f477a3"},
			want:  "Hermes Agent v0.11.0 (2026.4.23) · upstream b2f477a3",
		},
		{
			name:  "one carried commit",
			state: &BannerGitState{Upstream: "b2f477a3", Local: "af8aad31", Ahead: 1},
			want:  "Hermes Agent v0.11.0 (2026.4.23) · upstream b2f477a3 · local af8aad31 (+1 carried commit)",
		},
		{
			name:  "multiple carried commits",
			state: &BannerGitState{Upstream: "b2f477a3", Local: "af8aad31", Ahead: 3},
			want:  "Hermes Agent v0.11.0 (2026.4.23) · upstream b2f477a3 · local af8aad31 (+3 carried commits)",
		},
	}

	for _, tt := range tests {
		version := base
		version.GitState = tt.state
		if got := FormatBannerVersionLabel(version); got != tt.want {
			t.Fatalf("%s: FormatBannerVersionLabel() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOutputLinesMatchUpstreamPlainGolden(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "info", got: FormatInfo("Checking config"), want: "  Checking config\n"},
		{name: "success", got: FormatSuccess("Saved"), want: "✓ Saved\n"},
		{name: "warning", got: FormatWarning("Missing optional token"), want: "⚠ Missing optional token\n"},
		{name: "error", got: FormatError("Failed"), want: "✗ Failed\n"},
		{name: "header", got: FormatHeader("Gateway Setup"), want: "\n  Gateway Setup\n"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s output = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestOutputWritersUseDeterministicNewlines(t *testing.T) {
	var out strings.Builder
	for _, write := range []func(*strings.Builder) error{
		func(w *strings.Builder) error { return WriteInfo(w, "First") },
		func(w *strings.Builder) error { return WriteSuccess(w, "Second") },
		func(w *strings.Builder) error { return WriteWarning(w, "Third") },
		func(w *strings.Builder) error { return WriteError(w, "Fourth") },
		func(w *strings.Builder) error { return WriteHeader(w, "Section") },
	} {
		if err := write(&out); err != nil {
			t.Fatalf("write output: %v", err)
		}
	}

	want := "  First\n✓ Second\n⚠ Third\n✗ Fourth\n\n  Section\n"
	if got := out.String(); got != want {
		t.Fatalf("writer output = %q, want %q", got, want)
	}
}

func TestOutputPromptFormattingAndDefaultsMatchUpstream(t *testing.T) {
	if got, want := FormatPrompt("Provider", "openrouter"), "  Provider [openrouter]: "; got != want {
		t.Fatalf("FormatPrompt with default = %q, want %q", got, want)
	}
	if got, want := FormatPrompt("API key", ""), "  API key: "; got != want {
		t.Fatalf("FormatPrompt without default = %q, want %q", got, want)
	}
	if got, want := ResolvePromptInput("  custom \n", "fallback"), "custom"; got != want {
		t.Fatalf("ResolvePromptInput custom = %q, want %q", got, want)
	}
	if got, want := ResolvePromptInput(" \t", "fallback"), "fallback"; got != want {
		t.Fatalf("ResolvePromptInput default = %q, want %q", got, want)
	}
	if got, want := ResolvePromptInput("", ""), ""; got != want {
		t.Fatalf("ResolvePromptInput empty = %q, want %q", got, want)
	}
}

func TestOutputYesNoPromptBehaviorMatchesUpstream(t *testing.T) {
	if got, want := FormatYesNoPrompt("Continue anyway?", true), "  Continue anyway? (Y/n): "; got != want {
		t.Fatalf("FormatYesNoPrompt true = %q, want %q", got, want)
	}
	if got, want := FormatYesNoPrompt("Remove service?", false), "  Remove service? (y/N): "; got != want {
		t.Fatalf("FormatYesNoPrompt false = %q, want %q", got, want)
	}
	if !ResolveYesNoAnswer("", true) {
		t.Fatal("ResolveYesNoAnswer empty with true default = false, want true")
	}
	if ResolveYesNoAnswer("", false) {
		t.Fatal("ResolveYesNoAnswer empty with false default = true, want false")
	}
	if !ResolveYesNoAnswer(" yes ", false) {
		t.Fatal("ResolveYesNoAnswer yes = false, want true")
	}
	if ResolveYesNoAnswer("no", true) {
		t.Fatal("ResolveYesNoAnswer no = true, want false")
	}
}
