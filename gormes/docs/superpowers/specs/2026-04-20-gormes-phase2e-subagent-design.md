# Phase 2.E — Subagent System Spec

**Date:** 2026-04-20 (revised 2026-04-20 — scope reduced, types reconciled)
**Status:** Approved — pending implementation plan
**Priority:** P0
**Upstream reference (informational only):** `tools/delegate_tool.py`, `run_agent.py`

---

## Scope and Non-Goals

This spec covers **the subagent lifecycle core**: goroutine isolation, recursive cancellation, depth limits, batch concurrency, the ToolExecutor wrapper, and the `delegate_task` tool entry point. The actual LLM loop that a subagent would run is **deferred** to a follow-up subphase (2.E.7) and is replaced in this slice by a swappable `StubRunner`.

**In scope (this slice):**
- `Subagent`, `SubagentConfig`, `SubagentEvent`, `SubagentResult` types
- `SubagentRegistry` (process-wide tracker)
- `SubagentManager` interface + implementation (Spawn / SpawnBatch / Interrupt / Collect / Close)
- `Runner` interface + `StubRunner` implementation
- `ToolExecutor` interface + `InProcessToolExecutor` wrapping the real `tools.Tool`
- `delegate_task` tool (Go-native, registered in main)
- `[delegation]` config section
- Recursive context cancellation
- Crypto-strong subagent IDs
- Strict TDD discipline with always-green commits and `-race -shuffle=on`

**Out of scope (deferred):**
- Real LLM inner loop inside a subagent (waits on Phase 4 architecture clarity)
- TUI / CLI surface for `/delegate`, `/subagents`, `/stop-subagent` (no LLM = no realistic end-to-end CLI flow yet)
- Enforcing `EnabledTools` filtering inside the runner (StubRunner ignores tools)
- Audit / insights emission for subagent lifecycle (Phase 3.E.5)
- BlockedTools enforcement beyond `delegate_task` itself (the other listed names — `clarify`, `memory`, `send_message`, `execute_code` — do not exist in the current Gormes tool surface; the list stays as forward-looking documentation)

---

## Goal

Implement the Gormes Subagent System — **execution isolation for parallel workstreams**. A subagent runs in its own goroutine with its own `context.Context`, communicates with the parent through typed channels, and is cancelled deterministically when the parent's context cancels. The lifecycle, isolation, and cancellation primitives are correct and tested with `-race -shuffle=on`. The runner that consumes the lifecycle is pluggable: this slice ships `StubRunner`; 2.E.7 ships the real LLM runner.

---

## Why Goroutines, Not Processes

Python's Hermes uses threads for subagents. Gormes inherits the in-process model via goroutines. The architectural win is that Go gives us **deterministic context cancellation** and **typed channel communication** natively — no thread-global interrupt flags. The cost is shared address space; we pay that cost knowingly because the LLM-loop work is I/O-bound (HTTP to provider) and we get no isolation benefit from a process per subagent at this stage.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ Parent (depth=0)                                             │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ SubagentManager                                      │    │
│  │   children   map[string]*Subagent                    │    │
│  │   executor   ToolExecutor                            │    │
│  │   registry   SubagentRegistry                        │    │
│  │   newRunner  func() Runner                           │    │
│  └────────────────────┬────────────────────────────────┘    │
│                       │ Spawn → goroutine                    │
└───────────────────────┼─────────────────────────────────────┘
                        ▼
