package whatsapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func TestBotSend_BlocksUnpairedTargetBeforeTransport(t *testing.T) {
	client := newSendContractClient()
	bot := New(client, IdentityContext{Runtime: RuntimeKindBridge}, nil)

	_, err := bot.Send(context.Background(), "15551234567", "reply")
	if err == nil {
		t.Fatal("Send() error = nil, want degraded unpaired target")
	}

	var degraded DegradedSendError
	if !errors.As(err, &degraded) {
		t.Fatalf("Send() error = %T, want DegradedSendError", err)
	}
	if degraded.Reason != SendDegradedUnpairedTarget {
		t.Fatalf("degraded reason = %q, want %q", degraded.Reason, SendDegradedUnpairedTarget)
	}
	if degraded.ChatID != "15551234567" {
		t.Fatalf("degraded chat = %q, want 15551234567", degraded.ChatID)
	}

	status := bot.OutboundStatus()
	if status.State != OutboundStateDegraded || status.Reason != SendDegradedUnpairedTarget {
		t.Fatalf("OutboundStatus() = %+v, want degraded unpaired target", status)
	}
	if got := client.sendCount(); got != 0 {
		t.Fatalf("transport send count = %d, want 0", got)
	}
}

func TestBotSend_BlocksUnresolvedPairedTargetBeforeTransport(t *testing.T) {
	client := newSendContractClient()
	bot := New(client, IdentityContext{Runtime: RuntimeKindBridge}, nil)
	bot.PairOutboundTarget(InboundResult{
		Decision: InboundDecisionRoute,
		Event: gateway.InboundEvent{
			Platform: platformName,
			ChatID:   "15551234567",
			MsgID:    "wamid.missing.raw",
		},
		Reply: ReplyTarget{ChatKind: ChatKindDirect},
	})

	_, err := bot.Send(context.Background(), "15551234567", "reply")
	if err == nil {
		t.Fatal("Send() error = nil, want degraded unresolved target")
	}

	var degraded DegradedSendError
	if !errors.As(err, &degraded) {
		t.Fatalf("Send() error = %T, want DegradedSendError", err)
	}
	if degraded.Reason != SendDegradedUnresolvedTarget {
		t.Fatalf("degraded reason = %q, want %q", degraded.Reason, SendDegradedUnresolvedTarget)
	}
	status := bot.OutboundStatus()
	if status.State != OutboundStateDegraded || status.Reason != SendDegradedUnresolvedTarget {
		t.Fatalf("OutboundStatus() = %+v, want degraded unresolved target", status)
	}
	if got := client.sendCount(); got != 0 {
		t.Fatalf("transport send count = %d, want 0", got)
	}
}

func TestBotSend_BridgeDirectTargetUsesRawPeerAndReplyMetadata(t *testing.T) {
	client := newSendContractClient()
	bot := New(client, IdentityContext{
		Runtime: RuntimeKindBridge,
		AliasMappings: []IdentityAlias{
			{From: "999999999999999@lid", To: "15551234567@s.whatsapp.net"},
		},
	}, nil)
	inbox := runSendContractBot(t, bot)

	client.push(InboundMessage{
		ChatID:    "999999999999999@lid",
		ChatKind:  ChatKindDirect,
		UserID:    "999999999999999@lid",
		UserName:  "Alice",
		MessageID: "wamid.bridge.1",
		Text:      "hello",
	})
	assertInboundChat(t, inbox, "15551234567")

	msgID, err := bot.Send(context.Background(), "15551234567", "reply one")
	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if msgID != "wam-send-1" {
		t.Fatalf("Send() msgID = %q, want wam-send-1", msgID)
	}

	sends := client.sendsSnapshot()
	if len(sends) != 1 {
		t.Fatalf("transport send count = %d, want 1", len(sends))
	}
	got := sends[0]
	if got.Runtime != RuntimeKindBridge {
		t.Fatalf("Runtime = %q, want %q", got.Runtime, RuntimeKindBridge)
	}
	if got.ChatID != "999999999999999@lid" {
		t.Fatalf("raw ChatID = %q, want raw LID peer", got.ChatID)
	}
	if got.ChatKind != ChatKindDirect {
		t.Fatalf("ChatKind = %q, want %q", got.ChatKind, ChatKindDirect)
	}
	if got.Options.ReplyToMessageID != "wamid.bridge.1" {
		t.Fatalf("ReplyToMessageID = %q, want wamid.bridge.1", got.Options.ReplyToMessageID)
	}
	if got.Text != "reply one" {
		t.Fatalf("Text = %q, want reply one", got.Text)
	}
}

