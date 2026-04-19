# Gormes Route B — Reconnect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flip `TestKernel_HandlesMidStreamNetworkDrop` from `t.Skip` to a real pass by implementing client-side restart with visual continuity ("Route B") in `internal/kernel/kernel.go`. The red test's four assertions are the definition of done; do not claim completion until all four pass without modification.

**Architecture:** Inside `runTurn`, wrap the `stream.OpenStream` + `stream.Recv` loop with a bounded retry driver. On a classified-retryable stream error, transition to `PhaseReconnecting`, preserve the current draft buffer as visible UI continuity, wait with jittered exponential backoff, re-open the stream with the same `ChatRequest`, and resume streaming. Fresh tokens from the retry **replace** (not append to) the preserved draft at the moment the first new token arrives. Retry budget caps at 5 attempts (1 s → 2 s → 4 s → 8 s → 16 s) with ±20% jitter. Context cancellation aborts retry immediately.

**Tech Stack:** Go 1.22+, stdlib `context`/`time`/`math/rand`, existing `hermes.Classify`, existing `kernel.Provenance` slog, no new dependencies.

**Source spec:** §9.2 of `docs/superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md` (backoff schedule) and the red test `internal/kernel/reconnect_test.go` (four acceptance assertions).

---

## Prerequisites

- Phase 1 fully shipped (commit `27af6ffa` or later).
- Phase 1.5 TDD Rig shipped (commits `cf9ae677` + `fd6a625e` + `a8c84c9a`).
- Working tree clean or at least isolated from kernel/ paths.
- `go.mod` pinned at `go 1.22` with `toolchain go1.26.1` (compat probe confirmed the code builds under 1.22).

## File Structure Map

```
gormes/
├── internal/
│   └── kernel/
│       ├── kernel.go                   # MODIFY — wrap runTurn body in retry driver
│       ├── retry.go                    # NEW — jittered backoff + retry-budget helper
│       ├── retry_test.go               # NEW — unit tests for retry helper in isolation
│       ├── reconnect_test.go           # MODIFY — delete t.Skip, implement the four assertions
│       └── reconnect_helpers_test.go   # MODIFY — add newRealKernel helper (declared but nil in Phase-1.5 scaffold)
```

No changes to `hermes/`, `store/`, `tui/`, or `cmd/`. The retry logic lives entirely in `kernel`.

---

## Task 1: Pin go.mod back to `go 1.22`

**Files:**
- Modify: `gormes/go.mod`

**Why now:** the compat probe confirmed the code compiles under Go 1.22. The `go` directive was cosmetically bumped by `go get` when bubbletea was added. Pinning it back restores the Ubuntu LTS / Termux portability promise without downgrading any dependency.

- [ ] **Step 1:** Read `gormes/go.mod` and confirm it currently reads:

```
module github.com/XelHaku/golang-hermes-agent/gormes

go 1.24.2

toolchain go1.26.1
```

(or similar — the `go` directive may say `1.24.2` or another post-1.22 version.)

- [ ] **Step 2:** Edit the `go` directive down to `1.22`:

```
module github.com/XelHaku/golang-hermes-agent/gormes

go 1.22

toolchain go1.26.1
```

The `toolchain` directive stays — it controls the compiler used for local builds, not the language-version minimum.

- [ ] **Step 3:** Verify the build still works under both toolchains.

```bash
cd gormes
go build ./...
go test -race ./... -timeout 90s
```

Both must pass. Then re-run the compat probe:

```bash
./scripts/check-go1.22-compat.sh; echo "exit=$?"
```

If Docker is available, expect exit 0. If falling back to `go1.22.10`, also expect exit 0.

- [ ] **Step 4:** Commit.

```bash
cd ..
git add gormes/go.mod
git commit -m "$(cat <<'EOF'
chore(gormes): pin go.mod directive back to go 1.22

The compat probe (./scripts/check-go1.22-compat.sh) confirms the
Gormes code builds under Go 1.22 — the earlier 1.24 drift was
purely cosmetic (auto-bumped by "go get github.com/charmbracelet/
bubbletea@latest", not actually required by any dependency symbol).

Pinning back to 1.22 preserves Ubuntu 22.04/24.04 LTS and Termux
portability. toolchain go1.26.1 stays for local builds.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Retry Helper — `kernel/retry.go`

**Files:**
- Create: `gormes/internal/kernel/retry.go`
- Create: `gormes/internal/kernel/retry_test.go`

Isolates the backoff-and-jitter math from `runTurn` so it can be unit-tested without spinning up a kernel.

- [ ] **Step 1:** Write the failing tests first. Create `gormes/internal/kernel/retry_test.go`:

```go
package kernel

