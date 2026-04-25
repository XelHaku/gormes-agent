# Planner Self-Healing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the autoloop ↔ planner feedback loop so the planner reacts to autoloop quarantine events within minutes, retries on validation rejection, audits its own effectiveness, and escalates intractable rows for human review — with a per-run ledger and topical focus mode for operator-driven runs.

**Architecture:** Six layers added to `internal/architectureplanner/`, with small extensions to `internal/autoloop/` and one typed-field addition to `internal/progress/`. A new `Item.PlannerVerdict` block (planner-owned, autoloop-preserved) mirrors Phase B's `Item.Health` (autoloop-owned, planner-preserved) — symmetric ownership, structural preservation via Phase B's typed-struct round-trip. Event delivery is file-based (`triggers.jsonl` + cursor) consumed via systemd path unit; no daemons.

**Tech Stack:** Go 1.25+, append-only JSONL ledgers (matches Phase B autoloop pattern), atomic temp+rename writes, systemd path units, `crypto/sha256` (already in `internal/progress` from Phase B).

**Reference spec:** `docs/superpowers/specs/2026-04-24-planner-self-healing-design.md`

**Baseline commit (spec):** `c8d78421`

---

## File Structure

**New files:**

```text
internal/progress/preservation_test.go
internal/architectureplanner/ledger.go
internal/architectureplanner/ledger_test.go
internal/architectureplanner/triggers.go
internal/architectureplanner/triggers_test.go
internal/architectureplanner/triggers_concurrent_test.go
internal/architectureplanner/retry.go
internal/architectureplanner/retry_test.go
internal/architectureplanner/evaluation.go
internal/architectureplanner/evaluation_test.go
internal/architectureplanner/verdict.go
internal/architectureplanner/verdict_test.go
internal/architectureplanner/topics.go
internal/architectureplanner/topics_test.go
internal/architectureplanner/lifecycle_test.go
internal/architectureplanner/status_test.go
```

**Modified files:**

```text
internal/progress/progress.go
internal/progress/health_compat_test.go
internal/architectureplanner/run.go
internal/architectureplanner/prompt.go
internal/architectureplanner/context.go
internal/architectureplanner/config.go
internal/architectureplanner/service.go
internal/architectureplanner/config_test.go
internal/autoloop/health_writer.go
internal/autoloop/run.go
internal/autoloop/candidates.go
internal/autoloop/config.go
cmd/architecture-planner-loop/main.go
```

**Responsibility map:**

- `internal/progress/progress.go`: extend `Item` with `PlannerVerdict *PlannerVerdict` field as the LAST field (after `Health` from Phase B); add `PlannerVerdict` typed struct.
- `internal/progress/preservation_test.go`: cross-cutting symmetric-preservation regression test.
- `internal/architectureplanner/ledger.go`: per-run ledger types + `AppendLedgerEvent` / `LoadLedger` / `LoadLedgerWindow`.
- `internal/architectureplanner/triggers.go`: `TriggerEvent` type, cursor type, `AppendTriggerEvent` / `ReadTriggersSinceCursor` / `LoadCursor` / `SaveCursor`.
- `internal/architectureplanner/retry.go`: `RetryFeedback` formatter + `retryAttempt` type.
- `internal/architectureplanner/evaluation.go`: `Evaluate` correlates planner ledger ↔ autoloop ledger, returns `[]ReshapeOutcome`.
- `internal/architectureplanner/verdict.go`: `StampVerdicts` deterministic post-processing pass.
- `internal/architectureplanner/topics.go`: `MatchKeywords` + `FilterContextByKeywords`.
- `internal/architectureplanner/run.go`: orchestrate L1+L2+L3+L4+L5 inside `RunOnce`.
- `internal/architectureplanner/prompt.go`: render `PreviousReshapes`, trigger-events bullets, topical clause; add `SELF-EVALUATION (SOFT RULE)` clause.
- `internal/architectureplanner/context.go`: extend `ContextBundle` with `PreviousReshapes`, `TriggerEvents`, `Keywords`.
- `internal/architectureplanner/config.go`: add `MaxRetries`, `EvaluationWindow`, `EscalationThreshold`, `IncludeNeedsHuman`, `PlannerTriggersPath`, `TriggersCursorPath`, `AutoloopRunRoot` fields.
- `internal/architectureplanner/service.go`: render+install `gormes-architecture-planner.path` unit alongside the existing `.timer` and `.service`.
- `internal/autoloop/health_writer.go`: `classifyForTrigger` helper; `Flush` returns triggered events alongside error.
- `internal/autoloop/run.go`: emit triggered events via `AppendTriggerEvent` after Flush succeeds.
- `internal/autoloop/candidates.go`: skip `PlannerVerdict.NeedsHuman` rows; add `Candidate.NeedsHumanFlag`; surface in `SelectionReason()`.
- `internal/autoloop/config.go`: add `IncludeNeedsHuman`, `PlannerTriggersPath` env-driven fields.
- `cmd/architecture-planner-loop/main.go`: parse positional keyword args after `run`; extend `status` to render outcomes + NeedsHuman rows + `Keywords:` line.

---

## Conventions Used In Every Task

- Each task is one TDD cycle ending in one commit.
- Always run failing test first; never write implementation before a red test.
- Run `go vet ./...` and `gofmt -l .` before each commit; both must be clean for the touched packages.
- Run focused-then-wider test suite before each commit:
  ```
  go test ./internal/progress/... ./internal/autoloop/... ./internal/architectureplanner/... ./cmd/architecture-planner-loop/...
  ```
- Commit message format:
  - `feat(progress): ...`
  - `feat(planner): ...`
  - `feat(autoloop): ...`
  - `test(planner): ...`
  - `test(progress): ...`
- Never modify `Item` field ordering. New `PlannerVerdict` field MUST be appended after `Health` (the Phase B field), preserving Phase B's "Health is last" → "PlannerVerdict is last" discipline.
- All new env vars default to current behavior (back-compat). Tests verify defaults.
- All file IO that writes `progress.json` continues to go through `internal/progress.SaveProgress` (Phase B). Phase C does not introduce any new path that bypasses it.

---

## Task 1: Add `PlannerVerdict` Schema To `internal/progress`

**Files:**
- Modify: `internal/progress/progress.go`

- [ ] **Step 1.1: Write the failing test for the schema and round-trip**

Append to `internal/progress/health_test.go` (existing Phase B test file):

```go
func TestPlannerVerdict_RoundTrip(t *testing.T) {
	verdict := &PlannerVerdict{
		NeedsHuman:   true,
		Reason:       "auto: 3 reshapes without unsticking; last category report_validation_failed",
		Since:        "2026-04-24T12:00:00Z",
		ReshapeCount: 3,
		LastReshape:  "2026-04-24T11:00:00Z",
		LastOutcome:  "still_failing",
	}

	data, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PlannerVerdict
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.NeedsHuman {
		t.Fatal("NeedsHuman should round-trip true")
	}
	if got.ReshapeCount != 3 {
		t.Fatalf("ReshapeCount = %d, want 3", got.ReshapeCount)
	}
	if got.LastOutcome != "still_failing" {
		t.Fatalf("LastOutcome = %q, want still_failing", got.LastOutcome)
	}
}

func TestPlannerVerdict_OmitemptyKeepsZeroFieldsOut(t *testing.T) {
	v := &PlannerVerdict{}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("zero-value PlannerVerdict should marshal to {}, got %s", data)
	}
}

func TestItem_PlannerVerdictOmitemptyByDefault(t *testing.T) {
	item := &Item{Name: "x", Status: StatusPlanned}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "planner_verdict") {
		t.Fatalf("Item with no verdict should not emit planner_verdict key, got %s", data)
	}
}
```

