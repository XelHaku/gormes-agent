# Phase 2.E — Subagent System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Gormes subagent lifecycle core — goroutine-per-subagent execution with context-cancel cascade, depth limit, batched concurrency, ToolExecutor wrapping the real `tools.Tool` interface, and the `delegate_task` tool — using a swappable `Runner` interface whose only implementation in this slice is `StubRunner`. Real LLM execution (`LLMRunner`) and the TUI/CLI surface are deferred to 2.E.7 and 2.E.8.

**Architecture:** A `SubagentManager` spawns a child goroutine per subagent. Each child runs a `Runner` (StubRunner this slice) against a child `context.Context` derived via `WithCancel(parentCtx)` and optionally `WithTimeout`. Events flow runner → internal channel → forwarder goroutine → public read-only channel that closes after the runner returns. Result is published via `setResult` + `close(done)`; consumers use `WaitForResult(ctx)`. Cancellation is recursive through the ctx tree. `SpawnBatch` bounds parallelism with an `errgroup`-coordinated semaphore. `delegate_task` is a Go-native tool registered when `[delegation].enabled = true`.

**Tech Stack:** Go 1.26 stdlib (`context`, `crypto/rand`, `encoding/base32`, `encoding/json`, `errors`, `fmt`, `strings`, `sync`, `sync/atomic`, `time`); `golang.org/x/sync/errgroup` (already in go.sum); existing `internal/tools.Registry`/`internal/tools.Tool`; existing `internal/config`.

**TDD discipline:** Every task = one green commit. Within a task: write failing test → run to confirm red → write minimal impl → run to confirm green → commit. The branch tip is never red. Test invocation is `go test ./internal/subagent/... ./internal/tools/... ./internal/config/... -race -shuffle=on -v`.

---

## File Structure

```
gormes/
  internal/
    subagent/
      types.go         — SubagentConfig, SubagentEvent, SubagentResult, EventType, ResultStatus, ToolCallInfo, ProgressInfo
      types_test.go
      blocked.go       — BlockedTools map + lifecycle constants (MaxDepth, defaults)
      blocked_test.go
      errors.go        — ErrMaxDepth, ErrSubagentNotFound
      errors_test.go
      ids.go           — newSubagentID using crypto/rand
      ids_test.go
      runner.go        — Runner interface + StubRunner
      runner_test.go
      subagent.go      — Subagent struct, Events(), WaitForResult(ctx), setResult
      subagent_test.go
      registry.go      — SubagentRegistry interface + impl
      registry_test.go
      manager.go       — SubagentManager interface, ManagerOpts, manager struct, Spawn, Interrupt, Collect, Close
      manager_test.go
      batch.go         — SpawnBatch
      batch_test.go
      delegate_tool.go — DelegateTool implementing tools.Tool
      delegate_tool_test.go
    tools/
      executor.go      — ToolExecutor interface + InProcessToolExecutor
      executor_test.go
    config/
      config.go        — DelegationCfg added to Config
      config_test.go   — extended with [delegation] decode test
```

Module path prefix: `github.com/TrebuchetDynamics/gormes-agent/gormes`.

---

## Task 1: Subagent types — config, events, result

**Files:**
- Create: `gormes/internal/subagent/types.go`
- Test: `gormes/internal/subagent/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/types_test.go
package subagent

import (
	"testing"
	"time"
)

func TestSubagentConfigZeroValue(t *testing.T) {
	var cfg SubagentConfig
	if cfg.Goal != "" {
		t.Errorf("Goal: want empty, got %q", cfg.Goal)
	}
	if cfg.MaxIterations != 0 {
		t.Errorf("MaxIterations: want 0, got %d", cfg.MaxIterations)
	}
	if cfg.Timeout != 0 {
		t.Errorf("Timeout: want 0, got %v", cfg.Timeout)
	}
	if cfg.EnabledTools != nil {
		t.Errorf("EnabledTools: want nil, got %v", cfg.EnabledTools)
	}
}

func TestEventTypeStringValues(t *testing.T) {
	cases := map[EventType]string{
		EventStarted:     "started",
		EventProgress:    "progress",
		EventToolCall:    "tool_call",
		EventOutput:      "output",
		EventCompleted:   "completed",
		EventFailed:      "failed",
		EventInterrupted: "interrupted",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("EventType: want %q, got %q", want, string(got))
		}
	}
}

func TestResultStatusStringValues(t *testing.T) {
	cases := map[ResultStatus]string{
		StatusCompleted:   "completed",
		StatusFailed:      "failed",
		StatusInterrupted: "interrupted",
		StatusError:       "error",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("ResultStatus: want %q, got %q", want, string(got))
		}
	}
}

func TestSubagentResultZeroValue(t *testing.T) {
	var r SubagentResult
	if r.ID != "" || r.Status != "" || r.Duration != time.Duration(0) {
		t.Errorf("zero-value SubagentResult: unexpected fields populated: %+v", r)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestSubagentConfigZeroValue|TestEventTypeStringValues|TestResultStatusStringValues|TestSubagentResultZeroValue" -v`
Expected: FAIL — `undefined: SubagentConfig` (or similar).

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/types.go

// Package subagent implements goroutine-per-subagent execution isolation
// with deterministic context cancellation, bounded batch concurrency, and
// a swappable Runner interface. See gormes/docs/superpowers/specs/2026-04-20-gormes-phase2e-subagent-design.md.
package subagent

import "time"

// SubagentConfig is the per-subagent configuration handed to the Runner.
// Defaults are applied at Spawn time, not at TOML decode time.
type SubagentConfig struct {
	Goal          string
	Context       string
	MaxIterations int           // 0 → DefaultMaxIterations at Spawn time
	EnabledTools  []string      // empty → all parent tools minus BlockedTools (enforcement deferred to 2.E.7)
	Model         string        // empty → inherit from parent
	Timeout       time.Duration // 0 → no timeout
}

// EventType discriminates SubagentEvent values streamed from runner to parent.
type EventType string

const (
	EventStarted     EventType = "started"
	EventProgress    EventType = "progress"
	EventToolCall    EventType = "tool_call"
	EventOutput      EventType = "output"
	EventCompleted   EventType = "completed"
	EventFailed      EventType = "failed"
	EventInterrupted EventType = "interrupted"
)

// SubagentEvent is a single observation streamed back to the parent during execution.
type SubagentEvent struct {
	Type     EventType
	Message  string
	ToolCall *ToolCallInfo
	Progress *ProgressInfo
}

// ToolCallInfo summarises a tool invocation observed from inside the subagent.
type ToolCallInfo struct {
	Name       string
	ArgsBytes  int
	ResultSize int
	Status     string
}

// ProgressInfo is an iteration tick from a long-running runner.
type ProgressInfo struct {
	Iteration int
	Message   string
}

// ResultStatus is the terminal status of a subagent.
type ResultStatus string

const (
	StatusCompleted   ResultStatus = "completed"
	StatusFailed      ResultStatus = "failed"
	StatusInterrupted ResultStatus = "interrupted"
	StatusError       ResultStatus = "error"
)