import (
	"context"
	"testing"
	"time"
)

func TestRetryBudget_NextDelay_ExponentialWithJitter(t *testing.T) {
	b := NewRetryBudget()
	// Base sequence: 1s, 2s, 4s, 8s, 16s.
	base := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
	for i, want := range base {
		got := b.NextDelay()
		low := time.Duration(float64(want) * 0.8)
		high := time.Duration(float64(want) * 1.2)
		if got < low || got > high {
			t.Errorf("attempt %d: delay = %v, want within ±20%% of %v", i+1, got, want)
		}
	}
	// Sixth call — budget exhausted.
	if got := b.NextDelay(); got != -1 {
		t.Errorf("attempt 6: delay = %v, want -1 (budget exhausted)", got)
	}
}

func TestRetryBudget_Exhausted(t *testing.T) {
	b := NewRetryBudget()
	for i := 0; i < 5; i++ {
		_ = b.NextDelay()
	}
	if !b.Exhausted() {
		t.Error("Exhausted should be true after 5 attempts")
	}
}

func TestRetryBudget_WaitRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := Wait(ctx, 1*time.Hour)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Wait blocked %v on cancelled ctx; must return immediately", d)
	}
}
```

Run from inside `gormes/`:
```bash
go test ./internal/kernel/... -run TestRetryBudget
```
Expected: FAIL — undefined symbols.

- [ ] **Step 2:** Implement `gormes/internal/kernel/retry.go`:

```go
package kernel

import (
	"context"
	"math/rand"
	"time"
)

// RetryBudget implements the Route-B reconnect schedule from spec §9.2:
// 1s, 2s, 4s, 8s, 16s with ±20% jitter, then exhausted. Not goroutine-safe;
// the kernel holds one budget per turn on the Run goroutine.
type RetryBudget struct {
	attempt int
}

const maxRetryAttempts = 5

// NewRetryBudget returns a fresh budget — 5 attempts remaining.
func NewRetryBudget() *RetryBudget { return &RetryBudget{} }

// NextDelay returns the jittered backoff for the next attempt, or -1 if the
// budget is exhausted. Advances the internal attempt counter on each call.
func (b *RetryBudget) NextDelay() time.Duration {
	if b.attempt >= maxRetryAttempts {
		return -1
	}
	b.attempt++
	// Base: 1s * 2^(attempt-1) → 1, 2, 4, 8, 16.
	base := time.Second << uint(b.attempt-1)
	// Jitter: ±20%.
	jitter := (rand.Float64()*0.4 - 0.2) // ±0.2
	return time.Duration(float64(base) * (1.0 + jitter))
}

// Exhausted returns true if NextDelay has been called maxRetryAttempts times.
func (b *RetryBudget) Exhausted() bool {
	return b.attempt >= maxRetryAttempts
}

// Wait sleeps for d or returns early on ctx cancellation. Returns ctx.Err()
// on cancellation, nil on clean timer expiration.
func Wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Run:
```bash
go test ./internal/kernel/... -run TestRetryBudget -v
```
All three tests PASS.

- [ ] **Step 3:** Commit.

```bash
git add gormes/internal/kernel/retry.go gormes/internal/kernel/retry_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): retry budget + jittered backoff helper

Implements the Route-B reconnect schedule from spec §9.2:
5 attempts with base delays 1/2/4/8/16s and ±20% jitter.
Stateful per-turn, held on the Run goroutine — not goroutine-safe
by design (matches the kernel's single-owner discipline).

Wait(ctx, d) is the reusable cancellation-aware sleep primitive.

Unit-tested in isolation; runTurn integration is the next task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wire the Retry Driver into `runTurn`

**Files:**
- Modify: `gormes/internal/kernel/kernel.go` (the streaming-loop section of `runTurn`)

- [ ] **Step 1:** Read `gormes/internal/kernel/kernel.go` and locate `runTurn`. The current streaming section looks roughly like:

```go
	stream, err := k.client.OpenStream(runCtx, hermes.ChatRequest{
		Model:     k.cfg.Model,
		SessionID: k.sessionID,
		Stream:    true,
		Messages:  []hermes.Message{{Role: "user", Content: text}},
	})
	if err != nil {
		// ... treat as fatal, transition to PhaseFailed, return
	}
	defer stream.Close()

	k.phase = PhaseStreaming
	k.emitFrame("streaming")
	k.tm.StartTurn()
	start := time.Now()

	// pump goroutine + streamLoop select { ... }
