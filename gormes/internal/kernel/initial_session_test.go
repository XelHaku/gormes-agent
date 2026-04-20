package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// TestKernel_InitialSessionIDPrimesFirstRequest proves that InitialSessionID
// on kernel.Config is copied into k.sessionID before the Run loop starts,
// so the first outbound ChatRequest carries that session_id in the
// X-Hermes-Session-Id header. Without this, --resume would silently no-op.
func TestKernel_InitialSessionIDPrimesFirstRequest(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "ok", TokensOut: 1},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1},
	}, "sess-from-server")

	k := New(Config{
		Model:            "hermes-agent",
		Endpoint:         "http://mock",
		Admission:        Admission{MaxBytes: 200_000, MaxLines: 10_000},
		InitialSessionID: "sess-primed-from-disk",
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // initial idle

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	reqs := mc.Requests()
	if len(reqs) == 0 {
		t.Fatal("mock client received zero requests")
	}
	if got := reqs[0].SessionID; got != "sess-primed-from-disk" {
		t.Errorf("first request.SessionID = %q, want %q", got, "sess-primed-from-disk")
	}
}

// TestKernel_InitialSessionIDEmptyKeepsExistingBehavior proves zero-value
// InitialSessionID does not change existing kernel behavior — all prior
// kernel tests must remain unaffected.
func TestKernel_InitialSessionIDEmptyKeepsExistingBehavior(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-fresh")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"})
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle
	}, 2*time.Second)

	reqs := mc.Requests()
	if len(reqs) == 0 {
		t.Fatal("zero requests")
	}
	if got := reqs[0].SessionID; got != "" {
		t.Errorf("first request.SessionID = %q, want \"\" (zero-value Initial)", got)
	}
}
