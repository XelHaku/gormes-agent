package whatsapp

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestNormalizeInbound_DirectChatFallsBackToSenderAndParsesCommand(t *testing.T) {
	ev, ok := NormalizeInbound(InboundMessage{
		UserID:    "15551234567@s.whatsapp.net",
		UserName:  "Alice",
		MessageID: "wamid-1",
		Text:      " /help ",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if ev.Platform != "whatsapp" {
		t.Fatalf("Platform = %q, want whatsapp", ev.Platform)
	}
	if ev.ChatID != "15551234567" {
		t.Fatalf("ChatID = %q, want direct chat fallback from sender", ev.ChatID)
	}
	if ev.UserID != "15551234567" {
		t.Fatalf("UserID = %q, want normalized sender ID", ev.UserID)
	}
	if ev.UserName != "Alice" {
		t.Fatalf("UserName = %q, want Alice", ev.UserName)
	}
	if ev.MsgID != "wamid-1" {
		t.Fatalf("MsgID = %q, want wamid-1", ev.MsgID)
	}
	if ev.Kind != gateway.EventStart {
		t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventStart)
	}
	if ev.Text != "" {
		t.Fatalf("Text = %q, want empty command body", ev.Text)
	}
}

func TestNormalizeInbound_GroupMentionPrefixStrippedBeforeCommandParsing(t *testing.T) {
	ev, ok := NormalizeInbound(InboundMessage{
		ChatID:    "120363025000000000@g.us",
		ChatKind:  ChatKindGroup,
		UserID:    "15557654321@s.whatsapp.net",
		UserName:  "Bob",
		MessageID: "wamid-2",
		Text:      "@Hermes /new",
		Mentioned: true,
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if ev.ChatID != "120363025000000000" {
		t.Fatalf("ChatID = %q, want normalized group ID", ev.ChatID)
	}
	if ev.UserID != "15557654321" {
		t.Fatalf("UserID = %q, want normalized sender ID", ev.UserID)
	}
	if ev.Kind != gateway.EventReset {
		t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventReset)
	}
	if ev.Text != "" {
		t.Fatalf("Text = %q, want empty command body", ev.Text)
	}
}

func TestNormalizeInbound_UnknownSlashCommandStillPassesThroughSharedParser(t *testing.T) {
	ev, ok := NormalizeInbound(InboundMessage{
		ChatID:   "15551230000@c.us",
		UserID:   "15551230000@c.us",
		ChatKind: ChatKindDirect,
		Text:     "/wat",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if ev.Kind != gateway.EventUnknown {
		t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventUnknown)
	}
	if ev.Text != "" {
		t.Fatalf("Text = %q, want empty unknown-command body", ev.Text)
	}
}

func TestNormalizeInbound_RejectsMissingSenderAndEmptyText(t *testing.T) {
	if _, ok := NormalizeInbound(InboundMessage{
		ChatID: "15551230000",
		Text:   "hello",
	}); ok {
		t.Fatal("NormalizeInbound() ok = true for missing sender, want false")
	}

	if _, ok := NormalizeInbound(InboundMessage{
		UserID: "15551230000",
		Text:   "   ",
	}); ok {
		t.Fatal("NormalizeInbound() ok = true for empty text, want false")
	}
}