┌──────────────────────────────────────────────────────────────┐
│ Child goroutine (depth=1)                                    │
│   ctx, cancel := context.WithCancel(parentCtx)               │
│                                                              │
│   internalEvents := make(chan SubagentEvent, 16)             │
│   resultCh       := make(chan *SubagentResult, 1)            │
│                                                              │
│   go runner.Run(ctx, cfg, internalEvents) → resultCh         │
│                                                              │
│   forwarder goroutine: internalEvents → publicEvents         │
│                       (closes publicEvents on result)        │
│                                                              │
│   on done:                                                   │
│     - sa.setResult(r)                                        │
│     - close(sa.done)                                         │
│     - registry.Unregister(sa.ID)                             │
└──────────────────────────────────────────────────────────────┘
```

Key invariants:
- The child ctx is `context.WithCancel(parentCtx)`; cancelling the parent cancels every descendant transitively.
- `Subagent.Events()` returns a read-only channel that closes exactly once when the runner returns.
- `Subagent.WaitForResult(ctx)` blocks on `sa.done` (closed by the lifecycle goroutine after `setResult`); never spins, never sleeps.
- All access to `sa.result` goes through `setResult` / `WaitForResult` and is mutex-guarded.

---

## Data Structures

```go
// internal/subagent/subagent.go

type Subagent struct {
    ID       string
    ParentID string
    Depth    int

    cfg     SubagentConfig
    ctx     context.Context
    cancel  context.CancelFunc       // cancels child ctx (composed: WithCancel + optional WithTimeout)
    timeoutCancel context.CancelFunc // optional; nil if cfg.Timeout == 0

    publicEvents chan SubagentEvent  // closed by lifecycle goroutine after runner returns
    done         chan struct{}       // closed after result is set

    interruptMsg atomic.Value        // string; written by Interrupt before sa.cancel()

    mu     sync.RWMutex
    result *SubagentResult
}

func (s *Subagent) Events() <-chan SubagentEvent { return s.publicEvents }

// WaitForResult blocks until the subagent finishes or ctx is cancelled.
// If the caller's ctx cancels first, the subagent is NOT cancelled — use Manager.Interrupt for that.
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

type SubagentConfig struct {
    Goal          string
    Context       string
    MaxIterations int           // 0 → DefaultMaxIterations
    EnabledTools  []string      // empty → all parent tools minus BlockedTools (enforcement deferred)
    Model         string        // empty → inherit
    Timeout       time.Duration // 0 → no timeout
}

type SubagentEvent struct {
    Type     EventType
    Message  string
    ToolCall *ToolCallInfo
    Progress *ProgressInfo
}

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

type ResultStatus string

const (
    StatusCompleted   ResultStatus = "completed"
    StatusFailed      ResultStatus = "failed"
    StatusInterrupted ResultStatus = "interrupted"
    StatusError       ResultStatus = "error"
)

type ToolCallInfo struct {
    Name       string
    ArgsBytes  int
    ResultSize int
    Status     string
}

type ProgressInfo struct {
    Iteration int
    Message   string
}

const (
    MaxDepth             = 2  // parent=0 → child=1 OK → grandchild=2 rejected
    DefaultMaxConcurrent = 3
    DefaultMaxIterations = 50
)

