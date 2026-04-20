# Gormes Phase 2.A — Tool Registry Design Spec

**Date:** 2026-04-19
**Author:** Xel (via Claude Code brainstorm)
**Status:** Approved for plan phase
**Scope:** Phase 2.A of the 5-phase roadmap — the first executable step of the "Gateway / Wiring Harness" phase.

**Vocabulary decision:** there is no `internal/gateway` package in Phase 2. The "Gateway" remains a marketing term for the conceptual boundary between Gormes and external services. The Go-native artefact is `internal/tools` — a Tool interface, an in-process Registry, and a kernel extension that executes tool calls inside the turn loop.

## Related Documents

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Specs Index](README.md)
- [Phase 2.B.1 — Telegram Scout](2026-04-19-gormes-phase2b-telegram.md)

---

## 1. Purpose

Give the Gormes kernel the ability to execute **Go-native tools** in response to LLM `tool_calls`, without calling into Python. The LLM still runs inside Python's `api_server` in Phase 2; only the tool-execution step moves to Go. This is the first concrete step that shrinks Python's territory inside the 5-phase "Ship of Theseus" rewrite.

A secondary goal: establish the **interface shape** for Go-native tools so that future integrations — including the FeCIM Lattice physics package — plug in without re-architecting.

---

## 2. Relationship to Phase 1 and Phase 1.5

| Phase | What it owns |
|---|---|
| Phase 1 | TUI render + kernel state machine + hermes HTTP/SSE client + zero-store |
| Phase 1.5 | Route-B reconnect resilience + compat-probe + discipline tests + `.ai` rename |
| **Phase 2.A (this spec)** | **`internal/tools` + `tool_calls` flow in `runTurn`** |
| Phase 2.B (future) | Multi-platform gateway adapters (Telegram/Discord/Slack in Go) |
| Phase 3 | `internal/brain` — prompt assembly, native LLM provider |
| Phase 4 | Agent-kernel ownership of the LLM call — Python stops being the brain |
| Phase 5 | Python tool scripts retired; pure Go (or WASM) tools only |

Phase 2.A does NOT:
- Bind a port
- Proxy Python's `api_server`
- Introduce an MCP client or server (`internal/tools` stays MCP-agnostic)
- Create `internal/brain`
- Implement prompt assembly in Go
- Ship real FeCIM integration (only the interface shape + a stub Tool)

Phase 2.A DOES:
- Add the `Tool` interface and in-process `Registry`
- Ship 2–3 built-in Go-native tools as proof-of-life (`echo`, `now`, `rand`)
- Extend `hermes.ChatRequest` with a `Tools` field and `hermes.Event`/`Message` with tool-call plumbing
- Teach the SSE stream parser to accumulate tool-call deltas
- Wrap `runTurn`'s retry loop in a **tool loop** that executes tools between stream iterations
- Add the `internal/tools/fecim` package skeleton (interface-only, no physics yet)

---

## 3. Locked Architectural Decisions

| Decision | Value | Rationale |
|---|---|---|
| G-1 Vocabulary | No Gateway package. `internal/tools` + extensions to `internal/hermes` and `internal/kernel` | The "Gateway" fossilises a proxy concept we don't need in Phase 2. Tools are the real artefact. |
| G-2 Port | No new listener | Python keeps `:8642`. Port ownership flips in Phase 4. |
| M-1 MCP | Neither client nor server | Tool interface is MCP-agnostic; adapters ship when we actually need to bridge MCP. |
| M-2 Registration | Static Go Registry populated explicitly by `main.go` | No init() magic. Testable. Matches Phase-5 "100% Go tools" endgame. |
| B-1 Brain | Not in Phase 2 | Tool-call handling is turn-lifecycle work; stays in `kernel`. `internal/brain` lands in Phase 3. |
| Tool-call limit | 10 iterations per turn | Prevents runaway multi-step agent loops; configurable via `Config.MaxToolIterations`. |
| Tool exec timeout | 30 s default | Configurable per Tool via `Tool.Timeout()`; hard-capped at `Config.MaxToolDuration`. |
| Error-on-tool-panic | Recover, return as tool-result error | Never crash the kernel because a Tool did something stupid. |

