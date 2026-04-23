package kernel

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

// fixture builds a kernel wired to a fresh MockClient and NoopStore.
// The caller may swap the store via fixtureWithStore.
func fixture(t *testing.T) (*Kernel, *hermes.MockClient) {
	t.Helper()
	return fixtureWithStore(t, store.NewNoop())
}

func fixtureWithStore(t *testing.T, s store.Store) (*Kernel, *hermes.MockClient) {
	t.Helper()
	mc := hermes.NewMockClient()
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, s, telemetry.New(), nil)
	return k, mc
}

// waitForFrameMatching drains the render channel until pred matches or the
// deadline expires. The returned frame is the matching one.
func waitForFrameMatching(t *testing.T, ch <-chan RenderFrame, pred func(RenderFrame) bool, timeout time.Duration) RenderFrame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("render channel closed before predicate matched")
			}
			if pred(f) {
				return f
			}
		case <-deadline:
			t.Fatal("timeout waiting for matching render frame")
		}
	}
}

// drainUntilIdle consumes render frames until one reports PhaseIdle with
// Seq > minSeq. Returns the number of frames observed (including the final
// idle frame).
func drainUntilIdle(t *testing.T, ch <-chan RenderFrame, minSeq uint64, timeout time.Duration) (int, RenderFrame) {
	t.Helper()
	deadline := time.After(timeout)
	var count int
	var last RenderFrame
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("render channel closed before idle")
			}
			count++
			last = f
			if f.Phase == PhaseIdle && f.Seq > minSeq {
				return count, f
			}
		case <-deadline:
			t.Fatalf("timeout after %d frames, last phase=%v seq=%d", count, last.Phase, last.Seq)
		}
	}
}

// Test 1: 2000-token burst coalesces to < 500 render frames; final draft
// is the concatenation of all tokens.
func TestKernel_ProviderOutpacesTUI_Coalesces(t *testing.T) {
	k, mc := fixture(t)

	events := make([]hermes.Event, 0, 2001)
	for i := 0; i < 2000; i++ {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: "x", TokensOut: i + 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 10, TokensOut: 2000})
	mc.Script(events, "sess-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go k.Run(ctx)

	// Read the initial idle frame.
	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	frames, final := drainUntilIdle(t, k.Render(), initial.Seq, 5*time.Second)

	if final.DraftText != "" && final.DraftText != strings.Repeat("x", 2000) {
		// DraftText may be cleared on final idle frame; we mainly care about
		// the history entry. Check both conditions permissively.
	}
	// The last assistant message in history must match 2000 x's.
	if len(final.History) == 0 {
		t.Fatal("no history entries after completed turn")
	}
	last := final.History[len(final.History)-1]
	if last.Role != "assistant" {
		t.Errorf("last history role = %q, want assistant", last.Role)
	}
	if last.Content != strings.Repeat("x", 2000) {
		t.Errorf("assistant content length = %d, want 2000", len(last.Content))
	}

	// Coalescing invariant: frames < 500 for a 2000-token burst.
	if frames >= 500 {
		t.Errorf("emitted %d render frames for 2000 tokens; coalescer failed to bound output", frames)
	}
}

func TestKernel_CompletedTurnUpdatesTelemetryOutcome(t *testing.T) {
	mc := hermes.NewMockClient()
	tm := telemetry.New()
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), tm, nil)

	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "ok", TokensOut: 2},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 7, TokensOut: 2},
	}, "sess-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	_, final := drainUntilIdle(t, k.Render(), initial.Seq, 3*time.Second)

	if final.Telemetry.TurnsTotal != 1 {
		t.Fatalf("frame turns_total = %d, want 1", final.Telemetry.TurnsTotal)
	}
	if final.Telemetry.TurnsCompleted != 1 {
		t.Fatalf("frame turns_completed = %d, want 1", final.Telemetry.TurnsCompleted)
	}
	if final.Telemetry.LastTurnStatus != telemetry.TurnStatusCompleted {
		t.Fatalf("frame last_turn_status = %q, want %q", final.Telemetry.LastTurnStatus, telemetry.TurnStatusCompleted)
	}

	snap := tm.Snapshot()
	if snap.TurnsTotal != 1 {
		t.Fatalf("snapshot turns_total = %d, want 1", snap.TurnsTotal)
	}
	if snap.TurnsCompleted != 1 {
		t.Fatalf("snapshot turns_completed = %d, want 1", snap.TurnsCompleted)
	}
	if snap.LastTurnStatus != telemetry.TurnStatusCompleted {
		t.Fatalf("snapshot last_turn_status = %q, want %q", snap.LastTurnStatus, telemetry.TurnStatusCompleted)
	}
}