- [ ] **Step 1.2: Run the failing tests**

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/gormes-agent
go test ./internal/progress/ -run 'TestPlannerVerdict|TestItem_PlannerVerdictOmitemptyByDefault' -v
```
Expected: FAIL because `PlannerVerdict` and `Item.PlannerVerdict` do not exist.

- [ ] **Step 1.3: Add `PlannerVerdict` type and field**

Append to `internal/progress/health.go` (keep all existing types intact):

```go
// PlannerVerdict is execution-history metadata about one progress.json item,
// OWNED by the architecture-planner runtime. Autoloop READS it (to skip rows
// escalated for human review) and MUST preserve it verbatim across writes
// (structural via typed JSON round-trip).
//
// Symmetric to RowHealth (autoloop-owned + planner-preserved).
type PlannerVerdict struct {
	// NeedsHuman is sticky: once true, only a human edit can clear it.
	// Planner runtime never auto-unsets it.
	NeedsHuman   bool   `json:"needs_human,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Since        string `json:"since,omitempty"`         // RFC3339; set when NeedsHuman first triggers
	ReshapeCount int    `json:"reshape_count,omitempty"` // monotonic; total times planner reshaped this row
	LastReshape  string `json:"last_reshape,omitempty"`  // RFC3339 of most recent reshape
	LastOutcome  string `json:"last_outcome,omitempty"`  // "unstuck" | "still_failing" | "no_attempts_yet"
}
```

Modify `internal/progress/progress.go` — add `PlannerVerdict` as the LAST field of `Item` (immediately after `Health`):

```go
type Item struct {
	// ... existing fields preserved ...
	Health         *RowHealth      `json:"health,omitempty"`
	PlannerVerdict *PlannerVerdict `json:"planner_verdict,omitempty"`
}
```

- [ ] **Step 1.4: Re-run the failing tests**

```bash
go test ./internal/progress/ -run 'TestPlannerVerdict|TestItem_PlannerVerdictOmitemptyByDefault' -v
```
Expected: PASS for all three.

- [ ] **Step 1.5: Verify no existing tests regressed**

```bash
go test ./internal/progress/...
go vet ./internal/progress/...
gofmt -l internal/progress/
```
Expected: all pass; vet clean; no gofmt diffs.

- [ ] **Step 1.6: Commit**

```bash
git add internal/progress/health.go internal/progress/progress.go internal/progress/health_test.go
git commit -m "feat(progress): add PlannerVerdict schema for planner self-healing"
```

---

## Task 2: Symmetric Preservation Regression Test

**Files:**
- Create: `internal/progress/preservation_test.go`
- Modify: `internal/progress/health_compat_test.go` (add idempotency case with both blocks)

- [ ] **Step 2.1: Write the failing tests**

Create `internal/progress/preservation_test.go`:

```go
package progress

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSymmetricPreservation_AutoloopWritesPreserveVerdict verifies that
// autoloop's ApplyHealthUpdates does not erase Item.PlannerVerdict, which
// the planner owns. The preservation is structural via typed JSON round-trip.
func TestSymmetricPreservation_AutoloopWritesPreserveVerdict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x",
             "health": {"attempt_count": 2, "consecutive_failures": 2},
             "planner_verdict": {"reshape_count": 1, "last_outcome": "still_failing"}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Autoloop-side write: increment Health.AttemptCount.
	err := ApplyHealthUpdates(path, []HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-1",
		Mutate: func(h *RowHealth) {
			h.AttemptCount = 3
			h.ConsecutiveFailures = 3
		},
	}})
	if err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := &prog.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict == nil {
		t.Fatal("PlannerVerdict was erased by autoloop's write")
	}
	if row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatalf("PlannerVerdict.ReshapeCount = %d, want 1 (preserved)", row.PlannerVerdict.ReshapeCount)
	}
	if row.PlannerVerdict.LastOutcome != "still_failing" {
		t.Fatalf("PlannerVerdict.LastOutcome = %q, want still_failing (preserved)", row.PlannerVerdict.LastOutcome)
	}
	// The Health update did land:
	if row.Health.AttemptCount != 3 {
		t.Fatalf("Health.AttemptCount = %d, want 3", row.Health.AttemptCount)
	}
}

// TestSymmetricPreservation_PlannerWritesPreserveHealth verifies that a
// SaveProgress call with verdict-only changes preserves Health.
func TestSymmetricPreservation_PlannerWritesPreserveHealth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x",
             "health": {"attempt_count": 2, "consecutive_failures": 2,
                        "quarantine": {"reason": "auto", "threshold": 3, "spec_hash": "abc"}}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	originalHealth := *prog.Phases["1"].Subphases["1.A"].Items[0].Health

	// Planner-side write: stamp PlannerVerdict (mimics StampVerdicts).
	prog.Phases["1"].Subphases["1.A"].Items[0].PlannerVerdict = &PlannerVerdict{
		ReshapeCount: 1,
		LastReshape:  "2026-04-24T12:00:00Z",
		LastOutcome:  "still_failing",
	}
	if err := SaveProgress(path, prog); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	// Reload and verify Health survived byte-equal.
	prog2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	row := &prog2.Phases["1"].Subphases["1.A"].Items[0]
	if !reflect.DeepEqual(*row.Health, originalHealth) {
		t.Fatalf("Health was modified by planner's write\nbefore: %+v\nafter:  %+v", originalHealth, *row.Health)
	}
	if row.PlannerVerdict == nil || row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatal("PlannerVerdict was not persisted")
	}
}

// TestSymmetricPreservation_BothBlocksRoundTrip combines both directions
// and asserts the spec hash is stable after a full round-trip with both
// blocks populated.
func TestSymmetricPreservation_BothBlocksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x", "blocked_by": ["dep-a"],
             "health": {"attempt_count": 1},
             "planner_verdict": {"needs_human": true, "reason": "auto", "reshape_count": 4}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	hashBefore := ItemSpecHash(&prog.Phases["1"].Subphases["1.A"].Items[0])

	if err := SaveProgress(path, prog); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	prog2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	row := &prog2.Phases["1"].Subphases["1.A"].Items[0]
	hashAfter := ItemSpecHash(row)

	if hashBefore != hashAfter {
		t.Fatalf("spec hash changed across round-trip:\nbefore: %s\nafter:  %s", hashBefore, hashAfter)
	}
	if row.Health == nil || row.PlannerVerdict == nil {
		t.Fatal("one of the blocks went missing across round-trip")
	}
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("PlannerVerdict.NeedsHuman flipped across round-trip")
	}
}
```

- [ ] **Step 2.2: Run the failing tests**

```bash
go test ./internal/progress/ -run TestSymmetricPreservation -v
```
Expected: PASS for all three (Phase B's typed-struct round-trip already preserves unknown fields). If any fails, that's a real Phase B regression to investigate.

> The tests should pass on first run because Task 1's `PlannerVerdict` is a typed field on `Item` and Go's `encoding/json` round-trips typed fields naturally. The point of these tests is REGRESSION protection: if anyone later changes `Save`/`Load` to drop unknown fields or reorder, this catches it.

- [ ] **Step 2.3: Extend the existing compat round-trip with both blocks**

Modify `internal/progress/health_compat_test.go` — append a new test:

```go
func TestSaveProgress_IdempotentWithBothHealthAndVerdict(t *testing.T) {
	src := filepath.Join("..", "..", "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	original, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("checked-in progress.json not found, skipping: %v", err)
	}

	tmp1 := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(tmp1, original, 0o644); err != nil {
		t.Fatalf("write tmp1: %v", err)
	}

	// Mutation that touches BOTH blocks on the same row.
	if err := ApplyHealthUpdates(tmp1, []HealthUpdate{{
		PhaseID:    "1",
		SubphaseID: "1.A",
		ItemName:   "Bubble Tea shell",
		Mutate: func(h *RowHealth) {
			h.AttemptCount = 1
		},
	}}); err != nil {
		t.Fatalf("first ApplyHealthUpdates: %v", err)
	}
	// Now stamp a PlannerVerdict on the same row via direct Load+Save.
	prog, _ := Load(tmp1)
	prog.Phases["1"].Subphases["1.A"].Items[0].PlannerVerdict = &PlannerVerdict{
		ReshapeCount: 2,
		LastOutcome:  "still_failing",
	}
	if err := SaveProgress(tmp1, prog); err != nil {
		t.Fatalf("SaveProgress 1: %v", err)
	}
	pass1, _ := os.ReadFile(tmp1)

	// Round-trip 2: Load + SaveProgress with no mutation. Must be byte-equal.
	tmp2 := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(tmp2, pass1, 0o644); err != nil {
		t.Fatalf("write tmp2: %v", err)
	}
	prog2, _ := Load(tmp2)
	if err := SaveProgress(tmp2, prog2); err != nil {
		t.Fatalf("SaveProgress 2: %v", err)
	}
	pass2, _ := os.ReadFile(tmp2)

	if !bytes.Equal(pass1, pass2) {
		t.Fatalf("SaveProgress not idempotent with both blocks; len pass1=%d pass2=%d", len(pass1), len(pass2))
	}
}
```

- [ ] **Step 2.4: Run and commit**

```bash
go test ./internal/progress/... -count=1
go vet ./internal/progress/...
gofmt -l internal/progress/
```
All pass; vet clean; no gofmt diffs.

```bash
git add internal/progress/preservation_test.go internal/progress/health_compat_test.go
git commit -m "test(progress): symmetric preservation regression for both blocks"
```

---

## Task 3: L1 Planner Ledger Types And IO

**Files:**
- Create: `internal/architectureplanner/ledger.go`
- Create: `internal/architectureplanner/ledger_test.go`

- [ ] **Step 3.1: Write failing tests for the ledger types and IO**

Create `internal/architectureplanner/ledger_test.go`:

```go
package architectureplanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLedgerEvent_RoundTrip(t *testing.T) {
	event := LedgerEvent{
		TS:           "2026-04-25T10:00:00Z",
		RunID:        "20260425T100000Z",
		Trigger:      "event",
		TriggerEvents: []string{"trig-1", "trig-2"},
		Backend:      "codexu",
		Mode:         "safe",
		Status:       "ok",
		BeforeStats:  ProgressStats{Shipped: 10, Planned: 50, Quarantined: 2},
		AfterStats:   ProgressStats{Shipped: 11, Planned: 49, Quarantined: 1},
		RowsChanged: []RowChange{
			{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"},
		},
		Keywords: []string{"honcho"},
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got LedgerEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RunID != "20260425T100000Z" || got.Trigger != "event" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.RowsChanged) != 1 || got.RowsChanged[0].Kind != "spec_changed" {
		t.Fatalf("RowsChanged round-trip failed: %+v", got.RowsChanged)
	}
}

func TestAppendLedgerEvent_AppendsOneJSONLineAndIsParseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	for i := 0; i < 3; i++ {
		err := AppendLedgerEvent(path, LedgerEvent{
			TS:    time.Date(2026, 4, 25, 10, i, 0, 0, time.UTC).Format(time.RFC3339),
			RunID: "run-" + string(rune('A'+i)),
			Status: "ok",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	body, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), body)
	}
	for i, line := range lines {
		var event LedgerEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d not parseable JSON: %v\n%s", i, err, line)
		}
	}
}

func TestAppendLedgerEvent_AppendsAtomicallyAcrossWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	const N = 8
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = AppendLedgerEvent(path, LedgerEvent{
				TS:     time.Now().UTC().Format(time.RFC3339Nano),
				RunID:  "run-" + string(rune('A'+idx)),
				Status: "ok",
			})
		}(i)
	}
	wg.Wait()
	events, err := LoadLedger(path)
	if err != nil {
		t.Fatalf("LoadLedger: %v", err)
	}
	if len(events) != N {
		t.Fatalf("got %d events, want %d", len(events), N)
	}
}

func TestLoadLedger_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	good1 := `{"ts":"2026-04-25T10:00:00Z","run_id":"a","status":"ok"}`
	bad := `{this is not json`
	good2 := `{"ts":"2026-04-25T10:01:00Z","run_id":"b","status":"ok"}`
	if err := os.WriteFile(path, []byte(good1+"\n"+bad+"\n"+good2+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	events, err := LoadLedger(path)
	if err != nil {
		t.Fatalf("LoadLedger: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 good events, got %d", len(events))
	}
}

func TestLoadLedgerWindow_BoundsByTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for i := -10; i <= 0; i++ {
		_ = AppendLedgerEvent(path, LedgerEvent{
			TS:     now.Add(time.Duration(i) * 24 * time.Hour).Format(time.RFC3339),
			RunID:  "run",
			Status: "ok",
		})
	}
	events, err := LoadLedgerWindow(path, 7*24*time.Hour, now)
	if err != nil {
		t.Fatalf("LoadLedgerWindow: %v", err)
	}
	// Window includes events from 7 days ago to now → 8 events (-7..0).
	if len(events) != 8 {
		t.Fatalf("expected 8 events in 7-day window, got %d", len(events))
	}
}
```

- [ ] **Step 3.2: Run the failing tests**

```bash
go test ./internal/architectureplanner/ -run 'TestLedger|TestAppendLedger|TestLoadLedger' -v
```
Expected: FAIL because the types and functions don't exist.

- [ ] **Step 3.3: Implement `internal/architectureplanner/ledger.go`**

```go
package architectureplanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LedgerEvent is one entry in the planner runs.jsonl ledger.
type LedgerEvent struct {
	TS            string        `json:"ts"`              // RFC3339
	RunID         string        `json:"run_id"`
	Trigger       string        `json:"trigger"`         // "scheduled" | "event" | "manual" | "retry"
	TriggerEvents []string      `json:"trigger_events,omitempty"`
	Backend       string        `json:"backend"`
	Mode          string        `json:"mode"`
	Status        string        `json:"status"`          // "ok" | "validation_rejected" | "backend_failed" | "no_changes" | "needs_human_set"
	Detail        string        `json:"detail,omitempty"`
	BeforeStats   ProgressStats `json:"before_stats,omitempty"`
	AfterStats    ProgressStats `json:"after_stats,omitempty"`
	RowsChanged   []RowChange   `json:"rows_changed,omitempty"`
	RetryAttempt  int           `json:"retry_attempt,omitempty"`
	Keywords      []string      `json:"keywords,omitempty"` // L6 topical focus
}

// RowChange records one mutation to a progress.json row in a planner run.
type RowChange struct {
	PhaseID    string `json:"phase_id"`
	SubphaseID string `json:"subphase_id"`
	ItemName   string `json:"item_name"`
	Kind       string `json:"kind"` // "added" | "deleted" | "spec_changed" | "verdict_set"
	Detail     string `json:"detail,omitempty"`
}

// ProgressStats is a snapshot of progress.json composition at a point in time.
type ProgressStats struct {
	Shipped     int `json:"shipped,omitempty"`
	InProgress  int `json:"in_progress,omitempty"`
	Planned     int `json:"planned,omitempty"`
	Quarantined int `json:"quarantined,omitempty"`
	NeedsHuman  int `json:"needs_human,omitempty"`
}

// AppendLedgerEvent atomically appends one event as a single JSON line.
// Uses O_APPEND|O_CREATE|O_WRONLY for POSIX-atomic line writes (lines under
// PIPE_BUF (4096 bytes on Linux) are atomic per the syscall contract).
func AppendLedgerEvent(path string, event LedgerEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir ledger dir: %w", err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal ledger event: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()
	_, err = f.Write(body)
	return err
}

// LoadLedger reads all events from the ledger file. Bad lines are logged
// and skipped; they do not abort the load.
func LoadLedger(path string) ([]LedgerEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []LedgerEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1 MiB per line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event LedgerEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// Skip corrupt lines; do not propagate.
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return events, err
	}
	return events, nil
}

// LoadLedgerWindow returns events within [now-window, now] inclusive. Bad
// timestamps are skipped.
func LoadLedgerWindow(path string, window time.Duration, now time.Time) ([]LedgerEvent, error) {
	all, err := LoadLedger(path)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-window)
	out := []LedgerEvent{}
	for _, ev := range all {
		t, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) && !t.After(now) {
			out = append(out, ev)
		}
	}
	return out, nil
}
```

- [ ] **Step 3.4: Run and commit**

```bash
go test ./internal/architectureplanner/ -run 'TestLedger|TestAppendLedger|TestLoadLedger' -v
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```
All pass; vet clean; no gofmt diffs.

```bash
git add internal/architectureplanner/ledger.go internal/architectureplanner/ledger_test.go
git commit -m "feat(planner): add per-run ledger with atomic append IO"
```

---

## Task 4: Wire Planner Ledger Into RunOnce

**Files:**
- Modify: `internal/architectureplanner/run.go`
- Modify: `internal/architectureplanner/config.go`
- Modify: `internal/architectureplanner/run_test.go`

- [ ] **Step 4.1: Write failing wire-in tests**

Append to `internal/architectureplanner/run_test.go`:

```go
func TestRunOnce_AppendsLedgerEventOnSuccess(t *testing.T) {
	t.Skip("FILL IN: use existing run_test.go fixtures (mock runner that produces a clean regen); assert ledger entry has status='ok' and rowsChanged length matches the mock's mutation set")
}

func TestRunOnce_AppendsLedgerEventOnValidationReject(t *testing.T) {
	t.Skip("FILL IN: mock runner produces a regen that drops a Health block; assert ledger entry has status='validation_rejected' AND RunOnce returns error")
}

func TestRunOnce_LedgerWriteFailureIsSoftFail(t *testing.T) {
	t.Skip("FILL IN: chmod cfg.RunRoot/state to read-only after run starts; assert RunOnce returns nil but logs the ledger write failure")
}
```

The skip-stubs are intentional: the existing `run_test.go` uses a specific fixture pattern (mocked Runner that returns specific stdout/stderr); the implementer should follow that idiom. Required test names are pinned above. Replace each `t.Skip(...)` with a real test using the existing fixture style.

- [ ] **Step 4.2: Run failing tests (they will skip; that's expected)**

```bash
go test ./internal/architectureplanner/ -run 'TestRunOnce_(AppendsLedgerEvent|LedgerWriteFailureIsSoftFail)' -v
```
Expected: SKIP for all three until the implementer fills them in.

- [ ] **Step 4.3: Add new Config field for autoloop ledger path**

Modify `internal/architectureplanner/config.go`. Add to `Config` struct:

```go
type Config struct {
    // ... existing fields ...
    AutoloopRunRoot string // path to autoloop's run root, e.g. "<repo>/.codex/orchestrator"; used by L4 evaluation
}
```

In `ConfigFromEnv`, default `AutoloopRunRoot` to `filepath.Join(repoRoot, ".codex", "orchestrator")` and honor `AUTOLOOP_RUN_ROOT` env override.

- [ ] **Step 4.4: Add `diffRows` and `computeStats` helpers**

Append to `internal/architectureplanner/run.go`:

```go
// computeStats walks a Progress doc and counts rows by status, including
// the new Phase C buckets (Quarantined, NeedsHuman) which aren't in the
// existing Progress.Stats() function.
func computeStats(prog *progress.Progress) ProgressStats {
	if prog == nil {
		return ProgressStats{}
	}
	var stats ProgressStats
	for _, phase := range prog.Phases {
		if phase == nil {
			continue
		}
		for _, sub := range phase.Subphases {
			if sub == nil {
				continue
			}
			for i := range sub.Items {
				it := &sub.Items[i]
				switch it.Status {
				case progress.StatusComplete:
					stats.Shipped++
				case progress.StatusInProgress:
					stats.InProgress++
				default:
					stats.Planned++
				}
				if it.Health != nil && it.Health.Quarantine != nil {
					stats.Quarantined++
				}
				if it.PlannerVerdict != nil && it.PlannerVerdict.NeedsHuman {
					stats.NeedsHuman++
				}
			}
		}
	}
	return stats
}

// diffRows compares before/after docs and returns RowChange records for
// added/deleted/spec_changed rows. Spec change is detected via
// progress.ItemSpecHash.
func diffRows(before, after *progress.Progress) []RowChange {
	var out []RowChange
	beforeIndex := indexItems(before)  // existing helper from Phase B Task 7
	afterIndex := indexItems(after)

	for key, beforeItem := range beforeIndex {
		afterItem, exists := afterIndex[key]
		if !exists {
			out = append(out, RowChange{
				PhaseID: key.phaseID, SubphaseID: key.subphaseID,
				ItemName: key.itemName, Kind: "deleted",
			})
			continue
		}
		if progress.ItemSpecHash(beforeItem) != progress.ItemSpecHash(afterItem) {
			out = append(out, RowChange{
				PhaseID: key.phaseID, SubphaseID: key.subphaseID,
				ItemName: key.itemName, Kind: "spec_changed",
			})
		}
	}
	for key := range afterIndex {
		if _, existed := beforeIndex[key]; !existed {
			out = append(out, RowChange{
				PhaseID: key.phaseID, SubphaseID: key.subphaseID,
				ItemName: key.itemName, Kind: "added",
			})
		}
	}
	return out
}
```

- [ ] **Step 4.5: Wire ledger append into `RunOnce`**

Inside `internal/architectureplanner/run.go::RunOnce`, after the existing validation step succeeds and `summary` is populated, add:

```go
// Determine status for the ledger entry from the run outcome.
runStatus := "ok"
if afterDoc == nil || beforeDoc == nil {
    runStatus = "no_changes"
}
// (validation_rejected and backend_failed paths return early with their own
// LedgerEvent emission — see steps below)

ledgerPath := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
event := LedgerEvent{
    TS:          now.UTC().Format(time.RFC3339),
    RunID:       summary.RunID,
    Trigger:     "scheduled", // L2 will override; default is scheduled
    Backend:     cfg.Backend,
    Mode:        cfg.Mode,
    Status:      runStatus,
    BeforeStats: computeStats(beforeDoc),
    AfterStats:  computeStats(afterDoc),
    RowsChanged: diffRows(beforeDoc, afterDoc),
}
if err := AppendLedgerEvent(ledgerPath, event); err != nil {
    log.Printf("planner: append ledger failed: %v", err)
}
```

In the validation-rejected branch (where `RunOnce` currently returns an error), emit the ledger event with `Status: "validation_rejected"` BEFORE returning the error.

In the backend-failed branch (where the runner returns an error), emit `Status: "backend_failed"` BEFORE returning.

- [ ] **Step 4.6: Run all planner tests to confirm no regression**

```bash
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

- [ ] **Step 4.7: Commit**

```bash
git add internal/architectureplanner/run.go internal/architectureplanner/config.go internal/architectureplanner/run_test.go
git commit -m "feat(planner): wire per-run ledger into RunOnce"
```

---

## Task 5: L6 Topical Focus Mode

**Files:**
- Create: `internal/architectureplanner/topics.go`
- Create: `internal/architectureplanner/topics_test.go`
- Modify: `internal/architectureplanner/context.go`
- Modify: `internal/architectureplanner/prompt.go`
- Modify: `internal/architectureplanner/prompt_test.go`
- Modify: `internal/architectureplanner/run.go`
- Modify: `cmd/architecture-planner-loop/main.go`
- Modify: `cmd/architecture-planner-loop/main_test.go` (or create if missing)

- [ ] **Step 5.1: Write failing topics tests**

Create `internal/architectureplanner/topics_test.go`:

```go
package architectureplanner

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestMatchKeywords_EmptyKeywordsMatchesAll(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-a", Contract: "do a"},
					{Name: "row-b", Contract: "do b"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, nil)
	if len(matched) != 2 {
		t.Fatalf("expected all 2 rows, got %d", len(matched))
	}
}

func TestMatchKeywords_SubstringMatchesItemName(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "honcho-client", Contract: "x"})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesContract(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "row-x", Contract: "Wire Honcho client"})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesSourceRefs(t *testing.T) {
	prog := docOneItem(progress.Item{
		Name:       "row-x",
		Contract:   "x",
		SourceRefs: []string{"../honcho/api.py"},
	})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesSubphaseName(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"3": {Name: "Memory", Subphases: map[string]*progress.Subphase{
				"3.A": {Name: "Honcho integration", Items: []progress.Item{
					{Name: "row-1", Contract: "x"},
					{Name: "row-2", Contract: "y"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 2 {
		t.Fatalf("subphase name match should bring all items; got %d", len(matched))
	}
}

func TestMatchKeywords_OrSemanticsAcrossKeywords(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-honcho", Contract: "x"},
					{Name: "row-memory", Contract: "y"},
					{Name: "row-other", Contract: "z"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, []string{"honcho", "memory"})
	if len(matched) != 2 {
		t.Fatalf("OR across keywords should match 2, got %d", len(matched))
	}
}

func TestMatchKeywords_CaseInsensitive(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "row-x", Contract: "Wire Honcho"})
	matched := matchKeywordsInDoc(prog, []string{"HONCHO"})
	if len(matched) != 1 {
		t.Fatalf("case-insensitive match expected; got %d", len(matched))
	}
}

func TestFilterContextByKeywords_NarrowsBundleSelectively(t *testing.T) {
	bundle := ContextBundle{
		QuarantinedRows: []QuarantinedRowContext{
			{ItemName: "honcho-row", Contract: "x"},
			{ItemName: "other-row", Contract: "y"},
		},
		AutoloopAudit: AutoloopAudit{}, // would be aggregate-only
	}
	narrowed := FilterContextByKeywords(bundle, []string{"honcho"})
	if len(narrowed.QuarantinedRows) != 1 || narrowed.QuarantinedRows[0].ItemName != "honcho-row" {
		t.Fatalf("QuarantinedRows narrowing failed: %+v", narrowed.QuarantinedRows)
	}
	// AutoloopAudit must remain intact (aggregate, not row-level).
}

// docOneItem is a small builder used by topics tests.
func docOneItem(item progress.Item) *progress.Progress {
	return &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{item}},
			}},
		},
	}
}
```

- [ ] **Step 5.2: Run failing tests**

```bash
go test ./internal/architectureplanner/ -run 'TestMatchKeywords|TestFilterContext' -v
```
Expected: FAIL because `matchKeywordsInDoc`, `FilterContextByKeywords` don't exist.

- [ ] **Step 5.3: Implement `internal/architectureplanner/topics.go`**

```go
package architectureplanner

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// itemMatchesKeywords returns true if any keyword (case-insensitive
// substring) matches the item's name, contract, source_refs, write_scope,
// fixture, or any of the parent subphase/phase names.
func itemMatchesKeywords(item *progress.Item, phaseName, subphaseName string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		needle := strings.ToLower(kw)
		if strings.Contains(strings.ToLower(item.Name), needle) ||
			strings.Contains(strings.ToLower(item.Contract), needle) ||
			strings.Contains(strings.ToLower(item.Fixture), needle) ||
			strings.Contains(strings.ToLower(phaseName), needle) ||
			strings.Contains(strings.ToLower(subphaseName), needle) {
			return true
		}
		for _, ref := range item.SourceRefs {
			if strings.Contains(strings.ToLower(ref), needle) {
				return true
			}
		}
		for _, scope := range item.WriteScope {
			if strings.Contains(strings.ToLower(scope), needle) {
				return true
			}
		}
	}
	return false
}

