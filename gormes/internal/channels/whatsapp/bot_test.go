package whatsapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Run_StartsBridgeAndForwardsNormalizedInbound(t *testing.T) {
	bridge := newMockBridge()
	events := make(chan BridgeEvent, 1)
	bridge.queueStart(startResult{events: events})

	b := New(bridge, Config{Mode: ModeSelfChat}, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx, inbox) }()

	events <- BridgeEvent{Message: &InboundMessage{
		UserID:    "15551234567@s.whatsapp.net",
		UserName:  "Alice",
		MessageID: "wamid-1",
		Text:      "hello there",
	}}

	select {
	case got := <-inbox:
		if got.Platform != "whatsapp" {
			t.Fatalf("Platform = %q, want whatsapp", got.Platform)
		}
		if got.ChatID != "15551234567" {
			t.Fatalf("ChatID = %q, want normalized direct chat ID", got.ChatID)
		}
		if got.Text != "hello there" {
			t.Fatalf("Text = %q, want hello there", got.Text)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for forwarded inbound event")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Run() to exit")
	}

	if got := bridge.startModes(); len(got) != 1 || got[0] != ModeSelfChat {
		t.Fatalf("Start() modes = %v, want [self-chat]", got)
	}
}

func TestBot_Run_ReconnectsAfterDisconnectUsingConfiguredBackoff(t *testing.T) {
	bridge := newMockBridge()
	first := make(chan BridgeEvent)
	second := make(chan BridgeEvent, 1)
	bridge.queueStart(startResult{events: first})
	bridge.queueStart(startResult{events: second})

	var (
		waitMu sync.Mutex
		waits  []time.Duration
	)
	waitFn := func(ctx context.Context, d time.Duration) error {
		waitMu.Lock()
		waits = append(waits, d)
		waitMu.Unlock()
		return nil
	}

	b := New(bridge, Config{
		Mode:              ModeBot,
		ReconnectSchedule: []time.Duration{25 * time.Millisecond, 50 * time.Millisecond},
		Wait:              waitFn,
	}, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx, inbox) }()

	close(first)

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return bridge.startCallCount() == 2
	})

	second <- BridgeEvent{Message: &InboundMessage{
		UserID:    "15557654321@s.whatsapp.net",
		MessageID: "wamid-2",
		Text:      "after reconnect",
	}}

	select {
	case got := <-inbox:
		if got.Text != "after reconnect" {
			t.Fatalf("Text after reconnect = %q, want after reconnect", got.Text)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for post-reconnect inbound event")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Run() to exit")
	}

	waitMu.Lock()
	defer waitMu.Unlock()
	if len(waits) != 1 || waits[0] != 25*time.Millisecond {
		t.Fatalf("waits = %v, want [25ms]", waits)
	}
}

func TestBot_Send_UsesRecordedReplyTargetAndReplyMetadata(t *testing.T) {
	bridge := newMockBridge()
	events := make(chan BridgeEvent, 1)
	bridge.queueStart(startResult{events: events})
	bridge.sendMessageID = "bridge-send-1"

	b := New(bridge, Config{}, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx, inbox) }()

	events <- BridgeEvent{Message: &InboundMessage{
		ChatID:    "120363025000000000@g.us",
		ChatKind:  ChatKindGroup,
		UserID:    "15557654321@s.whatsapp.net",
		UserName:  "Bob",
		MessageID: "wamid-group-1",
		Text:      "hello group",
	}}

	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for normalized inbound event")
	}

	msgID, err := b.Send(context.Background(), "120363025000000000", "reply one")
	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if msgID != "bridge-send-1" {
		t.Fatalf("Send() msgID = %q, want bridge-send-1", msgID)
	}

	call := bridge.lastSend()
	if call.ChatID != "120363025000000000@g.us" {
		t.Fatalf("Send() chatID = %q, want original transport group JID", call.ChatID)
	}
	if call.Options.ReplyToMessageID != "wamid-group-1" {
		t.Fatalf("Send() reply metadata = %+v, want reply-to wamid-group-1", call.Options)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Run() to exit")
	}
}

func TestBot_Send_RejectsUnknownChat(t *testing.T) {
	b := New(newMockBridge(), Config{}, nil)
	if _, err := b.Send(context.Background(), "missing", "reply"); err == nil {
		t.Fatal("Send() error = nil, want unknown chat error")
	}
}

type startResult struct {
	events <-chan BridgeEvent
	err    error
}

type sendCall struct {
	ChatID  string
	Text    string
	Options SendOptions
}

type mockBridge struct {
	mu            sync.Mutex
	startQueue    []startResult
	startedModes  []Mode
	sendCalls     []sendCall
	sendMessageID string
	closeCount    int
}

func newMockBridge() *mockBridge {
	return &mockBridge{}
}

func (m *mockBridge) Start(_ context.Context, mode Mode) (<-chan BridgeEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startedModes = append(m.startedModes, mode)
	if len(m.startQueue) == 0 {
		return nil, errors.New("unexpected start")
	}
	next := m.startQueue[0]
	m.startQueue = m.startQueue[1:]
	return next.events, next.err
}

func (m *mockBridge) Send(_ context.Context, chatID, text string, opts SendOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalls = append(m.sendCalls, sendCall{ChatID: chatID, Text: text, Options: opts})
	return m.sendMessageID, nil
}

func (m *mockBridge) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCount++
	return nil
}

func (m *mockBridge) queueStart(result startResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startQueue = append(m.startQueue, result)
}

func (m *mockBridge) startModes() []Mode {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Mode, len(m.startedModes))
	copy(out, m.startedModes)
	return out
}

func (m *mockBridge) startCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.startedModes)
}

func (m *mockBridge) lastSend() sendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sendCalls) == 0 {
		return sendCall{}
	}
	return m.sendCalls[len(m.sendCalls)-1]
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}
