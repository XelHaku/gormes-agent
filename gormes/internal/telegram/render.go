package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

// maxTelegramText is the character budget for a single message. Telegram's
// hard limit is 4096; we truncate at 4000 to leave headroom for UTF-8
// edge cases and the "…" suffix.
const maxTelegramText = 4000

// formatStream renders an in-flight RenderFrame as Telegram-wire text.
// Includes the assistant DraftText plus a trailing italic soul-event line
// when a tool is active. User content is MarkdownV2-escaped; trailing
// wrappers are literal markup added after escape.
func formatStream(f kernel.RenderFrame) string {
	body := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.DraftText)
	body = truncateForTelegram(body)

	tail := ""
	if len(f.SoulEvents) > 0 {
		last := f.SoulEvents[len(f.SoulEvents)-1]
		if last.Text != "" && last.Text != "idle" {
			tail = "\n\n_" + tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "🔧 "+last.Text) + "_"
		}
	}
	if f.Phase == kernel.PhaseReconnecting {
		tail += "\n\n_reconnecting…_"
	}
	return body + tail
}

// formatFinal renders the final assistant message (no soul line). Pulls
// from History since DraftText is cleared on PhaseIdle.
func formatFinal(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncateForTelegram(
				tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.History[i].Content))
		}
	}
	return "_\\(empty reply\\)_"
}

// formatError renders a PhaseFailed/Cancelling frame as "❌ " + LastError.
func formatError(f kernel.RenderFrame) string {
	text := "❌ " + f.LastError
	if f.LastError == "" {
		text = "❌ cancelled"
	}
	return truncateForTelegram(
		tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, text))
}

// truncateForTelegram clamps to maxTelegramText runes with "…" suffix.
func truncateForTelegram(s string) string {
	runes := []rune(s)
	if len(runes) <= maxTelegramText {
		return s
	}
	return string(runes[:maxTelegramText-1]) + "…"
}