func TestBotSend_NativeGroupTargetUsesRawPeerAndReplyMetadata(t *testing.T) {
	client := newSendContractClient()
	bot := New(client, IdentityContext{
		Runtime:     RuntimeKindNative,
		NativeBotID: "18005550100:5@s.whatsapp.net",
	}, nil)
	inbox := runSendContractBot(t, bot)

	client.push(InboundMessage{
		ChatID:    "120363000000000000@g.us",
		ChatKind:  ChatKindGroup,
		UserID:    "15557654321:47@s.whatsapp.net",
		UserName:  "Bob",
		MessageID: "wamid.native.2",
		Text:      "hello group",
	})
	assertInboundChat(t, inbox, "120363000000000000")

	msgID, err := bot.Send(context.Background(), "120363000000000000", "reply group")
	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if msgID != "wam-send-1" {
		t.Fatalf("Send() msgID = %q, want wam-send-1", msgID)
	}

	sends := client.sendsSnapshot()
	if len(sends) != 1 {
		t.Fatalf("transport send count = %d, want 1", len(sends))
	}
	got := sends[0]
	if got.Runtime != RuntimeKindNative {
		t.Fatalf("Runtime = %q, want %q", got.Runtime, RuntimeKindNative)
	}
	if got.ChatID != "120363000000000000@g.us" {
		t.Fatalf("raw ChatID = %q, want raw group peer", got.ChatID)
	}
	if got.ChatKind != ChatKindGroup {
		t.Fatalf("ChatKind = %q, want %q", got.ChatKind, ChatKindGroup)
	}
	if got.Options.ReplyToMessageID != "wamid.native.2" {
		t.Fatalf("ReplyToMessageID = %q, want wamid.native.2", got.Options.ReplyToMessageID)
	}
}

func runSendContractBot(t *testing.T, bot *Bot) <-chan gateway.InboundEvent {
	t.Helper()

	inbox := make(chan gateway.InboundEvent, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = bot.Run(ctx, inbox) }()
	return inbox
}

func assertInboundChat(t *testing.T, inbox <-chan gateway.InboundEvent, want string) {
	t.Helper()

	select {
	case ev := <-inbox:
		if ev.ChatID != want {
			t.Fatalf("inbound ChatID = %q, want %q", ev.ChatID, want)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}
}

type sendContractClient struct {
	events chan InboundMessage
	sends  []SendRequest
}

func newSendContractClient() *sendContractClient {
	return &sendContractClient{events: make(chan InboundMessage, 8)}
}

func (c *sendContractClient) Events() <-chan InboundMessage { return c.events }

func (c *sendContractClient) SendWhatsApp(_ context.Context, req SendRequest) (string, error) {
	c.sends = append(c.sends, req)
	return "wam-send-1", nil
}

func (c *sendContractClient) Close() error { return nil }

func (c *sendContractClient) push(msg InboundMessage) {
	c.events <- msg
}

func (c *sendContractClient) sendCount() int {
	return len(c.sends)
}

func (c *sendContractClient) sendsSnapshot() []SendRequest {
	out := make([]SendRequest, len(c.sends))
	copy(out, c.sends)
	return out
}
