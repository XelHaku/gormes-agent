package wecom

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if got := b.Name(); got != "wecom" {
		t.Fatalf("Name() = %q, want wecom", got)
	}
}

func TestBot_Run_AppliesPolicyGuards(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		DMPolicy:       "allowlist",
		AllowFrom:      []string{"user-1"},
		GroupPolicy:    "allowlist",
		GroupAllowFrom: []string{"group-1"},
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeDirect,
		ChatID:    "dm-1",
		UserID:    "user-9",
		MessageID: "msg-1",
		Text:      "hello",
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-9",
		UserID:    "user-1",
		MessageID: "msg-2",
		Text:      "hello",
	})
	assertNoInbound(t, inbox)
}

func TestBot_Send_UsesReplyModeWhileTurnIsActive(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeDirect,
		ChatID:    "chat-1",
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "hello",
		RequestID: "req-1",
	})
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	msgID, err := b.Send(context.Background(), "chat-1", "reply")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "reply-1" {
		t.Fatalf("msgID = %q, want reply-1", msgID)
	}

	if len(mc.replySent) != 1 {
		t.Fatalf("reply send count = %d, want 1", len(mc.replySent))
	}
	if mc.replySent[0].RequestID != "req-1" {
		t.Fatalf("reply request id = %q, want req-1", mc.replySent[0].RequestID)
	}
}

func TestBot_Send_FallsBackToActivePushWhenReplyModeFails(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-1",
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "hello",
		RequestID: "req-1",
	})
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	mc.replyErr = errReplyFailed
	msgID, err := b.Send(context.Background(), "group-1", "reply")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "push-1" {
		t.Fatalf("msgID = %q, want push-1", msgID)
	}

	if len(mc.replySent) != 1 {
		t.Fatalf("reply send count = %d, want 1", len(mc.replySent))
	}
	if len(mc.pushSent) != 1 {
		t.Fatalf("push send count = %d, want 1", len(mc.pushSent))
	}
	if mc.pushSent[0].ChatType != ChatTypeGroup {
		t.Fatalf("push chat type = %q, want %q", mc.pushSent[0].ChatType, ChatTypeGroup)
	}
}

type mockClient struct {
	events    chan InboundMessage
	replySent []replyMessage
	pushSent  []pushMessage
	replyErr  error
	pushErr   error
}

type replyMessage struct {
	RequestID string
	Text      string
}

type pushMessage struct {
	ChatID   string
	ChatType string
	Text     string
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan InboundMessage, 16)}
}

func (m *mockClient) Events() <-chan InboundMessage { return m.events }

func (m *mockClient) SendReply(_ context.Context, requestID, text string) (string, error) {
	m.replySent = append(m.replySent, replyMessage{RequestID: requestID, Text: text})
	if m.replyErr != nil {
		return "", m.replyErr
	}
	return "reply-1", nil
}

func (m *mockClient) SendPush(_ context.Context, chatID, chatType, text string) (string, error) {
	m.pushSent = append(m.pushSent, pushMessage{ChatID: chatID, ChatType: chatType, Text: text})
	if m.pushErr != nil {
		return "", m.pushErr
	}
	return "push-1", nil
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

var errReplyFailed = errors.New("reply failed")
