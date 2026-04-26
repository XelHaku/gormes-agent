package cli

import (
	"strings"
	"testing"
)

func TestRenderDumpSummary_StableOrder(t *testing.T) {
	in := DumpInput{
		Version:     "0.1.0",
		OS:          "linux",
		Arch:        "amd64",
		ProfileName: "default",
		Toolsets:    []string{"core", "web"},
	}
	got := RenderDumpSummary(in)
	want := "version: 0.1.0\nos: linux\narch: amd64\nprofile: default\ntoolsets: core, web\n"
	if got != want {
		t.Fatalf("RenderDumpSummary stable order:\n got=%q\nwant=%q", got, want)
	}
}

func TestRenderDumpSummary_RedactsSecrets(t *testing.T) {
	in := DumpInput{
		Version:         "v1.0-sk-abcdef",
		OS:              "linux",
		Arch:            "amd64",
		ProfileName:     "default",
		Toolsets:        []string{"core"},
		SecretsLikeKeys: []string{"sk-abcdef"},
	}
	got := RenderDumpSummary(in)
	if strings.Contains(got, "sk-abcdef") {
		t.Fatalf("expected secret 'sk-abcdef' to be redacted, got: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("expected '[redacted]' marker in output, got: %q", got)
	}
	if !strings.Contains(got, "version: v1.0-[redacted]") {
		t.Fatalf("expected version line to keep prefix and redact secret, got: %q", got)
	}
}

func TestRenderDumpSummary_HandlesMissingFields(t *testing.T) {
	got := RenderDumpSummary(DumpInput{})
	want := "version: unknown\nos: unknown\narch: unknown\nprofile: unknown\ntoolsets: (none)\n"
	if got != want {
		t.Fatalf("RenderDumpSummary missing fields:\n got=%q\nwant=%q", got, want)
	}
}

func TestRenderDumpSummary_DeterministicAcrossCalls(t *testing.T) {
	in := DumpInput{
		Version:         "0.2.0",
		OS:              "darwin",
		Arch:            "arm64",
		ProfileName:     "alpha",
		Toolsets:        []string{"core", "web"},
		SecretsLikeKeys: []string{"sk-zzzz"},
	}
	first := RenderDumpSummary(in)
	second := RenderDumpSummary(in)
	if first != second {
		t.Fatalf("RenderDumpSummary not deterministic across calls:\n first=%q\nsecond=%q", first, second)
	}
}

func TestRenderDumpSummary_NoTrailingWhitespace(t *testing.T) {
	in := DumpInput{
		Version:     "0.1.0",
		OS:          "linux",
		Arch:        "amd64",
		ProfileName: "default",
		Toolsets:    []string{"core", "web"},
	}
	got := RenderDumpSummary(in)
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected output to end in single '\\n', got: %q", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected single trailing newline, got: %q", got)
	}
	body := strings.TrimSuffix(got, "\n")
	for i, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimRight(line, " \t"); trimmed != line {
			t.Fatalf("line %d has trailing whitespace: %q", i, line)
		}
	}
}