---

## 4. Package Layout

```
gormes/internal/
├── tools/
│   ├── tool.go                  # Tool interface + ToolDescriptor + Registry
│   ├── tool_test.go             # registry collision, list, execute-unknown tests
│   ├── builtin.go               # Echo, Now, Rand tools (proof-of-life)
│   ├── builtin_test.go
│   └── fecim/
│       ├── fecim.go             # FecimTool skeleton (stub Execute returns canned JSON)
│       └── fecim_test.go        # proves FecimTool satisfies Tool
├── hermes/
│   ├── client.go                # MODIFY — ChatRequest gets Tools; Event gets ToolCalls; Message gets ToolCalls + ToolCallID
│   ├── stream.go                # MODIFY — accumulate tool_call deltas across chunks
│   └── (all other files unchanged)
└── kernel/
    ├── kernel.go                # MODIFY — wrap retry loop in tool loop; dispatch tool_calls via Registry
    ├── toolexec.go              # NEW — executeToolCalls helper with recover + timeout
    ├── toolexec_test.go         # NEW — executeToolCalls unit tests
    └── tools_test.go            # NEW — red test for kernel/Tool integration
```

No changes to `internal/config`, `internal/store`, `internal/telemetry`, `internal/tui`, or `internal/pybridge`.

---

## 5. The `Tool` Interface

```go
// Package tools defines the Go-native tool surface that the kernel can
// execute when the LLM emits tool_calls. Every Tool is a Go type compiled
// into the Gormes binary; the Registry is populated explicitly by main.go.
package tools

type Tool interface {
    // Name is the identifier the LLM uses in tool_calls[].function.name.
    // Must be unique within a Registry and match [A-Za-z_][A-Za-z0-9_-]{0,63}.
    Name() string

    // Description is the human-readable text sent to the LLM in the
    // tool definition. Keep under 256 chars for token economy.
    Description() string

    // Schema returns the JSON-Schema for the tool's arguments, matching
    // OpenAI's tool-calling spec (a JSON object with "type":"object",
    // "properties": {...}, "required": [...]). Must be a complete, valid
    // schema — the kernel passes it verbatim to Python's api_server.
    Schema() json.RawMessage

    // Timeout returns the per-call execution budget. Returning zero means
    // use the Config default (30s). Must not exceed Config.MaxToolDuration.
    Timeout() time.Duration

    // Execute runs the tool with the given JSON args. Returns the result
    // payload as JSON (OpenAI expects strings for tool-result content, so
    // the kernel will json.Marshal this).
    //
    // Context respects cancellation from the enclosing turn's runCtx
    // (which cascades from the root ctx). Honour it promptly.
    //
    // A returned error becomes an error-shaped tool result forwarded to
    // the LLM, not a kernel-level fatal. The LLM chooses what to do.
    Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}
```

### 5.1 Descriptor (what gets sent to the LLM)

```go
type ToolDescriptor struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Schema      json.RawMessage `json:"parameters"` // OpenAI uses "parameters"
}

// Serialised form matches OpenAI tool definition:
//   {"type":"function","function":{"name":"...","description":"...","parameters":{...}}}
// The Marshal wraps the descriptor accordingly.
```

### 5.2 Registry

```go
type Registry struct {
    mu    sync.RWMutex
    tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

// Register adds a Tool. Returns ErrDuplicate on name collision. Safe for
// concurrent use though the expected call-site is main.go at startup.
func (r *Registry) Register(t Tool) error

// MustRegister is the main-package convenience; panics on collision.
func (r *Registry) MustRegister(t Tool)

// Get returns the Tool for name, or (nil, false).
func (r *Registry) Get(name string) (Tool, bool)

// Descriptors returns the ToolDescriptor list for ChatRequest.Tools.
// Stable-sorted by Name for deterministic request bodies (makes
// diffing easier in tests and logs).
func (r *Registry) Descriptors() []ToolDescriptor

var ErrDuplicate = errors.New("tools: duplicate tool name")
var ErrUnknownTool = errors.New("tools: unknown tool name")
```

