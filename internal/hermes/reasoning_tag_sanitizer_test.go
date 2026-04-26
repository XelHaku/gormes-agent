package hermes

import "testing"

func TestSanitizeReasoningTags_StoredAssistantText(t *testing.T) {
	raw := "The answer starts here.\n\n<think>\nprivate chain of thought\n</think>\n\nThe answer ends here."

	got := SanitizeReasoningTags(raw)
	want := "The answer starts here.\nThe answer ends here."
	if got != want {
		t.Fatalf("SanitizeReasoningTags() = %q, want %q", got, want)
	}
}

func TestSanitizeReasoningTags_VisibleTextDropsUnterminatedThink(t *testing.T) {
	raw := "Visible answer before truncation.\n<think>\nprivate reasoning that never closes"

	got := SanitizeReasoningTags(raw)
	want := "Visible answer before truncation."
	if got != want {
		t.Fatalf("SanitizeReasoningTags() = %q, want %q", got, want)
	}
}

func TestSanitizeReasoningTags_ResumeRecapVariants(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "complete think",
			raw:  "<think>hidden</think>Visible recap.",
			want: "Visible recap.",
		},
		{
			name: "unterminated think",
			raw:  "Visible recap.\n<think>hidden",
			want: "Visible recap.",
		},
		{
			name: "complete thinking",
			raw:  "<thinking>hidden</thinking>Visible recap.",
			want: "Visible recap.",
		},
		{
			name: "unterminated thinking",
			raw:  "Visible recap.\n<thinking>hidden",
			want: "Visible recap.",
		},
		{
			name: "complete reasoning",
			raw:  "<reasoning>hidden</reasoning>Visible recap.",
			want: "Visible recap.",
		},
		{
			name: "unterminated reasoning",
			raw:  "Visible recap.\n<reasoning>hidden",
			want: "Visible recap.",
		},
		{
			name: "complete thought",
			raw:  "<thought>hidden</thought>Visible recap.",
			want: "Visible recap.",
		},
		{
			name: "unterminated thought",
			raw:  "Visible recap.\n<thought>hidden",
			want: "Visible recap.",
		},
		{
			name: "complete reasoning scratchpad",
			raw:  "<REASONING_SCRATCHPAD>hidden</REASONING_SCRATCHPAD>Visible recap.",
			want: "Visible recap.",
		},
		{
			name: "unterminated reasoning scratchpad",
			raw:  "Visible recap.\n<REASONING_SCRATCHPAD>hidden",
			want: "Visible recap.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeReasoningTags(tt.raw)
			if got != tt.want {
				t.Fatalf("SanitizeReasoningTags() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeReasoningTags_RawAuditTextUnchanged(t *testing.T) {
	raw := "Stored raw stream bytes.\n<think>audit-only reasoning</think>\nVisible answer."
	original := raw

	got := SanitizeReasoningTags(raw)
	want := "Stored raw stream bytes.\nVisible answer."
	if got != want {
		t.Fatalf("SanitizeReasoningTags() = %q, want %q", got, want)
	}
	if raw != original {
		t.Fatalf("raw audit text mutated: got %q, want original %q", raw, original)
	}
	if raw == got {
		t.Fatalf("SanitizeReasoningTags() returned raw text; want sanitized copy distinct from audit text")
	}
}