// matchKeywordsInDoc returns the subset of items in prog that match any of
// the keywords. Empty keywords returns all items.
type matchedRow struct {
	PhaseID    string
	SubphaseID string
	Item       *progress.Item
}

func matchKeywordsInDoc(prog *progress.Progress, keywords []string) []matchedRow {
	var out []matchedRow
	if prog == nil {
		return out
	}
	for phaseID, phase := range prog.Phases {
		if phase == nil {
			continue
		}
		for subphaseID, sub := range phase.Subphases {
			if sub == nil {
				continue
			}
			for i := range sub.Items {
				it := &sub.Items[i]
				if itemMatchesKeywords(it, phase.Name, sub.Name, keywords) {
					out = append(out, matchedRow{PhaseID: phaseID, SubphaseID: subphaseID, Item: it})
				}
			}
		}
	}
	return out
}

// FilterContextByKeywords narrows the bundle's row-level slices
// (QuarantinedRows, PreviousReshapes if present) to only rows matching ANY
// of the keywords. Empty keywords returns the bundle unchanged.
// AutoloopAudit and SourceRoots are intentionally NOT narrowed.
func FilterContextByKeywords(bundle ContextBundle, keywords []string) ContextBundle {
	if len(keywords) == 0 {
		return bundle
	}

	matchesAny := func(haystacks ...string) bool {
		for _, kw := range keywords {
			needle := strings.ToLower(kw)
			for _, h := range haystacks {
				if strings.Contains(strings.ToLower(h), needle) {
					return true
				}
			}
		}
		return false
	}

	narrowed := bundle
	if len(bundle.QuarantinedRows) > 0 {
		filtered := []QuarantinedRowContext{}
		for _, r := range bundle.QuarantinedRows {
			if matchesAny(r.ItemName, r.Contract) {
				filtered = append(filtered, r)
			}
		}
		narrowed.QuarantinedRows = filtered
	}
	// PreviousReshapes is added in Task 10 (L4); when present, narrow it too.
	// At Task 5 time the field doesn't exist yet; that's fine — the type
	// extension lands in Task 10 and the FilterContextByKeywords body will
	// gain a matching block then.
	return narrowed
}
```

- [ ] **Step 5.4: Verify topics tests pass**

```bash
go test ./internal/architectureplanner/ -run 'TestMatchKeywords|TestFilterContext' -v
```
Expected: PASS for all 8.

- [ ] **Step 5.5: Add `RunOptions.Keywords` and wire into RunOnce**

Modify `internal/architectureplanner/run.go`:

```go
type RunOptions struct {
    // ... existing fields ...
    Keywords []string
}
```

In `RunOnce`, after `bundle, err := CollectContext(cfg, now)`:

```go
if len(opts.Keywords) > 0 {
    bundle = FilterContextByKeywords(bundle, opts.Keywords)
}
```

Pass keywords into `BuildPrompt` (next step).

- [ ] **Step 5.6: Add topical clause to prompt**

Modify `internal/architectureplanner/prompt.go`. Change `BuildPrompt` signature:

```go
func BuildPrompt(bundle ContextBundle, keywords []string) string {
    // ... existing content ...
    if len(keywords) > 0 {
        builder.WriteString(formatTopicalClause(keywords))
    }
    // ... rest ...
}