**Registration policy: explicit from `cmd/gormes/main.go`.** Each built-in is registered via `reg.MustRegister(&tools.EchoTool{})` at startup. This is chosen over `init()` auto-registration for two concrete reasons:
1. Tests construct a registry with a *subset* of tools (e.g. just `MockTool`) without having to carry every global init. With `init()`, any import of the `tools` package pulls every registered tool into every test binary.
2. The registry is an explicit dependency passed into `kernel.Config.Tools` — a plain value, not a package-level global. Future multi-tenant or per-session scenarios are unconstrained.

`init()` registration is **not forbidden** for third-party packages outside Gormes's core (e.g. a community-maintained tool pack may use it). The Registry type accepts either pattern.

---

## 6. Built-in Tools (proof-of-life)

Three tools ship in `internal/tools/builtin.go`:

| Name | Args | Returns | Purpose |
|---|---|---|---|
| `echo` | `{"text": string}` | `{"text": string}` | Round-trip test, no side effects |
| `now` | `{}` | `{"unix": int, "iso": string}` | Current time, proves zero-arg tools work |
| `rand_int` | `{"min": int, "max": int}` | `{"value": int}` | Bounds-checked random int, proves arg validation |

Each is a 30-line Go type implementing `Tool`. The Phase 2.A plan creates all three with full TDD coverage.

---

## 7. Hermes Client Extensions

### 7.1 `hermes.ChatRequest` — new `Tools` field

```go
type ChatRequest struct {
    Model     string
    Messages  []Message
    SessionID string
    Stream    bool
    Tools     []ToolDescriptor // NEW. nil means "don't send the tools field"
}
```

The serialised JSON body adds `"tools": [...]` only when the slice is non-empty. Existing tests unaffected.

### 7.2 `hermes.Message` — tool-call plumbing

```go
type Message struct {
    Role       string // existing; adds "tool" as a valid role
    Content    string
    ToolCalls  []ToolCall // assistant-role messages with tool_calls set; nil otherwise
    ToolCallID string     // tool-role messages reply to this tool_call id; empty otherwise
    Name       string     // tool-role messages echo the tool name; empty for non-tool
}

type ToolCall struct {
    ID        string          // e.g. "call_abc123"
    Name      string          // function name; matches a Tool.Name()
    Arguments json.RawMessage // JSON object, already complete (not a partial delta)
}
```

Serialised JSON per message:
- role="assistant" with tool_calls: `{"role":"assistant","content":"","tool_calls":[...]}`
- role="tool": `{"role":"tool","tool_call_id":"call_abc123","name":"echo","content":"{\"text\":\"hi\"}"}`

### 7.3 `hermes.Event` — tool-call signal

```go
type Event struct {
    Kind         EventKind
    Token        string
    Reasoning    string
    FinishReason string
    TokensIn     int
    TokensOut    int
    ToolCalls    []ToolCall // NEW. Set only on EventDone with FinishReason=="tool_calls"
    Raw          json.RawMessage
}
```

No new `EventKind` — the existing `EventDone` with `FinishReason=="tool_calls"` is the marker.

### 7.4 SSE stream parser — accumulate tool-call deltas

OpenAI streams tool_calls as partial deltas, typically:

```
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_x","type":"function","function":{"name":"echo","arguments":""}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"tex"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"t\":\"hi\"}"}}]}}]}
data: {"choices":[{"finish_reason":"tool_calls"}]}
```

The `chatStream` inside `gormes/internal/hermes/stream.go` gains a `pendingCalls map[int]*partialCall` that accumulates by `index`. When a chunk carries `finish_reason: "tool_calls"`, the stream emits a single `EventDone` with `FinishReason=="tool_calls"` and `ToolCalls` populated from the pending map (indexes sorted ascending, so ordering is deterministic).

Malformed tool-call deltas (bad JSON, missing index) are silently dropped — matches the Phase-1 policy of "skip malformed frames, keep stream alive". Logged at slog DEBUG.

---

## 8. Kernel: the Tool Loop

Current `runTurn` has a **retry loop** (for network drops). Phase 2.A wraps it in an outer **tool loop** (for tool-call rounds). The retry loop stays unchanged in semantics; the tool loop is new.

