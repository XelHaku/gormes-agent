package whatsapp

import "testing"

func TestNormalizeInboundWithIdentity_UnsafeRawSenderRejected(t *testing.T) {
	got := NormalizeInboundWithIdentity(safeInboundMessage(func(msg InboundMessage) InboundMessage {
		msg.UserID = "15551234567/../../secret"
		return msg
	}), IdentityContext{})

	assertUnsafeInboundEvidence(t, got)
	if got.Event.UserID != "" {
		t.Fatalf("Event.UserID = %q, want empty", got.Event.UserID)
	}
	if got.Identity.UserID != "" {
		t.Fatalf("Identity.UserID = %q, want empty", got.Identity.UserID)
	}
	if got.Reply.ChatID != "" {
		t.Fatalf("Reply.ChatID = %q, want empty", got.Reply.ChatID)
	}
}

func TestNormalizeInboundWithIdentity_UnsafeChatRejected(t *testing.T) {
	got := NormalizeInboundWithIdentity(safeInboundMessage(func(msg InboundMessage) InboundMessage {
		msg.ChatID = "../15551234567"
		return msg
	}), IdentityContext{})

	assertUnsafeInboundEvidence(t, got)
	if got.Event.ChatID != "" {
		t.Fatalf("Event.ChatID = %q, want empty", got.Event.ChatID)
	}
	if got.Identity.ChatID != "" {
		t.Fatalf("Identity.ChatID = %q, want empty", got.Identity.ChatID)
	}
	if got.Reply.ChatID != "" {
		t.Fatalf("Reply.ChatID = %q, want empty", got.Reply.ChatID)
	}

	bot := New(nil, IdentityContext{}, nil)
	if bot.PairOutboundTarget(got) {
		t.Fatal("PairOutboundTarget returned true for unsafe chat result, want false")
	}
}

func TestNormalizeInboundWithIdentity_UnsafeReplyTargetRejected(t *testing.T) {
	got := NormalizeInboundWithIdentity(safeInboundMessage(func(msg InboundMessage) InboundMessage {
		msg.ReplyChatID = "15551234567%2fsecret"
		return msg
	}), IdentityContext{})

	assertUnsafeInboundEvidence(t, got)
	if got.Identity.UserID != "15551234567" {
		t.Fatalf("Identity.UserID = %q, want safe sender normalized", got.Identity.UserID)
	}
	if got.Identity.ChatID != "15551234567" {
		t.Fatalf("Identity.ChatID = %q, want safe chat normalized", got.Identity.ChatID)
	}
	if got.Event.ChatID != "" {
		t.Fatalf("Event.ChatID = %q, want empty", got.Event.ChatID)
	}
	if got.Reply.ChatID != "" {
		t.Fatalf("Reply.ChatID = %q, want empty", got.Reply.ChatID)
	}
}

func safeInboundMessage(mutator func(InboundMessage) InboundMessage) InboundMessage {
	msg := InboundMessage{
		ChatID:    "15551234567@s.whatsapp.net",
		ChatKind:  ChatKindDirect,
		UserID:    "15551234567@s.whatsapp.net",
		UserName:  "Alice",
		MessageID: "wamid-inbound-safety-1",
		Text:      "hello",
	}
	if mutator != nil {
		msg = mutator(msg)
	}
	return msg
}

func assertUnsafeInboundEvidence(t *testing.T, got InboundResult) {
	t.Helper()

	if got.Routed() {
		t.Fatal("Routed() = true, want false")
	}
	if got.Decision != InboundDecisionUnresolvedIdentity {
		t.Fatalf("Decision = %q, want %q", got.Decision, InboundDecisionUnresolvedIdentity)
	}
	if got.Status.Reason != string(WhatsAppIdentifierUnsafeEvidence) {
		t.Fatalf("Status.Reason = %q, want %q", got.Status.Reason, WhatsAppIdentifierUnsafeEvidence)
	}
}
