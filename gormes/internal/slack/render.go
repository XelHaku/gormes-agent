package slack

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

const maxSlackText = 40000

func formatPending() string {
	return "⏳"
}

func formatStream(f kernel.RenderFrame) string {
	parts := make([]string, 0, 3)
	if text := strings.TrimSpace(f.DraftText); text != "" {
		parts = append(parts, text)
	}
	if len(f.SoulEvents) > 0 {
		last := f.SoulEvents[len(f.SoulEvents)-1]
		if text := strings.TrimSpace(last.Text); text != "" && text != "idle" {
			parts = append(parts, "🔧 "+text)
		}
	}
	if f.Phase == kernel.PhaseReconnecting {
		parts = append(parts, "reconnecting...")
	}
	if len(parts) == 0 {
		return formatPending()
	}
	return truncateSlack(strings.Join(parts, "\n\n"))
}

func formatFinal(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncateSlack(f.History[i].Content)
		}
	}
	return "(empty reply)"
}

func formatError(f kernel.RenderFrame) string {
	text := strings.TrimSpace(f.LastError)
	if text == "" {
		text = "cancelled"
	}
	return truncateSlack("❌ " + text)
}

func truncateSlack(s string) string {
	runes := []rune(s)
	if len(runes) <= maxSlackText {
		return s
	}
	return string(runes[:maxSlackText-1]) + "…"
}
