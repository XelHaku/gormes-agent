package gateway

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

const maxMessageLen = 4000

// FormatStreamPlain renders a streaming frame as plain text.
func FormatStreamPlain(f kernel.RenderFrame) string {
	body := f.DraftText
	tail := ""
	if len(f.SoulEvents) > 0 {
		last := f.SoulEvents[len(f.SoulEvents)-1]
		if last.Text != "" && last.Text != "idle" {
			tail = "\n\n🔧 " + last.Text
		}
	}
	if f.Phase == kernel.PhaseReconnecting {
		tail += "\n\nreconnecting…"
	}
	return truncate(body + tail)
}

// FormatFinalPlain returns the final assistant text from render history.
func FormatFinalPlain(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncate(f.History[i].Content)
		}
	}
	return "(empty reply)"
}

// FormatErrorPlain renders a terminal error frame.
func FormatErrorPlain(f kernel.RenderFrame) string {
	text := "❌ " + f.LastError
	if f.LastError == "" {
		text = "❌ cancelled"
	}
	return truncate(text)
}

// FormatStreamTelegram renders a streaming frame using Telegram MarkdownV2.
func FormatStreamTelegram(f kernel.RenderFrame) string {
	body := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.DraftText)
	body = truncate(body)
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

// FormatFinalTelegram renders the final assistant message for Telegram.
func FormatFinalTelegram(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncate(tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.History[i].Content))
		}
	}
	return "_\\(empty reply\\)_"
}

// FormatErrorTelegram renders an error frame for Telegram MarkdownV2.
func FormatErrorTelegram(f kernel.RenderFrame) string {
	text := "❌ " + f.LastError
	if f.LastError == "" {
		text = "❌ cancelled"
	}
	return truncate(tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, text))
}

func truncate(s string) string {
	runes := []rune(s)
	if len(runes) <= maxMessageLen {
		return s
	}
	return string(runes[:maxMessageLen-1]) + "…"
}