const topicalClauseTemplate = `
TOPICAL FOCUS

This run was invoked with keyword arguments: %s. The context above
(Quarantined Rows, Previous Reshapes, Implementation Inventory) has been
narrowed to only rows that mechanically match these keywords.

Focus your refinement work on these areas. You may still adjust adjacent
rows if a topical row's blocked_by/unblocks dependencies require it, but
do NOT widen the scope to unrelated phases. If you believe a topical
keyword needs structural rework that crosses phase boundaries, set
contract_status="draft" on the affected rows and add a degraded_mode note
explaining the cross-phase dependency rather than reshaping the whole
graph.
`

func formatTopicalClause(keywords []string) string {
    quoted := make([]string, len(keywords))
    for i, kw := range keywords {
        quoted[i] = strconv.Quote(kw)
    }
    return fmt.Sprintf(topicalClauseTemplate, "["+strings.Join(quoted, ", ")+"]")
}
```

Update every existing caller of `BuildPrompt` (search the codebase) to pass `nil` for keywords until Task 10/11 update them.

Add a test in `prompt_test.go`:

```go
func TestBuildPrompt_TopicalClauseAppearsWithKeywords(t *testing.T) {
    bundle := ContextBundle{}
    prompt := BuildPrompt(bundle, []string{"honcho", "memory"})
    if !strings.Contains(prompt, "TOPICAL FOCUS") {
        t.Fatal("topical clause missing when keywords present")
    }
    if !strings.Contains(prompt, `"honcho"`) || !strings.Contains(prompt, `"memory"`) {
        t.Fatalf("topical clause should name keywords; got:\n%s", prompt)
    }
}

func TestBuildPrompt_NoTopicalClauseWithoutKeywords(t *testing.T) {
    bundle := ContextBundle{}
    prompt := BuildPrompt(bundle, nil)
    if strings.Contains(prompt, "TOPICAL FOCUS") {
        t.Fatal("topical clause should be omitted when no keywords")
    }
}
```

- [ ] **Step 5.7: Parse keyword arguments in cmd**

Modify `cmd/architecture-planner-loop/main.go::parseRunOptions`:

```go
type runOptions struct {
    // ... existing fields ...
    keywords []string
}

func parseRunOptions(args []string) (runOptions, error) {
    opts := runOptions{}
    for i := 0; i < len(args); i++ {
        arg := args[i]
        switch arg {
        case "--dry-run":
            opts.dryRun = true
        case "--codexu":
            opts.backend = "codexu"
        case "--claudeu":
            opts.backend = "claudeu"
        case "--mode":
            if i+1 >= len(args) {
                return runOptions{}, fmt.Errorf(usage)
            }
            i++
            opts.mode = args[i]
        case "--help", "-h":
            opts.help = true
        default:
            // Treat as positional keyword argument. Multi-word keywords
            // (e.g. "skills tools") get split on whitespace.
            for _, kw := range strings.Fields(arg) {
                opts.keywords = append(opts.keywords, kw)
            }
        }
    }
    return opts, nil
}
```

In the `case "run":` branch, pass `opts.keywords` to `RunOptions.Keywords`.

Add a test in `cmd/architecture-planner-loop/main_test.go`:

```go
func TestParseRunOptions_PositionalKeywords(t *testing.T) {
    opts, err := parseRunOptions([]string{"--codexu", "honcho", "memory"})
    if err != nil {
        t.Fatal(err)
    }
    if opts.backend != "codexu" {
        t.Errorf("backend = %q", opts.backend)
    }
    want := []string{"honcho", "memory"}
    if !reflect.DeepEqual(opts.keywords, want) {
        t.Errorf("keywords = %v, want %v", opts.keywords, want)
    }
}

func TestParseRunOptions_QuotedMultiwordKeywordsSplitOnWhitespace(t *testing.T) {
    opts, err := parseRunOptions([]string{"skills tools"})
    if err != nil {
        t.Fatal(err)
    }
    want := []string{"skills", "tools"}
    if !reflect.DeepEqual(opts.keywords, want) {
        t.Errorf("keywords = %v, want %v", opts.keywords, want)
    }
}
```

- [ ] **Step 5.8: Add Keywords to ledger entry**

In `RunOnce` where the ledger event is built (Task 4), add:

```go
event.Keywords = opts.Keywords
```

- [ ] **Step 5.9: Run all tests + commit**

```bash
go test ./internal/architectureplanner/...
go test ./cmd/architecture-planner-loop/...
go vet ./internal/architectureplanner/... ./cmd/architecture-planner-loop/...
gofmt -l internal/architectureplanner/ cmd/architecture-planner-loop/
```
All pass; vet clean; no gofmt diffs.

```bash
git add internal/architectureplanner/topics.go internal/architectureplanner/topics_test.go internal/architectureplanner/prompt.go internal/architectureplanner/prompt_test.go internal/architectureplanner/run.go cmd/architecture-planner-loop/main.go cmd/architecture-planner-loop/main_test.go
git commit -m "feat(planner): topical focus via positional keyword arguments"
```

---

## Task 6: L2 Autoloop Side — Emit Triggers

**Files:**
- Modify: `internal/autoloop/health_writer.go`
- Modify: `internal/autoloop/run.go`
- Modify: `internal/autoloop/config.go`
- Modify: `internal/autoloop/health_writer_test.go`

- [ ] **Step 6.1: Write failing tests for trigger emission**

Append to `internal/autoloop/health_writer_test.go`:

```go
func TestFlush_NewQuarantineEmitsTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	triggersPath := filepath.Join(dir, "triggers.jsonl")
	writeBaseProgress(t, progressPath)

	// Pre-load row at CF=2; one more failure triggers quarantine.
	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.AttemptCount = 2
			h.ConsecutiveFailures = 2
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom")
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 trigger event, got %d", len(events))
	}
	if events[0].Kind != "quarantine_added" {
		t.Fatalf("event kind = %q, want quarantine_added", events[0].Kind)
	}
	if events[0].ItemName != "row-1" {
		t.Fatalf("event.ItemName = %q", events[0].ItemName)
	}

	_ = triggersPath // Task 7's planner side reads this path
}

func TestFlush_PureFailureBelowThresholdEmitsNoTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom")
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no trigger events for sub-threshold failure, got %d", len(events))
	}
}

func TestFlush_StaleClearEmitsTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	// Pre-quarantine the row.
	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 5
			h.Quarantine = &progress.Quarantine{Reason: "auto", Threshold: 3, SpecHash: "stale"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.MarkStaleQuarantine(candidateOf("2", "2.B", "row-1", "do x"))
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "quarantine_stale_cleared" {
		t.Fatalf("expected 1 quarantine_stale_cleared event, got %+v", events)
	}
}
```

- [ ] **Step 6.2: Run failing tests**

```bash
go test ./internal/autoloop/ -run 'TestFlush_(NewQuarantineEmitsTrigger|PureFailureBelowThresholdEmitsNoTrigger|StaleClearEmitsTrigger)' -v
```
Expected: FAIL because `FlushWithTriggers` does not exist.

- [ ] **Step 6.3: Add `FlushWithTriggers` and trigger types**

Append to `internal/autoloop/health_writer.go`:

```go
// FlushedTriggerEvent represents one trigger event the accumulator
// determined should fire after the batched ApplyHealthUpdates landed.
// Lifted to the run.go layer for actual emission to the planner trigger
// ledger (Task 7's AppendTriggerEvent path).
type FlushedTriggerEvent struct {
	Kind          string // "quarantine_added" | "quarantine_stale_cleared"
	PhaseID       string
	SubphaseID    string
	ItemName      string
	Reason        string
	AutoloopRunID string
}

// FlushWithTriggers performs the same work as Flush but also returns the
// list of trigger events the run loop should emit to the planner trigger
// ledger. To classify, the accumulator loads the BEFORE state from disk,
// applies updates as normal, then loads the AFTER state and compares per
// row.
//
// Soft contract: if the after-state load fails, returns the existing flush
// error AND nil triggers (don't emit triggers we can't validate).
func (a *healthAccumulator) FlushWithTriggers(progressPath string, hashOf SpecHashProvider) ([]FlushedTriggerEvent, error) {
	if len(a.rows) == 0 {
		return nil, nil
	}

	// Snapshot before-state for trigger classification.
	beforeProg, _ := progress.Load(progressPath)
	beforeIndex := indexHealthByKey(beforeProg)

	// Reuse the existing Flush logic.
	if err := a.Flush(progressPath, hashOf); err != nil {
		return nil, err
	}

	afterProg, err := progress.Load(progressPath)
	if err != nil {
		return nil, nil // soft: don't emit unverifiable triggers
	}
	afterIndex := indexHealthByKey(afterProg)

	var events []FlushedTriggerEvent
	for key, pending := range a.rows {
		before := beforeIndex[key]
		after := afterIndex[key]
		kind, fire := classifyForTrigger(before, after, pending)
		if !fire {
			continue
		}
		reason := ""
		if after != nil && after.Quarantine != nil {
			reason = after.Quarantine.Reason
		}
		events = append(events, FlushedTriggerEvent{
			Kind:          kind,
			PhaseID:       key.phaseID,
			SubphaseID:    key.subphaseID,
			ItemName:      key.itemName,
			Reason:        reason,
			AutoloopRunID: a.runID,
		})
	}
	return events, nil
}

func classifyForTrigger(before, after *progress.RowHealth, p *pendingHealth) (string, bool) {
	// New quarantine just set this run.
	if (before == nil || before.Quarantine == nil) && after != nil && after.Quarantine != nil {
		return "quarantine_added", true
	}
	// Stale quarantine cleared this run.
	if before != nil && before.Quarantine != nil && after != nil && after.Quarantine == nil && p.staleClear {
		return "quarantine_stale_cleared", true
	}
	return "", false
}