### 8.1 Flow

```
toolIteration := 0
request := ChatRequest{Messages: [user], Tools: registry.Descriptors()}

toolLoop:
    retryBudget := NewRetryBudget()   // fresh per tool iteration
    retryLoop:
        stream = client.OpenStream(request)
        outcome = streamInner(stream)
        close stream
        switch outcome:
            Retryable → backoff + continue retryLoop
            Fatal     → finalize as failed, return
            Cancelled → finalize as cancelled, return
            Done:
                break out of retryLoop

    // retryLoop exited cleanly — inspect why
    if !gotFinal:
        treat as fatal, finalize failed

    if finalDelta.FinishReason == "tool_calls":
        toolIteration++
        if toolIteration > cfg.MaxToolIterations:
            finalize failed with "tool iteration limit exceeded"
            return

        // Execute each call via executeToolCalls helper
        results := k.executeToolCalls(runCtx, finalDelta.ToolCalls)

        // Append assistant message with tool_calls, then one tool message per result
        assistantMsg := Message{Role: "assistant", Content: k.draft, ToolCalls: finalDelta.ToolCalls}
        request.Messages = append(request.Messages, assistantMsg)
        for _, result := range results:
            request.Messages = append(request.Messages, Message{
                Role: "tool", ToolCallID: result.ID, Name: result.Name, Content: result.Content,
            })

        // Preserve draft across tool iterations? NO. Clear it — next LLM
        // response starts fresh. The assistant message we just appended
        // captures what we had so far.
        k.draft = ""
        k.emitFrame("executing tools")
        continue toolLoop
    
    // FinishReason is "stop" or similar — actual end of turn
    break toolLoop

// finalize normally (append assistant msg to history, PhaseIdle, etc)
```

**Important:** the retry loop's draft-preservation semantic (Route B) is DIFFERENT from the tool loop's. Inside a retry, `replaceOnNextToken` preserves the draft visually across network drops. Inside the tool loop, the draft IS cleared between iterations — each LLM turn is genuinely a new response.

### 8.2 `executeToolCalls` helper (`kernel/toolexec.go`)

```go
type toolResult struct {
    ID      string
    Name    string
    Content string // JSON string — even errors are JSON-encoded
}

// executeToolCalls runs each tool call with per-call timeout and panic recovery.
// Returns results in the SAME ORDER as the input calls. Honours runCtx
// cancellation between calls.
func (k *Kernel) executeToolCalls(runCtx context.Context, calls []hermes.ToolCall) []toolResult {
    results := make([]toolResult, len(calls))
    for i, call := range calls {
        select {
        case <-runCtx.Done():
            results[i] = toolResult{
                ID: call.ID, Name: call.Name,
                Content: `{"error":"cancelled before execution"}`,
            }
            continue
        default:
        }

        tool, ok := k.tools.Get(call.Name)
        if !ok {
            results[i] = toolResult{
                ID: call.ID, Name: call.Name,
                Content: fmt.Sprintf(`{"error":"unknown tool: %q"}`, call.Name),
            }
            k.addSoul("tool unknown: " + call.Name)
            continue
        }

        timeout := tool.Timeout()
        if timeout <= 0 {
            timeout = k.cfg.MaxToolDuration // 30s default
        }

        // Per-call context cascaded from runCtx.
        callCtx, cancel := context.WithTimeout(runCtx, timeout)

        k.addSoul("tool: " + call.Name)
        k.emitFrame("executing tool: " + call.Name)

        payload, err := safeExecute(callCtx, tool, call.Arguments)
        cancel()

        if err != nil {
            results[i] = toolResult{
                ID: call.ID, Name: call.Name,
                Content: fmt.Sprintf(`{"error":%q}`, err.Error()),
            }
            k.addSoul("tool error: " + call.Name + ": " + err.Error())
            continue
        }
        results[i] = toolResult{ID: call.ID, Name: call.Name, Content: string(payload)}
        k.addSoul("tool done: " + call.Name)
    }
    return results
}

// safeExecute wraps Tool.Execute with panic recovery so a misbehaving tool
// cannot crash the kernel.
func safeExecute(ctx context.Context, t tools.Tool, args json.RawMessage) (result json.RawMessage, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("tool panicked: %v", r)
            result = nil
        }
    }()
    return t.Execute(ctx, args)
}
```