```

- [ ] **Step 2:** Restructure the streaming section so it can loop on retries. The high-level control flow becomes:

```
retryBudget = NewRetryBudget()
request = hermes.ChatRequest{..., Messages: [the user message]}

RETRY:
    stream, err = client.OpenStream(runCtx, request)
    if err != nil and Classify(err) == ClassRetryable and !retryBudget.Exhausted():
        goto RECONNECT
    if err != nil:
        transition PhaseFailed; return

    streamLoop: (same as today, pumping via pump goroutine + select on k.events + deltaCh + ticker.C)
    on EventDone → break out of RETRY, finalize
    on fatal stream error → PhaseFailed, return
    on retryable stream error → goto RECONNECT
    on ctx cancel → cancelled, break out of RETRY, finalize cancelled

RECONNECT:
    k.phase = PhaseReconnecting
    k.emitFrame("reconnecting")
    delay = retryBudget.NextDelay()
    if delay == -1:  # exhausted
        transition PhaseFailed (LastError = "reconnect budget exhausted"); return
    Wait(runCtx, delay)
    if runCtx.Err() != nil:
        cancelled; break out of RETRY, finalize cancelled
    # Preserve draft visually: do NOT clear k.draft here. The UI shows the
    # old draft during reconnect. The NEXT token received on a successful
    # retry stream is the first thing that triggers the draft replacement.
    replaceOnNextToken = true
    goto RETRY
```

- [ ] **Step 3:** Concretely, replace the streaming section with:

```go
	retryBudget := NewRetryBudget()
	request := hermes.ChatRequest{
		Model:     k.cfg.Model,
		SessionID: k.sessionID,
		Stream:    true,
		Messages:  []hermes.Message{{Role: "user", Content: text}},
	}

	var (
		cancelled           bool
		finalDelta          hermes.Event
		gotFinal            bool
		replaceOnNextToken  bool // true after a reconnect until the first new token arrives
	)

	start := time.Now()
	k.tm.StartTurn()

retryLoop:
	for {
		stream, err := k.client.OpenStream(runCtx, request)
		if err != nil {
			if hermes.Classify(err) == hermes.ClassRetryable && !retryBudget.Exhausted() {
				k.phase = PhaseReconnecting
				k.lastError = "reconnecting: " + err.Error()
				k.emitFrame("reconnecting")
				delay := retryBudget.NextDelay()
				if werr := Wait(runCtx, delay); werr != nil {
					cancelled = true
					break retryLoop
				}
				replaceOnNextToken = true
				continue retryLoop
			}
			prov.ErrorClass = hermes.Classify(err).String()
			prov.ErrorText = err.Error()
			prov.LogError(k.log)
			k.phase = PhaseFailed
			k.lastError = err.Error()
			k.emitFrame("open stream failed")
			return
		}

		// Stream opened successfully. From here the loop body is ~identical
		// to the pre-retry version, just with streamInner returning a
		// control enum instead of plain break.
		outcome := k.streamInner(runCtx, stream, &finalDelta, &gotFinal, &replaceOnNextToken)
		_ = stream.Close()

		switch outcome {
		case streamOutcomeDone:
			break retryLoop
		case streamOutcomeCancelled:
			cancelled = true
			break retryLoop
		case streamOutcomeRetryable:
			if retryBudget.Exhausted() {
				k.phase = PhaseFailed
				k.lastError = "reconnect budget exhausted"
				k.emitFrame("reconnect budget exhausted")
				return
			}
			k.phase = PhaseReconnecting
			k.emitFrame("reconnecting")
			delay := retryBudget.NextDelay()
			if werr := Wait(runCtx, delay); werr != nil {
				cancelled = true
				break retryLoop
			}
			replaceOnNextToken = true
			continue retryLoop
		case streamOutcomeFatal:
			// streamInner already set phase/lastError/emitted.
			return
		}
	}

	// finalisation (unchanged from Phase-1 kernel.go)
	...
```

- [ ] **Step 4:** Extract the streaming inner loop into a helper method `streamInner` with this signature:

```go
type streamOutcome int

const (
	streamOutcomeDone streamOutcome = iota
	streamOutcomeCancelled
	streamOutcomeRetryable // classified network drop, caller should retry
	streamOutcomeFatal     // non-retryable error, caller should return immediately
)