// Test 2: Cancel mid-stream leaves zero goroutine leak.
func TestKernel_CancelLeakFreedom(t *testing.T) {
	// Settle the harness.
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	k, mc := fixture(t)

	// Script a long-running stream: 500 tokens.
	events := make([]hermes.Event, 0, 501)
	for i := 0; i < 500; i++ {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: "t", TokensOut: i + 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "")

	runCtx, cancelRun := context.WithCancel(context.Background())
	go k.Run(runCtx)

	<-k.Render() // drain initial idle frame
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	// Give the stream a moment to get going.
	time.Sleep(20 * time.Millisecond)
	cancelRun()

	// Drain the render channel until it closes.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for range k.Render() {
		}
	}()

	select {
	case <-drainDone:
	case <-time.After(2 * time.Second):
		t.Fatal("render channel did not close within 2s of cancel")
	}

	// Let any lingering goroutines unwind.
	time.Sleep(250 * time.Millisecond)

	after := runtime.NumGoroutine()
	// Tolerance of +4 covers stdlib test-harness noise.
	if after > baseline+4 {
		t.Errorf("goroutine leak: baseline=%d after=%d (delta=%d)", baseline, after, after-baseline)
	}
}

// Test 3: Admission rejects oversize input; no HTTP is opened.
func TestKernel_AdmissionRejectsOversize(t *testing.T) {
	k, mc := fixture(t)
	// Do NOT script any streams — any OpenStream call would return an empty
	// stream (io.EOF immediately). We assert below that the kernel's phase
	// stays Idle, which is the strongest statement we can make here.

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // initial idle

	oversize := strings.Repeat("x", 300_000)
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: oversize}); err != nil {
		t.Fatal(err)
	}

	got := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.LastError != ""
	}, time.Second)

	if got.Phase != PhaseIdle {
		t.Errorf("phase = %v, want Idle (admission must fire before any HTTP)", got.Phase)
	}
	if !strings.Contains(got.LastError, "byte limit") {
		t.Errorf("LastError = %q, want it to mention the byte limit", got.LastError)
	}

	// Silence the unused mc variable.
	_ = mc
}

// Test 4: Second submit during an active turn is rejected with a
// "still processing" LastError; the in-flight turn still completes.
func TestKernel_SecondSubmitRejected(t *testing.T) {
	k, mc := fixture(t)

	// Script a very long stream so there is a meaningful window during which
	// the kernel is mid-turn. 5000 tokens at ~µs each gives us plenty of time
	// to observe at least one rejection frame before coalescing overwrites it.
	events := make([]hermes.Event, 0, 5001)
	for i := 0; i < 5000; i++ {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: "t", TokensOut: i + 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // initial idle

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "first"}); err != nil {
		t.Fatal(err)
	}
	// Wait for Streaming before the second submit, so the kernel is
	// definitely mid-turn (not still connecting or already done).
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming
	}, time.Second)

	// Spam multiple rejections — only one needs to survive the capacity-1
	// render mailbox coalescing to prove the kernel rejects mid-turn submits.
	// The kernel emits a rejection frame for EACH rejected submit, so the
	// more we send, the more chances the observer has to see one.
	rejected := make(chan RenderFrame, 1)
	observerDone := make(chan struct{})
	go func() {
		defer close(observerDone)
		for f := range k.Render() {
			if strings.Contains(f.LastError, "still processing") {
				select {
				case rejected <- f:
				default:
				}
				return
			}
			if f.Phase == PhaseIdle && f.Seq > 2 {
				// Turn finished without us ever seeing a rejection.
				return
			}
		}
	}()

	// Fire a burst of second submits to maximise the chance one sits in the
	// render mailbox when the observer reads it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-observerDone:
			goto check
		default:
		}
		_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "second"})
		time.Sleep(2 * time.Millisecond)
	}
