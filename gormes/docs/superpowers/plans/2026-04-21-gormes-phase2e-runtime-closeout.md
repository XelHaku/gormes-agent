# Phase 2.E Runtime Core Closeout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining Phase 2.E runtime-core gaps in the current tree by aligning subagent lifecycle code/comments with shipped behavior, proving binary wiring end-to-end, and updating Phase 2 status docs to reflect the actual delivered runtime seam.

**Architecture:** The codebase already contains a working `internal/subagent` runtime, `[delegation]` config, and `delegate_task` tool registration. This plan is intentionally a closeout plan rather than a greenfield build. The work is to remove stale “later task” drift, add any missing integration proof around registry wiring, and document the ship criterion so Track A can be treated as a stable substrate while Track B lands the gateway chassis and Discord.

**Tech Stack:** Go 1.25+, stdlib (`context`, `time`), existing `internal/subagent`, `internal/config`, `cmd/gormes`, Hugo docs under `gormes/docs/content/building-gormes/architecture_plan/`.

**Spec:** [`../specs/2026-04-21-gormes-phase2-dual-track-pass-design.md`](../specs/2026-04-21-gormes-phase2-dual-track-pass-design.md)

---

## File Structure

**Modify:**
- `gormes/internal/subagent/manager.go` — remove misleading “implemented in a later task” comments now that the methods exist
- `gormes/internal/subagent/runner.go` — tighten the current StubRunner scope note so it reads as an intentional runtime seam, not an unbounded TODO
- `gormes/cmd/gormes/registry_test.go` — keep binary-level `delegate_task` wiring covered
- `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md` — mark 2.E as in progress or shipped runtime core, not merely planned
- `gormes/docs/content/building-gormes/architecture_plan/progress.json` — reflect the same ledger state
- `gormes/docs/content/building-gormes/architecture_plan/_index.md` — regenerate if the progress page is derived from `progress.json`

**Verify only:**
- `gormes/internal/subagent/*.go`
- `gormes/internal/config/*.go`
- `gormes/cmd/gormes/*.go`

---

## Task 1: Remove stale lifecycle drift from runtime comments

**Files:**
- Modify: `gormes/internal/subagent/manager.go`
- Modify: `gormes/internal/subagent/runner.go`

- [ ] **Step 1: Read the current comment drift before editing**

Run:

```bash
sed -n '1,320p' gormes/internal/subagent/manager.go
sed -n '1,220p' gormes/internal/subagent/runner.go
```

Expected: comments still mention “implemented in a later task” and “2.E.7” in places where the runtime is already intentionally wired and tested today.

- [ ] **Step 2: Patch the comments to match shipped behavior**

Update `gormes/internal/subagent/manager.go` so the comments above `Interrupt`, `Collect`, `Close`, and `SpawnBatch` describe what the methods actually do today instead of saying they are deferred.

Use comment text in this shape:

```go
// Interrupt records the message (for the final interrupted event) and
// cancels the subagent's context.
```

```go
// Collect returns the terminal result when the subagent is done, otherwise nil.
```

```go
// Close cancels every live child, waits for them to finish, and closes the
// manager exactly once.
```

```go
// SpawnBatch executes multiple subagent specs with bounded concurrency and
// returns one result per input config in order.
```

Update `gormes/internal/subagent/runner.go` so the `StubRunner` comment clearly says it is the intentionally shipped runtime seam for this slice, not a forgotten placeholder. Use wording like:

```go
// StubRunner is the runtime-core placeholder shipped in Phase 2.E closeout.
// It proves lifecycle, cancellation, and tool-surface wiring without yet
// adding a nested LLM loop.
```

- [ ] **Step 3: Format the touched files**

Run:

```bash
gofmt -w gormes/internal/subagent/manager.go gormes/internal/subagent/runner.go
```

Expected: no output.

- [ ] **Step 4: Run focused runtime tests**

Run:

```bash
cd gormes && go test ./internal/subagent -count=1 -race
```

