package cron

import (
	"strings"
	"testing"
)

func TestHeartbeatPrefix_ContainsLoadBearingPhrases(t *testing.T) {
	p := CronHeartbeatPrefix
	for _, want := range []string{
		"[SYSTEM:",
		"scheduled cron job",
		"DELIVERY:",
		"automatically delivered",
		"do NOT use send_message",
		"SILENT:",
		"\"[SILENT]\"",
		"nothing more",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("CronHeartbeatPrefix missing %q", want)
		}
	}
}

func TestBuildPrompt_PrependsPrefix(t *testing.T) {
	full := BuildPrompt("Give me a status summary")
	if !strings.HasPrefix(full, "[SYSTEM:") {
		t.Errorf("BuildPrompt does not start with [SYSTEM: — got %q", full[:40])
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