// BlockedTools is forward-looking. Of these, only delegate_task exists in the
// current Gormes tool surface; the others are placeholders for tools that
// will be added in later phases. Enforcement of EnabledTools/BlockedTools
// inside the runner is deferred to 2.E.7.
var BlockedTools = map[string]bool{
    "delegate_task": true, // prevents subagent → subagent loops
    "clarify":       true, // future
    "memory":        true, // future
    "send_message":  true, // future
    "execute_code":  true, // future
}
```

ID generation uses `crypto/rand`:

```go
func newSubagentID() string {
    var b [8]byte
    if _, err := rand.Read(b[:]); err != nil {
        panic(fmt.Errorf("subagent: crypto/rand failed: %w", err))
    }
    return "sa_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
```

`crypto/rand.Read` is documented as never returning short reads on supported platforms; a failure here means the OS RNG is broken and the process should die.

---

## Runner Interface

The Runner is the swappable inner loop. This slice ships `StubRunner`; 2.E.7 will ship `LLMRunner`.

```go
// internal/subagent/runner.go

type Runner interface {
    // Run executes the subagent's work to completion or context cancellation.
    // The runner sends events to the events channel; the channel must NOT be
    // closed by the runner — the lifecycle goroutine owns its lifecycle.
    // Run returns a non-nil SubagentResult; cancellation is signalled via
    // ctx, not via the return value.
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

---

## SubagentRegistry

```go
// internal/subagent/registry.go

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

func NewRegistry() SubagentRegistry { return &registry{subagents: map[string]*Subagent{}} }
```

`InterruptAll` cancels every live subagent's context. `List` returns a snapshot.

---

## SubagentManager

```go
// internal/subagent/manager.go

type SubagentManager interface {
    Spawn(ctx context.Context, cfg SubagentConfig) (*Subagent, error)
    SpawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error)
    Interrupt(sa *Subagent, message string) error
    Collect(sa *Subagent) *SubagentResult  // non-blocking; nil if not finished
    Close() error
}

type ManagerOpts struct {
    ParentCtx context.Context
    ParentID  string
    Depth     int
    Executor  ToolExecutor
    Registry  SubagentRegistry
    NewRunner func() Runner
}

func NewManager(opts ManagerOpts) SubagentManager { ... }
```

`Spawn` lifecycle:
1. Take `mu`; if `depth >= MaxDepth`, return `ErrMaxDepth`.
2. Apply config defaults (`MaxIterations`, `Timeout`).
3. Create child ctx: `WithCancel(parentCtx)`, then `WithTimeout` if `cfg.Timeout > 0`.
4. Allocate `Subagent` with `publicEvents` (buf 16) and `done` (unbuffered).
5. Insert into `children` and `registry.Register`.
6. Spawn lifecycle goroutine; release `mu`; return.

**Runner contracts** (binding on every `Runner` implementation):
1. `Run` MUST return promptly after `ctx.Done()` fires. "Promptly" means within a small bounded time, not blocked forever.
2. `Run` MUST NOT close the events channel. The manager owns the channel lifecycle.
3. `Run` MAY emit zero or more events.
4. `Run` MUST return a non-nil `*SubagentResult`.

**Lifecycle invariants** (enforced by the manager goroutine):
1. `publicEvents` is closed exactly once, after the runner has returned.
2. `EventInterrupted` (if emitted) is written before `publicEvents` closes.
3. `sa.done` is closed exactly once, after `sa.result` is set and after `publicEvents` has closed.
4. `setResult` is called exactly once per subagent.

Lifecycle goroutine (illustrative):
```go
internalEvents := make(chan SubagentEvent, 16)
resultCh       := make(chan *SubagentResult, 1)
runnerDone     := make(chan struct{})

// runner wrapper
go func() {
    defer close(runnerDone)
    defer func() {
        if r := recover(); r != nil {
            resultCh <- &SubagentResult{
                Status: StatusError, ExitReason: "panic",
                Error: fmt.Sprintf("%v", r),
            }
        }
    }()
    resultCh <- runner.Run(sa.ctx, sa.cfg, internalEvents)
}()

// forwarder: drains internalEvents to publicEvents until internalEvents closes
go func() {
    defer close(sa.publicEvents)
    for ev := range internalEvents {
        sa.publicEvents <- ev
    }
}()

// observe cancellation; emit EventInterrupted before runner finalises
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

// wait for runner to return (contract: returns promptly on ctx.Done)
result := <-resultCh

// fill fields the runner can't know
result.ID = sa.ID
if result.Duration == 0 {
    result.Duration = time.Since(start)
}

// ordered cleanup
close(internalEvents)            // forwarder drains and closes publicEvents
if sa.timeoutCancel != nil {
    sa.timeoutCancel()           // release WithTimeout resources
}
sa.setResult(result)
close(sa.done)
m.removeChild(sa.ID)
m.registry.Unregister(sa.ID)
```

The forwarder does NOT select on `ctx.Done()` — if it returned early, events buffered before cancellation would be lost. Instead, it drains `internalEvents` to completion; the runner's own ctx-respect contract bounds how long that takes.

`SpawnBatch`:
- Validate `maxConcurrent > 0`; default to `DefaultMaxConcurrent`.
- Allocate `results []*SubagentResult` of length `len(cfgs)` so output order matches input order.
- Use a `chan struct{}` of size `maxConcurrent` as a semaphore.
- For each `cfg[i]`: acquire semaphore, `Spawn`, then in the same goroutine `WaitForResult(ctx)` and write `results[i]`.
- `errgroup.Group` (golang.org/x/sync/errgroup) coordinates the wait. **No `time.Sleep` polling anywhere.**

`Interrupt(sa, msg)`:
- Look up by `sa.ID`; if absent, return `ErrSubagentNotFound`.
- `sa.interruptMsg.Store(msg)` (atomic write; happens-before `sa.cancel()`).
- Call `sa.cancel()`. The lifecycle's interrupt-observer goroutine reads `interruptMsg`, emits `EventInterrupted{Message: msg}` on the internal events channel, and the runner returns. Idempotent: calling Interrupt twice is safe (second cancel is a no-op).

`Collect(sa)`:
- Returns `sa.result` snapshot; nil if `sa.done` is not yet closed.

`Close()`:
- Cancels every child, waits for each `sa.done` to close, returns nil. Idempotent — guarded by `sync.Once` so repeated calls are no-ops after the first completes.

Error sentinels:
```go
var (
    ErrMaxDepth         = errors.New("subagent: max depth reached")
    ErrSubagentNotFound = errors.New("subagent: not found")
)
```

`Spawn` does not enforce a max-concurrent ceiling — `SpawnBatch` is the only place where bounded parallelism applies. A future subphase may add a per-manager hard cap if observed need warrants it.

---

## ToolExecutor

```go
// internal/tools/executor.go

type ToolExecutor interface {
    Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error)
}

type ToolRequest struct {
    AgentID  string
    ToolName string
    Input    json.RawMessage
    Metadata map[string]string
}

type ToolEvent struct {
    Type   string         // "started" | "output" | "completed" | "failed"
    Output json.RawMessage
    Err    error
}

type InProcessToolExecutor struct {
    registry *Registry  // existing tools.Registry
}

func NewInProcessToolExecutor(reg *Registry) *InProcessToolExecutor {
    return &InProcessToolExecutor{registry: reg}
}

func (e *InProcessToolExecutor) Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error) {
    tool, ok := e.registry.Get(req.ToolName)
    if !ok {
        return nil, fmt.Errorf("%w: %s", ErrUnknownTool, req.ToolName)
    }

    ch := make(chan ToolEvent, 4)

    go func() {
        defer close(ch)

        ch <- ToolEvent{Type: "started"}

        timeout := tool.Timeout()
        execCtx := ctx
        if timeout > 0 {
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

Wraps the **real** `tools.Tool` interface (`Execute(ctx, json.RawMessage) (json.RawMessage, error)` with `Timeout() time.Duration`). No fictional `Handler` or `ToolEntry`.

---

## delegate_task Tool

```go
// internal/subagent/delegate_tool.go

type DelegateTool struct {
    manager SubagentManager
}

func NewDelegateTool(m SubagentManager) *DelegateTool { return &DelegateTool{manager: m} }

func (*DelegateTool) Name() string        { return "delegate_task" }
func (*DelegateTool) Description() string { return "Delegate a task to a subagent for parallel execution. Returns a structured JSON result." }
func (*DelegateTool) Timeout() time.Duration { return 0 } // governed by subagent timeout instead

func (*DelegateTool) Schema() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "goal":           {"type": "string", "description": "Task goal"},
            "context":        {"type": "string", "description": "Optional context"},
            "max_iterations": {"type": "integer"},
            "toolsets":       {"type": "string", "description": "Comma-separated toolset names"}
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
        // caller's ctx cancelled — propagate cancel to subagent before returning
        _ = t.manager.Interrupt(sa, "parent ctx cancelled")
        return nil, err
    }

    return json.Marshal(map[string]any{
        "id":          result.ID,
        "status":      string(result.Status),
        "summary":     result.Summary,
        "exit_reason": result.ExitReason,
        "duration_ms": result.Duration.Milliseconds(),
        "iterations":  result.Iterations,
        "error":       result.Error,
    })
}
```

Registered in main.go (or wherever the tool registry is built) when `[delegation].enabled = true`.

---

## Configuration

```toml
[delegation]
enabled                 = true
max_depth               = 2
max_concurrent_children = 3
default_max_iterations  = 50
default_timeout         = "1h"
```

```go
// internal/config/config.go