// SubagentResult is published exactly once when a subagent finishes.
type SubagentResult struct {
	ID         string
	Status     ResultStatus
	Summary    string
	ExitReason string
	Duration   time.Duration
	Iterations int
	ToolCalls  []ToolCallInfo
	Error      string
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/types.go gormes/internal/subagent/types_test.go
git commit -m "feat(subagent): add SubagentConfig, SubagentEvent, SubagentResult types"
```

---

## Task 2: Lifecycle constants and BlockedTools

**Files:**
- Create: `gormes/internal/subagent/blocked.go`
- Test: `gormes/internal/subagent/blocked_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/blocked_test.go
package subagent

import "testing"

func TestLifecycleConstants(t *testing.T) {
	if MaxDepth != 2 {
		t.Errorf("MaxDepth: want 2, got %d", MaxDepth)
	}
	if DefaultMaxConcurrent != 3 {
		t.Errorf("DefaultMaxConcurrent: want 3, got %d", DefaultMaxConcurrent)
	}
	if DefaultMaxIterations != 50 {
		t.Errorf("DefaultMaxIterations: want 50, got %d", DefaultMaxIterations)
	}
}

func TestBlockedToolsForwardLooking(t *testing.T) {
	want := []string{"delegate_task", "clarify", "memory", "send_message", "execute_code"}
	for _, name := range want {
		if !BlockedTools[name] {
			t.Errorf("BlockedTools[%q]: want true, got false", name)
		}
	}
	if BlockedTools["echo"] {
		t.Errorf("BlockedTools[\"echo\"]: want false (real tool, not blocked), got true")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestLifecycleConstants|TestBlockedToolsForwardLooking" -v`
Expected: FAIL — `undefined: MaxDepth`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/blocked.go
package subagent

const (
	// MaxDepth bounds the subagent depth tree. Parent depth=0; a Spawn at
	// depth >= MaxDepth returns ErrMaxDepth. Default policy: parent → child OK,
	// grandchild rejected.
	MaxDepth = 2

	// DefaultMaxConcurrent is SpawnBatch's default semaphore size when the
	// caller passes maxConcurrent <= 0.
	DefaultMaxConcurrent = 3

	// DefaultMaxIterations is the per-subagent iteration budget applied at
	// Spawn time when SubagentConfig.MaxIterations <= 0. The StubRunner
	// ignores this; LLMRunner (2.E.7) will honour it.
	DefaultMaxIterations = 50
)

// BlockedTools is the forward-looking list of tool names that subagents
// must not be allowed to invoke. Of these names, only delegate_task exists
// in the current Gormes tool surface; the others are placeholders for
// tools that will be added in later phases. Enforcement of EnabledTools /
// BlockedTools filtering inside the runner is deferred to 2.E.7.
var BlockedTools = map[string]bool{
	"delegate_task": true,
	"clarify":       true,
	"memory":        true,
	"send_message":  true,
	"execute_code":  true,
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS (6 tests cumulative).

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/blocked.go gormes/internal/subagent/blocked_test.go
git commit -m "feat(subagent): add lifecycle constants and BlockedTools forward-looking set"
```

---

## Task 3: Sentinel errors

**Files:**
- Create: `gormes/internal/subagent/errors.go`
- Test: `gormes/internal/subagent/errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/errors_test.go
package subagent

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelIdentity(t *testing.T) {
	wrapped := fmt.Errorf("wrapped: %w", ErrMaxDepth)
	if !errors.Is(wrapped, ErrMaxDepth) {
		t.Errorf("errors.Is(wrapped, ErrMaxDepth): want true, got false")
	}
	if errors.Is(ErrMaxDepth, ErrSubagentNotFound) {
		t.Errorf("errors.Is(ErrMaxDepth, ErrSubagentNotFound): want false, got true (sentinels must be distinct)")
	}
}

func TestSentinelMessages(t *testing.T) {
	if ErrMaxDepth.Error() == "" || ErrSubagentNotFound.Error() == "" {
		t.Errorf("sentinel errors must have non-empty messages")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestSentinelIdentity|TestSentinelMessages" -v`
Expected: FAIL — `undefined: ErrMaxDepth`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/errors.go
package subagent

import "errors"

var (
	// ErrMaxDepth is returned by SubagentManager.Spawn when the manager's
	// depth equals or exceeds MaxDepth.
	ErrMaxDepth = errors.New("subagent: max depth reached")

	// ErrSubagentNotFound is returned by SubagentManager.Interrupt when the
	// supplied *Subagent is not currently tracked by the manager.
	ErrSubagentNotFound = errors.New("subagent: not found")
)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/errors.go gormes/internal/subagent/errors_test.go
git commit -m "feat(subagent): add ErrMaxDepth and ErrSubagentNotFound sentinels"
```

---

## Task 4: crypto/rand subagent IDs

**Files:**
- Create: `gormes/internal/subagent/ids.go`
- Test: `gormes/internal/subagent/ids_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/ids_test.go
package subagent

import (
	"strings"
	"testing"
)

func TestNewSubagentIDPrefix(t *testing.T) {
	id := newSubagentID()
	if !strings.HasPrefix(id, "sa_") {
		t.Errorf("newSubagentID: want prefix %q, got %q", "sa_", id)
	}
}

func TestNewSubagentIDLengthAndCharset(t *testing.T) {
	id := newSubagentID()
	body := strings.TrimPrefix(id, "sa_")
	// 8 bytes → ceil(8*8 / 5) = 13 base32 chars (no padding).
	if len(body) != 13 {
		t.Errorf("newSubagentID body length: want 13, got %d (id=%q)", len(body), id)
	}
	for _, r := range body {
		if !((r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7')) {
			t.Errorf("newSubagentID body charset: want base32 (A-Z, 2-7), got %q in %q", r, id)
		}
	}
}

func TestNewSubagentIDUniqueness(t *testing.T) {
	const N = 1000
	seen := make(map[string]struct{}, N)
	for i := 0; i < N; i++ {
		id := newSubagentID()
		if _, dup := seen[id]; dup {
			t.Fatalf("newSubagentID collision after %d calls: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestNewSubagentID" -v`
Expected: FAIL — `undefined: newSubagentID`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/ids.go
package subagent

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
)

// newSubagentID returns a fresh subagent ID of the form "sa_<13-char-base32>".
// 8 bytes of crypto/rand entropy → 13 base32 (no-padding) characters, giving
// 64 bits of randomness — collision-resistant for any realistic subagent volume.
func newSubagentID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read is documented as never returning short reads on
		// supported platforms. A failure here means the OS RNG is broken;
		// continuing would silently undermine ID uniqueness.
		panic(fmt.Errorf("subagent: crypto/rand failed: %w", err))
	}
	return "sa_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/ids.go gormes/internal/subagent/ids_test.go
git commit -m "feat(subagent): add crypto/rand subagent ID generator"
```

---

## Task 5: Runner interface + StubRunner

**Files:**
- Create: `gormes/internal/subagent/runner.go`
- Test: `gormes/internal/subagent/runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/runner_test.go
package subagent

import (
	"context"
	"testing"
	"time"
)

func TestStubRunnerHappyPath(t *testing.T) {
	cfg := SubagentConfig{Goal: "do the thing"}
	events := make(chan SubagentEvent, 4)
	runner := StubRunner{}

	result := runner.Run(context.Background(), cfg, events)
	close(events)

	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if result.Status != StatusCompleted {
		t.Errorf("Status: want %q, got %q", StatusCompleted, result.Status)
	}
	if result.Summary != "do the thing" {
		t.Errorf("Summary: want %q, got %q", "do the thing", result.Summary)
	}
	if result.ExitReason != "stub_runner_no_llm_yet" {
		t.Errorf("ExitReason: want %q, got %q", "stub_runner_no_llm_yet", result.ExitReason)
	}

	got := drain(events)
	if len(got) != 2 {
		t.Fatalf("event count: want 2, got %d (%v)", len(got), got)
	}
	if got[0].Type != EventStarted || got[1].Type != EventCompleted {
		t.Errorf("event sequence: want started→completed, got %v→%v", got[0].Type, got[1].Type)
	}
}

func TestStubRunnerCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Unbuffered channel with no reader — first send would block. The runner
	// must observe ctx.Done() instead and return promptly.
	events := make(chan SubagentEvent)
	runner := StubRunner{}

	done := make(chan *SubagentResult, 1)
	go func() { done <- runner.Run(ctx, SubagentConfig{Goal: "x"}, events) }()

	select {
	case result := <-done:
		if result.Status != StatusInterrupted {
			t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
		}
		if result.ExitReason != "ctx_cancelled_before_start" {
			t.Errorf("ExitReason: want %q, got %q", "ctx_cancelled_before_start", result.ExitReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StubRunner did not honour ctx cancellation within 2s")
	}
}

func TestStubRunnerCancelledDuringEmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Buffered channel: first send (started) succeeds; reader never drains, so
	// second send (completed) blocks. Cancel after a moment to force the
	// "during" branch.
	events := make(chan SubagentEvent, 1)
	runner := StubRunner{}

	done := make(chan *SubagentResult, 1)
	go func() { done <- runner.Run(ctx, SubagentConfig{Goal: "x"}, events) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case result := <-done:
		if result.Status != StatusInterrupted {
			t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
		}
		if result.ExitReason != "ctx_cancelled_during_stub" {
			t.Errorf("ExitReason: want %q, got %q", "ctx_cancelled_during_stub", result.ExitReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StubRunner did not honour ctx cancellation within 2s")
	}
}

func drain(ch <-chan SubagentEvent) []SubagentEvent {
	var out []SubagentEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestStubRunner" -v`
Expected: FAIL — `undefined: StubRunner`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/runner.go
package subagent

import (
	"context"
	"time"
)

// Runner is the swappable inner loop of a subagent. This slice ships
// StubRunner; 2.E.7 will ship LLMRunner.
//
// Contracts (binding on every implementation):
//
//  1. Run MUST return promptly after ctx.Done() fires. "Promptly" means within
//     a small bounded time, not blocked forever.
//  2. Run MUST NOT close the events channel. The manager owns the channel
//     lifecycle.
//  3. Run MAY emit zero or more events.
//  4. Run MUST return a non-nil *SubagentResult.
type Runner interface {
	Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult
}

// StubRunner emits started → completed and returns immediately. ExitReason
// carries an explicit TODO marker so it is unmistakable in logs and tests.
type StubRunner struct{}

func (StubRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	start := time.Now()

	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_before_start",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	select {
	case events <- SubagentEvent{Type: EventCompleted, Message: "stub"}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_during_stub",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	return &SubagentResult{
		Status:     StatusCompleted,
		Summary:    cfg.Goal,
		ExitReason: "stub_runner_no_llm_yet",
		Duration:   time.Since(start),
		Iterations: 0,
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/runner.go gormes/internal/subagent/runner_test.go
git commit -m "feat(subagent): add Runner interface and StubRunner with ctx-cancel honour"
```

---

## Task 6: Subagent struct — Events, WaitForResult, setResult

**Files:**
- Create: `gormes/internal/subagent/subagent.go`
- Test: `gormes/internal/subagent/subagent_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/subagent_test.go
package subagent

import (
	"context"
	"testing"
	"time"
)

func TestSubagentEventsReadOnly(t *testing.T) {
	sa := newTestSubagent()
	// Compile-time guarantee: Events() returns a receive-only channel. We
	// can't write to it, but we can confirm the runtime type assertion path.
	var _ <-chan SubagentEvent = sa.Events()
}

func TestSubagentWaitForResultBlocksUntilDone(t *testing.T) {
	sa := newTestSubagent()
	got := make(chan *SubagentResult, 1)

	go func() {
		r, err := sa.WaitForResult(context.Background())
		if err != nil {
			t.Errorf("WaitForResult error: %v", err)
		}
		got <- r
	}()

	select {
	case <-got:
		t.Fatal("WaitForResult returned before done was closed")
	case <-time.After(50 * time.Millisecond):
	}

	want := &SubagentResult{ID: "sa_test", Status: StatusCompleted}
	sa.setResult(want)
	close(sa.done)

	select {
	case r := <-got:
		if r != want {
			t.Errorf("WaitForResult: want %+v, got %+v", want, r)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForResult did not return after done was closed")
	}
}

func TestSubagentWaitForResultRespectsCallerCtx(t *testing.T) {
	sa := newTestSubagent()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r, err := sa.WaitForResult(ctx)
	if err != context.Canceled {
		t.Errorf("err: want context.Canceled, got %v", err)
	}
	if r != nil {
		t.Errorf("result: want nil, got %+v", r)
	}
}

func newTestSubagent() *Subagent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Subagent{
		ID:           "sa_test",
		Depth:        1,
		ctx:          ctx,
		cancel:       cancel,
		publicEvents: make(chan SubagentEvent),
		done:         make(chan struct{}),
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestSubagent" -v`
Expected: FAIL — `undefined: Subagent`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/subagent.go
package subagent

import (
	"context"
	"sync"
	"sync/atomic"
)

// Subagent represents a single child execution. Construction is the
// SubagentManager's responsibility; consumers interact via Events() and
// WaitForResult() only.
type Subagent struct {
	ID       string
	ParentID string
	Depth    int

	cfg           SubagentConfig
	ctx           context.Context
	cancel        context.CancelFunc // cancels the composed child ctx
	timeoutCancel context.CancelFunc // optional; nil if cfg.Timeout == 0

	publicEvents chan SubagentEvent // closed by lifecycle goroutine after runner returns
	done         chan struct{}      // closed after result is set

	interruptMsg atomic.Value // string; written by Manager.Interrupt before sa.cancel()

	mu     sync.RWMutex
	result *SubagentResult
}

// Events returns a receive-only channel that receives every SubagentEvent
// emitted by the runner, in order. The channel is closed exactly once when
// the runner has returned and all events have been forwarded.
func (s *Subagent) Events() <-chan SubagentEvent { return s.publicEvents }

// WaitForResult blocks until the subagent finishes (returning the result) or
// the supplied ctx is cancelled (returning ctx.Err()). The caller's ctx
// cancellation does NOT cancel the subagent — use Manager.Interrupt for that.
func (s *Subagent) WaitForResult(ctx context.Context) (*SubagentResult, error) {
	select {
	case <-s.done:
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// setResult is called exactly once by the lifecycle goroutine before close(s.done).
func (s *Subagent) setResult(r *SubagentResult) {
	s.mu.Lock()
	s.result = r
	s.mu.Unlock()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/subagent.go gormes/internal/subagent/subagent_test.go
git commit -m "feat(subagent): add Subagent struct with Events() and WaitForResult"
```

---

## Task 7: SubagentRegistry

**Files:**
- Create: `gormes/internal/subagent/registry.go`
- Test: `gormes/internal/subagent/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/registry_test.go
package subagent

import (
	"context"
	"testing"
)

func TestRegistryRegisterListUnregister(t *testing.T) {
	r := NewRegistry()
	if got := len(r.List()); got != 0 {
		t.Errorf("empty registry List(): want 0, got %d", got)
	}

	sa := newTestSubagent()
	r.Register(sa)
	if got := len(r.List()); got != 1 {
		t.Fatalf("after Register: want 1, got %d", got)
	}

	r.Unregister(sa.ID)
	if got := len(r.List()); got != 0 {
		t.Errorf("after Unregister: want 0, got %d", got)
	}
}

func TestRegistryUnregisterMissingIsNoOp(t *testing.T) {
	r := NewRegistry()
	r.Unregister("not_present") // must not panic
}

func TestRegistryInterruptAllCancelsContexts(t *testing.T) {
	r := NewRegistry()

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	r.Register(&Subagent{ID: "a", ctx: ctx1, cancel: cancel1})
	r.Register(&Subagent{ID: "b", ctx: ctx2, cancel: cancel2})

	r.InterruptAll("shutdown")

	for name, ctx := range map[string]context.Context{"a": ctx1, "b": ctx2} {
		select {
		case <-ctx.Done():
		default:
			t.Errorf("subagent %q ctx not cancelled by InterruptAll", name)
		}
	}
}

func TestRegistryListIsSnapshot(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 5; i++ {
		r.Register(&Subagent{ID: newSubagentID()})
	}
	snap := r.List()
	if len(snap) != 5 {
		t.Errorf("List length: want 5, got %d", len(snap))
	}
	// Mutating the returned slice must not affect future List() calls.
	snap[0] = nil
	again := r.List()
	for _, sa := range again {
		if sa == nil {
			t.Error("List returned shared underlying array (got nil after mutating prior snapshot)")
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestRegistry" -v`
Expected: FAIL — `undefined: NewRegistry`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/registry.go
package subagent

import "sync"

// SubagentRegistry tracks every live subagent in the process so a graceful
// shutdown can cancel them all.
type SubagentRegistry interface {
	Register(*Subagent)
	Unregister(id string)
	InterruptAll(message string)
	List() []*Subagent
}

type registry struct {
	mu        sync.RWMutex
	subagents map[string]*Subagent
}

// NewRegistry returns an empty SubagentRegistry. Safe for concurrent use.
func NewRegistry() SubagentRegistry {
	return &registry{subagents: make(map[string]*Subagent)}
}

func (r *registry) Register(sa *Subagent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subagents[sa.ID] = sa
}

func (r *registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subagents, id)
}

// InterruptAll cancels every live subagent's context. The message parameter
// is reserved for future per-subagent annotation; today every cancellation
// records the same reason via the per-subagent interruptMsg field, so the
// shared message argument is currently informational only.
func (r *registry) InterruptAll(message string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sa := range r.subagents {
		sa.interruptMsg.Store(message)
		sa.cancel()
	}
}

// List returns a fresh slice of the live subagents. Mutating the returned
// slice does not affect the registry.
func (r *registry) List() []*Subagent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Subagent, 0, len(r.subagents))
	for _, sa := range r.subagents {
		out = append(out, sa)
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/registry.go gormes/internal/subagent/registry_test.go
git commit -m "feat(subagent): add SubagentRegistry with InterruptAll"
```

---

## Task 8: SubagentManager — Spawn (happy path with StubRunner)

**Files:**
- Create: `gormes/internal/subagent/manager.go`
- Test: `gormes/internal/subagent/manager_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/manager_test.go
package subagent

import (
	"context"
	"testing"
	"time"
)

func newStubManager(t *testing.T, depth int) (SubagentManager, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ManagerOpts{
		ParentCtx: ctx,
		ParentID:  "parent_test",
		Depth:     depth,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return StubRunner{} },
	})
	return m, ctx, cancel
}

func TestManagerSpawnHappyPath(t *testing.T) {
	m, _, cancel := newStubManager(t, 0)
	defer cancel()

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "go"})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}
	if sa == nil {
		t.Fatal("Spawn returned nil subagent")
	}
	if sa.Depth != 1 {
		t.Errorf("Depth: want 1, got %d", sa.Depth)
	}
	if sa.ParentID != "parent_test" {
		t.Errorf("ParentID: want %q, got %q", "parent_test", sa.ParentID)
	}

	result, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("Status: want %q, got %q", StatusCompleted, result.Status)
	}
	if result.ID != sa.ID {
		t.Errorf("Result.ID: want %q, got %q", sa.ID, result.ID)
	}

	// Events channel must drain and close.
	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-sa.Events():
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("Events channel did not close within 1s")
		}
	}
}

func TestManagerSpawnAppliesIterationDefault(t *testing.T) {
	m, _, cancel := newStubManager(t, 0)
	defer cancel()

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "x"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sa.cfg.MaxIterations != DefaultMaxIterations {
		t.Errorf("MaxIterations default: want %d, got %d", DefaultMaxIterations, sa.cfg.MaxIterations)
	}
	_, _ = sa.WaitForResult(context.Background())
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestManagerSpawn" -v`
Expected: FAIL — `undefined: NewManager`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/manager.go
package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SubagentManager owns the goroutine lifecycle for every subagent it spawns.
type SubagentManager interface {
	Spawn(ctx context.Context, cfg SubagentConfig) (*Subagent, error)
	SpawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error)
	Interrupt(sa *Subagent, message string) error
	Collect(sa *Subagent) *SubagentResult
	Close() error
}

// ManagerOpts configures NewManager.
type ManagerOpts struct {
	// ParentCtx is the context every spawned subagent's ctx will derive from
	// via WithCancel. Cancelling ParentCtx cancels every child.
	ParentCtx context.Context

	// ParentID is recorded on every spawned Subagent.ParentID. Informational.
	ParentID string

	// Depth is the manager's depth in the subagent tree. Children of a
	// manager at depth D are spawned at depth D+1. Spawn returns ErrMaxDepth
	// when Depth >= MaxDepth.
	Depth int

	// Registry tracks every live subagent process-wide.
	Registry SubagentRegistry

	// NewRunner mints a Runner for each spawned subagent. This slice always
	// passes a func returning StubRunner{}; 2.E.7 will pass an LLMRunner factory.
	NewRunner func() Runner
}

type manager struct {
	opts ManagerOpts

	mu       sync.RWMutex
	children map[string]*Subagent

	closeOnce sync.Once
	closed    chan struct{}
}

// NewManager constructs a SubagentManager.
func NewManager(opts ManagerOpts) SubagentManager {
	if opts.NewRunner == nil {
		opts.NewRunner = func() Runner { return StubRunner{} }
	}
	if opts.Registry == nil {
		opts.Registry = NewRegistry()
	}
	if opts.ParentCtx == nil {
		opts.ParentCtx = context.Background()
	}
	return &manager{
		opts:     opts,
		children: make(map[string]*Subagent),
		closed:   make(chan struct{}),
	}
}

func (m *manager) Spawn(_ context.Context, cfg SubagentConfig) (*Subagent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.opts.Depth >= MaxDepth {
		return nil, fmt.Errorf("%w (depth=%d)", ErrMaxDepth, m.opts.Depth)
	}

	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}

	childCtx, cancel := context.WithCancel(m.opts.ParentCtx)
	var timeoutCancel context.CancelFunc
	if cfg.Timeout > 0 {
		childCtx, timeoutCancel = context.WithTimeout(childCtx, cfg.Timeout)
	}

	sa := &Subagent{
		ID:            newSubagentID(),
		ParentID:      m.opts.ParentID,
		Depth:         m.opts.Depth + 1,
		cfg:           cfg,
		ctx:           childCtx,
		cancel:        cancel,
		timeoutCancel: timeoutCancel,
		publicEvents:  make(chan SubagentEvent, 16),
		done:          make(chan struct{}),
	}

	m.children[sa.ID] = sa
	m.opts.Registry.Register(sa)

	go m.run(sa)
	return sa, nil
}

// run is the per-subagent lifecycle goroutine. See spec §Lifecycle goroutine.
func (m *manager) run(sa *Subagent) {
	start := time.Now()
	runner := m.opts.NewRunner()

	internalEvents := make(chan SubagentEvent, 16)
	resultCh := make(chan *SubagentResult, 1)
	runnerDone := make(chan struct{})

	// Runner wrapper goroutine.
	go func() {
		defer close(runnerDone)
		defer func() {
			if r := recover(); r != nil {
				resultCh <- &SubagentResult{
					Status:     StatusError,
					ExitReason: "panic",
					Error:      fmt.Sprintf("%v", r),
				}
			}
		}()
		resultCh <- runner.Run(sa.ctx, sa.cfg, internalEvents)
	}()

	// Forwarder: drain internalEvents to publicEvents until internalEvents
	// closes. Drains to completion; the runner contract bounds how long that
	// takes.
	forwarderDone := make(chan struct{})
	go func() {
		defer close(forwarderDone)
		defer close(sa.publicEvents)
		for ev := range internalEvents {
			sa.publicEvents <- ev
		}
	}()

	// Interrupt observer: emit EventInterrupted when ctx cancels, before the
	// runner reports its result.
	go func() {
		select {
		case <-sa.ctx.Done():
			msg, _ := sa.interruptMsg.Load().(string)
			select {
			case internalEvents <- SubagentEvent{Type: EventInterrupted, Message: msg}:
			case <-runnerDone:
			}
		case <-runnerDone:
		}
	}()

	// Wait for runner to return.
	result := <-resultCh

	// Fill fields the runner can't know.
	result.ID = sa.ID
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}

	// Ordered cleanup.
	close(internalEvents) // forwarder drains then closes publicEvents
	<-forwarderDone

	if sa.timeoutCancel != nil {
		sa.timeoutCancel()
	}

	sa.setResult(result)
	close(sa.done)

	m.removeChild(sa.ID)
	m.opts.Registry.Unregister(sa.ID)
}

func (m *manager) removeChild(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.children, id)
}

// Interrupt is implemented in Task 9.
func (m *manager) Interrupt(_ *Subagent, _ string) error {
	return fmt.Errorf("subagent: Interrupt not implemented")
}

// Collect is implemented in Task 11.
func (m *manager) Collect(_ *Subagent) *SubagentResult {
	return nil
}

// Close is implemented in Task 11.
func (m *manager) Close() error {
	return nil
}

// SpawnBatch is implemented in Task 13.
func (m *manager) SpawnBatch(_ context.Context, _ []SubagentConfig, _ int) ([]*SubagentResult, error) {
	return nil, fmt.Errorf("subagent: SpawnBatch not implemented")
}
```

The unimplemented methods return explicit "not implemented" errors so the package compiles green and tests for them in subsequent tasks fail clearly, not panic.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/manager_test.go
git commit -m "feat(subagent): add SubagentManager.Spawn with lifecycle goroutine"
```

---

## Task 9: Manager Interrupt + interruptMsg propagation

**Files:**
- Modify: `gormes/internal/subagent/manager.go`
- Test: `gormes/internal/subagent/manager_test.go`

- [ ] **Step 1: Append the failing test to manager_test.go**

```go
// blockingRunner blocks on ctx.Done() then returns a completed-as-cancelled
// result. Used to keep a subagent alive long enough to observe Interrupt
// behaviour.
type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
	}
	<-ctx.Done()
	return &SubagentResult{
		Status:     StatusInterrupted,
		ExitReason: "ctx_cancelled",
		Error:      ctx.Err().Error(),
	}
}

func newBlockingManager(t *testing.T, depth int) (SubagentManager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	return NewManager(ManagerOpts{
		ParentCtx: ctx,
		ParentID:  "parent_test",
		Depth:     depth,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return blockingRunner{} },
	}), cancel
}

func TestManagerInterruptDeliversMessage(t *testing.T) {
	m, cancel := newBlockingManager(t, 0)
	defer cancel()

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if err := m.Interrupt(sa, "user_stop"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	result, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusInterrupted {
		t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
	}

	var sawInterrupt bool
	var interruptMsg string
	for ev := range sa.Events() {
		if ev.Type == EventInterrupted {
			sawInterrupt = true
			interruptMsg = ev.Message
		}
	}
	if !sawInterrupt {
		t.Errorf("Events: want at least one EventInterrupted, got none")
	}
	if interruptMsg != "user_stop" {
		t.Errorf("EventInterrupted.Message: want %q, got %q", "user_stop", interruptMsg)
	}
}

func TestManagerInterruptUnknownReturnsErr(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()

	stranger := &Subagent{ID: "sa_stranger"}
	err := m.Interrupt(stranger, "nope")
	if err == nil || !errorsIs(err, ErrSubagentNotFound) {
		t.Errorf("err: want ErrSubagentNotFound, got %v", err)
	}
}

// errorsIs is a tiny shim so the test file doesn't need to import "errors"
// just for one call. Prefer errors.Is at call sites.
func errorsIs(err, target error) bool {
	if err == nil {
		return false
	}
	for {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
		if err == nil {
			return false
		}
	}
}

func TestManagerInterruptIsIdempotent(t *testing.T) {
	m, cancel := newBlockingManager(t, 0)
	defer cancel()

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if err := m.Interrupt(sa, "first"); err != nil {
		t.Fatalf("first Interrupt: %v", err)
	}
	if _, err := sa.WaitForResult(context.Background()); err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	// After completion the subagent has been removed; second Interrupt should
	// surface ErrSubagentNotFound, not panic.
	err = m.Interrupt(sa, "second")
	if err == nil || !errorsIs(err, ErrSubagentNotFound) {
		t.Errorf("second Interrupt: want ErrSubagentNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestManagerInterrupt" -v`
Expected: FAIL — Interrupt currently returns "not implemented".

- [ ] **Step 3: Replace Interrupt's stub in manager.go**

Replace the existing `func (m *manager) Interrupt(...) error { ... }` body with:

```go
func (m *manager) Interrupt(sa *Subagent, message string) error {
	m.mu.RLock()
	tracked, ok := m.children[sa.ID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrSubagentNotFound, sa.ID)
	}
	tracked.interruptMsg.Store(message)
	tracked.cancel()
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/manager_test.go
git commit -m "feat(subagent): implement Manager.Interrupt with message propagation"
```

---

## Task 10: Parent ctx cancellation cascade

**Files:**
- Modify: `gormes/internal/subagent/manager_test.go` (append test only)

- [ ] **Step 1: Append the failing test**

```go
func TestManagerParentCtxCancellationCascades(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	m := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return blockingRunner{} },
	})

	const N = 3
	subs := make([]*Subagent, N)
	for i := 0; i < N; i++ {
		sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
		if err != nil {
			t.Fatalf("Spawn[%d]: %v", i, err)
		}
		subs[i] = sa
	}

	cancelParent()

	for i, sa := range subs {
		result, err := sa.WaitForResult(context.Background())
		if err != nil {
			t.Fatalf("WaitForResult[%d]: %v", i, err)
		}
		if result.Status != StatusInterrupted {
			t.Errorf("subagent %d Status: want %q, got %q", i, StatusInterrupted, result.Status)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -run "TestManagerParentCtxCancellationCascades" -race -shuffle=on -v`
Expected: PASS — already implemented by `WithCancel(parentCtx)` in Spawn. This task documents the invariant with a regression test; no impl change needed. If it fails, the bug is in Task 8's Spawn.

- [ ] **Step 3: Commit (test-only commit is green and is the regression guard)**

```bash
git add gormes/internal/subagent/manager_test.go
git commit -m "test(subagent): add regression test for parent ctx cancellation cascade"
```

---

## Task 11: Manager depth limit

**Files:**
- Modify: `gormes/internal/subagent/manager_test.go` (append test only)

- [ ] **Step 1: Append the failing test**

```go
func TestManagerSpawnAtMaxDepthRejected(t *testing.T) {
	m := NewManager(ManagerOpts{
		ParentCtx: context.Background(),
		ParentID:  "parent_test",
		Depth:     MaxDepth,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return StubRunner{} },
	})

	_, err := m.Spawn(context.Background(), SubagentConfig{Goal: "x"})
	if err == nil || !errorsIs(err, ErrMaxDepth) {
		t.Errorf("err: want ErrMaxDepth, got %v", err)
	}
}

func TestManagerSpawnAtMaxDepthMinusOneAllowed(t *testing.T) {
	m := NewManager(ManagerOpts{
		ParentCtx: context.Background(),
		ParentID:  "parent_test",
		Depth:     MaxDepth - 1,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return StubRunner{} },
	})

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "x"})
	if err != nil {
		t.Fatalf("Spawn at MaxDepth-1: want OK, got %v", err)
	}
	if sa.Depth != MaxDepth {
		t.Errorf("Depth: want %d, got %d", MaxDepth, sa.Depth)
	}
	_, _ = sa.WaitForResult(context.Background())
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -run "TestManagerSpawnAtMaxDepth" -race -shuffle=on -v`
Expected: PASS — depth check is already in Task 8's Spawn. This task is the regression test.

- [ ] **Step 3: Commit**

```bash
git add gormes/internal/subagent/manager_test.go
git commit -m "test(subagent): add depth-limit regression tests"
```

---

## Task 12: Manager Collect + Close (sync.Once)

**Files:**
- Modify: `gormes/internal/subagent/manager.go`
- Test: `gormes/internal/subagent/manager_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestManagerCollectBeforeAndAfterDone(t *testing.T) {
	m, cancel := newBlockingManager(t, 0)
	defer cancel()

	sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if got := m.Collect(sa); got != nil {
		t.Errorf("Collect before done: want nil, got %+v", got)
	}

	if err := m.Interrupt(sa, "stop"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if _, err := sa.WaitForResult(context.Background()); err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}

	got := m.Collect(sa)
	if got == nil {
		t.Errorf("Collect after done: want non-nil, got nil")
	}
	if got != nil && got.Status != StatusInterrupted {
		t.Errorf("Collect Status: want %q, got %q", StatusInterrupted, got.Status)
	}
}

func TestManagerCloseCancelsAllAndIsIdempotent(t *testing.T) {
	m, cancel := newBlockingManager(t, 0)
	defer cancel()

	subs := make([]*Subagent, 3)
	for i := range subs {
		sa, err := m.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
		if err != nil {
			t.Fatalf("Spawn[%d]: %v", i, err)
		}
		subs[i] = sa
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: want nil, got %v", err)
	}

	for i, sa := range subs {
		select {
		case <-sa.done:
		case <-time.After(2 * time.Second):
			t.Fatalf("subagent %d not finished after Close", i)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestManagerCollect|TestManagerClose" -v`
Expected: FAIL — Collect returns nil always; Close is a no-op stub.

- [ ] **Step 3: Replace Collect and Close in manager.go**

Replace the placeholder Collect:

```go
func (m *manager) Collect(sa *Subagent) *SubagentResult {
	select {
	case <-sa.done:
		sa.mu.RLock()
		defer sa.mu.RUnlock()
		return sa.result
	default:
		return nil
	}
}
```

Replace the placeholder Close:

```go
func (m *manager) Close() error {
	m.closeOnce.Do(func() {
		// Snapshot under lock; cancel outside lock to avoid holding it while
		// each lifecycle goroutine reaches its cleanup (which calls
		// removeChild, which would re-acquire the lock).
		m.mu.RLock()
		snap := make([]*Subagent, 0, len(m.children))
		for _, sa := range m.children {
			snap = append(snap, sa)
		}
		m.mu.RUnlock()

		for _, sa := range snap {
			sa.cancel()
		}
		for _, sa := range snap {
			<-sa.done
		}
		close(m.closed)
	})
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/manager_test.go
git commit -m "feat(subagent): implement Manager.Collect and idempotent Manager.Close"
```

---

## Task 13: SpawnBatch with errgroup + semaphore

**Files:**
- Create: `gormes/internal/subagent/batch.go`
- Test: `gormes/internal/subagent/batch_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/batch_test.go
package subagent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concurrencyProbeRunner records how many of its instances are simultaneously
// inside Run, and the maximum observed.
type concurrencyProbeRunner struct {
	mu      sync.Mutex
	current int
	maxSeen *atomic.Int64
	hold    time.Duration
}

func (c *concurrencyProbeRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	c.mu.Lock()
	c.current++
	if int64(c.current) > c.maxSeen.Load() {
		c.maxSeen.Store(int64(c.current))
	}
	c.mu.Unlock()

	select {
	case <-time.After(c.hold):
	case <-ctx.Done():
	}

	c.mu.Lock()
	c.current--
	c.mu.Unlock()

	return &SubagentResult{Status: StatusCompleted, Summary: cfg.Goal, ExitReason: "probe_done"}
}

func TestSpawnBatchEnforcesMaxConcurrent(t *testing.T) {
	maxSeen := &atomic.Int64{}
	probe := &concurrencyProbeRunner{maxSeen: maxSeen, hold: 30 * time.Millisecond}

	m := NewManager(ManagerOpts{
		ParentCtx: context.Background(),
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return probe },
	})

	cfgs := make([]SubagentConfig, 10)
	for i := range cfgs {
		cfgs[i] = SubagentConfig{Goal: "p"}
	}

	results, err := m.SpawnBatch(context.Background(), cfgs, 2)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	if len(results) != len(cfgs) {
		t.Fatalf("results len: want %d, got %d", len(cfgs), len(results))
	}
	if maxSeen.Load() > 2 {
		t.Errorf("maxSeen: want <= 2, got %d", maxSeen.Load())
	}
	for i, r := range results {
		if r == nil {
			t.Errorf("results[%d] nil", i)
		}
	}
}

func TestSpawnBatchPreservesInputOrder(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	cfgs := []SubagentConfig{
		{Goal: "alpha"},
		{Goal: "bravo"},
		{Goal: "charlie"},
		{Goal: "delta"},
	}
	results, err := m.SpawnBatch(context.Background(), cfgs, 0)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	for i, want := range cfgs {
		if results[i] == nil {
			t.Fatalf("results[%d] nil", i)
		}
		if results[i].Summary != want.Goal {
			t.Errorf("results[%d].Summary: want %q, got %q", i, want.Goal, results[i].Summary)
		}
	}
}

func TestSpawnBatchEmptyInput(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	results, err := m.SpawnBatch(context.Background(), nil, 3)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results len: want 0, got %d", len(results))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestSpawnBatch" -v`
Expected: FAIL — SpawnBatch is the "not implemented" stub from Task 8.

- [ ] **Step 3: Write batch.go and remove the stub from manager.go**

Create `gormes/internal/subagent/batch.go`:

```go
// gormes/internal/subagent/batch.go
package subagent

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// spawnBatch is the implementation that SubagentManager.SpawnBatch delegates to.
// It runs at most maxConcurrent subagents in parallel and returns results in
// input order. Errors from individual Spawn calls are surfaced inline as
// StatusError results so a single bad config does not mask the rest.
func (m *manager) spawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}

	results := make([]*SubagentResult, len(cfgs))
	sem := make(chan struct{}, maxConcurrent)
	g, gctx := errgroup.WithContext(ctx)

	for i := range cfgs {
		i, cfg := i, cfgs[i]
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				results[i] = &SubagentResult{Status: StatusInterrupted, ExitReason: "batch_ctx_cancelled", Error: gctx.Err().Error()}
				return nil
			}
			defer func() { <-sem }()

			sa, err := m.Spawn(gctx, cfg)
			if err != nil {
				results[i] = &SubagentResult{Status: StatusError, ExitReason: "spawn_failed", Error: err.Error()}
				return nil
			}

			r, err := sa.WaitForResult(gctx)
			if err != nil {
				// gctx cancelled — surface the partial state without aborting peers.
				results[i] = &SubagentResult{ID: sa.ID, Status: StatusInterrupted, ExitReason: "batch_ctx_cancelled", Error: err.Error()}
				return nil
			}
			results[i] = r
			return nil
		})
	}

	// errgroup returns error only when a goroutine returns one; we never do.
	_ = g.Wait()
	return results, nil
}
```

Now replace the SpawnBatch stub in `manager.go` with:

```go
func (m *manager) SpawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error) {
	return m.spawnBatch(ctx, cfgs, maxConcurrent)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/batch.go gormes/internal/subagent/manager.go gormes/internal/subagent/batch_test.go
git commit -m "feat(subagent): implement SpawnBatch with errgroup + semaphore (no polling)"
```

---

## Task 14: ToolExecutor + InProcessToolExecutor

**Files:**
- Create: `gormes/internal/tools/executor.go`
- Test: `gormes/internal/tools/executor_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/tools/executor_test.go
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestInProcessExecutorEchoTool(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&EchoTool{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	exec := NewInProcessToolExecutor(reg)

	ch, err := exec.Execute(context.Background(), ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{"text":"hi"}`),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got []ToolEvent
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("event count: want 3, got %d (%+v)", len(got), got)
	}
	if got[0].Type != "started" || got[1].Type != "output" || got[2].Type != "completed" {
		t.Errorf("event sequence: want started→output→completed, got %s→%s→%s", got[0].Type, got[1].Type, got[2].Type)
	}
	if !strings.Contains(string(got[1].Output), `"hi"`) {
		t.Errorf("output payload: want contains \"hi\", got %s", got[1].Output)
	}
}

func TestInProcessExecutorUnknownTool(t *testing.T) {
	reg := NewRegistry()
	exec := NewInProcessToolExecutor(reg)

	_, err := exec.Execute(context.Background(), ToolRequest{ToolName: "nope"})
	if err == nil {
		t.Fatal("Execute: want error, got nil")
	}
	if !errors.Is(err, ErrUnknownTool) {
		t.Errorf("err: want ErrUnknownTool, got %v", err)
	}
}

func TestInProcessExecutorToolErrorEmitsFailed(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&EchoTool{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	exec := NewInProcessToolExecutor(reg)

	// EchoTool returns an error when "text" is missing.
	ch, err := exec.Execute(context.Background(), ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute (registration error path is separate): %v", err)
	}

	var got []ToolEvent
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 2 {
		t.Fatalf("event count: want 2 (started+failed), got %d (%+v)", len(got), got)
	}
	if got[0].Type != "started" || got[1].Type != "failed" {
		t.Errorf("event sequence: want started→failed, got %s→%s", got[0].Type, got[1].Type)
	}
	if got[1].Err == nil {
		t.Errorf("failed event Err: want non-nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/tools/... -run "TestInProcessExecutor" -v`
Expected: FAIL — `undefined: NewInProcessToolExecutor`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/tools/executor.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor executes tools on behalf of an agent. Subagents use this
// interface so the execution model is swappable: in-process today;
// sidecar / remote in later phases.
type ToolExecutor interface {
	Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error)
}

// ToolRequest is a single tool invocation request submitted to a ToolExecutor.
type ToolRequest struct {
	AgentID  string
	ToolName string
	Input    json.RawMessage
	Metadata map[string]string
}

// ToolEvent is one observation from a tool invocation: started, output,
// completed, or failed.
type ToolEvent struct {
	Type   string // "started" | "output" | "completed" | "failed"
	Output json.RawMessage
	Err    error
}

// InProcessToolExecutor wraps a Registry and runs tools directly in the
// current binary, honouring tool.Timeout() per call.
type InProcessToolExecutor struct {
	registry *Registry
}

// NewInProcessToolExecutor returns an InProcessToolExecutor backed by reg.
func NewInProcessToolExecutor(reg *Registry) *InProcessToolExecutor {
	return &InProcessToolExecutor{registry: reg}
}

// Execute looks up the named tool, then runs it in a goroutine, streaming
// started → output → completed (or started → failed) on the returned channel.
// Returns ErrUnknownTool synchronously if the tool name is not registered.
func (e *InProcessToolExecutor) Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error) {
	tool, ok := e.registry.Get(req.ToolName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, req.ToolName)
	}

	ch := make(chan ToolEvent, 4)

	go func() {
		defer close(ch)

		ch <- ToolEvent{Type: "started"}

		execCtx := ctx
		if timeout := tool.Timeout(); timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		out, err := tool.Execute(execCtx, req.Input)
		if err != nil {
			ch <- ToolEvent{Type: "failed", Err: err}
			return
		}

		ch <- ToolEvent{Type: "output", Output: out}
		ch <- ToolEvent{Type: "completed"}
	}()

	return ch, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/tools/... -race -shuffle=on -v`
Expected: PASS (existing tools tests + 3 new).

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/tools/executor.go gormes/internal/tools/executor_test.go
git commit -m "feat(tools): add ToolExecutor interface + InProcessToolExecutor wrapping tools.Tool"
```

---

## Task 15: DelegationCfg in config

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

- [ ] **Step 1: Append the failing test**

Add this test to `gormes/internal/config/config_test.go`:

```go
func TestDelegationCfgDecode(t *testing.T) {
	const tomlText = `
[delegation]
enabled                 = true
max_depth               = 2
max_concurrent_children = 3
default_max_iterations  = 50
default_timeout         = "1h"
`
	var cfg Config
	if err := toml.Unmarshal([]byte(tomlText), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !cfg.Delegation.Enabled {
		t.Errorf("Enabled: want true, got false")
	}
	if cfg.Delegation.MaxDepth != 2 {
		t.Errorf("MaxDepth: want 2, got %d", cfg.Delegation.MaxDepth)
	}
	if cfg.Delegation.MaxConcurrentChildren != 3 {
		t.Errorf("MaxConcurrentChildren: want 3, got %d", cfg.Delegation.MaxConcurrentChildren)
	}
	if cfg.Delegation.DefaultMaxIterations != 50 {
		t.Errorf("DefaultMaxIterations: want 50, got %d", cfg.Delegation.DefaultMaxIterations)
	}
	if cfg.Delegation.DefaultTimeout != time.Hour {
		t.Errorf("DefaultTimeout: want 1h, got %v", cfg.Delegation.DefaultTimeout)
	}
}
```

If `toml` and `time` are not already imported in the test file, add them:

```go
import (
	// ... existing imports ...
	"time"

	"github.com/pelletier/go-toml/v2"
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/config/... -run "TestDelegationCfgDecode" -v`
Expected: FAIL — `cfg.Delegation undefined`.

- [ ] **Step 3: Add DelegationCfg to config.go**

In `gormes/internal/config/config.go`, add the new type alongside the other `*Cfg` types (e.g. immediately after `CronCfg`):

```go
// DelegationCfg configures Phase 2.E subagent execution. Enabled gates
// registration of the delegate_task tool; the rest are defaults applied at
// SubagentManager.Spawn time.
type DelegationCfg struct {
	Enabled               bool          `toml:"enabled"`
	MaxDepth              int           `toml:"max_depth"`
	MaxConcurrentChildren int           `toml:"max_concurrent_children"`
	DefaultMaxIterations  int           `toml:"default_max_iterations"`
	DefaultTimeout        time.Duration `toml:"default_timeout"`
}
```

In the `Config` struct, add:

```go
Delegation DelegationCfg `toml:"delegation"`
```

(Insert the line right after the `Cron` field for stylistic locality.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/config/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "feat(config): add [delegation] TOML section for Phase 2.E"
```

---

## Task 16: delegate_task tool

**Files:**
- Create: `gormes/internal/subagent/delegate_tool.go`
- Test: `gormes/internal/subagent/delegate_tool_test.go`

- [ ] **Step 1: Write the failing test**

```go
// gormes/internal/subagent/delegate_tool_test.go
package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDelegateToolMetadata(t *testing.T) {
	tool := NewDelegateTool(nil) // metadata methods don't dereference the manager
	if tool.Name() != "delegate_task" {
		t.Errorf("Name: want %q, got %q", "delegate_task", tool.Name())
	}
	if tool.Description() == "" {
		t.Errorf("Description: want non-empty")
	}
	if tool.Timeout() != 0 {
		t.Errorf("Timeout: want 0 (governed by subagent timeout), got %v", tool.Timeout())
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Errorf("Schema: invalid JSON: %v", err)
	}
}

func TestDelegateToolExecuteHappyPath(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	tool := NewDelegateTool(m)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"research X","context":"channels only"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["status"] != "completed" {
		t.Errorf("status: want %q, got %v", "completed", got["status"])
	}
	if got["summary"] != "research X" {
		t.Errorf("summary: want %q, got %v", "research X", got["summary"])
	}
	if got["exit_reason"] != "stub_runner_no_llm_yet" {
		t.Errorf("exit_reason: want %q, got %v", "stub_runner_no_llm_yet", got["exit_reason"])
	}
	id, _ := got["id"].(string)
	if !strings.HasPrefix(id, "sa_") {
		t.Errorf("id: want %q-prefixed, got %v", "sa_", got["id"])
	}
}

func TestDelegateToolMissingGoal(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	tool := NewDelegateTool(m)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("Execute: want error for missing goal, got nil")
	}
}

func TestDelegateToolInvalidArgs(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	tool := NewDelegateTool(m)
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Errorf("Execute: want error for invalid JSON, got nil")
	}
}

func TestDelegateToolToolsetsParsing(t *testing.T) {
	m, cancel := newStubManager(t, 0)
	defer cancel()
	defer m.Close()

	tool := NewDelegateTool(m)
	// We can't observe EnabledTools through the public surface in this slice
	// (StubRunner ignores it). The test just confirms the call succeeds with
	// a toolsets argument so the parsing path is exercised.
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"x","toolsets":"a,b , c"}`))
	if err != nil {
		t.Errorf("Execute with toolsets: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd gormes && go test ./internal/subagent/... -run "TestDelegateTool" -v`
Expected: FAIL — `undefined: NewDelegateTool`.

- [ ] **Step 3: Write the minimal implementation**

```go
// gormes/internal/subagent/delegate_tool.go
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DelegateTool is the Go-native delegate_task tool. It implements the
// gormes/internal/tools.Tool interface and is registered when
// [delegation].enabled = true in config.
type DelegateTool struct {
	manager SubagentManager
}

// NewDelegateTool wires a DelegateTool to the supplied SubagentManager.
func NewDelegateTool(m SubagentManager) *DelegateTool { return &DelegateTool{manager: m} }

// Name implements tools.Tool.
func (*DelegateTool) Name() string { return "delegate_task" }

// Description implements tools.Tool.
func (*DelegateTool) Description() string {
	return "Delegate a task to a subagent for parallel execution. The subagent runs with its own context, returns a structured JSON result."
}

// Timeout implements tools.Tool. Returns 0 so the executor does not impose
// a deadline; per-subagent timeouts are governed via SubagentConfig.Timeout.
func (*DelegateTool) Timeout() time.Duration { return 0 }

// Schema implements tools.Tool.
func (*DelegateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
        "type": "object",
        "properties": {
            "goal":           {"type": "string", "description": "Task goal for the subagent"},
            "context":        {"type": "string", "description": "Optional additional context"},
            "max_iterations": {"type": "integer", "description": "Max LLM turns for the subagent"},
            "toolsets":       {"type": "string", "description": "Comma-separated toolset names to enable"}
        },
        "required": ["goal"]
    }`)
}

type delegateArgs struct {
	Goal          string `json:"goal"`
	Context       string `json:"context"`
	MaxIterations int    `json:"max_iterations"`
	Toolsets      string `json:"toolsets"`
}

// Execute implements tools.Tool. Spawns a subagent, waits for the result,
// and returns a JSON-encoded summary.
func (t *DelegateTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a delegateArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("delegate_task: invalid args: %w", err)
	}
	if a.Goal == "" {
		return nil, errors.New("delegate_task: goal is required")
	}

	var enabled []string
	if a.Toolsets != "" {
		for _, s := range strings.Split(a.Toolsets, ",") {
			if s = strings.TrimSpace(s); s != "" {
				enabled = append(enabled, s)
			}
		}
	}

	sa, err := t.manager.Spawn(ctx, SubagentConfig{
		Goal:          a.Goal,
		Context:       a.Context,
		MaxIterations: a.MaxIterations,
		EnabledTools:  enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("delegate_task: spawn: %w", err)
	}

	result, err := sa.WaitForResult(ctx)
	if err != nil {
		// Caller's ctx cancelled — propagate cancel to the subagent before returning.
		_ = t.manager.Interrupt(sa, "parent ctx cancelled")
		return nil, err
	}

	out := map[string]any{
		"id":          result.ID,
		"status":      string(result.Status),
		"summary":     result.Summary,
		"exit_reason": result.ExitReason,
		"duration_ms": result.Duration.Milliseconds(),
		"iterations":  result.Iterations,
		"error":       result.Error,
	}
	return json.Marshal(out)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd gormes && go test ./internal/subagent/... -race -shuffle=on -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/delegate_tool.go gormes/internal/subagent/delegate_tool_test.go
git commit -m "feat(subagent): add delegate_task tool wrapping SubagentManager"
```

---

## Final Verification

- [ ] **Step 1: Full test sweep with race + shuffle**

Run: `cd gormes && go test ./internal/subagent/... ./internal/tools/... ./internal/config/... -race -shuffle=on -count=3 -v`
Expected: PASS on every iteration. `-count=3` runs the suite three times to flush out flakes; `-shuffle=on` reorders tests each iteration.

- [ ] **Step 2: Build cleanly**

Run: `cd gormes && go build ./...`
Expected: no output.

- [ ] **Step 3: Vet**

Run: `cd gormes && go vet ./...`
Expected: no output.

- [ ] **Step 4: Binary size delta sanity check**

Run:
```bash
cd gormes && \
  git stash && \
  make build && \
  cp bin/gormes /tmp/gormes-pre-2e && \
  git stash pop && \
  make build && \
  ls -l /tmp/gormes-pre-2e bin/gormes
```
Expected: `bin/gormes` no more than ~200 KB larger than `/tmp/gormes-pre-2e`. (If you don't have the pre-2.E binary, just sanity-check that `bin/gormes` is still under the 25 MB hard moat.)

- [ ] **Step 5: Update docs/content/building-gormes/architecture_plan/_index.md and progress.json**

This is intentionally NOT automated by this plan — `progress.json` is touched by `make build`'s `progress-gen` step. After all 16 commits, verify:

```bash
cd gormes && make build
git status
```

Any modifications to `docs/content/building-gormes/architecture_plan/_index.md` or `progress.json` are the auto-generated rollup; commit them with:

```bash
git add gormes/docs/content/building-gormes/architecture_plan/_index.md \
        gormes/docs/content/building-gormes/architecture_plan/progress.json \
        gormes/www.gormes.ai/internal/site/data/progress.json
git commit -m "docs(progress): regenerate rollup after Phase 2.E lifecycle core"
```

---

## Self-Review

**Spec coverage:**
- §Data Structures → Tasks 1–4
- §Runner Interface → Task 5
- §SubagentRegistry → Task 7
- §SubagentManager (Spawn / lifecycle / Interrupt / Collect / Close) → Tasks 8, 9, 10, 11, 12
- §SpawnBatch → Task 13
- §ToolExecutor + InProcessToolExecutor → Task 14
- §Configuration → Task 15
- §delegate_task tool → Task 16
- §Cancellation Semantics — verified by Task 9 (Interrupt), Task 10 (parent cascade), Task 12 (Close)
- §TDD Discipline (always-green commits, `-race -shuffle=on`) → enforced in every Step 4
- §Acceptance Criteria 1–10 — all map to test cases above; criterion 9 (`-race -shuffle=on` green per commit) is enforced by Step 4 of every task; criterion 10 (binary size) is verified in Final Verification Step 4

**Placeholder scan:** No "TBD"/"TODO" markers. The intentional `"stub_runner_no_llm_yet"` literal is part of the spec contract for the deferred LLM loop, asserted by tests in Tasks 5, 8, and 16.

**Type consistency:**
- `SubagentManager` is the interface; `manager` is the unexported implementation. `NewManager` returns the interface. Used consistently across Tasks 8–13, 16.
- `Runner` interface signature: `Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult` — used identically in Tasks 5, 8, 13.
- `ToolExecutor.Execute(ctx, ToolRequest) (<-chan ToolEvent, error)` — Task 14. (The 2.E.7 LLMRunner will receive a `ToolExecutor` via ManagerOpts in a future revision; not threaded through this slice's `ManagerOpts` because StubRunner doesn't use it. Adding the field early would be YAGNI — punt to 2.E.7.)
- `tools.Tool` interface (`Name`/`Description`/`Schema`/`Timeout`/`Execute`) — DelegateTool implements it in Task 16; InProcessToolExecutor consumes it in Task 14.

**One spec → plan deviation worth flagging:** The spec sketches `ManagerOpts.Executor ToolExecutor`, but this slice's `ManagerOpts` (Task 8) omits the field because no Runner consumes it yet (StubRunner doesn't, and the LLMRunner is deferred). Adding the field now would force every test and call site to pass an unused executor. 2.E.7 will add the field when LLMRunner needs it. The `delegate_task` tool itself is wired directly to a `SubagentManager` (not to a separately-constructed executor), which is what Task 16 reflects.

---

**Plan complete and saved to `gormes/docs/superpowers/plans/2026-04-20-gormes-phase2e-subagent.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task (16 tasks total), review between tasks, fast iteration, isolation from current chat context.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
