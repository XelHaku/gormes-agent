package weixin

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if got := b.Name(); got != "weixin" {
		t.Fatalf("Name() = %q, want weixin", got)
	}
}

func TestBot_Run_DefaultGroupPolicyIsDisabled(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:     ChatTypeGroup,
		ChatID:       "group-1",
		UserID:       "user-1",
		MessageID:    "msg-1",
		Text:         "hello",
		ContextToken: "ctx-group",
	})

	assertNoInbound(t, inbox)
}

func TestBot_Run_DMAllowlistBlocksUnknownUser(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		DMPolicy:  "allowlist",
		AllowFrom: []string{"user-1"},
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:     ChatTypeDirect,
		ChatID:       "dm-1",
		UserID:       "user-9",
		MessageID:    "msg-1",
		Text:         "hello",
		ContextToken: "ctx-1",
	})

	assertNoInbound(t, inbox)
}

func TestBot_Send_UsesStoredContextToken(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:     ChatTypeDirect,
		ChatID:       "dm-1",
		UserID:       "user-1",
		MessageID:    "msg-1",
		Text:         "hello",
		ContextToken: "ctx-1",
	})
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	msgID, err := b.Send(context.Background(), "dm-1", "**bold**")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "send-1" {
		t.Fatalf("msgID = %q, want send-1", msgID)
	}

	if len(mc.sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(mc.sent))
	}
	if mc.sent[0].ContextToken != "ctx-1" {
		t.Fatalf("context token = %q, want ctx-1", mc.sent[0].ContextToken)
	}
	if mc.sent[0].Text != "**bold**" {
		t.Fatalf("text = %q, want markdown preserved", mc.sent[0].Text)
	}
}

type mockClient struct {
	events chan InboundMessage
	sent   []sentMessage
}

type sentMessage struct {
	ChatID       string
	ContextToken string
	Text         string
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan InboundMessage, 16)}
}

func (m *mockClient) Events() <-chan InboundMessage { return m.events }

func (m *mockClient) SendWithContext(_ context.Context, chatID, contextToken, text string) (string, error) {
	m.sent = append(m.sent, sentMessage{ChatID: chatID, ContextToken: contextToken, Text: text})
	return "send-1", nil
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
