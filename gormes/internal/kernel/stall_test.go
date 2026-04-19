package kernel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// TestKernel_NonBlockingUnderTUIStall proves the capacity-1 replace-latest
// render mailbox invariant (spec §7.8) holds under a maliciously-stalled
// consumer. If the kernel ever blocks on an emitFrame send — e.g. if a
// future refactor changed the mailbox from capacity-1 + drain-then-send to
// an unbuffered or blocking channel — this test deadlocks and fails the
// 5-second timeout.
//
// Treats the kernel as a black box: we inspect the render channel only,
// never internal state. No test-only accessors on production types.
func TestKernel_NonBlockingUnderTUIStall(t *testing.T) {
	mc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, 1001)
	for i := 0; i < 1000; i++ {
		events = append(events, hermes.Event{
			Kind: hermes.EventToken, Token: "t", TokensOut: i + 1,
		})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1000})
	mc.Script(events, "sess-stall")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)

	// Read only the initial idle frame so the kernel enters its main select,
	// then submit and stop reading. The replace-latest invariant says the
	// kernel must keep making progress even though nobody consumes frames.
	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial = %v, want PhaseIdle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// STALL: do NOT drain k.Render() for 2 seconds. If the kernel blocks
	// on emit, it will not complete the 1000-token turn in this window.
	time.Sleep(2 * time.Second)

	// Peek the single frame in the capacity-1 mailbox. It must be the
	// LATEST state — a stale mid-stream frame violates replace-latest.
	var peeked RenderFrame
	select {
	case peeked = <-k.Render():
	default:
		t.Fatal("no frame available after 2s stall — kernel may have deadlocked on emit")
	}

	// Valid peek states:
	//   (a) Idle with assistant history "t"*1000  — turn fully completed
	//   (b) Streaming with DraftText "t"*1000     — very-late mid-stream frame
	//   (c) Finalizing with DraftText "t"*1000    — edge-case finalization frame
	// A stale mid-stream frame with partial draft is a FAILURE.
	ok := false
	wantAssistant := strings.Repeat("t", 1000)

	if peeked.Phase == PhaseIdle {
		assistant := lastAssistantMessage(peeked.History)
		if assistant != nil && assistant.Content == wantAssistant {
			ok = true
		}
	}
	if peeked.Phase == PhaseStreaming && peeked.DraftText == wantAssistant {
		ok = true
	}
	if peeked.Phase == PhaseFinalizing && peeked.DraftText == wantAssistant {
		ok = true
	}

	if !ok {
		t.Fatalf("replace-latest invariant violated — peeked stale frame: phase=%v seq=%d draftLen=%d historyLen=%d",
			peeked.Phase, peeked.Seq, len(peeked.DraftText), len(peeked.History))
	}

	// Drain remaining frames so the kernel exits cleanly on ctx timeout.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range k.Render() {
		}
	}()
	<-ctx.Done()
	<-done
}

// lastAssistantMessage returns a pointer to the last hermes.Message with
// Role "assistant", or nil if none exist.
func lastAssistantMessage(history []hermes.Message) *hermes.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" {
			return &history[i]
		}
	}
	return nil
}
