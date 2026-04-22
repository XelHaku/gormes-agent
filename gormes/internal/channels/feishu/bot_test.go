package feishu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if got := b.Name(); got != "feishu" {
		t.Fatalf("Name() = %q, want feishu", got)
	}
}

func TestBot_Run_WebhookRequiresVerificationToken(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		ConnectionMode:    ModeWebhook,
		VerificationToken: "verify-me",
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		Source:      SourceWebhook,
		ChatID:      "chat-1",
		ChatType:    ChatTypeDirect,
		UserID:      "user-1",
		MessageID:   "msg-1",
		Text:        "hello",
		VerifyToken: "bad-token",
	})

	assertNoInbound(t, inbox)
}

func TestBot_Run_GroupRequiresMentionAndAllowlist(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		ConnectionMode: ModeWebsocket,
		GroupPolicy:    "allowlist",
		AllowedUserIDs: []string{"user-1"},
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		Source:    SourceWebsocket,
		ChatID:    "chat-1",
		ChatType:  ChatTypeGroup,
		UserID:    "user-2",
		MessageID: "msg-1",
		Text:      "@Hermes /help",
		Mentioned: true,
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		Source:    SourceWebsocket,
		ChatID:    "chat-1",
		ChatType:  ChatTypeGroup,
		UserID:    "user-1",
		MessageID: "msg-2",
		Text:      "@Hermes /help",
		Mentioned: false,
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		Source:       SourceWebsocket,
		ChatID:       "chat-1",
		ChatType:     ChatTypeGroup,
		UserID:       "user-1",
		UserName:     "Alice",
		MessageID:    "msg-3",
		Text:         "@Hermes /help",
		Mentioned:    true,
		ThreadRootID: "root-1",
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "feishu" || ev.ChatID != "chat-1" || ev.UserID != "user-1" {
			t.Fatalf("unexpected event identity: %+v", ev)
		}
		if ev.Kind != gateway.EventStart {
			t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventStart)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_Send_MarkdownFallsBackToTextAndUsesReplyTarget(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ConnectionMode: ModeWebsocket}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		Source:       SourceWebsocket,
		ChatID:       "chat-1",
		ChatType:     ChatTypeDirect,
		UserID:       "user-1",
		MessageID:    "msg-1",
		Text:         "hello",
		ThreadRootID: "root-1",
	})
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	mc.richErr = errSendFailed
	msgID, err := b.Send(context.Background(), "chat-1", "## Title")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "text-1" {
		t.Fatalf("msgID = %q, want text-1", msgID)
	}

	if len(mc.richSent) != 1 {
		t.Fatalf("rich send count = %d, want 1", len(mc.richSent))
	}
	if mc.richSent[0].Options.ReplyToMessageID != "msg-1" {
		t.Fatalf("rich reply target = %q, want msg-1", mc.richSent[0].Options.ReplyToMessageID)
	}
	if len(mc.textSent) != 1 {
		t.Fatalf("text send count = %d, want 1", len(mc.textSent))
	}
	if mc.textSent[0].Options.ReplyToMessageID != "msg-1" {
		t.Fatalf("text reply target = %q, want msg-1", mc.textSent[0].Options.ReplyToMessageID)
	}
}

type mockClient struct {
	events   chan InboundMessage
	richSent []sentMessage
	textSent []sentMessage
	richErr  error
	textErr  error
}

type sentMessage struct {
	ChatID  string
	Text    string
	Options SendOptions
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan InboundMessage, 16)}
}

func (m *mockClient) Events() <-chan InboundMessage { return m.events }

func (m *mockClient) SendRichText(_ context.Context, chatID, text string, opts SendOptions) (string, error) {
	m.richSent = append(m.richSent, sentMessage{ChatID: chatID, Text: text, Options: opts})
	if m.richErr != nil {
		return "", m.richErr
	}
	return "rich-1", nil
}

func (m *mockClient) SendText(_ context.Context, chatID, text string, opts SendOptions) (string, error) {
	m.textSent = append(m.textSent, sentMessage{ChatID: chatID, Text: text, Options: opts})
	if m.textErr != nil {
		return "", m.textErr
	}
	return "text-1", nil
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

var errSendFailed = errors.New("send failed")