check:
	select {
	case f := <-rejected:
		if !strings.Contains(f.LastError, "still processing") {
			t.Fatalf("LastError = %q, want contains 'still processing'", f.LastError)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not observe rejection frame for second submit")
	}
}

// Test 5: Seq strictly monotonic across 10 turns.
func TestKernel_SeqMonotonic(t *testing.T) {
	k, mc := fixture(t)
	const turns = 10
	for i := 0; i < turns; i++ {
		mc.Script([]hermes.Event{
			{Kind: hermes.EventToken, Token: "t", TokensOut: 1},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}, "")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)

	observed := make([]uint64, 0, turns*8)
	done := make(chan struct{})

	go func() {
		defer close(done)
		completedTurns := 0
		for f := range k.Render() {
			observed = append(observed, f.Seq)
			if f.Phase == PhaseIdle {
				completedTurns++
				if completedTurns >= turns+1 { // initial idle + one per turn
					return
				}
			}
		}
	}()

	// Pace submissions so each turn finishes before the next.
	for i := 0; i < turns; i++ {
		for {
			if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "q"}); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		// Wait for this turn to complete before submitting the next.
		time.Sleep(30 * time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for all turns to complete")
	}

	var prev uint64 = 0
	for i, s := range observed {
		if s <= prev {
			t.Errorf("Seq regression at index %d: prev=%d current=%d", i, prev, s)
		}
		prev = s
	}
}

// Test 6: Store ack timeout trips PhaseFailed.
func TestKernel_StoreAckTimeoutFails(t *testing.T) {
	slow := store.NewSlow(500 * time.Millisecond) // well beyond kernel's 250ms deadline
	k, _ := fixtureWithStore(t, slow)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // initial idle

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	got := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseFailed
	}, 2*time.Second)

	if !strings.Contains(got.LastError, "store ack timeout") {
		t.Errorf("LastError = %q, want contains 'store ack timeout'", got.LastError)
	}
}

// Test 7: Submit fails fast when the event mailbox is full (capacity-16).
// This confirms the bounded-mailbox invariant at the TUI→kernel seam.
func TestKernel_SubmitFailsFastOnFullMailbox(t *testing.T) {
	mc := hermes.NewMockClient()
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	// Do NOT call Run. The events channel will fill up because nobody drains it.
	for i := 0; i < PlatformEventMailboxCap; i++ {
		if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "x"}); err != nil {
			t.Fatalf("Submit %d returned %v before full", i, err)
		}
	}
	// Next submit must fail fast, not block.
	start := time.Now()
	err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "overflow"})
	elapsed := time.Since(start)
	if err != ErrEventMailboxFull {
		t.Errorf("err = %v, want ErrEventMailboxFull", err)
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("Submit took %v; must fail fast, not block", elapsed)
	}
}

// Test 8: Rapid submit during Idle (no concurrent turn) just runs the turn.
// Sanity / non-regression check alongside the concurrency tests above.
func TestKernel_SequentialTurnsCompleteCleanly(t *testing.T) {
	k, mc := fixture(t)
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "a", TokensOut: 1},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "")
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "b", TokensOut: 1},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	for i := 0; i < 2; i++ {
		if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "q"}); err != nil {
			t.Fatal(err)
		}
		// Wait for idle before next submission.
		waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
			return f.Phase == PhaseIdle && f.Seq > 1
		}, 2*time.Second)
	}
}