Expected: `ok` for `internal/subagent`.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/runner.go
git commit -m "docs(subagent): align lifecycle comments with shipped runtime"
```

---

## Task 2: Prove binary-level delegation wiring stays intact

**Files:**
- Modify: `gormes/cmd/gormes/registry_test.go`

- [ ] **Step 1: Add a wiring test that executes the registered tool**

Append a test in `gormes/cmd/gormes/registry_test.go` with this shape:

```go
func TestBuildDefaultRegistryDelegationToolExecutes(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{
		Enabled:               true,
		MaxDepth:              2,
		MaxConcurrentChildren: 3,
		DefaultMaxIterations:  50,
		DefaultTimeout:        time.Second,
	})

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"audit runtime"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Contains(out, []byte(`"status":"completed"`)) {
		t.Fatalf("output = %s, want completed status", out)
	}
}
```

- [ ] **Step 2: Run the focused command-package tests and verify failure first**

Run:

```bash
cd gormes && go test ./cmd/gormes -run "TestBuildDefaultRegistryDelegation" -count=1
```

Expected: first run fails if imports or assertions are missing.

- [ ] **Step 3: Add any required imports and make the test pass**

Required imports for the new test will likely include:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)
```

- [ ] **Step 4: Re-run focused tests**

Run:

```bash
cd gormes && go test ./cmd/gormes -run "TestBuildDefaultRegistryDelegation" -count=1
```

Expected: `ok` for `cmd/gormes`.

- [ ] **Step 5: Commit**

```bash
git add gormes/cmd/gormes/registry_test.go
git commit -m "test(cmd): prove delegate_task wiring through default registry"
```

---

## Task 3: Update Phase 2 status docs to reflect runtime-core delivery

**Files:**
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/progress.json`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/_index.md`

- [ ] **Step 1: Read the current 2.E ledger text**

Run:

```bash
sed -n '1,220p' gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md
sed -n '1,220p' gormes/docs/content/building-gormes/architecture_plan/progress.json
```

Expected: 2.E is still described as planned even though the runtime core is already in-tree and wired.

- [ ] **Step 2: Update the ledger text**

Adjust the 2.E entry so it no longer reads as untouched planning. Use language in this shape:

```md
| **Phase 2.E — Subagent System** | 🔨 in progress | **P0** | Runtime core landed: `internal/subagent` lifecycle manager, registry, bounded batch execution, `[delegation]` config, and Go-native `delegate_task`. Real child LLM loop and higher-level reviewed promotion remain follow-up slices. |
```

Mirror that same state in `progress.json`.

- [ ] **Step 3: Regenerate derived progress output if needed**

If `_index.md` is generated from the progress source, run the local generator already used in this repo:

```bash
cd gormes && make generate-progress
```

Expected: either `_index.md` changes or the command is a no-op with success.

- [ ] **Step 4: Verify docs and focused tests remain green**

Run:

```bash
cd gormes && go test ./docs ./cmd/gormes ./internal/config ./internal/subagent -count=1
```

Expected: all four packages pass.

- [ ] **Step 5: Commit**

```bash
git add \
  gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md \
  gormes/docs/content/building-gormes/architecture_plan/progress.json \
  gormes/docs/content/building-gormes/architecture_plan/_index.md
git commit -m "docs(phase2): mark subagent runtime core as in progress"
```

---

## Task 4: Run closeout verification for Track A

**Files:**
- Verify only: `gormes/internal/subagent/*`, `gormes/internal/config/*`, `gormes/cmd/gormes/*`

- [ ] **Step 1: Run the focused race suite**

Run:

```bash
cd gormes && go test ./internal/subagent ./internal/config ./cmd/gormes -count=1 -race
```

Expected: all three packages pass under `-race`.

- [ ] **Step 2: Run the broad suite after Track B lands**

Run:

```bash
cd gormes && go test ./... -race
```

Expected: full suite green once the gateway chassis and Discord work are merged in the same branch.

- [ ] **Step 3: Commit closeout marker if broad verification required additional tiny fixes**

```bash
git add -A
git commit -m "test(subagent): close runtime-core verification gaps"
```

Only do this commit if Task 4 required actual code or test-file edits.
