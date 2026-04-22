package bluebubbles

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Run_RequiresWebhookAuthAndNormalizesCommands(t *testing.T) {
	mc := newMockClient()
	b := New(Config{Password: "secret"}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		AuthToken:      "wrong",
		ChatGUID:       "chat-guid-1",
		ChatIdentifier: "friend@example.com",
		Sender:         "friend@example.com",
		Text:           "/help",
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		AuthToken:      "secret",
		MessageID:      "msg-1",
		ChatGUID:       "chat-guid-1",
		ChatIdentifier: "friend@example.com",
		Sender:         "friend@example.com",
		SenderName:     "Alice",
		Text:           "/help",
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "bluebubbles" || ev.ChatID != "chat-guid-1" || ev.UserID != "friend@example.com" {
			t.Fatalf("unexpected event identity: %+v", ev)
		}
		if ev.Kind != gateway.EventStart {
			t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventStart)
		}
		if ev.Text != "" {
			t.Fatalf("Text = %q, want empty after /help parse", ev.Text)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_Send_UsesCachedChatGUIDAndHomeChannelFallback(t *testing.T) {
	mc := newMockClient()
	mc.resolved["+15551234567"] = "chat-guid-home"
	b := New(Config{
		Password:    "secret",
		HomeChannel: "+15551234567",
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		AuthToken:      "secret",
		MessageID:      "msg-1",
		ChatGUID:       "chat-guid-1",
		ChatIdentifier: "friend@example.com",
		Sender:         "friend@example.com",
		Text:           "hello",
	})

	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	msgID, err := b.Send(context.Background(), "friend@example.com", "**reply**")
	if err != nil {
		t.Fatalf("Send(cached) error = %v", err)
	}
	if msgID != "send-1" {
		t.Fatalf("Send(cached) msgID = %q, want send-1", msgID)
	}
	if len(mc.resolveCalls) != 0 {
		t.Fatalf("resolve calls = %v, want cache hit without resolve", mc.resolveCalls)
	}
	if len(mc.sent) != 1 {
		t.Fatalf("send count after cached send = %d, want 1", len(mc.sent))
	}
	if mc.sent[0].ChatGUID != "chat-guid-1" {
		t.Fatalf("cached chat guid = %q, want chat-guid-1", mc.sent[0].ChatGUID)
	}
	if mc.sent[0].Text != "reply" {
		t.Fatalf("cached text = %q, want stripped markdown reply", mc.sent[0].Text)
	}

	msgID, err = b.Send(context.Background(), "", "_notice_")
	if err != nil {
		t.Fatalf("Send(home) error = %v", err)
	}
	if msgID != "send-2" {
		t.Fatalf("Send(home) msgID = %q, want send-2", msgID)
	}
	if len(mc.resolveCalls) != 1 || mc.resolveCalls[0] != "+15551234567" {
		t.Fatalf("resolve calls = %v, want [+15551234567]", mc.resolveCalls)
	}
	if len(mc.sent) != 2 {
		t.Fatalf("send count after home send = %d, want 2", len(mc.sent))
	}
	if mc.sent[1].ChatGUID != "chat-guid-home" {
		t.Fatalf("home chat guid = %q, want chat-guid-home", mc.sent[1].ChatGUID)
	}
	if mc.sent[1].Text != "notice" {
		t.Fatalf("home text = %q, want stripped markdown notice", mc.sent[1].Text)
	}
}

type mockClient struct {
	events       chan InboundMessage
	resolved     map[string]string
	resolveCalls []string
	sent         []sentMessage
}

type sentMessage struct {
	ChatGUID string
	Text     string
}

func newMockClient() *mockClient {
	return &mockClient{
		events:   make(chan InboundMessage, 16),
		resolved: map[string]string{},
	}
}

func (m *mockClient) Events() <-chan InboundMessage { return m.events }

func (m *mockClient) ResolveChat(_ context.Context, target string) (string, error) {
	m.resolveCalls = append(m.resolveCalls, target)
	return m.resolved[target], nil
}

func (m *mockClient) SendText(_ context.Context, chatGUID, text string) (string, error) {
	m.sent = append(m.sent, sentMessage{ChatGUID: chatGUID, Text: text})
	return "send-" + string(rune('0'+len(m.sent))), nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(msg InboundMessage) {
	m.events <- msg
}

func assertNoInbound(t *testing.T, inbox <-chan gateway.InboundEvent) {
	t.Helper()
	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound event, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}