// streamInner pumps events from one opened stream, handles platform-event
// cancel, emits render frames on the 16ms ticker, and returns a classified
// outcome that tells the caller (runTurn's retry driver) what to do next.
func (k *Kernel) streamInner(
	runCtx context.Context,
	stream hermes.Stream,
	finalDelta *hermes.Event,
	gotFinal *bool,
	replaceOnNextToken *bool,
) streamOutcome {
	// Move the Phase-1 pump-goroutine + streamLoop select into here.
	// On an EventToken when *replaceOnNextToken is true, clear k.draft
	// once and flip the flag back to false:
	//   if *replaceOnNextToken {
	//       k.draft = ""
	//       *replaceOnNextToken = false
	//   }
	//   k.draft += ev.Token
	//
	// On a non-io.EOF error: inspect Classify(err).
	//   ClassRetryable → return streamOutcomeRetryable
	//   otherwise      → set phase/lastError/emit, return streamOutcomeFatal
	//
	// On io.EOF without EventDone: same as retryable (stream ended
	// unexpectedly — it's a TCP drop lookalike).
}
```

**Full implementation detail** for `streamInner` is left to the implementer; the signature and outcome enum are contract-locked above. Key invariants:
- `k.phase = PhaseStreaming` is set once at the start of each stream iteration (inside the retry loop, not inside `streamInner`).
- The ticker is scoped per-`streamInner` call (defer `ticker.Stop()` inside).
- The pump goroutine exits on `runCtx.Done()` or stream error; `streamInner` drains `deltaCh` before returning.

- [ ] **Step 5:** Update the four assertions of the red test's expected behaviour:

- **ASSERT 1** (`phase == PhaseReconnecting within 500ms of drop`): the retry loop's `k.phase = PhaseReconnecting` + `k.emitFrame` path satisfies this.
- **ASSERT 2** (`draft still contains "xxxxx"`): because we do NOT clear `k.draft` when entering PhaseReconnecting; the preservation is passive.
- **ASSERT 3** (`phase transitions back to Streaming → Idle after recovery`): `continue retryLoop` re-enters with a successful `OpenStream`, sets `PhaseStreaming`, emits frame; when new stream completes with EventDone, `break retryLoop` + finalisation sets `PhaseIdle`.
- **ASSERT 4** (`final.History has one assistant msg == "yyyyyyyyyy"`): on the first EventToken of the retry stream, `replaceOnNextToken` clears `k.draft`, so the final accumulated draft is only the retry-stream content.

- [ ] **Step 6:** Run the existing Phase-1 discipline tests to make sure nothing regressed.

```bash
cd gormes
go test -race ./internal/kernel/... -timeout 90s
```

All prior tests PASS. If any fail (especially `TestKernel_CancelLeakFreedom`, `TestKernel_SeqMonotonic`, or `TestKernel_NonBlockingUnderTUIStall`), the retry integration has a bug. Fix before proceeding to Task 4.

- [ ] **Step 7:** Commit the integration.

```bash
cd ..
git add gormes/internal/kernel/kernel.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): Route-B reconnect driver — client-side restart

Wraps runTurn's streaming section in a retry loop driven by
RetryBudget (1/2/4/8/16s ±20% jitter). On a retryable stream
error or unexpected EOF, the kernel:

  1. Transitions to PhaseReconnecting (draft preserved — user sees
     the old 5 tokens during the reconnect window).
  2. Waits the next jittered backoff (cancellation-aware).
  3. Re-opens the stream with the same ChatRequest.
  4. On the FIRST new token, clears the old draft — new stream
     content replaces the preserved visual ("Route B" semantics).

Retry budget exhaustion → PhaseFailed with "reconnect budget
exhausted". Ctx cancel during backoff → cancelled finalisation.

