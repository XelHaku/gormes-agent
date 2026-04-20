package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// TestKernel_ResetSession_IdleSucceeds: after a completed turn, history
// carries the user + assistant messages and sessionID is non-empty.
// Calling ResetSession when the kernel is Idle clears all of it and
// returns nil.
func TestKernel_ResetSession_IdleSucceeds(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "ok", TokensOut: 1},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1},
	}, "sess-to-reset")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial = %v, want Idle", initial.Phase)
	}

	// Submit a turn and wait for it to complete.
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	completed := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && len(f.History) >= 2 && f.SessionID != ""
	}, 2*time.Second)
	if len(completed.History) < 2 {
		t.Fatalf("pre-reset history len = %d, want >= 2", len(completed.History))
	}
	if completed.SessionID == "" {
		t.Fatalf("pre-reset SessionID empty; expected server-assigned value")
	}

	// Call ResetSession while Idle — must succeed.
	if err := k.ResetSession(); err != nil {
		t.Fatalf("ResetSession while Idle should succeed: %v", err)
	}

	// Next frame should reflect the cleared state.
	after := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return len(f.History) == 0 && f.SessionID == ""
	}, 1*time.Second)
	if len(after.History) != 0 {
		t.Errorf("post-reset history len = %d, want 0", len(after.History))
	}
	if after.SessionID != "" {
		t.Errorf("post-reset SessionID = %q, want empty", after.SessionID)
	}
}

// TestKernel_ResetSession_StreamingFails: during an in-flight stream,
// ResetSession must return ErrResetDuringTurn. History is preserved
// (the in-flight user turn is still present).
func TestKernel_ResetSession_StreamingFails(t *testing.T) {
	mc := hermes.NewMockClient()
	// Long stream — enough tokens that we can observe PhaseStreaming before
	// completion.
	events := make([]hermes.Event, 0, 200)
	for i := 0; i < 199; i++ {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: "t", TokensOut: i + 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "sess-busy")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // drain initial idle

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "go"}); err != nil {
		t.Fatal(err)
	}

	// Wait until PhaseStreaming is observed. At that point history already
	// contains the user message.
	streaming := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming
	}, 500*time.Millisecond)
	preResetHistoryLen := len(streaming.History)
	if preResetHistoryLen == 0 {
		t.Fatalf("pre-reset streaming frame history empty; expected at least the user turn")
	}

	// ResetSession must reject.
	err := k.ResetSession()
	if !errors.Is(err, ErrResetDuringTurn) {
		t.Errorf("ResetSession during Streaming = %v, want ErrResetDuringTurn", err)
	}

	// Drain remaining frames until turn completes; history must be preserved
	// throughout (at least the user message).
	done := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && len(f.History) >= 2
	}, 2*time.Second)
	if len(done.History) < preResetHistoryLen {
		t.Errorf("post-failed-reset history shrank from %d to %d — reset should NOT mutate",
			preResetHistoryLen, len(done.History))
	}
}
