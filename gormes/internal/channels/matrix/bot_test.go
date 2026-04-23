package matrix

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/threadtext"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_RunNormalizesMatrixEventAndRepliesInExistingThread(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyMode: threadtext.ReplyModeThread}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(Event{
		RoomID:       "!ops:matrix.example",
		RoomName:     "Ops",
		SenderID:     "@alice:matrix.example",
		SenderName:   "Alice",
		EventID:      "$reply-2",
		Body:         " /start ",
		ThreadID:     "$reply-2",
		ThreadRootID: "$root-1",
	})

	ev := receiveInbound(t, inbox)
	if ev.Platform != "matrix" || ev.ChatID != "!ops:matrix.example" || ev.UserID != "@alice:matrix.example" {
		t.Fatalf("unexpected event identity: %+v", ev)
	}
	if ev.ChatName != "Ops" || ev.UserName != "Alice" {
		t.Fatalf("unexpected event names: %+v", ev)
	}
	if ev.MsgID != "$reply-2" || ev.ThreadID != "$root-1" {
		t.Fatalf("unexpected event reply metadata: %+v", ev)
	}
	if ev.Kind != gateway.EventStart || ev.Text != "" {
		t.Fatalf("unexpected command parse: kind=%v text=%q", ev.Kind, ev.Text)
	}

	msgID, err := b.Send(context.Background(), ev.ChatID, "thread reply")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "$sent-1" {
		t.Fatalf("Send() msgID = %q, want $sent-1", msgID)
	}

	sent := mc.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if sent[0].RoomID != "!ops:matrix.example" || sent[0].Text != "thread reply" {
		t.Fatalf("unexpected send body: %+v", sent[0])
	}
	if sent[0].Options.ThreadID != "$root-1" || sent[0].Options.ReplyToEventID != "$reply-2" {
		t.Fatalf("send options = %+v, want thread root and reply event", sent[0].Options)
	}
}

func TestBot_SendStartsMatrixThreadFromRootEventWhenConfigured(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyMode: threadtext.ReplyModeThread}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(Event{
		RoomID:   "!ops:matrix.example",
		SenderID: "@bob:matrix.example",
		EventID:  "$root-2",
		Body:     "plain question",
	})
	ev := receiveInbound(t, inbox)

	if _, err := b.Send(context.Background(), ev.ChatID, "thread reply"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	sent := mc.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if sent[0].Options.ThreadID != "$root-2" || sent[0].Options.ReplyToEventID != "$root-2" {
		t.Fatalf("send options = %+v, want root event as new Matrix thread", sent[0].Options)
	}
}

func TestBot_IgnoresMatrixSelfEventsAndInvalidMessages(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(Event{
		RoomID:   "!ops:matrix.example",
		SenderID: "@bot:matrix.example",
		EventID:  "$self",
		Body:     "ignore me",
		FromSelf: true,
	})
	mc.push(Event{
		RoomID:   "!ops:matrix.example",
		SenderID: "@alice:matrix.example",
		EventID:  "$empty",
		Body:     "   ",
	})

	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound event, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

type mockClient struct {
	events chan Event
	sent   []sentMessage
}

type sentMessage struct {
	RoomID  string
	Text    string
	Options SendOptions
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan Event, 16)}
}

func (m *mockClient) Events() <-chan Event { return m.events }

func (m *mockClient) SendMessage(_ context.Context, roomID, text string, opts SendOptions) (string, error) {
	m.sent = append(m.sent, sentMessage{RoomID: roomID, Text: text, Options: opts})
	return "$sent-1", nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(event Event) {
	m.events <- event
}

func (m *mockClient) sentSnapshot() []sentMessage {
	out := make([]sentMessage, len(m.sent))
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