type DelegationCfg struct {
    Enabled               bool          `toml:"enabled"`
    MaxDepth              int           `toml:"max_depth"`
    MaxConcurrentChildren int           `toml:"max_concurrent_children"`
    DefaultMaxIterations  int           `toml:"default_max_iterations"`
    DefaultTimeout        time.Duration `toml:"default_timeout"`
}
```

Defaults are applied at Spawn time, not at TOML decode time, so zero-value semantics stay clean.

---

## Cancellation Semantics

| Trigger                      | Effect                                                                   |
|------------------------------|--------------------------------------------------------------------------|
| Parent ctx cancelled         | `WithCancel(parentCtx)` → child ctx done → cascades to grandchildren     |
| `Manager.Interrupt(sa, msg)` | Emits `EventInterrupted{Message: msg}`, then `sa.cancel()`                |
| `Manager.Close()`            | `sa.cancel()` for every child, waits on every `sa.done`                  |
| Subagent timeout             | `WithTimeout` on child ctx fires → same path as parent cancellation      |
| Runner panic                 | Recovered in runner goroutine; result emitted with `Status=error`        |

The forwarder goroutine respects `sa.ctx.Done()` so a cancelled subagent does not block on a slow consumer of `Events()`.

---

## Resource Limits

| Resource        | Limit                            | Enforcement                                                  |
|-----------------|----------------------------------|--------------------------------------------------------------|
| Max depth       | 2 (grandchild rejected)          | `if m.depth >= MaxDepth { return ErrMaxDepth }`              |
| Max concurrent  | 3 default, configurable          | `SpawnBatch` semaphore                                       |
| Max iterations  | Stored on cfg, unused this slice | StubRunner ignores; LLMRunner will honour in 2.E.7           |
| Timeout         | `cfg.Timeout` (0 = none)         | `context.WithTimeout` on child ctx                           |
| Memory          | Not enforced                     | Removed from spec — `runtime.GC()` is not a real bound       |

---

## TDD Discipline

All commits in this slice are **green**. The discipline:

1. Write the failing test in the working tree. Do not commit.
2. Implement the minimum to make it pass.
3. Refactor with the green tests as a safety net.
4. Run `go test ./internal/subagent/... ./internal/tools/... -race -shuffle=on -v` and confirm clean.
5. Commit test + impl together.

Why `-shuffle=on`: this subsystem is goroutines, channels, and shared state. `-race` catches data races; `-shuffle=on` catches order-dependent test bugs (one test leaving a goroutine alive that perturbs the next, for example) which `-race` alone misses.

Each task in the implementation plan = one commit. The plan is structured so every commit:
- Compiles (`go build ./...`)
- Passes its tests under `-race -shuffle=on`
- Does not regress earlier tests

If a task can't end green, it's the wrong task boundary — split it.

---

## Acceptance Criteria

1. `Manager.Spawn(ctx, cfg)` returns a `*Subagent` whose `Events()` channel emits `started → completed` for `StubRunner`, then closes.
2. `sa.WaitForResult(ctx)` resolves to a non-nil `*SubagentResult` with `ExitReason = "stub_runner_no_llm_yet"`.
3. `Manager.Interrupt(sa, msg)` causes `sa.WaitForResult` to return a result with `Status = interrupted`; `Events()` carries an `EventInterrupted{Message: msg}` before close.
4. Cancelling the parent ctx cascades to every live subagent (verified by a test that spawns N subagents whose StubRunners block on ctx).
5. Spawning at `depth >= MaxDepth` returns `ErrMaxDepth`. Parent depth=0, child=1 is OK, grandchild=2 rejected.
6. `Manager.SpawnBatch(ctx, cfgs, n)` runs at most `n` in parallel (verified by a test using a runner that signals concurrency); returns results in input order; uses zero `time.Sleep`.
7. `delegate_task` end-to-end: schema validates → spawn → wait → JSON `{id, status, summary, exit_reason, duration_ms, iterations, error}`.
8. `InProcessToolExecutor.Execute` delegates to the real `tools.Tool` interface, honours `tool.Timeout()`, and emits `started → output → completed` (or `failed`).
9. `go test ./... -race -shuffle=on -v` passes after every commit on the branch.
10. Binary size delta < 200 KB (no LLM integration in this slice).

---

## Subphase Ledger

| Subphase | Status | Description |
|---|---|---|
| 2.E.1 — Data structures + IDs | ⏳ planned | `Subagent`, `SubagentConfig`, `SubagentEvent`, `SubagentResult`, `BlockedTools`, crypto/rand IDs |
| 2.E.2 — `Runner` interface + `StubRunner` | ⏳ planned | Swappable runner; stub emits started→completed |
| 2.E.3 — `SubagentRegistry` | ⏳ planned | Process-wide tracker; Register/Unregister/InterruptAll/List |
| 2.E.4 — `SubagentManager`: Spawn / Interrupt / Collect / Close | ⏳ planned | Lifecycle goroutine, forwarder, channel-based completion |
| 2.E.5 — `SpawnBatch` with semaphore + errgroup | ⏳ planned | Bounded parallelism; results in input order; no polling |
| 2.E.6 — `ToolExecutor` + `InProcessToolExecutor` + `delegate_task` tool + config | ⏳ planned | Wraps real `tools.Tool`; tool registered when `[delegation].enabled` |
| 2.E.7 — Real LLM runner | ⏸ deferred | Pending Phase 4 architecture clarity |
| 2.E.8 — CLI: `/delegate`, `/subagents`, `/stop-subagent` | ⏸ deferred | Waits on 2.E.7 |

**Ship criterion:** `delegate_task(goal="...", context="...")` returns a JSON result. `Interrupt` and parent-ctx cancellation both cause `Status=interrupted`. Grandchildren are rejected. `go test ./... -race -shuffle=on` is green on every commit. Binary size delta < 200 KB.

---

## Dependencies

- `context`, `crypto/rand`, `encoding/base32`, `encoding/json`, `errors`, `fmt`, `strings`, `sync`, `time` (stdlib)
- `golang.org/x/sync/errgroup` (already in go.sum if present; otherwise added in 2.E.5)
- `internal/tools` — `Registry`, `Tool` interface
- `internal/config` — adds `DelegationCfg`

No new heavy deps. Binary size budget < 200 KB delta.

---

## Forward-Looking Notes

- **Why deferring the LLM runner is right:** Phase 4 (Brain Transplant) defines the native Go agent loop. Building an LLMRunner inside 2.E now would either duplicate Phase 4's loop or pre-commit to its shape. The `Runner` interface is the seam; 2.E.7 bolts the real runner in once Phase 4 has produced a reusable inner loop.
- **Why CLI is deferred:** without an LLM runner, `/delegate` from the TUI would only ever produce `stub_runner_no_llm_yet`. The CLI surface is more useful when bundled with 2.E.7.
- **BlockedTools list is forward-looking:** of the five names, only `delegate_task` exists today. The list is documented now to fix the policy in writing; enforcement of `EnabledTools` filtering inside the runner lands with 2.E.7.
