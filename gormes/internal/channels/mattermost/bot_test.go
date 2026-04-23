package mattermost

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/threadtext"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_RunNormalizesMattermostPostAndRepliesInExistingThread(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyMode: threadtext.ReplyModeThread}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(PostEvent{
		ChannelID:   "town-square",
		ChannelName: "Town Square",
		UserID:      "user-1",
		UserName:    "alice",
		PostID:      "post-9",
		Message:     " /new ",
		RootID:      "root-1",
	})

	ev := receiveInbound(t, inbox)
	if ev.Platform != "mattermost" || ev.ChatID != "town-square" || ev.UserID != "user-1" {
		t.Fatalf("unexpected event identity: %+v", ev)
	}
	if ev.ChatName != "Town Square" || ev.UserName != "alice" {
		t.Fatalf("unexpected event names: %+v", ev)
	}
	if ev.MsgID != "post-9" || ev.ThreadID != "root-1" {
		t.Fatalf("unexpected event reply metadata: %+v", ev)
	}
	if ev.Kind != gateway.EventReset || ev.Text != "" {
		t.Fatalf("unexpected command parse: kind=%v text=%q", ev.Kind, ev.Text)
	}

	msgID, err := b.Send(context.Background(), ev.ChatID, "thread reply")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "sent-1" {
		t.Fatalf("Send() msgID = %q, want sent-1", msgID)
	}

	sent := mc.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if sent[0].ChannelID != "town-square" || sent[0].Message != "thread reply" {
		t.Fatalf("unexpected send body: %+v", sent[0])
	}
	if sent[0].Options.RootID != "root-1" {
		t.Fatalf("send options = %+v, want RootID root-1", sent[0].Options)
	}
}

func TestBot_SendStartsMattermostThreadFromRootPostWhenConfigured(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyMode: threadtext.ReplyModeThread}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(PostEvent{
		ChannelID: "town-square",
		UserID:    "user-2",
		PostID:    "post-root",
		Message:   "plain question",
	})
	ev := receiveInbound(t, inbox)

	if _, err := b.Send(context.Background(), ev.ChatID, "thread reply"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	sent := mc.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if sent[0].Options.RootID != "post-root" {
		t.Fatalf("send options = %+v, want root post as Mattermost thread", sent[0].Options)
	}
}

func TestBot_IgnoresMattermostSelfEventsAndInvalidPosts(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(PostEvent{
		ChannelID: "town-square",
		UserID:    "bot-user",
		PostID:    "self",
		Message:   "ignore me",
		FromSelf:  true,
	})
	mc.push(PostEvent{
		ChannelID: "town-square",
		UserID:    "user-1",
		PostID:    "empty",
		Message:   "   ",
	})

	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound event, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

type mockClient struct {
	events chan PostEvent
	sent   []sentPost
}

type sentPost struct {
	ChannelID string
	Message   string
	Options   SendOptions
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan PostEvent, 16)}
}

func (m *mockClient) Events() <-chan PostEvent { return m.events }

func (m *mockClient) CreatePost(_ context.Context, channelID, message string, opts SendOptions) (string, error) {
	m.sent = append(m.sent, sentPost{ChannelID: channelID, Message: message, Options: opts})
	return "sent-1", nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(event PostEvent) {
	m.events <- event
}

func (m *mockClient) sentSnapshot() []sentPost {
	out := make([]sentPost, len(m.sent))
	copy(out, m.sent)
	return out
}

func receiveInbound(t *testing.T, inbox <-chan gateway.InboundEvent) gateway.InboundEvent {
	t.Helper()
	select {
	case ev := <-inbox:
		return ev
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
		return gateway.InboundEvent{}
	}
}
