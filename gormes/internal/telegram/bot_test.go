package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// TestBot_StreamsAssistantDraft: with a scripted hermes.MockClient replying
// "hello", the Bot's outbound goroutine emits a placeholder + at least one
// edit whose text contains "hello". Exercises T4's render + coalescer +
// outbound path end-to-end using only mocks.
func TestBot_StreamsAssistantDraft(t *testing.T) {
	mc := newMockClient()

	// Build a kernel whose MockClient is scripted to answer "hello".
	hmc := hermes.NewMockClient()
	reply := "hello"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{
		Kind: hermes.EventDone, FinishReason: "stop",
		TokensIn: 1, TokensOut: len(reply),
	})
	hmc.Script(events, "sess-t4-stream")

	k := kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hmc, store.NewNoop(), telemetry.New(), nil)

	b := New(Config{
		AllowedChatID: 42,
		CoalesceMs:    200, // tight window so test finishes quickly
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go k.Run(ctx)
	<-k.Render() // initial idle
	go func() { _ = b.Run(ctx) }()

	// Directly submit to the kernel — bypasses handleUpdate's auth gate
	// because T5 hasn't wired kernel.Submit into handleUpdate yet.
	_ = k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "hi"})

	// Wait until at least one Send contains "hello".
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "hello") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	last := mc.lastSentText()
	if !strings.Contains(last, "hello") {
		t.Errorf("last bot msg = %q, want to contain 'hello'", last)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// newTestKernel builds a Kernel with MockClient + NoopStore. Shared across
// Bot tests that don't care about the kernel's internals beyond "takes
// PlatformEvents, emits RenderFrames".
func newTestKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)
}

// TestBot_RejectsUnauthorisedChat: inbound message from a non-allowed chat
// produces zero Send calls and zero kernel.Submit calls.
func TestBot_RejectsUnauthorisedChat(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{
		AllowedChatID:     11111,
		FirstRunDiscovery: false,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go k.Run(ctx)
	<-k.Render() // drain initial idle

	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(22222, "hello from nowhere")

	time.Sleep(50 * time.Millisecond)

	if got := len(mc.sentMessages()); got != 0 {
		t.Errorf("sent messages = %d, want 0 (silent drop for unauthorised chat)", got)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}

// TestBot_FirstRunDiscoveryRepliesWithChatID: zero AllowedChatID +
// FirstRunDiscovery enabled → one "not authorised" reply naming the
// allowed_chat_id config key.
func TestBot_FirstRunDiscoveryRepliesWithChatID(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{
		AllowedChatID:     0,
		FirstRunDiscovery: true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(77777, "hi")
	time.Sleep(50 * time.Millisecond)

	got := mc.lastSentText()
	if !strings.Contains(got, "not authorised") {
		t.Errorf("reply = %q, want to contain 'not authorised'", got)
	}
	if !strings.Contains(got, "allowed_chat_id") {
		t.Errorf("reply = %q, want to mention allowed_chat_id config key", got)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}