### 8.3 `Kernel.Config` extensions

```go
type Config struct {
    Model              string
    Endpoint           string
    Admission          Admission
    Tools              *tools.Registry // nil means no tools
    MaxToolIterations  int             // default 10
    MaxToolDuration    time.Duration   // default 30s
}
```

A nil `Tools` registry means the kernel sends `ChatRequest.Tools = nil` and treats any `finish_reason: "tool_calls"` as a fatal error ("received tool_calls but no registry configured").

---

## 9. External Tool Integration Shape (generic)

**Gormes ships NO domain-specific tools.** The built-ins in §6 (Echo, Now, RandInt) are the entirety of Gormes's tool surface in Phase 2.A. Scientific or business-domain tools — FeCIM (ferroelectrics), any Trebuchet Dynamics physics package, any third-party Go module — live in **their own repositories** and ship as **separate Go modules** that consumers import alongside Gormes.

This decision is a first-class boundary, not an oversight:

- **Gormes stays domain-neutral.** The public binary includes only general-purpose agent skills, which keeps the package tree, test matrix, and documentation focused on agentic capabilities. Scientific tooling evolves on its own release cycle.
- **Licensing + IP cleanliness.** Private scientific packages don't end up vendored into a public agent binary. The user imports them explicitly in their own `cmd/gormes` fork when needed.
- **Clean `tools.Tool` interface.** The interface is stable enough that external packages can satisfy it without any Gormes-side changes.

### 9.1 How an external package wraps itself as a Tool

Any Go package (`example.com/mylab/lattice`) exposes a Tool by defining a type that satisfies `tools.Tool`:

```go
package latticetool

import (
    "context"
    "encoding/json"
    "time"

    "github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
    "example.com/mylab/lattice"
)

type HysteresisTool struct{}

var _ tools.Tool = (*HysteresisTool)(nil)

func (*HysteresisTool) Name() string                { return "lattice_hysteresis" }
func (*HysteresisTool) Description() string         { return "..." }
func (*HysteresisTool) Schema() json.RawMessage     { return json.RawMessage(`{...}`) }
func (*HysteresisTool) Timeout() time.Duration      { return 30 * time.Second }
func (*HysteresisTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
    // unmarshal args, call into lattice package, marshal result
    return json.Marshal(lattice.ComputeHysteresis(/* ... */))
}
```

The consumer's fork of Gormes registers it at startup:

```go
// cmd/gormes/main.go in the consumer's private fork
reg := tools.NewRegistry()
reg.MustRegister(&tools.EchoTool{})
reg.MustRegister(&tools.NowTool{})
reg.MustRegister(&tools.RandIntTool{})
reg.MustRegister(&latticetool.HysteresisTool{})  // third-party
```

### 9.2 What this means for Phase 2.A

- No `internal/tools/fecim/` directory is created.
- No `internal/tools/<any-domain>/` is created.
- Phase 2.A's test matrix uses only the three built-ins plus `MockTool` for kernel integration tests.
- The "Scientific Handshake" test in §11.2 is renamed **Tool-Call Handshake** and uses the built-in `Echo` tool — same end-to-end proof that the Kernel↔Tool contract works, no domain entanglement.

---

## 10. Error Handling

| Failure mode | Where | What happens |
|---|---|---|
| Unknown tool name from LLM | `executeToolCalls` | Returns `{"error":"unknown tool: X"}` as tool result; LLM re-plans |
| Tool.Execute panics | `safeExecute` | Recovers, returns `{"error":"tool panicked: ..."}` |
| Tool.Execute returns error | `executeToolCalls` | Returns `{"error":"..."}` as tool result |
| Tool exceeds per-call timeout | context deadline inside `Execute` | Tool should return ctx.Err(); result is error-shaped |
| Tool iteration limit exceeded | `runTurn` tool loop | PhaseFailed with "tool iteration limit exceeded" |
| Malformed tool_call delta from server | `stream.go` pending-calls map | Drop, log at DEBUG, continue |
| `nil` registry in config | `runTurn` on receipt of `tool_calls` finish_reason | PhaseFailed with "received tool_calls but no registry configured" |

