package dingtalk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if got := b.Name(); got != "dingtalk" {
		t.Fatalf("Name() = %q, want dingtalk", got)
	}
}

func TestBot_Run_IgnoresDisallowedSender(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedUserIDs: []string{"staff-1"}}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ConversationID:   "group-1",
		ConversationType: "2",
		SenderStaffID:    "staff-9",
		Text:             "@Hermes hello",
		SessionWebhook:   "https://example.invalid/hook",
		Mentioned:        true,
	})

	assertNoInbound(t, inbox)
}

func TestBot_Run_GroupMentionRequiredAndCommandNormalized(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ConversationID:   "group-1",
		ConversationType: "2",
		SenderStaffID:    "staff-1",
		Text:             "@Hermes /help",
		SessionWebhook:   "https://example.invalid/hook",
		Mentioned:        false,
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		MessageID:        "msg-1",
		ConversationID:   "group-1",
		ConversationType: "2",
		SenderStaffID:    "staff-1",
		SenderNick:       "Alice",
		Text:             "@Hermes /help",
		SessionWebhook:   "https://example.invalid/hook",
		Mentioned:        true,
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "dingtalk" || ev.ChatID != "group-1" || ev.UserID != "staff-1" {
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

func TestBot_Send_UsesStoredSessionWebhook(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		MessageID:        "msg-1",
		ConversationID:   "dm-42",
		ConversationType: "1",
		SenderID:         "user-42",
		Text:             "hello",
		SessionWebhook:   "https://example.invalid/reply",
	})

	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	msgID, err := b.Send(context.Background(), "dm-42", "reply")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "send-1" {
		t.Fatalf("Send() msgID = %q, want send-1", msgID)
	}

	sent := mc.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if sent[0].Webhook != "https://example.invalid/reply" {
		t.Fatalf("webhook = %q, want stored session webhook", sent[0].Webhook)
	}
	if sent[0].Text != "reply" {
		t.Fatalf("text = %q, want reply", sent[0].Text)
	}
}

func TestBot_Send_ErrorsWithoutSessionWebhook(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if _, err := b.Send(context.Background(), "missing-chat", "reply"); err == nil {
		t.Fatal("expected error for unknown session webhook")
	}
}

type mockClient struct {
	events  chan InboundMessage
	sent    []sentReply
	sendErr error
}

type sentReply struct {
	Webhook string
	Text    string
}

func newMockClient() *mockClient {
	return &mockClient{
		events: make(chan InboundMessage, 16),
	}
}

func (m *mockClient) Events() <-chan InboundMessage { return m.events }

func (m *mockClient) SendReply(_ context.Context, webhook, text string) (string, error) {
	if m.sendErr != nil {
		return "", m.sendErr
	}
	m.sent = append(m.sent, sentReply{Webhook: webhook, Text: text})
	return "send-1", nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(msg InboundMessage) {
	m.events <- msg
}

func (m *mockClient) sentSnapshot() []sentReply {
	out := make([]sentReply, len(m.sent))
	copy(out, m.sent)
	return out
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
