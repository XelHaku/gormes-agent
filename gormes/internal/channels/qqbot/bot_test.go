package qqbot

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{}, newMockClient(), nil)
	if got := b.Name(); got != "qqbot" {
		t.Fatalf("Name() = %q, want qqbot", got)
	}
}

func TestBot_Run_DirectAllowListBlocksUnknownUser(t *testing.T) {
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
		ChatType:  ChatTypeDirect,
		ChatID:    "dm-1",
		UserID:    "user-9",
		MessageID: "msg-1",
		Text:      "hello",
	})

	assertNoInbound(t, inbox)
}

func TestBot_Run_GroupRequiresMention(t *testing.T) {
	mc := newMockClient()
	b := New(Config{GroupPolicy: "open"}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-1",
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "@Hermes /help",
		Mentioned: false,
	})
	assertNoInbound(t, inbox)

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-1",
		UserID:    "user-1",
		MessageID: "msg-2",
		Text:      "@Hermes /help",
		Mentioned: true,
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "qqbot" || ev.ChatID != "group-1" || ev.UserID != "user-1" {
			t.Fatalf("unexpected event identity: %+v", ev)
		}
		if ev.Kind != gateway.EventStart {
			t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventStart)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_Run_GroupAllowListBlocksUnknownGroup(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		GroupPolicy:    "allowlist",
		GroupAllowFrom: []string{"group-1"},
	}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-9",
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "@Hermes hi",
		Mentioned: true,
	})

	assertNoInbound(t, inbox)
}

func TestBot_Send_UsesPassiveReplyMetadataForGroup(t *testing.T) {
	mc := newMockClient()
	b := New(Config{GroupPolicy: "open"}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(InboundMessage{
		ChatType:  ChatTypeGroup,
		ChatID:    "group-1",
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "@Hermes hello",
		Mentioned: true,
	})

	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event before send")
	}

	msgID, err := b.Send(context.Background(), "group-1", "reply one")
	if err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	if msgID != "group-send-1" {
		t.Fatalf("first msgID = %q, want group-send-1", msgID)
	}

	if _, err := b.Send(context.Background(), "group-1", "reply two"); err != nil {
		t.Fatalf("second Send() error = %v", err)
	}

	group := mc.groupSnapshot()
	if len(group) != 2 {
		t.Fatalf("group send count = %d, want 2", len(group))
	}
	if group[0].Options.ReplyToMessageID != "msg-1" || group[0].Options.Sequence != 1 {
		t.Fatalf("first send options = %+v", group[0].Options)
	}
	if group[1].Options.ReplyToMessageID != "msg-1" || group[1].Options.Sequence != 2 {
		t.Fatalf("second send options = %+v", group[1].Options)
	}
}

type mockClient struct {
	events chan InboundMessage
	group  []sentMessage
	direct []sentMessage
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

func (m *mockClient) SendDirect(_ context.Context, chatID, text string, opts SendOptions) (string, error) {
	m.direct = append(m.direct, sentMessage{ChatID: chatID, Text: text, Options: opts})
	return "direct-send-1", nil
}

func (m *mockClient) SendGroup(_ context.Context, chatID, text string, opts SendOptions) (string, error) {
	m.group = append(m.group, sentMessage{ChatID: chatID, Text: text, Options: opts})
	return "group-send-1", nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(msg InboundMessage) {
	m.events <- msg
}

func (m *mockClient) groupSnapshot() []sentMessage {
	out := make([]sentMessage, len(m.group))
	copy(out, m.group)
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