None of these crash the kernel or the process.

---

## 11. Testing Strategy

### 11.1 Unit tests (all green at merge)

- `tools/tool_test.go`:
  - `TestRegistry_RegisterDuplicateReturnsError`
  - `TestRegistry_MustRegister_PanicsOnDuplicate`
  - `TestRegistry_GetUnknown_ReturnsFalse`
  - `TestRegistry_DescriptorsSorted` — Descriptors() is deterministic by name
- `tools/builtin_test.go`: exercise Echo, Now, Rand happy paths + arg-validation edges.
- `tools/mock_test.go`: **MockTool** implementation shared across kernel tests (see §11.4).
<!-- No fecim tests — Gormes ships no domain-specific tools (see §9). -->

- `kernel/toolexec_test.go`:
  - `TestExecuteToolCalls_UnknownToolReturnsErrorResult`
  - `TestExecuteToolCalls_PanicRecovered`
  - `TestExecuteToolCalls_TimeoutHonored`
  - `TestExecuteToolCalls_CancelBetweenCalls` — runCtx cancel halts the loop
- `hermes/stream_tools_test.go`:
  - `TestStream_ToolCallDeltasAccumulate` — fixture with split tool-call deltas produces a single EventDone with correct ToolCall args

### 11.2 Red test (ships `t.Skip`'d)

- `kernel/tools_test.go::TestKernel_ExecutesToolCallsEndToEnd`:
  MockClient scripts TWO stream rounds:
  - Round 1 emits `tool_calls: [{echo, {"text":"hi"}}]` + finish_reason="tool_calls"
  - Round 2 emits `content: "ok"` + finish_reason="stop"
  Test registers the `echo` tool, submits a turn, expects final history to contain the round-2 assistant message, and the tool-round-trip to have invoked `echo` with `{"text":"hi"}`.

  Shipped with `t.Skip("RED TEST: Tool loop — plan Task 5 flips this")` so Plan Task 5 is the flip.

### 11.3 Phase-1 / Phase-1.5 regression

All 10 existing kernel discipline tests + 12 hermes tests + teatest + stall test + Route-B reconnect **MUST still pass** under `-race`. The ChatRequest/Event extensions are additive (new optional fields), so no existing test should require changes.

### 11.4 `MockTool` + Phase-1.5 invariant-preservation tests

`internal/tools/mock.go` ships a configurable test double:

```go
type MockTool struct {
    NameStr    string
    Desc       string
    SchemaJSON json.RawMessage
    TimeoutD   time.Duration
    // ExecuteFn controls behaviour: return a result, sleep, panic, block
    // until ctx cancellation, etc.
    ExecuteFn func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}
// var _ tools.Tool = (*MockTool)(nil)
```

Two new tests live in `kernel/tools_invariants_test.go` and prove that Phase-2 tool execution does NOT break Phase-1.5's stability guarantees:

- **`TestToolLoop_DoesNotBreakReplaceLatestMailbox`** — a MockTool that takes 1 second to Execute; stall the render-channel consumer during tool execution; assert the kernel does not block on `emitFrame` and that the mailbox peek after the stall shows the LATEST state (identical pattern to `TestKernel_NonBlockingUnderTUIStall`, but with a tool call in the middle of the turn).

- **`TestToolLoop_SurvivesMidStreamNetworkDropBetweenToolRounds`** — 2-round tool loop: first LLM response is `tool_calls`, tool executes, second LLM response is streaming text. Chaos-monkey close the server mid-second-stream; assert Route-B reconnect (PhaseReconnecting → PhaseStreaming → Idle) still works correctly AFTER a tool round. Uses `stableProxy` + `fiveTokenHandler` from `reconnect_helpers_test.go`.

These two tests are the user-requested invariant-preservation guard: tool-calling is introduced without regressing the two hardest-won Phase-1.5 guarantees.

---

## 12. Success Criteria

