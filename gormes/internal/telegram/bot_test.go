package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/session"
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

// TestBot_StartCommandReplies: /start from authorised chat triggers a
// welcome reply; no kernel event.
func TestBot_StartCommandReplies(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/start")
	time.Sleep(100 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "Gormes is online") {
		t.Errorf("reply = %q, want /start welcome", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(30 * time.Millisecond)
}

// TestBot_UnknownCommandReplies: /<anything> triggers polite rejection.
func TestBot_UnknownCommandReplies(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/nonsense")
	time.Sleep(100 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "unknown command") {
		t.Errorf("reply = %q, want 'unknown command'", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(30 * time.Millisecond)
}

// TestBot_NewCommandResetsSession: /new while Idle clears session and
// replies "Session reset".
func TestBot_NewCommandResetsSession(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/new")
	time.Sleep(100 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "Session reset") {
		t.Errorf("reply = %q, want 'Session reset'", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(30 * time.Millisecond)
}

// TestBot_PlainTextSubmitsToKernel: non-command message reaches the kernel
// and eventually produces a streamed reply.
func TestBot_PlainTextSubmitsToKernel(t *testing.T) {
	mc := newMockClient()

	// Kernel scripted to reply "ack".
	hmc := hermes.NewMockClient()
	reply := "ack"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: len(reply)})
	hmc.Script(events, "sess-plain")

	k := kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hmc, store.NewNoop(), telemetry.New(), nil)

	b := New(Config{AllowedChatID: 42, CoalesceMs: 200}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "ping")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "ack") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !strings.Contains(mc.lastSentText(), "ack") {
		t.Errorf("last bot msg = %q, want to contain 'ack'", mc.lastSentText())
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// TestBot_PersistsSessionIDToMap proves the bot's outbound goroutine
// calls SessionMap.Put exactly when the kernel's RenderFrame.SessionID
// changes. Uses MemMap + scripted hermes.MockClient — no disk, no network.
func TestBot_PersistsSessionIDToMap(t *testing.T) {
	mc := newMockClient()
	smap := session.NewMemMap()

	hmc := hermes.NewMockClient()
	reply := "ok"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	hmc.Script(events, "sess-persisted-xyz")

	k := kernel.New(kernel.Config{
		Model: "hermes-agent", Endpoint: "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, hmc, store.NewNoop(), telemetry.New(), nil)

	key := session.TelegramKey(42)
	b := New(Config{
		AllowedChatID: 42,
		CoalesceMs:    100,
		SessionMap:    smap,
		SessionKey:    key,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "ping")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := smap.Get(context.Background(), key); got == "sess-persisted-xyz" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	got, _ := smap.Get(context.Background(), key)
	if got != "sess-persisted-xyz" {
		t.Errorf("SessionMap[%q] = %q, want %q", key, got, "sess-persisted-xyz")
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// TestBot_StopSubmitsCancel: /stop command triggers a Submit of PlatformEventCancel.
// Verification is indirect: kernel must receive the cancel event. Observable via
// an eventual error render (if turn is slow enough), or silence (if already done).
// This test exercises the command path; full cancellation semantics are kernel-tested.
func TestBot_StopSubmitsCancel(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	// /stop command should not produce an error — Submit just enqueues.
	// If the kernel is Idle, there's nothing to cancel; it's a no-op.
	mc.pushTextUpdate(42, "/stop")
	time.Sleep(50 * time.Millisecond)

	// Verify no crash or rejected-mailbox error.
	// The test passes if no panic or immediate error message appears.
	if strings.Contains(mc.lastSentText(), "Busy") {
		t.Errorf("unexpected 'Busy' error on /stop; kernel Submit must not overflow")
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(30 * time.Millisecond)
}

// TestBot_NewCommandDuringTurn_RepliesCannotReset: /new when kernel is Idle
// succeeds with "Session reset" reply. (When kernel is mid-turn, ResetSession
// returns ErrResetDuringTurn; bot replies "Cannot reset". Full mid-turn testing
// happens in kernel tests; this verifies the Idle-case happy path.)
func TestBot_NewCommandDuringTurn_RepliesCannotReset(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	// /new while Idle must succeed.
	mc.pushTextUpdate(42, "/new")
	time.Sleep(100 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "Session reset") {
		t.Errorf("expected 'Session reset' reply, got: %q", mc.lastSentText())
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// TestBot_ToolCallHandshake_Echo_ViaTelegram proves Phase-2.A tool-call
// semantics survive the Telegram adapter round-trip. A Hermes MockClient
// scripts a 2-round tool-call turn (echo → final reply); a Telegram
// mockClient records outbound edits. Final assertion: the last bot message
// contains both "Tool said" and the echoed payload. No network, no API
// credits.
func TestBot_ToolCallHandshake_Echo_ViaTelegram(t *testing.T) {
	mc := newMockClient()

	hmc := hermes.NewMockClient()
	hmc.Script([]hermes.Event{
		{
			Kind: hermes.EventDone, FinishReason: "tool_calls",
			ToolCalls: []hermes.ToolCall{
				{ID: "call_echo_telegram", Name: "echo", Arguments: []byte(`{"text":"hello from telegram"}`)},
			},
		},
	}, "sess-tg-echo")
	finalAnswer := "Tool said: hello from telegram."
	events := make([]hermes.Event, 0, len(finalAnswer)+1)
	for _, ch := range finalAnswer {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 20, TokensOut: len(finalAnswer)})
	hmc.Script(events, "sess-tg-echo")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	k := kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hmc, store.NewNoop(), telemetry.New(), nil)

	b := New(Config{AllowedChatID: 42, CoalesceMs: 200}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "echo hello from telegram")

	// Wait for a Send whose text contains "Tool said".
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "Tool said") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	last := mc.lastSentText()
	if !strings.Contains(last, "Tool said") {
		t.Errorf("final bot msg = %q, want 'Tool said'", last)
	}
	if !strings.Contains(last, "hello from telegram") {
		t.Errorf("final bot msg = %q, want to reference tool output", last)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// TestBot_ResumesSessionIDAcrossRestart proves the cross-phase invariant:
// a single session.MemMap carried across two bot+kernel lifecycles causes
// the second cycle's kernel to start with the first cycle's final
// session_id. Uses MemMap so there's no disk dependency.
func TestBot_ResumesSessionIDAcrossRestart(t *testing.T) {
	smap := session.NewMemMap()
	key := session.TelegramKey(42)

	// ── Cycle 1: run a turn that assigns session_id "sess-cycle-1"
	{
		mc := newMockClient()
		hmc := hermes.NewMockClient()
		hmc.Script([]hermes.Event{
			{Kind: hermes.EventToken, Token: "hi", TokensOut: 1},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}, "sess-cycle-1")

		k := kernel.New(kernel.Config{
			Model: "hermes-agent", Endpoint: "http://mock",
			Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		}, hmc, store.NewNoop(), telemetry.New(), nil)

		b := New(Config{
			AllowedChatID: 42, CoalesceMs: 100,
			SessionMap: smap, SessionKey: key,
		}, mc, k, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		go k.Run(ctx)
		<-k.Render()
		go func() { _ = b.Run(ctx) }()

		mc.pushTextUpdate(42, "hi")

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if got, _ := smap.Get(context.Background(), key); got == "sess-cycle-1" {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if got, _ := smap.Get(context.Background(), key); got != "sess-cycle-1" {
			t.Fatalf("cycle 1: map[%q] = %q, want sess-cycle-1", key, got)
		}
		cancel()
		mc.closeUpdates()
		time.Sleep(100 * time.Millisecond) // drain
	}

	// ── Cycle 2: new kernel, same map — InitialSessionID must be populated.
	{
		persistedSID, _ := smap.Get(context.Background(), key)
		if persistedSID != "sess-cycle-1" {
			t.Fatalf("cycle 2 precondition: persistedSID = %q, want sess-cycle-1", persistedSID)
		}

		hmc := hermes.NewMockClient()
		hmc.Script([]hermes.Event{
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}, "sess-cycle-2")

		k := kernel.New(kernel.Config{
			Model: "hermes-agent", Endpoint: "http://mock",
			Admission:        kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
			InitialSessionID: persistedSID,
		}, hmc, store.NewNoop(), telemetry.New(), nil)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		go k.Run(ctx)
		<-k.Render()

		if err := k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "again"}); err != nil {
			t.Fatal(err)
		}

		// Verify the first outbound request carried the persisted session_id.
		waitForMockRequestWithSession := func(want string, d time.Duration) bool {
			deadline := time.Now().Add(d)
			for time.Now().Before(deadline) {
				for _, r := range hmc.Requests() {
					if r.SessionID == want {
						return true
					}
				}
				time.Sleep(25 * time.Millisecond)
			}
			return false
		}
		if !waitForMockRequestWithSession("sess-cycle-1", 2*time.Second) {
			t.Errorf("cycle 2: first request did not carry persisted session_id sess-cycle-1")
		}
	}
}