func indexHealthByKey(prog *progress.Progress) map[rowKey]*progress.RowHealth {
	out := map[rowKey]*progress.RowHealth{}
	if prog == nil {
		return out
	}
	for phaseID, phase := range prog.Phases {
		if phase == nil {
			continue
		}
		for subID, sub := range phase.Subphases {
			if sub == nil {
				continue
			}
			for i := range sub.Items {
				it := &sub.Items[i]
				out[rowKey{phaseID, subID, it.Name}] = it.Health
			}
		}
	}
	return out
}
```

- [ ] **Step 6.4: Add `PlannerTriggersPath` to autoloop Config**

Modify `internal/autoloop/config.go`. Add field to `Config`:

```go
PlannerTriggersPath string // PLANNER_TRIGGERS_PATH; default: <repoRoot>/.codex/architecture-planner/triggers.jsonl
```

In `ConfigFromEnv`, default and env-override.

- [ ] **Step 6.5: Wire trigger emission in run.go's flushHealth closure**

Modify `internal/autoloop/run.go`. In the existing `flushHealth` closure (Phase B), replace the `acc.Flush` call with `acc.FlushWithTriggers` and emit events:

```go
flushHealth := func() error {
    events, err := acc.FlushWithTriggers(opts.Config.ProgressJSON, hashOf)
    if err != nil {
        // ... existing failed-event ledger emission ...
        return fmt.Errorf("flush health: %w", err)
    }
    // ... existing health_updated event emission ...

    // Emit trigger events. Soft-fail: log but don't break the autoloop run.
    for _, ev := range events {
        triggerEvent := architectureplanner.TriggerEvent{
            Source:        "autoloop",
            Kind:          ev.Kind,
            PhaseID:       ev.PhaseID,
            SubphaseID:    ev.SubphaseID,
            ItemName:      ev.ItemName,
            Reason:        ev.Reason,
            AutoloopRunID: ev.AutoloopRunID,
        }
        if err := architectureplanner.AppendTriggerEvent(opts.Config.PlannerTriggersPath, triggerEvent); err != nil {
            log.Printf("autoloop: append trigger failed: %v", err)
        }
    }
    return nil
}
```

> NOTE: `architectureplanner.TriggerEvent` and `architectureplanner.AppendTriggerEvent` come from Task 7. Task 6 introduces the IMPORT cycle awareness — autoloop imports the planner package's types. Verify there's no circular import (the planner imports autoloop; autoloop importing planner here would create a cycle). If circular, define `TriggerEvent` and `AppendTriggerEvent` in a NEW package `internal/plannertriggers` that BOTH autoloop and planner import. Add this package in Task 6 instead of Task 7 in that case.

- [ ] **Step 6.6: Run tests + commit**

```bash
go test ./internal/autoloop/...
go vet ./internal/autoloop/...
gofmt -l internal/autoloop/
```

```bash
git add internal/autoloop/health_writer.go internal/autoloop/run.go internal/autoloop/config.go internal/autoloop/health_writer_test.go
git commit -m "feat(autoloop): emit planner trigger events on quarantine state changes"
```

---

## Task 7: L2 Planner Side — Triggers and Cursor

**Files:**
- Create: `internal/architectureplanner/triggers.go` (or new `internal/plannertriggers/triggers.go` if Task 6 found a circular import)
- Create: `internal/architectureplanner/triggers_test.go`
- Create: `internal/architectureplanner/triggers_concurrent_test.go`
- Modify: `internal/architectureplanner/config.go`
- Modify: `internal/architectureplanner/context.go`
- Modify: `internal/architectureplanner/prompt.go`
- Modify: `internal/architectureplanner/run.go`

- [ ] **Step 7.1: Write failing trigger consumer tests**

Create `internal/architectureplanner/triggers_test.go`:

```go
package architectureplanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendTriggerEvent_GeneratesIDIfEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	body, _ := os.ReadFile(path)
	var ev TriggerEvent
	if err := json.Unmarshal(body[:len(body)-1], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.ID == "" {
		t.Fatal("AppendTriggerEvent should generate an ID when empty")
	}
}

func TestReadTriggersSinceCursor_EmptyCursorReturnsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	for i := 0; i < 3; i++ {
		_ = AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added", PhaseID: "p", ItemName: "i"})
	}
	events, err := ReadTriggersSinceCursor(path, TriggerCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3, got %d", len(events))
	}
}

func TestReadTriggersSinceCursor_AdvancesPastCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	var ids []string
	for i := 0; i < 5; i++ {
		ev := TriggerEvent{Kind: "quarantine_added"}
		_ = AppendTriggerEvent(path, ev)
		// We need each ID for the cursor; re-read to capture them.
	}
	all, _ := ReadTriggersSinceCursor(path, TriggerCursor{})
	for _, e := range all {
		ids = append(ids, e.ID)
	}
	cursor := TriggerCursor{LastConsumedID: ids[2]}
	events, err := ReadTriggersSinceCursor(path, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events past cursor, got %d", len(events))
	}
}

func TestSaveCursor_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cursor.json")
	cursor := TriggerCursor{LastConsumedID: "abc", LastReadAt: time.Now().UTC().Format(time.RFC3339)}
	if err := SaveCursor(path, cursor); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCursor(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastConsumedID != "abc" {
		t.Fatalf("LastConsumedID = %q", got.LastConsumedID)
	}
}

func TestLoadCursor_MissingFileReturnsZeroValue(t *testing.T) {
	dir := t.TempDir()
	cursor, err := LoadCursor(filepath.Join(dir, "nonexistent.json"))
	if err != nil {
		t.Fatalf("LoadCursor missing file should not error, got: %v", err)
	}
	if cursor.LastConsumedID != "" {
		t.Fatalf("expected zero-value cursor, got %+v", cursor)
	}
}
```

- [ ] **Step 7.2: Run failing tests**

```bash
go test ./internal/architectureplanner/ -run 'TestAppendTrigger|TestReadTriggers|TestSaveCursor|TestLoadCursor' -v
```
Expected: FAIL because the types and functions don't exist.

- [ ] **Step 7.3: Implement `internal/architectureplanner/triggers.go`**

```go
package architectureplanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// TriggerEvent is one event in the autoloop→planner triggers.jsonl ledger.
// Autoloop appends; planner reads.
type TriggerEvent struct {
	ID            string `json:"id"`           // ULID-style: TS + monotonic counter
	TS            string `json:"ts"`           // RFC3339
	Source        string `json:"source"`       // "autoloop"
	Kind          string `json:"kind"`         // "quarantine_added" | "quarantine_stale_cleared" | "manual"
	PhaseID       string `json:"phase_id,omitempty"`
	SubphaseID    string `json:"subphase_id,omitempty"`
	ItemName      string `json:"item_name,omitempty"`
	Reason        string `json:"reason,omitempty"`
	AutoloopRunID string `json:"autoloop_run_id,omitempty"`
}

// TriggerCursor is the planner's bookmark in triggers.jsonl. Advances after
// each planner run consumes events.
type TriggerCursor struct {
	LastConsumedID string `json:"last_consumed_id"`
	LastReadAt     string `json:"last_read_at"`
}

var triggerIDCounter atomic.Uint64

// AppendTriggerEvent atomically appends one TriggerEvent. Generates a
// process-monotonic ID if event.ID is empty. Defaults TS to now if empty.
func AppendTriggerEvent(path string, event TriggerEvent) error {
	if event.ID == "" {
		now := time.Now().UTC()
		seq := triggerIDCounter.Add(1)
		event.ID = fmt.Sprintf("%s-%06d", now.Format("20060102T150405.000Z"), seq)
	}
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Source == "" {
		event.Source = "autoloop"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir trigger dir: %w", err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal trigger event: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open triggers: %w", err)
	}
	defer f.Close()
	_, err = f.Write(body)
	return err
}

// ReadTriggersSinceCursor returns events strictly after cursor.LastConsumedID
// in append order. If LastConsumedID is empty, returns all events. Bad lines
// are skipped.
func ReadTriggersSinceCursor(path string, cursor TriggerCursor) ([]TriggerEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []TriggerEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev TriggerEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		all = append(all, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if cursor.LastConsumedID == "" {
		return all, nil
	}
	for i, ev := range all {
		if ev.ID == cursor.LastConsumedID {
			return all[i+1:], nil
		}
	}
	// Cursor not found in current file; return all (cursor is stale).
	return all, nil
}

// LoadCursor reads triggers_cursor.json. Missing file returns zero value.
func LoadCursor(path string) (TriggerCursor, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TriggerCursor{}, nil
		}
		return TriggerCursor{}, err
	}
	var c TriggerCursor
	if err := json.Unmarshal(body, &c); err != nil {
		return TriggerCursor{}, err
	}
	return c, nil
}

// SaveCursor atomically writes the cursor via temp + rename.
func SaveCursor(path string, cursor TriggerCursor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cursor-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
```

- [ ] **Step 7.4: Add concurrent test**

Create `internal/architectureplanner/triggers_concurrent_test.go`:

```go
package architectureplanner

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestAppendTriggerEvent_ConcurrentWritersAllSucceed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	const N = 8
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := AppendTriggerEvent(path, TriggerEvent{
				Kind:    "quarantine_added",
				PhaseID: "p",
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent append error: %v", err)
	}
	all, err := ReadTriggersSinceCursor(path, TriggerCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != N {
		t.Fatalf("expected %d events, got %d", N, len(all))
	}
	// Verify no two events share an ID.
	seen := map[string]bool{}
	for _, ev := range all {
		if seen[ev.ID] {
			t.Fatalf("duplicate ID: %s", ev.ID)
		}
		seen[ev.ID] = true
	}
}
```

- [ ] **Step 7.5: Add config fields**

Modify `internal/architectureplanner/config.go`:

```go
type Config struct {
    // ... existing fields ...
    PlannerTriggersPath string // PLANNER_TRIGGERS_PATH
    TriggersCursorPath  string // not env-overridable; lives next to ledger
}
```

In `ConfigFromEnv`, default `PlannerTriggersPath` to `filepath.Join(repoRoot, ".codex", "architecture-planner", "triggers.jsonl")` and `TriggersCursorPath` to `filepath.Join(cfg.RunRoot, "state", "triggers_cursor.json")`.

- [ ] **Step 7.6: Wire trigger reading into RunOnce + prompt section**

Modify `internal/architectureplanner/run.go::RunOnce`. Before building the prompt:

```go
cursor, _ := LoadCursor(cfg.TriggersCursorPath)
triggerEvents, _ := ReadTriggersSinceCursor(cfg.PlannerTriggersPath, cursor)

// Trigger source for the ledger.
trigger := "scheduled"
if len(triggerEvents) > 0 {
    trigger = "event"
}
bundle.TriggerEvents = triggerEvents
```

Modify `internal/architectureplanner/context.go`:

```go
type ContextBundle struct {
    // ... existing fields ...
    TriggerEvents []TriggerEvent `json:"trigger_events,omitempty"`
}
```

Modify `internal/architectureplanner/prompt.go::BuildPrompt` to render a trigger-events bullet section when `len(bundle.TriggerEvents) > 0`:

```go
func formatTriggerEvents(events []TriggerEvent) string {
    if len(events) == 0 {
        return ""
    }
    var b strings.Builder
    b.WriteString("\n## Recent Autoloop Signals (Since Last Planner Run)\n\nThese rows changed state in autoloop and may need attention this run:\n\n")
    for _, ev := range events {
        fmt.Fprintf(&b, "- %s/%s/%s — %s — %s\n", ev.PhaseID, ev.SubphaseID, ev.ItemName, ev.Kind, ev.Reason)
    }
    return b.String()
}
```

After `RunOnce` completes (success OR failure), advance the cursor:

```go
defer func() {
    if len(triggerEvents) > 0 {
        newCursor := TriggerCursor{
            LastConsumedID: triggerEvents[len(triggerEvents)-1].ID,
            LastReadAt:     now.UTC().Format(time.RFC3339),
        }
        _ = SaveCursor(cfg.TriggersCursorPath, newCursor) // soft-fail
    }
}()
```

Update Task 4's ledger event population to use `trigger` and the consumed event IDs:

```go
event.Trigger = trigger
for _, ev := range triggerEvents {
    event.TriggerEvents = append(event.TriggerEvents, ev.ID)
}
```

- [ ] **Step 7.7: Run + commit**

```bash
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

```bash
git add internal/architectureplanner/triggers.go internal/architectureplanner/triggers_test.go internal/architectureplanner/triggers_concurrent_test.go internal/architectureplanner/config.go internal/architectureplanner/context.go internal/architectureplanner/prompt.go internal/architectureplanner/run.go
git commit -m "feat(planner): consume autoloop trigger ledger via cursor"
```

---

## Task 8: L2 systemd path unit

**Files:**
- Modify: `internal/architectureplanner/service.go`
- Modify: `internal/architectureplanner/service_test.go`

- [ ] **Step 8.1: Write failing tests for path unit rendering**

Append to `internal/architectureplanner/service_test.go`:

```go
func TestRenderPlannerPathUnit_ContainsExpectedDirectives(t *testing.T) {
	rendered := RenderPlannerPathUnit(PlannerPathUnitOptions{
		Description:  "Trigger Gormes architecture planner on autoloop signal",
		PathToWatch:  "/home/test/.codex/architecture-planner/triggers.jsonl",
		ServiceUnit:  "gormes-architecture-planner.service",
	})
	wants := []string{
		"PathChanged=/home/test/.codex/architecture-planner/triggers.jsonl",
		"TriggerLimitIntervalSec=60",
		"TriggerLimitBurst=1",
		"Unit=gormes-architecture-planner.service",
		"WantedBy=default.target",
	}
	for _, w := range wants {
		if !strings.Contains(rendered, w) {
			t.Errorf("rendered unit missing %q\n%s", w, rendered)
		}
	}
}

func TestInstallPlannerService_WritesAllThreeUnits(t *testing.T) {
	dir := t.TempDir()
	opts := PlannerServiceInstallOptions{
		Runner:    fakeServiceRunner{},
		UnitDir:   dir,
		UnitName:  "gormes-architecture-planner.service",
		TimerName: "gormes-architecture-planner.timer",
		PathName:  "gormes-architecture-planner.path",
		PlannerPath: "/usr/local/bin/planner.sh",
		WorkDir:   "/repo",
	}
	if err := InstallPlannerService(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"gormes-architecture-planner.service",
		"gormes-architecture-planner.timer",
		"gormes-architecture-planner.path",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("unit %s not written: %v", name, err)
		}
	}
}
```

- [ ] **Step 8.2: Implement `RenderPlannerPathUnit` and extend install**

In `internal/architectureplanner/service.go`:

```go
type PlannerPathUnitOptions struct {
    Description string
    PathToWatch string
    ServiceUnit string
}

func RenderPlannerPathUnit(opts PlannerPathUnitOptions) string {
    return fmt.Sprintf(`[Unit]
Description=%s

[Path]
PathChanged=%s
TriggerLimitIntervalSec=60
TriggerLimitBurst=1
Unit=%s

[Install]
WantedBy=default.target
`, opts.Description, opts.PathToWatch, opts.ServiceUnit)
}
```

Extend `PlannerServiceInstallOptions`:

```go
type PlannerServiceInstallOptions struct {
    // ... existing fields ...
    PathName string // e.g. "gormes-architecture-planner.path"; defaults if empty
}
```

In `InstallPlannerService`, after writing the `.timer` file, also write the `.path` file. Use the `PlannerTriggersPath` from Config (or pass it via opts) to get the watched path.

- [ ] **Step 8.3: Run + commit**

```bash
go test ./internal/architectureplanner/ -run TestRenderPlannerPath -v
go test ./internal/architectureplanner/ -run TestInstallPlannerService -v
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

```bash
git add internal/architectureplanner/service.go internal/architectureplanner/service_test.go
git commit -m "feat(planner): install path unit alongside timer for event triggers"
```

---

## Task 9: L3 Retry-with-feedback

**Files:**
- Create: `internal/architectureplanner/retry.go`
- Create: `internal/architectureplanner/retry_test.go`
- Modify: `internal/architectureplanner/run.go`
- Modify: `internal/architectureplanner/config.go`
- Modify: `internal/architectureplanner/run_test.go`

- [ ] **Step 9.1: Write failing tests**

Create `internal/architectureplanner/retry_test.go`:

```go
package architectureplanner

import (
	"errors"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestRetryFeedback_NamesAllDroppedRows(t *testing.T) {
	before := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: &progress.RowHealth{AttemptCount: 3}},
					{Name: "row-y", Health: &progress.RowHealth{AttemptCount: 2}},
				}},
			}},
		},
	}
	after := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: nil}, // dropped
					{Name: "row-y", Health: &progress.RowHealth{AttemptCount: 2}},
				}},
			}},
		},
	}
	feedback := RetryFeedback(errors.New("validation error"), before, after)
	if !strings.Contains(feedback, "1/1.A/row-x") {
		t.Fatalf("feedback should name dropped row, got:\n%s", feedback)
	}
	if !strings.Contains(feedback, "HEALTH BLOCK PRESERVATION") {
		t.Fatal("feedback missing HARD RULE reference")
	}
}

