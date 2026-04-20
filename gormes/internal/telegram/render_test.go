package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

func TestFormatStream_PlainDraft(t *testing.T) {
	f := kernel.RenderFrame{DraftText: "hello world", Phase: kernel.PhaseStreaming}
	got := formatStream(f)
	if !strings.Contains(got, "hello world") {
		t.Errorf("render = %q, want to contain 'hello world'", got)
	}
	if strings.Contains(got, "🔧") {
		t.Errorf("render = %q, no soul line expected", got)
	}
}

func TestFormatStream_SoulLineAppears(t *testing.T) {
	f := kernel.RenderFrame{
		DraftText:  "thinking...",
		Phase:      kernel.PhaseStreaming,
		SoulEvents: []kernel.SoulEntry{{At: time.Now(), Text: "tool: echo"}},
	}
	got := formatStream(f)
	if !strings.Contains(got, "🔧") {
		t.Errorf("render = %q, want tool soul line", got)
	}
	if !strings.Contains(got, "echo") {
		t.Errorf("render = %q, want 'echo' in soul line", got)
	}
}

func TestFormatStream_ReconnectingMarker(t *testing.T) {
	f := kernel.RenderFrame{DraftText: "xxxxx", Phase: kernel.PhaseReconnecting}
	got := formatStream(f)
	if !strings.Contains(got, "reconnecting") {
		t.Errorf("render = %q, want reconnecting marker", got)
	}
}

func TestFormatStream_EscapesMarkdown(t *testing.T) {
	f := kernel.RenderFrame{
		DraftText: "use *bold* and _italic_",
		Phase:     kernel.PhaseStreaming,
	}
	got := formatStream(f)
	if strings.Contains(got, "*bold*") {
		t.Errorf("render = %q, should escape '*' chars", got)
	}
	if !strings.Contains(got, `\*bold\*`) {
		t.Errorf("render = %q, expected escaped markdown", got)
	}
}

func TestFormatFinal_ReadsFromHistory(t *testing.T) {
	f := kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "ping"},
			{Role: "assistant", Content: "pong"},
		},
	}
	got := formatFinal(f)
	if !strings.Contains(got, "pong") {
		t.Errorf("render = %q, want 'pong'", got)
	}
}

func TestFormatError_PrefixAndEscape(t *testing.T) {
	f := kernel.RenderFrame{LastError: "bad thing (really)"}
	got := formatError(f)
	if !strings.Contains(got, "❌") {
		t.Errorf("render = %q, want ❌ prefix", got)
	}
	if !strings.Contains(got, `\(really\)`) {
		t.Errorf("render = %q, want escaped parens", got)
	}
}

func TestTruncateForTelegram_RespectsLimit(t *testing.T) {
	big := strings.Repeat("a", 5000)
	got := truncateForTelegram(big)
	if n := len([]rune(got)); n > maxTelegramText {
		t.Errorf("truncated len = %d, want <= %d", n, maxTelegramText)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated output should end with ellipsis")
	}
}
