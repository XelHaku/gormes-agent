package cron

import (
	"strings"
	"testing"
)

func TestCronHeartbeatPrefixConstantValue(t *testing.T) {
	want := "[IMPORTANT: You are running as a scheduled cron job. " +
		"DELIVERY: Your final response will be automatically delivered " +
		"to the user — do NOT use send_message or try to deliver " +
		"the output yourself. Just produce your report/output as your " +
		"final response and the system handles the rest. " +
		"SILENT: If there is genuinely nothing new to report, respond " +
		"with exactly \"[SILENT]\" (nothing else) to suppress delivery. " +
		"Never combine [SILENT] with content — either report your " +
		"findings normally, or say [SILENT] and nothing more.]\n\n"

	if CronHeartbeatPrefix != want {
		t.Fatalf("CronHeartbeatPrefix = %q, want %q", CronHeartbeatPrefix, want)
	}
}

func TestCronHeartbeatBuildPromptUsesImportantPrefix(t *testing.T) {
	full := BuildPrompt("Give me a status summary")
	if !strings.HasPrefix(full, "[IMPORTANT:") {
		t.Errorf("BuildPrompt does not start with [IMPORTANT: — got %q", full[:40])
	}
	if !strings.Contains(full, "scheduled cron job") {
		t.Errorf("BuildPrompt missing cron job body — got %q", full[:80])
	}
	if !strings.HasSuffix(full, "Give me a status summary") {
		t.Errorf("BuildPrompt does not end with user prompt — got %q", full[len(full)-40:])
	}
}

func TestDetectSilent_ExactMatchOnly(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"[SILENT]", true},
		{"  [SILENT]", true},
		{"[SILENT]\n", true},
		{"\n\t [SILENT] \t\n", true},
		{"[SILENT] followed by text", false},
		{"Status: [SILENT] means nothing to report", false},
		{"<silent>", false},
		{"silent", false},
		{"SILENT", false},
		{"[silent]", false},
		{"[SILENT][SILENT]", false},
		{"", false},
	}
	for _, c := range cases {
		got := DetectSilent(c.in)
		if got != c.want {
			t.Errorf("DetectSilent(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