func TestExtractDroppedRows_FindsDroppedAndModified(t *testing.T) {
	before := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: &progress.RowHealth{AttemptCount: 3}},
				}},
			}},
		},
	}
	after := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: nil},
				}},
			}},
		},
	}
	dropped := extractDroppedRows(before, after)
	if len(dropped) != 1 || dropped[0] != "1/1.A/row-x" {
		t.Fatalf("expected 1/1.A/row-x, got %v", dropped)
	}
}
```

- [ ] **Step 9.2: Implement `internal/architectureplanner/retry.go`**

```go
package architectureplanner

import (
	"fmt"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

const DefaultMaxRetries = 2

// retryAttempt records one LLM call's full lifecycle for ledger forensics.
type retryAttempt struct {
	Index       int      `json:"index"`
	Status      string   `json:"status"` // "ok" | "validation_rejected" | "backend_failed"
	Detail      string   `json:"detail,omitempty"`
	DroppedRows []string `json:"dropped_rows,omitempty"`
}

// RetryFeedback formats a one-paragraph correction prompt for the LLM after
// validateHealthPreservation rejects a regen. Names the dropped rows
// explicitly and references the HARD rule.
func RetryFeedback(rejection error, beforeDoc, afterDoc *progress.Progress) string {
	dropped := extractDroppedRows(beforeDoc, afterDoc)
	var b strings.Builder
	b.WriteString("\n\nRETRY: Your previous output dropped or modified the `health` block on the\nfollowing rows. Per the HEALTH BLOCK PRESERVATION (HARD RULE), you must\nreproduce every `health` block verbatim. Please regenerate the entire\nprogress.json output, this time preserving these rows' health metadata:\n\n")
	for _, row := range dropped {
		fmt.Fprintf(&b, "- %s\n", row)
	}
	b.WriteString("\nThe original task and quarantine priorities still apply. Do NOT re-do the\nupstream sync analysis or implementation inventory — just produce a corrected\nprogress.json with the health blocks restored.\n")
	return b.String()
}

// extractDroppedRows identifies rows whose Health block was dropped or
// modified between before and after. Used by RetryFeedback and the ledger
// retryAttempt forensics.
func extractDroppedRows(beforeDoc, afterDoc *progress.Progress) []string {
	var out []string
	beforeIndex := indexItems(beforeDoc) // existing helper
	afterIndex := indexItems(afterDoc)
	for key, beforeItem := range beforeIndex {
		afterItem, exists := afterIndex[key]
		if !exists {
			continue // intentional deletion is not a "dropped health"
		}
		if !healthEqual(beforeItem.Health, afterItem.Health) {
			out = append(out, fmt.Sprintf("%s/%s/%s", key.phaseID, key.subphaseID, key.itemName))
		}
	}
	return out
}
```

- [ ] **Step 9.3: Add `MaxRetries` to Config**

```go
type Config struct {
    // ... existing ...
    MaxRetries int // PLANNER_MAX_RETRIES; default 2
}
```

In `ConfigFromEnv`, default and env-override.

- [ ] **Step 9.4: Wire retry loop into RunOnce**

Refactor the existing single-call backend invocation in `RunOnce` into a retry loop:

```go
maxRetries := cfg.MaxRetries
prompt := initialPrompt
attempts := []retryAttempt{}
var afterDoc *progress.Progress

for i := 0; i <= maxRetries; i++ {
    result, err := runner.Run(ctx, autoloop.Command{
        Name: argv[0], Args: append(argv[1:], prompt), Dir: cfg.RepoRoot,
    })
    attempt := retryAttempt{Index: i}
    if err != nil {
        attempt.Status = "backend_failed"
        attempt.Detail = err.Error()
        attempts = append(attempts, attempt)
        // Backend failure is not retried; ledger emission + return below.
        return /* with ledger entry status="backend_failed" */
    }
    afterDoc, _ = loadProgressForValidation(cfg.ProgressJSON)
    if err := validateHealthPreservation(beforeDoc, afterDoc); err != nil {
        attempt.Status = "validation_rejected"
        attempt.Detail = err.Error()
        attempt.DroppedRows = extractDroppedRows(beforeDoc, afterDoc)
        attempts = append(attempts, attempt)
        if i == maxRetries {
            return /* with ledger entry status="validation_rejected", attempts populated */
        }
        prompt = initialPrompt + RetryFeedback(err, beforeDoc, afterDoc)
        continue
    }
    attempt.Status = "ok"
    attempts = append(attempts, attempt)
    break
}

event.RetryAttempt = attempts[len(attempts)-1].Index
// ledger entry's existing fields populated as before; attempts captured in
// LedgerEvent.Detail or a new field if you want full forensics
```

> The plan deliberately leaves the exact placement of `attempts` in the LedgerEvent to the implementer's discretion. Either add a new `Attempts []retryAttempt` field on LedgerEvent (cleanest), or serialize them into the Detail string. Choose what's easiest given the existing run.go shape.

- [ ] **Step 9.5: Run + commit**

```bash
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

```bash
git add internal/architectureplanner/retry.go internal/architectureplanner/retry_test.go internal/architectureplanner/run.go internal/architectureplanner/config.go internal/architectureplanner/run_test.go
git commit -m "feat(planner): retry with feedback on validation rejection"
```

---

## Task 10: L4 Self-evaluation

**Files:**
- Create: `internal/architectureplanner/evaluation.go`
- Create: `internal/architectureplanner/evaluation_test.go`
- Modify: `internal/architectureplanner/context.go`
- Modify: `internal/architectureplanner/prompt.go`
- Modify: `internal/architectureplanner/prompt_test.go`
- Modify: `internal/architectureplanner/run.go`
- Modify: `internal/architectureplanner/config.go`

- [ ] **Step 10.1: Write failing tests**

Create `internal/architectureplanner/evaluation_test.go`:

```go
package architectureplanner

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluate_UnstuckRowDetected(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	reshapeTS := now.Add(-2 * time.Hour)

	// Planner reshaped row.
	_ = AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: reshapeTS.Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	})

	// Autoloop later promoted the same row.
	autoloopEvent := map[string]any{
		"ts":     now.Add(-1 * time.Hour).Format(time.RFC3339),
		"event":  "worker_promoted",
		"task":   "2/2.B/row-1",
		"status": "promoted",
	}
	appendLineJSON(t, autoloopLedger, autoloopEvent)

	outcomes, err := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Outcome != "unstuck" {
		t.Fatalf("expected unstuck, got %q", outcomes[0].Outcome)
	}
}

func TestEvaluate_StillFailingDetected(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	reshapeTS := now.Add(-2 * time.Hour)

	_ = AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: reshapeTS.Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	})

	for i := 0; i < 3; i++ {
		appendLineJSON(t, autoloopLedger, map[string]any{
			"ts":     now.Add(-time.Duration(60-i*10) * time.Minute).Format(time.RFC3339),
			"event":  "worker_failed",
			"task":   "2/2.B/row-1",
			"status": "failed",
		})
	}

	outcomes, _ := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if outcomes[0].Outcome != "still_failing" {
		t.Fatalf("expected still_failing, got %q", outcomes[0].Outcome)
	}
}

func TestEvaluate_NoAttemptsYet(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")
	now := time.Now().UTC()

	_ = AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: now.Add(-time.Hour).Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	})
	// No autoloop ledger entries.

	outcomes, _ := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if outcomes[0].Outcome != "no_attempts_yet" {
		t.Fatalf("expected no_attempts_yet, got %q", outcomes[0].Outcome)
	}
}

// appendLineJSON appends one JSON-encoded map to an autoloop-style ledger.
// The autoloop ledger schema differs from the planner's; we use the same
// O_APPEND pattern for compatibility.
func appendLineJSON(t *testing.T, path string, obj map[string]any) {
	t.Helper()
	body, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 10.2: Implement `internal/architectureplanner/evaluation.go`**

```go
package architectureplanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const DefaultEvaluationWindow = 7 * 24 * time.Hour

// ReshapeOutcome correlates one planner-recorded RowChange{Kind:"spec_changed"}
// with what autoloop did to that row in subsequent runs.
type ReshapeOutcome struct {
	PhaseID            string `json:"phase_id"`
	SubphaseID         string `json:"subphase_id"`
	ItemName           string `json:"item_name"`
	ReshapedAt         string `json:"reshaped_at"`
	ReshapedBy         string `json:"reshaped_by"`
	Outcome            string `json:"outcome"`         // "unstuck" | "still_failing" | "no_attempts_yet"
	AutoloopRuns       int    `json:"autoloop_runs"`
	LastFailure        string `json:"last_failure,omitempty"`
	LastSuccess        string `json:"last_success,omitempty"`
	StaleClearObserved bool   `json:"stale_clear_observed"`
}

// Evaluate walks the planner ledger window, collects every row reshape, and
// correlates each with the autoloop ledger to determine the outcome.
func Evaluate(plannerLedgerPath, autoloopLedgerPath string, window time.Duration, now time.Time) ([]ReshapeOutcome, error) {
	plannerEvents, err := LoadLedgerWindow(plannerLedgerPath, window, now)
	if err != nil {
		return nil, fmt.Errorf("evaluate: load planner ledger: %w", err)
	}

	autoloopEvents, err := loadAutoloopLedgerLite(autoloopLedgerPath, window, now)
	if err != nil {
		// Don't fail evaluation if autoloop ledger is missing; treat as no attempts.
		autoloopEvents = nil
	}

	type latestReshape struct {
		event  LedgerEvent
		change RowChange
	}
	latest := map[string]latestReshape{}
	for _, ev := range plannerEvents {
		for _, rc := range ev.RowsChanged {
			if rc.Kind != "spec_changed" {
				continue
			}
			key := rc.PhaseID + "/" + rc.SubphaseID + "/" + rc.ItemName
			latest[key] = latestReshape{event: ev, change: rc}
		}
	}

	var out []ReshapeOutcome
	for key, reshape := range latest {
		reshapeTS, err := time.Parse(time.RFC3339, reshape.event.TS)
		if err != nil {
			continue
		}
		taskKey := reshape.change.PhaseID + "/" + reshape.change.SubphaseID + "/" + reshape.change.ItemName
		outcome := classifyOutcome(taskKey, reshapeTS, autoloopEvents)
		_ = key
		out = append(out, ReshapeOutcome{
			PhaseID:    reshape.change.PhaseID,
			SubphaseID: reshape.change.SubphaseID,
			ItemName:   reshape.change.ItemName,
			ReshapedAt: reshape.event.TS,
			ReshapedBy: reshape.event.RunID,
			Outcome:    outcome.kind,
			AutoloopRuns: outcome.runs,
			LastFailure:  outcome.lastFailure,
			LastSuccess:  outcome.lastSuccess,
			StaleClearObserved: outcome.staleClearObserved,
		})
	}
	return out, nil
}

type autoloopEventLite struct {
	TS     string `json:"ts"`
	Event  string `json:"event"`
	Task   string `json:"task"`
	Status string `json:"status"`
}

type outcomeClass struct {
	kind               string
	runs               int
	lastFailure        string
	lastSuccess        string
	staleClearObserved bool
}

func classifyOutcome(taskKey string, reshapeTS time.Time, events []autoloopEventLite) outcomeClass {
	var runs int
	var lastFailure, lastSuccess string
	var staleClear bool
	var promoted bool
	for _, ev := range events {
		if !taskMatches(ev.Task, taskKey) {
			continue
		}
		evTS, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil || !evTS.After(reshapeTS) {
			continue
		}
		runs++
		switch ev.Event {
		case "worker_promoted":
			promoted = true
			lastSuccess = ev.TS
		case "worker_failed", "worker_error":
			lastFailure = ev.Status
		case "backend_degraded":
			// not row-level; ignore
		}
		if ev.Status == "stale_quarantine_cleared" || ev.Event == "quarantine_stale_cleared" {
			staleClear = true
		}
	}
	if promoted {
		return outcomeClass{kind: "unstuck", runs: runs, lastSuccess: lastSuccess, staleClearObserved: staleClear}
	}
	if runs > 0 {
		return outcomeClass{kind: "still_failing", runs: runs, lastFailure: lastFailure, staleClearObserved: staleClear}
	}
	return outcomeClass{kind: "no_attempts_yet"}
}

func taskMatches(autoloopTask, taskKey string) bool {
	// Autoloop's "task" field encodes "phase/subphase/item" or similar.
	// Match exact OR contains.
	if autoloopTask == taskKey {
		return true
	}
	return strings.Contains(autoloopTask, taskKey)
}

// loadAutoloopLedgerLite reads autoloop's runs.jsonl, decoding only the
// fields evaluation cares about. Schema differences from the planner ledger
// are tolerated (unknown fields are ignored by encoding/json).
func loadAutoloopLedgerLite(path string, window time.Duration, now time.Time) ([]autoloopEventLite, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cutoff := now.Add(-window)
	var out []autoloopEventLite
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev autoloopEventLite
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}
```

- [ ] **Step 10.3: Wire into ContextBundle and BuildPrompt**

Modify `internal/architectureplanner/context.go`:

```go
type ContextBundle struct {
    // ... existing ...
    PreviousReshapes []ReshapeOutcome `json:"previous_reshapes,omitempty"`
}
```

Modify `internal/architectureplanner/run.go::RunOnce`:

```go
outcomes, _ := Evaluate(
    filepath.Join(cfg.RunRoot, "state", "runs.jsonl"),
    filepath.Join(cfg.AutoloopRunRoot, "state", "runs.jsonl"),
    cfg.EvaluationWindow,
    now,
)
bundle.PreviousReshapes = outcomes
```

Add `Config.EvaluationWindow` (env `PLANNER_EVALUATION_WINDOW`, default `7*24*time.Hour`).

Modify `internal/architectureplanner/prompt.go::BuildPrompt` to render a "Previous Reshape Outcomes" section when `bundle.PreviousReshapes` is non-empty, plus the `SELF-EVALUATION (SOFT RULE)` clause unconditionally:

```go
const selfEvaluationClause = `
SELF-EVALUATION (SOFT RULE)

The "Previous Reshape Outcomes" section reports what autoloop did with rows
you reshaped in past runs. Use this signal:
  - UNSTUCK rows confirm your previous approach worked
  - STILL FAILING rows have resisted reshape — try a different decomposition,
    escalate to "needs_human" via PlannerVerdict (L5), or tighten ready_when
  - NO ATTEMPTS YET rows may be legitimately blocked
`

func formatPreviousReshapes(outcomes []ReshapeOutcome) string {
    if len(outcomes) == 0 {
        return ""
    }
    // Bucket by outcome.
    var unstuck, still, none []ReshapeOutcome
    for _, o := range outcomes {
        switch o.Outcome {
        case "unstuck":
            unstuck = append(unstuck, o)
        case "still_failing":
            still = append(still, o)
        default:
            none = append(none, o)
        }
    }
    var b strings.Builder
    b.WriteString("\n## Previous Reshape Outcomes (Last 7 Days)\n\n")
    if len(unstuck) > 0 {
        fmt.Fprintf(&b, "UNSTUCK (%d):\n", len(unstuck))
        for _, o := range unstuck {
            fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop promoted %s\n",
                o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy, o.LastSuccess)
        }
    }
    if len(still) > 0 {
        fmt.Fprintf(&b, "\nSTILL FAILING (%d):\n", len(still))
        for _, o := range still {
            fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop attempted %d times, last category: %s\n",
                o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy, o.AutoloopRuns, o.LastFailure)
        }
    }
    if len(none) > 0 {
        fmt.Fprintf(&b, "\nNO ATTEMPTS YET (%d):\n", len(none))
        for _, o := range none {
            fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop has not selected this row since\n",
                o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy)
        }
    }
    return b.String()
}
```

In BuildPrompt, append both `selfEvaluationClause` and `formatPreviousReshapes(bundle.PreviousReshapes)` to the existing template.

Add prompt tests for the new section.

- [ ] **Step 10.4: Run + commit**

```bash
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