1. `gormes/internal/tools/` compiles with `Tool` + `Registry` + 3 built-in tools + `MockTool` test double.
2. No `gormes/internal/tools/<domain>/` subdirectory exists — domain-specific tools are external modules (see §9).
3. `hermes.ChatRequest.Tools`, `hermes.Event.ToolCalls`, `hermes.Message.ToolCalls`/`ToolCallID`/`Name` fields exist and serialise correctly (per-field JSON tags verified by round-trip tests).
4. `stream.go` accumulates tool-call deltas and emits a single `EventDone` with `ToolCalls` populated when `finish_reason == "tool_calls"`.
5. `Kernel.Config` gains `Tools`, `MaxToolIterations`, `MaxToolDuration`.
6. `runTurn` executes a tool loop: on `finish_reason == "tool_calls"`, dispatches via `executeToolCalls`, appends tool messages, issues a follow-up stream. Up to `MaxToolIterations` iterations per turn.
7. `executeToolCalls` recovers tool panics, honours per-call timeouts, and returns error-shaped tool results without crashing the kernel.
8. Red test `TestKernel_ExecutesToolCallsEndToEnd` is `t.Skip`'d. Green test suite covers Registry, SSE parser extension, executeToolCalls, and the two Phase-1.5 invariant-preservation tests (stall + reconnect with tool loop in play).
9. All Phase-1 + Phase-1.5 tests still pass under `-race`.
10. `go vet ./...` clean.
11. `go build ./cmd/gormes` still produces a working binary; `./bin/gormes --offline` still renders the TUI.
12. No `internal/gateway` or `internal/brain` directory exists.

---

## 13. Explicit Out-of-Scope

| Feature | Where it belongs |
|---|---|
| MCP client / server | Phase 2.B or Phase 3, behind a `tools.MCPAdapter` wrapper |
| Subprocess-hosted tools | Phase 2.B via `tools.SubprocessTool` adapter |
| Real FeCIM physics computation | When F-1..F-5 are answered |
| Multi-platform gateway adapters (Telegram/Discord/Slack) | Phase 2.B |
| Prompt assembly in Go | Phase 3 `internal/brain/prompt` |
| Direct OpenRouter client in Go | Phase 3 `internal/brain/provider` |
| Flipping the red `TestKernel_ExecutesToolCallsEndToEnd` to green | Plan Task 5 of the Phase 2.A implementation plan |
| Tool-call streaming to the TUI (showing partial tool invocations) | Phase 2.A renders via Soul Monitor only; richer display is Phase 1.5+ TUI polish |
| Tool-call persistence to any DB | Phase 3 (Go owns storage then) |
| Concurrent tool execution (parallel calls in one LLM round) | Phase 2.A executes sequentially; parallelism is a future enhancement |

---

## 14. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Python `api_server` doesn't forward `tools` field verbatim to the upstream LLM | Confirmed via `/v1/chat/completions` OpenAI compatibility; fallback is Phase 3's native OpenRouter client |
| Python `api_server` strips `tool_calls` from the response stream | Live integration test after this ships; if so, fall back to Python-resident tools temporarily and escalate Phase 3 |
| Tool schema generation drift (Go struct vs. hand-written JSON-Schema) | Phase 2.A uses hand-written schemas; a code-generator from Go struct tags is a Phase 2.B/3 enhancement |
| LLM hallucinates tool names | `executeToolCalls` returns `{"error":"unknown tool"}` and the LLM re-plans — observed good behavior with both Claude and GPT-4-class models |
| Tool-loop infinite loop | `MaxToolIterations=10` hard cap |
| Tool panics inside goroutine spawned by Tool.Execute | `safeExecute` recovers only the calling goroutine; Tools are asked to contain their own goroutines (documented in `Tool.Execute` godoc) |
| `ChatRequest.Tools` omitempty serialisation gotcha (empty slice vs. nil) | JSON marshal with `,omitempty` and always pass nil (never empty slice) when no tools registered |

---

## 15. Next Step

After this spec is approved, `superpowers:writing-plans` produces the Phase 2.A implementation plan. Expected size: 7–9 tasks, estimated ~3–4 hours of subagent work.

This spec is the source of truth for *what* Phase 2.A is. The plan is the source of truth for *how* it gets built.
