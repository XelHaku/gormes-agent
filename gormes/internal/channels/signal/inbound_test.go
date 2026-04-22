package signal

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestNormalizeInbound_DirectMessagePreservesPrimaryAndAlternateIdentity(t *testing.T) {
	got, ok := NormalizeInbound(InboundMessage{
		ChatType:   ChatTypeDirect,
		SenderID:   "+15551234567",
		SenderUUID: "uuid-alice",
		SenderName: "Alice",
		MessageID:  "msg-1",
		Text:       " /help ",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if got.Event.Platform != "signal" {
		t.Fatalf("Event.Platform = %q, want signal", got.Event.Platform)
	}
	if got.Event.ChatID != "+15551234567" {
		t.Fatalf("Event.ChatID = %q, want +15551234567", got.Event.ChatID)
	}
	if got.Event.UserID != "+15551234567" {
		t.Fatalf("Event.UserID = %q, want +15551234567", got.Event.UserID)
	}
	if got.Event.UserName != "Alice" {
		t.Fatalf("Event.UserName = %q, want Alice", got.Event.UserName)
	}
	if got.Event.MsgID != "msg-1" {
		t.Fatalf("Event.MsgID = %q, want msg-1", got.Event.MsgID)
	}
	if got.Event.Kind != gateway.EventStart {
		t.Fatalf("Event.Kind = %v, want %v", got.Event.Kind, gateway.EventStart)
	}
	if got.Event.Text != "" {
		t.Fatalf("Event.Text = %q, want empty command body", got.Event.Text)
	}
	if got.Identity.ChatType != ChatTypeDirect {
		t.Fatalf("Identity.ChatType = %q, want direct", got.Identity.ChatType)
	}
	if got.Identity.ChatID != "+15551234567" {
		t.Fatalf("Identity.ChatID = %q, want +15551234567", got.Identity.ChatID)
	}
	if got.Identity.UserID != "+15551234567" {
		t.Fatalf("Identity.UserID = %q, want +15551234567", got.Identity.UserID)
	}
	if got.Identity.UserIDAlt != "uuid-alice" {
		t.Fatalf("Identity.UserIDAlt = %q, want uuid-alice", got.Identity.UserIDAlt)
	}
	if got.Identity.ChatIDAlt != "" {
		t.Fatalf("Identity.ChatIDAlt = %q, want empty", got.Identity.ChatIDAlt)
	}
}

func TestNormalizeInbound_GroupMessageUsesGroupSessionIdentity(t *testing.T) {
	got, ok := NormalizeInbound(InboundMessage{
		ChatType:   ChatTypeGroup,
		GroupID:    "grp-123==",
		GroupName:  "Ops",
		SenderID:   "+15557654321",
		SenderUUID: "uuid-bob",
		SenderName: "Bob",
		MessageID:  "msg-2",
		Text:       "/new",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if got.Event.ChatID != "group:grp-123==" {
		t.Fatalf("Event.ChatID = %q, want group:grp-123==", got.Event.ChatID)
	}
	if got.Event.ChatName != "Ops" {
		t.Fatalf("Event.ChatName = %q, want Ops", got.Event.ChatName)
	}
	if got.Event.UserID != "+15557654321" {
		t.Fatalf("Event.UserID = %q, want +15557654321", got.Event.UserID)
	}
	if got.Event.Kind != gateway.EventReset {
		t.Fatalf("Event.Kind = %v, want %v", got.Event.Kind, gateway.EventReset)
	}
	if got.Identity.ChatType != ChatTypeGroup {
		t.Fatalf("Identity.ChatType = %q, want group", got.Identity.ChatType)
	}
	if got.Identity.ChatID != "group:grp-123==" {
		t.Fatalf("Identity.ChatID = %q, want group:grp-123==", got.Identity.ChatID)
	}
	if got.Identity.ChatIDAlt != "grp-123==" {
		t.Fatalf("Identity.ChatIDAlt = %q, want grp-123==", got.Identity.ChatIDAlt)
	}
	if got.Identity.UserIDAlt != "uuid-bob" {
		t.Fatalf("Identity.UserIDAlt = %q, want uuid-bob", got.Identity.UserIDAlt)
	}
}

func TestNormalizeInbound_UsesUUIDWhenPhoneNumberUnavailable(t *testing.T) {
	got, ok := NormalizeInbound(InboundMessage{
		ChatType:   ChatTypeDirect,
		SenderUUID: "uuid-only",
		SenderName: "UUID User",
		MessageID:  "msg-3",
		Text:       "hello",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if got.Event.ChatID != "uuid-only" {
		t.Fatalf("Event.ChatID = %q, want uuid-only", got.Event.ChatID)
	}
	if got.Event.UserID != "uuid-only" {
		t.Fatalf("Event.UserID = %q, want uuid-only", got.Event.UserID)
	}
	if got.Event.Kind != gateway.EventSubmit {
		t.Fatalf("Event.Kind = %v, want %v", got.Event.Kind, gateway.EventSubmit)
	}
	if got.Event.Text != "hello" {
		t.Fatalf("Event.Text = %q, want hello", got.Event.Text)
	}
	if got.Identity.UserIDAlt != "" {
		t.Fatalf("Identity.UserIDAlt = %q, want empty", got.Identity.UserIDAlt)
	}
}

func TestNormalizeInbound_RejectsMissingIdentityOrGroupID(t *testing.T) {
	if _, ok := NormalizeInbound(InboundMessage{
		ChatType:  ChatTypeDirect,
		MessageID: "msg-4",
		Text:      "hello",
	}); ok {
		t.Fatal("NormalizeInbound() ok = true for missing sender, want false")
	}

	if _, ok := NormalizeInbound(InboundMessage{
		ChatType:  ChatTypeGroup,
		SenderID:  "+15550001111",
		MessageID: "msg-5",
		Text:      "hello",
	}); ok {
		t.Fatal("NormalizeInbound() ok = true for missing group ID, want false")
	}

	if _, ok := NormalizeInbound(InboundMessage{
		ChatType: ChatTypeDirect,
		SenderID: "+15550001111",
		Text:     "   ",
	}); ok {
		t.Fatal("NormalizeInbound() ok = true for empty text, want false")
	}
}