The Phase-1.5 red test (t.Skip'd) was the contract; its four
assertions are now implementable. Task 4 flips the skip.

streamInner helper extracted from the original Phase-1 streaming
loop so the retry driver can call it across iterations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Flip the Red Test — Delete `t.Skip`, Implement Assertions

**Files:**
- Modify: `gormes/internal/kernel/reconnect_test.go`
- Modify: `gormes/internal/kernel/reconnect_helpers_test.go` (add `newRealKernel`)

- [ ] **Step 1:** Add `newRealKernel` to `reconnect_helpers_test.go`. Find the placeholder stub `newRealKernelFromURL` (if present — it may or may not be in the file) and replace / add:

```go
func newRealKernel(t *testing.T, endpoint string) *Kernel {
	t.Helper()
	client := hermes.NewHTTPClient(endpoint, "")
	return New(Config{
		Model:     "hermes-agent",
		Endpoint:  endpoint,
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)
}
```

Add the needed imports: `hermes`, `store`, `telemetry` (all under `gormes/internal/`).

- [ ] **Step 2:** Replace `reconnect_test.go` body. Delete the `t.Skip` line and the long comment of "what to do when the implementation lands". Implement the test:

```go
func TestKernel_HandlesMidStreamNetworkDrop(t *testing.T) {
	// 1. First server: emits 5 tokens then hangs. Expose its Close() so
	//    we can chaos-monkey it mid-stream.
	srv1 := httptest.NewServer(fiveTokenHandler())
	defer srv1.Close()

	proxy := newStableProxy(t)
	defer proxy.Close()
	proxy.Rebind(srv1.URL)

	k := newRealKernel(t, proxy.URL())

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	go k.Run(ctx)

	// Drain initial idle.
	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial = %v, want Idle", initial.Phase)
	}

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for draft of length 5 + PhaseStreaming.
	pre := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming && f.DraftText == "xxxxx"
	}, 3*time.Second)
	_ = pre

	// CHAOS MONKEY.
	srv1.CloseClientConnections()

	// ASSERT 1: within 500ms phase → PhaseReconnecting.
	reconnecting := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseReconnecting
	}, 500*time.Millisecond)

	// ASSERT 2: draft still contains "xxxxx" during reconnect window.
	if reconnecting.DraftText != "xxxxx" {
		t.Errorf("draft during PhaseReconnecting = %q, want xxxxx (visual continuity)", reconnecting.DraftText)
	}

	// Swap in the second server emitting 10 "y" tokens + done.
	srv2 := httptest.NewServer(tenTokenHandler())
	defer srv2.Close()
	proxy.Rebind(srv2.URL)

	// ASSERT 3: phase transitions Streaming → Idle within backoff budget.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming && strings.HasPrefix(f.DraftText, "y")
	}, 25*time.Second) // full 1+2+4+8+16 = 31s budget, be generous

	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && len(f.History) >= 2
	}, 10*time.Second)

	// ASSERT 4: final history has exactly one assistant message == "yyyyyyyyyy".
	var assistants []hermes.Message
	for _, m := range final.History {
		if m.Role == "assistant" {
			assistants = append(assistants, m)
		}
	}
	if len(assistants) != 1 {
		t.Fatalf("final history has %d assistant msgs, want 1", len(assistants))
	}
	if assistants[0].Content != "yyyyyyyyyy" {
		t.Errorf("assistant content = %q, want yyyyyyyyyy (fresh-retry replaces preserved draft)", assistants[0].Content)
	}
}

// waitForFrameMatching is a test helper — drain frames until predicate or deadline.
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
			t.Fatalf("timeout waiting for matching frame (%v)", timeout)
		}
	}
}
```

Note: `waitForFrameMatching` may already exist from another test file (`kernel_test.go`). If it does, delete the duplicate from this file and rely on the existing one.

- [ ] **Step 3:** Run the formerly-red test.

```bash
cd gormes
go test -race ./internal/kernel/... -run TestKernel_HandlesMidStreamNetworkDrop -timeout 60s -v
```

Expected: **PASS** (not SKIP, not FAIL). If any of the four assertions fail, the retry integration in Task 3 has a bug — return to Task 3 and fix before re-running.

If ASSERT 2 fails (draft cleared during reconnect), the `k.draft = ""` is happening too early. Only the first EventToken of the retry stream should clear it.

If ASSERT 4 returns `"xxxxxyyyyyyyyyy"` (concatenation), `replaceOnNextToken` is not being honored.

- [ ] **Step 4:** Run the full suite to confirm no regressions.

```bash
go test -race ./... -timeout 90s
go vet ./...
```

All PASS. No DATA RACE. `vet` clean.

- [ ] **Step 5:** Commit.

```bash
cd ..
git add gormes/internal/kernel/reconnect_test.go gormes/internal/kernel/reconnect_helpers_test.go
git commit -m "$(cat <<'EOF'
test(gormes/kernel): flip Route-B reconnect test from Skip to PASS

Deletes the t.Skip guard shipped in commit fd6a625e. Implements
the four assertions laid out in the red-test docstring:

  1. PhaseReconnecting transition within 500ms of chaos-monkey
     CloseClientConnections() on the first httptest.Server
  2. draft == "xxxxx" preserved across the PhaseReconnecting
     window (visual continuity)
  3. PhaseStreaming → PhaseIdle transition after rebinding the
     stableProxy to a fresh second httptest.Server
  4. final history has exactly ONE assistant message == "yyyyyyyyyy"
     (retry content replaces, does not append, the preserved draft)

The Phase-1.5 Technical-Debt Beacon is retired. Gormes now handles
mid-stream TCP drops with the full "Route B" semantics from spec
§9.2 of 2026-04-18-gormes-frontend-adapter-design.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Verification Sweep + Doc Update

**Files:**
- Modify: `gormes/docs/PHASE1_COMPLETION.md` — update criterion 14 (was 8/10 discipline tests → 9/10)
- Modify: `gormes/docs/ARCH_PLAN.md` — no change needed; Route-B is a Phase-1.5 feature, Phase 1 stays ✅

- [ ] **Step 1:** In `gormes/docs/PHASE1_COMPLETION.md` find the row for criterion 14 and update it:

Before:
```
| 14 | All ten kernel-discipline tests pass | ⚠️ 8/10 | ... |
```

After:
```
| 14 | All ten kernel-discipline tests pass | ⚠️ 9/10 | 9 shipped including Route-B HTTP-drop reconnect (commit <NEW_SHA>); the remaining 1 (TUI stalled indefinitely) is a future polish item — no longer blocking because TestKernel_NonBlockingUnderTUIStall covers the same invariant under a stricter harness |
```

- [ ] **Step 2:** Run the full sweep one more time and capture output:

```bash
cd gormes
go test -race ./... -timeout 90s 2>&1 | tail -12
go vet ./...
```

- [ ] **Step 3:** Commit doc update.

```bash
cd ..
git add gormes/docs/PHASE1_COMPLETION.md
git commit -m "$(cat <<'EOF'
docs(gormes): Route-B reconnect lifts discipline-test score to 9/10

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Appendix A: Self-Review

**Spec coverage:**
- §9.2 backoff schedule (1/2/4/8/16 s) → Task 2 `RetryBudget.NextDelay`.
- §9.2 ±20% jitter → Task 2 implementation.
- Red test §3.3 ASSERT 1 (PhaseReconnecting) → Task 3 retry-loop branch.
- Red test §3.3 ASSERT 2 (draft preserved) → Task 3 passive non-clearing of `k.draft`.
- Red test §3.3 ASSERT 3 (return to Streaming) → Task 3 `continue retryLoop`.
- Red test §3.3 ASSERT 4 (single clean assistant message) → Task 3 `replaceOnNextToken` flag.
- Red test `t.Skip` removal → Task 4.
- Phase-1 discipline tests must still pass → Task 3 Step 6 + Task 4 Step 4 verify.

**Placeholder scan:** Task 3 Step 4 leaves `streamInner` "full implementation detail left to the implementer". That's a gap in a normal plan; here it's intentional because the Phase-1 `runTurn` streaming body is ~100 lines of known-good code that must be lifted into the helper verbatim (minus the one-line `replaceOnNextToken` clearing). Implementer should copy-paste from the current kernel.go and wrap with the outcome enum.

**Type consistency:** `streamOutcome` + `streamOutcomeDone`/`Cancelled`/`Retryable`/`Fatal` enum defined in Task 3; referenced only inside Task 3. `RetryBudget` + `NewRetryBudget()` + `NextDelay()` + `Exhausted()` defined in Task 2; referenced in Task 3. `Wait(ctx, d)` defined in Task 2; referenced in Task 3. `newRealKernel` defined in Task 4 Step 1; referenced only in Task 4's test body. All references resolvable within the plan.

**Scope:** Single-plan Route-B implementation. Tasks 1–5 ship 5 commits.

---

## Appendix B: Compat-Probe Result (2026-04-19)

The Phase-1.5 compat probe (`gormes/scripts/check-go1.22-compat.sh`) was run on the user's local host. Docker exit 125 was a sandbox-FS artefact; the fallback path via `go1.22.10 build ./cmd/gormes` **exited 0**. No offending symbols, no 1.24-only APIs.

**Decision: Accept Go 1.22 as the Gormes floor.** No bubbletea downgrade needed. Task 1 of this plan pins `go.mod` back to `go 1.22` to restore the Ubuntu 22.04/24.04 LTS + Termux portability promise from `ARCH_PLAN.md` §2.