```bash
git add internal/architectureplanner/evaluation.go internal/architectureplanner/evaluation_test.go internal/architectureplanner/context.go internal/architectureplanner/prompt.go internal/architectureplanner/prompt_test.go internal/architectureplanner/run.go internal/architectureplanner/config.go
git commit -m "feat(planner): self-evaluation correlates planner ledger with autoloop"
```

---

## Task 11: L5 PlannerVerdict Stamping (Planner Side)

**Files:**
- Create: `internal/architectureplanner/verdict.go`
- Create: `internal/architectureplanner/verdict_test.go`
- Modify: `internal/architectureplanner/run.go`
- Modify: `internal/architectureplanner/config.go`

- [ ] **Step 11.1: Write failing tests**

Create `internal/architectureplanner/verdict_test.go`:

```go
package architectureplanner

import (
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestStampVerdicts_IncrementsReshapeCount(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c"},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	StampVerdicts(doc, rowsChanged, nil, 3, now)
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict == nil || row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatalf("ReshapeCount expected 1, got %+v", row.PlannerVerdict)
	}
	if row.PlannerVerdict.LastReshape != now.Format(time.RFC3339) {
		t.Fatal("LastReshape should be set to now")
	}
}

func TestStampVerdicts_SetsNeedsHumanWhenThresholdReachedAndStillFailing(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{ReshapeCount: 2}},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "still_failing", LastFailure: "report_validation_failed"},
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	StampVerdicts(doc, rowsChanged, outcomes, 3, now)
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("NeedsHuman should be set after threshold")
	}
	if row.PlannerVerdict.Reason == "" {
		t.Fatal("Reason should be set")
	}
}

func TestStampVerdicts_NeedsHumanIsSticky(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{
						NeedsHuman: true, Reason: "original reason", ReshapeCount: 5,
					}},
				}},
			}},
		},
	}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "unstuck", LastSuccess: "now"},
	}
	StampVerdicts(doc, nil, outcomes, 3, time.Now())
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("NeedsHuman must remain true (sticky)")
	}
	if row.PlannerVerdict.Reason != "original reason" {
		t.Fatal("Reason should not be overwritten")
	}
	if row.PlannerVerdict.LastOutcome != "unstuck" {
		t.Fatal("LastOutcome should be updated even when NeedsHuman is sticky")
	}
}

func TestStampVerdicts_DoesNotSetNeedsHumanIfUnstuck(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{ReshapeCount: 10}},
				}},
			}},
		},
	}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "unstuck"},
	}
	StampVerdicts(doc, nil, outcomes, 3, time.Now())
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict.NeedsHuman {
		t.Fatal("unstuck row should NOT trigger NeedsHuman regardless of ReshapeCount")
	}
}

func TestStampVerdicts_ReturnsVerdictChangesForLedger(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]*progress.Phase{
			"1": {Name: "P", Subphases: map[string]*progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c"},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	changes := StampVerdicts(doc, rowsChanged, nil, 3, time.Now())
	if len(changes) != 1 || changes[0].Kind != "verdict_set" {
		t.Fatalf("expected one verdict_set change, got %+v", changes)
	}
}
```

- [ ] **Step 11.2: Implement `internal/architectureplanner/verdict.go`**

```go
package architectureplanner

import (
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

const DefaultEscalationThreshold = 3

// StampVerdicts applies deterministic PlannerVerdict updates to the after-doc.
// Returns the list of rows whose verdict materially changed for ledger
// emission as RowChange{Kind:"verdict_set"}.
func StampVerdicts(afterDoc *progress.Progress, rowsChanged []RowChange, outcomes []ReshapeOutcome, threshold int, now time.Time) []RowChange {
	if afterDoc == nil {
		return nil
	}
	if threshold <= 0 {
		threshold = DefaultEscalationThreshold
	}

	// Index rows for fast lookup.
	idx := indexItems(afterDoc) // existing helper from Phase B
	nowStr := now.UTC().Format(time.RFC3339)

	var changed []RowChange

	// Step 1: increment ReshapeCount for every reshaped row.
	for _, rc := range rowsChanged {
		if rc.Kind != "spec_changed" {
			continue
		}
		key := itemKey{rc.PhaseID, rc.SubphaseID, rc.ItemName}
		item, ok := idx[key]
		if !ok {
			continue
		}
		if item.PlannerVerdict == nil {
			item.PlannerVerdict = &progress.PlannerVerdict{}
		}
		item.PlannerVerdict.ReshapeCount++
		item.PlannerVerdict.LastReshape = nowStr
		changed = append(changed, RowChange{
			PhaseID: rc.PhaseID, SubphaseID: rc.SubphaseID, ItemName: rc.ItemName,
			Kind: "verdict_set", Detail: "reshape_count incremented",
		})
	}

	// Step 2: apply outcome-based updates.
	for _, oc := range outcomes {
		key := itemKey{oc.PhaseID, oc.SubphaseID, oc.ItemName}
		item, ok := idx[key]
		if !ok {
			continue
		}
		if item.PlannerVerdict == nil {
			item.PlannerVerdict = &progress.PlannerVerdict{}
		}
		v := item.PlannerVerdict
		v.LastOutcome = oc.Outcome

		// Sticky: do not auto-clear NeedsHuman.
		if oc.Outcome == "still_failing" && !v.NeedsHuman && v.ReshapeCount >= threshold {
			v.NeedsHuman = true
			v.Reason = fmt.Sprintf("auto: %d reshapes without unsticking; last category %s", v.ReshapeCount, oc.LastFailure)
			v.Since = nowStr
			changed = append(changed, RowChange{
				PhaseID: oc.PhaseID, SubphaseID: oc.SubphaseID, ItemName: oc.ItemName,
				Kind: "verdict_set", Detail: "needs_human=true",
			})
		}
	}

	return changed
}
```

- [ ] **Step 11.3: Add `EscalationThreshold` to Config**

```go
type Config struct {
    // ... existing ...
    EscalationThreshold int // PLANNER_ESCALATION_THRESHOLD; default 3
}
```

In `ConfigFromEnv`, default and env-override.

- [ ] **Step 11.4: Wire StampVerdicts into RunOnce**

After `validateHealthPreservation` passes and BEFORE `SaveProgress`:

```go
verdictChanges := StampVerdicts(afterDoc, rowsChanged, outcomes, cfg.EscalationThreshold, now)
event.RowsChanged = append(event.RowsChanged, verdictChanges...)
// SaveProgress writes both the LLM regen AND the verdict stamps atomically.
```

- [ ] **Step 11.5: Run + commit**

```bash
go test ./internal/architectureplanner/...
go vet ./internal/architectureplanner/...
gofmt -l internal/architectureplanner/
```

```bash
git add internal/architectureplanner/verdict.go internal/architectureplanner/verdict_test.go internal/architectureplanner/run.go internal/architectureplanner/config.go
git commit -m "feat(planner): stamp PlannerVerdict after successful regeneration"
```

---

## Task 12: L5 Autoloop Selection Skip + Status Surface

**Files:**
- Modify: `internal/autoloop/candidates.go`
- Modify: `internal/autoloop/candidates_health_test.go`
- Modify: `internal/autoloop/config.go`
- Modify: `internal/autoloop/config_test.go`
- Modify: `cmd/architecture-planner-loop/main.go`
- Create: `internal/architectureplanner/status_test.go`

- [ ] **Step 12.1: Write failing autoloop selection tests**

Append to `internal/autoloop/candidates_health_test.go`:

```go
func TestNormalizeCandidates_NeedsHumanSkippedByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft",
             "planner_verdict": {"needs_human": true, "reason": "auto", "since": "2026-04-25T10:00:00Z"}},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ItemName != "row-b" {
		t.Fatalf("expected only row-b, got %+v", got)
	}
}

func TestNormalizeCandidates_IncludeNeedsHumanSurfacesAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft",
             "planner_verdict": {"needs_human": true}}
          ]
        }
      }
    }
  }
}
`)
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true, IncludeNeedsHuman: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if !got[0].NeedsHumanFlag {
		t.Fatal("NeedsHumanFlag should be true")
	}
}
```

- [ ] **Step 12.2: Implement skip + flag in candidates.go**

In `NormalizeCandidates`, after the existing quarantine filter (Phase B Task 5), add:

```go
if item.PlannerVerdict != nil && item.PlannerVerdict.NeedsHuman {
    if !opts.IncludeNeedsHuman {
        continue
    }
    candidate.NeedsHumanFlag = true
}
```

Add `Candidate.NeedsHumanFlag bool` field.

Add `CandidateOptions.IncludeNeedsHuman bool` field.

Update `SelectionReason()` to append `" needs_human_visible"` when `NeedsHumanFlag`.

In `internal/autoloop/config.go`, add `IncludeNeedsHuman` (env `GORMES_INCLUDE_NEEDS_HUMAN`, default `false`) and wire it into the call site that constructs `CandidateOptions` (`run.go`).

- [ ] **Step 12.3: Implement status surface extension**

Refactor `cmd/architecture-planner-loop/main.go::printStatus` so the bulk of the rendering logic lives in `internal/architectureplanner` and is testable. Move it to a new function:

```go
// internal/architectureplanner/status.go (or extend an existing file)

// RenderStatus returns the multi-line operator-facing status string.
// Combines current planner_state.json metadata + recent ledger outcomes +
// NeedsHuman row inventory.
func RenderStatus(opts RenderStatusOptions) (string, error) {
    // 1. Read planner_state.json
    // 2. Read planner ledger for last few entries (or pass evaluation outcomes)
    // 3. Read progress.json to inventory NeedsHuman rows
    // 4. Format per the spec
}
```

Update `cmd/architecture-planner-loop/main.go::printStatus` to call `architectureplanner.RenderStatus` and write the result.

Create `internal/architectureplanner/status_test.go`:

```go
package architectureplanner

import (
	"strings"
	"testing"
)

func TestRenderStatus_IncludesOutcomesAndNeedsHuman(t *testing.T) {
	t.Skip("FILL IN: synthesize planner ledger + progress.json with NeedsHuman rows; assert render output contains: 'Reshape outcomes (last 7d):', 'unstuck:', 'still failing:', 'Rows needing human attention:', and per-row entries with reason + suggested action")
}

func TestSuggestedActionForCategory_TableDriven(t *testing.T) {
	cases := []struct {
		category string
		want     string
	}{
		{"report_validation_failed", "split into smaller rows or set contract_status=\"draft\""},
		{"worker_error", "investigate infrastructure (backend or worktree state)"},
		{"backend_degraded", "investigate infrastructure (backend or worktree state)"},
		{"progress_summary_failed", "manual contract review — autoloop preflight is failing"},
		{"timeout", "split into smaller rows; the work is too large for the worker budget"},
		{"", "manual review"},
		{"unknown_category", "manual review"},
	}
	for _, c := range cases {
		got := SuggestedActionForCategory(c.category)
		if !strings.Contains(got, c.want) {
			t.Errorf("SuggestedActionForCategory(%q) = %q, want substring %q", c.category, got, c.want)
		}
	}
}
```

Implement `SuggestedActionForCategory` and the bulk of `RenderStatus` per the spec.

- [ ] **Step 12.4: Run + commit**

```bash
go test ./internal/autoloop/...
go test ./internal/architectureplanner/...
go test ./cmd/architecture-planner-loop/...
go vet ./...
gofmt -l .
```

```bash
git add internal/autoloop/candidates.go internal/autoloop/candidates_health_test.go internal/autoloop/config.go internal/autoloop/config_test.go internal/autoloop/run.go cmd/architecture-planner-loop/main.go internal/architectureplanner/status.go internal/architectureplanner/status_test.go
git commit -m "feat(autoloop): skip needs_human rows; planner status surfaces them"
```

---

## Task 13: End-To-End Lifecycle Test

**Files:**
- Create: `internal/architectureplanner/lifecycle_test.go`

- [ ] **Step 13.1: Write the lifecycle test**

Create `internal/architectureplanner/lifecycle_test.go`:

```go
package architectureplanner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// TestLifecycle_PlannerSelfHealingFullLoop walks one row through the full
// Phase C loop using real APIs (no LLM):
//
//   Run 1 (autoloop): row-x fails 3 times → quarantine → trigger emitted
//   Run 2 (planner, event): trigger consumed; row-x reshape; verdict.ReshapeCount=1
//   Run 3 (autoloop): stale-quarantine flagged; row-x attempted; fails again →
//                     quarantine re-set; trigger emitted again
//   Run 4 (planner, event): L4 outcome=still_failing; verdict.ReshapeCount=2
//   Run 5 (autoloop): row-x fails again → quarantine re-set
//   Run 6 (planner, event): verdict.ReshapeCount=3 → NeedsHuman=true
//   Run 7 (autoloop): selection EXCLUDES row-x (NeedsHuman)
//
// Drives autoloop's accumulator and planner's StampVerdicts directly. The
// LLM is mocked: each "planner run" simulates a successful regen by mutating
// the row's contract (which advances ItemSpecHash) without dropping Health.
func TestLifecycle_PlannerSelfHealingFullLoop(t *testing.T) {
	t.Skip("FILL IN: scaffold this test using existing accumulator API + StampVerdicts; mock the planner's LLM by directly mutating the row's contract before each Save")
}
```

> The lifecycle test is the most complex to scaffold. The implementer should look at Phase B's `internal/autoloop/lifecycle_test.go` for the pattern (driving the accumulator directly across simulated runs). Phase C extends that by also calling planner-side functions (`Evaluate`, `StampVerdicts`) between autoloop runs.

The required test scenario MUST match the comment exactly. If scaffolding the test reveals a real composition bug across L1-L5, REPORT BLOCKED with specifics.

- [ ] **Step 13.2: Run + commit**

```bash
go test ./internal/architectureplanner/ -run TestLifecycle -v
go test ./internal/progress/... ./internal/autoloop/... ./internal/architectureplanner/... ./cmd/architecture-planner-loop/...
go vet ./...
gofmt -l .
```

```bash
git add internal/architectureplanner/lifecycle_test.go
git commit -m "test(planner): end-to-end planner self-healing lifecycle"
```

---

## Self-Review Checklist

### Spec coverage

| Spec section | Implementing task |
|---|---|
| L1 Planner ledger | Task 3 (types + IO) + Task 4 (wire into RunOnce) |
| L2 Event-driven trigger (autoloop side) | Task 6 |
| L2 Event-driven trigger (planner side) | Task 7 |
| L2 systemd path unit | Task 8 |
| L3 Retry-with-feedback | Task 9 |
| L4 Self-evaluation | Task 10 |
| L5 PlannerVerdict schema | Task 1 |
| L5 PlannerVerdict stamping | Task 11 |
| L5 Autoloop selection skip + status | Task 12 |
| L6 Topical focus | Task 5 |
| Cross-cutting: symmetric preservation | Task 2 |
| Cross-cutting: trigger ledger concurrency | Task 7 (concurrent test) |
| Cross-cutting: backwards-compat round-trip with both blocks | Task 2 |
| Cross-cutting: status end-to-end | Task 12 |
| Cross-cutting: end-to-end lifecycle | Task 13 |

### Placeholder scan

- Tasks 4, 9, 12, 13 have deliberate `t.Skip("FILL IN: ...")` stubs because the existing planner test fixtures (`run_test.go`) and the lifecycle test require fixture-style scaffolding the implementer must follow rather than reinvent. The required test names and scenarios are pinned; only the exact fixture wiring is implementer discretion.
- No `TBD`, `TODO`, "implement later" markers anywhere else.
- Every task lists exact file paths.
- Every code-changing step contains the actual code.
- Every test step includes exact commands.

### Type / API consistency

Names cross-referenced across tasks:
- `progress.PlannerVerdict` (Task 1) used in Tasks 11, 12
- `progress.Item.PlannerVerdict` (Task 1) used in Tasks 2, 11, 12
- `architectureplanner.LedgerEvent` (Task 3) used in Tasks 4, 9, 10
- `architectureplanner.RowChange` (Task 3) used in Tasks 4, 11
- `architectureplanner.ProgressStats` (Task 3) used in Task 4
- `architectureplanner.AppendLedgerEvent` (Task 3) used in Tasks 4, 10
- `architectureplanner.LoadLedgerWindow` (Task 3) used in Task 10
- `architectureplanner.TriggerEvent` / `TriggerCursor` (Task 7) used in Tasks 6, 7
- `architectureplanner.AppendTriggerEvent` (Task 7) called from autoloop in Task 6 (cross-package)
- `architectureplanner.RetryFeedback` / `extractDroppedRows` / `retryAttempt` (Task 9)
- `architectureplanner.Evaluate` / `ReshapeOutcome` (Task 10) used in Tasks 11, 12
- `architectureplanner.StampVerdicts` (Task 11) used in Task 13
- `architectureplanner.matchKeywordsInDoc` / `FilterContextByKeywords` (Task 5)
- `architectureplanner.ContextBundle.QuarantinedRows` (Phase B), `.PreviousReshapes` (Task 10), `.TriggerEvents` (Task 7) — all read by `BuildPrompt`
- `autoloop.Candidate.NeedsHumanFlag` (Task 12)
- `autoloop.CandidateOptions.IncludeNeedsHuman` (Task 12)
- `autoloop.Config.IncludeNeedsHuman`, `.PlannerTriggersPath` (Tasks 6, 12)
- `architectureplanner.Config.MaxRetries`, `.EvaluationWindow`, `.EscalationThreshold`, `.AutoloopRunRoot`, `.PlannerTriggersPath`, `.TriggersCursorPath` (Tasks 4, 7, 9, 10, 11)

All names cross-reference correctly between tasks.

### Cross-cutting concerns

- **Import cycle risk:** Task 6 (autoloop emitting trigger events) imports `architectureplanner.TriggerEvent` and `AppendTriggerEvent`. Task 7 sets up the planner side. If autoloop importing planner creates a circular dependency (the planner already imports autoloop's `Runner` type and `BuildBackendCommand`), a new shared package `internal/plannertriggers` is needed. The plan flags this explicitly in Task 6 Step 6.5; the implementer must verify and pivot if needed.
- **Atomic IO patterns:** `AppendLedgerEvent` and `AppendTriggerEvent` use the same `O_APPEND|O_CREATE|O_WRONLY` pattern Phase B already uses for autoloop's runs.jsonl. POSIX-atomic for lines under 4 KiB.
- **Sticky `NeedsHuman`:** Spec invariant 3 (Section 2). Tested explicitly in Task 11.
- **Symmetric preservation:** Spec invariant 1. Tested in Task 2.
