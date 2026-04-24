# Progress Single-Source-of-Truth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `gormes/docs/content/building-gormes/architecture_plan/progress.json` the true single source of truth for Gormes roadmap progress — with automated regeneration of the README phase table, the docs.gormes.ai checklist, and the landing-page roadmap, plus a validator that fails the build on schema drift.

**Architecture:** One shared Go package (`internal/progress`) defines types, loading, derivation, validation, and rendering. A small CLI (`cmd/progress-gen`) wires the package into the Makefile, regenerating marker-bounded regions in `README.md` and `_index.md`. The landing page (a separate Go module) consumes the same package via a `replace` directive and a `go:embed` of `progress.json`, mapping status to presentation tones in a hand-maintained table.

**Tech Stack:**
- Go 1.25 (main module) / Go 1.22 (www.gormes.ai module)
- `encoding/json` for parse, `embed` for landing page bundle
- Standard `testing` package (table-driven, no testify)
- Cobra already present for CLI

**Spec:** `gormes/docs/superpowers/specs/2026-04-20-progress-single-source-of-truth-design.md`

---

## File Structure

**New files (main module):**
- `gormes/internal/progress/progress.go` — types, `Load()`, `Derive()`, `Stats()`
- `gormes/internal/progress/progress_test.go` — types + load tests
- `gormes/internal/progress/derive_test.go` — bubble-up tests (table-driven)
- `gormes/internal/progress/stats_test.go` — stats tests
- `gormes/internal/progress/validate.go` — `Validate()` function
- `gormes/internal/progress/validate_test.go` — validation tests
- `gormes/internal/progress/render.go` — `RenderReadmeRollup()`, `RenderDocsChecklist()`
- `gormes/internal/progress/render_test.go` — render tests (golden-ish, minimal)
- `gormes/internal/progress/markers.go` — marker detect + replace helpers
- `gormes/internal/progress/markers_test.go` — marker tests
- `gormes/cmd/progress-gen/main.go` — CLI entry (`-validate`, `-write`)

**New files (landing-page module):**
- `gormes/www.gormes.ai/internal/site/progress.go` — status→tone map + `buildRoadmapPhases()`
- `gormes/www.gormes.ai/internal/site/progress_test.go` — tone map tests

**Modified (main module):**
- `gormes/docs/content/building-gormes/architecture_plan/progress.json` — v2 schema migration
- `gormes/docs/content/building-gormes/architecture_plan/_index.md` — add markers, drop stats prose
- `gormes/../README.md` (root) — add markers inside `## Architecture` section
- `gormes/Makefile` — add `validate-progress` + `generate-progress` targets

**Modified (landing-page module):**
- `gormes/www.gormes.ai/go.mod` — add `require` + `replace` for main module
- `gormes/www.gormes.ai/internal/site/content.go` — replace hardcoded `RoadmapPhases` with call to `buildRoadmapPhases()`

---

## Conventions

- Module import path: `github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress`
- All tests run with `go test ./...` from `<repo>/gormes/`
- Test style: plain `*testing.T`, `t.Fatalf`/`t.Errorf`, no external assertion libs
- Commit messages: `feat(progress):`, `test(progress):`, `docs(progress):`, `build(progress):`

---

## Task 1: Scaffold `internal/progress` package with types + `Load()`

**Files:**
- Create: `gormes/internal/progress/progress.go`
- Create: `gormes/internal/progress/progress_test.go`
- Create: `gormes/internal/progress/testdata/minimal.json`

- [ ] **Step 1: Create the testdata fixture**

Create `gormes/internal/progress/testdata/minimal.json`:

```json
{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-20",
    "links": {
      "github_readme": "https://example.test/readme",
      "landing_page": "https://example.test",
      "docs_site": "https://example.test/docs",
      "source_code": "https://example.test/src"
    }
  },
  "phases": {
    "1": {
      "name": "Phase 1 — Test",
      "deliverable": "test deliverable",
      "subphases": {
        "1.A": {
          "name": "First subphase",
          "items": [
            {"name": "item one", "status": "complete"},
            {"name": "item two", "status": "planned"}
          ]
        }
      }
    }
  }
}
```

- [ ] **Step 2: Write the failing test**

Create `gormes/internal/progress/progress_test.go`:

```go
package progress

import (
	"path/filepath"
	"testing"
)

func TestLoad_MinimalFixture(t *testing.T) {
	p, err := Load(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if p.Meta.Version != "2.0" {
		t.Errorf("Meta.Version = %q, want %q", p.Meta.Version, "2.0")
	}
	if p.Meta.LastUpdated != "2026-04-20" {
		t.Errorf("Meta.LastUpdated = %q, want %q", p.Meta.LastUpdated, "2026-04-20")
	}
	ph, ok := p.Phases["1"]
	if !ok {
		t.Fatalf("Phases[\"1\"] missing")
	}
	sp, ok := ph.Subphases["1.A"]
	if !ok {
		t.Fatalf("Subphases[\"1.A\"] missing")
	}
	if len(sp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(sp.Items))
	}
	if sp.Items[0].Name != "item one" || sp.Items[0].Status != StatusComplete {
		t.Errorf("items[0] = %+v, want name=item one status=complete", sp.Items[0])
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd gormes && go test ./internal/progress/ -run TestLoad_MinimalFixture -v`
Expected: FAIL — package does not compile (`progress.go` missing).

- [ ] **Step 4: Implement the minimal code to make the test pass**

Create `gormes/internal/progress/progress.go`:

```go
// Package progress is the single source of truth for Gormes roadmap progress.
// It parses progress.json, derives phase/subphase status from items, and
// renders the canonical markdown sections consumed by README and docs.
package progress

import (
	"encoding/json"
	"fmt"
	"os"
)

type Status string

const (
	StatusComplete   Status = "complete"
	StatusInProgress Status = "in_progress"
	StatusPlanned    Status = "planned"
)

type Meta struct {
	Version     string            `json:"version"`
	LastUpdated string            `json:"last_updated"`
	Links       map[string]string `json:"links"`
}

type Item struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	// Optional, reserved, not rendered yet.
	PR    string `json:"pr,omitempty"`
	Owner string `json:"owner,omitempty"`
	ETA   string `json:"eta,omitempty"`
	Note  string `json:"note,omitempty"`
}

type Subphase struct {
	Name     string `json:"name"`
	Priority string `json:"priority,omitempty"`
	// Exactly one of Items or Status is set. Enforced by Validate.
	Items  []Item `json:"items,omitempty"`
	Status Status `json:"status,omitempty"`
}

type Phase struct {
	Name        string              `json:"name"`
	Deliverable string              `json:"deliverable"`
	// DependencyNote is a free-form string on some phases.
	DependencyNote string `json:"dependency_note,omitempty"`
	Subphases      map[string]Subphase `json:"subphases"`
}

type Progress struct {
	Meta   Meta             `json:"meta"`
	Phases map[string]Phase `json:"phases"`
}

// Load reads and parses progress.json from the given path.
func Load(path string) (*Progress, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("progress: read %s: %w", path, err)
	}
	var p Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("progress: parse %s: %w", path, err)
	}
	return &p, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd gormes && go test ./internal/progress/ -run TestLoad_MinimalFixture -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): scaffold package with types and Load

Introduces gormes/internal/progress with Progress/Phase/Subphase/Item
types and a Load() that parses progress.json v2 from disk. Covered by
a minimal fixture test.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Derive subphase status from items (bubble-up rule)

**Files:**
- Modify: `gormes/internal/progress/progress.go` — add `Subphase.DerivedStatus()`
- Create: `gormes/internal/progress/derive_test.go`

- [ ] **Step 1: Write the failing test**

Create `gormes/internal/progress/derive_test.go`:

```go
package progress

import "testing"

func TestSubphase_DerivedStatus(t *testing.T) {
	tests := []struct {
		name string
		sp   Subphase
		want Status
	}{
		{
			name: "explicit status, no items",
			sp:   Subphase{Status: StatusPlanned},
			want: StatusPlanned,
		},
		{
			name: "all items complete",
			sp: Subphase{Items: []Item{
				{Status: StatusComplete}, {Status: StatusComplete},
			}},
			want: StatusComplete,
		},
		{
			name: "any item complete -> in_progress",
			sp: Subphase{Items: []Item{
				{Status: StatusComplete}, {Status: StatusPlanned},
			}},
			want: StatusInProgress,
		},
		{
			name: "any item in_progress",
			sp: Subphase{Items: []Item{
				{Status: StatusInProgress}, {Status: StatusPlanned},
			}},
			want: StatusInProgress,
		},
		{
			name: "all planned",
			sp: Subphase{Items: []Item{
				{Status: StatusPlanned}, {Status: StatusPlanned},
			}},
			want: StatusPlanned,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.sp.DerivedStatus()
			if got != tc.want {
				t.Errorf("DerivedStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gormes && go test ./internal/progress/ -run TestSubphase_DerivedStatus -v`
Expected: FAIL — `sp.DerivedStatus undefined`.

- [ ] **Step 3: Implement the method**

Append to `gormes/internal/progress/progress.go`:

```go
// DerivedStatus computes subphase status.
// If explicit Status is set (and no items), returns it.
// Otherwise: all items complete -> complete; any complete or in_progress -> in_progress; else planned.
// Validate guarantees exactly one of Items or Status is set.
func (s Subphase) DerivedStatus() Status {
	if len(s.Items) == 0 {
		return s.Status
	}
	allComplete := true
	anyStarted := false
	for _, it := range s.Items {
		if it.Status != StatusComplete {
			allComplete = false
		}
		if it.Status == StatusComplete || it.Status == StatusInProgress {
			anyStarted = true
		}
	}
	switch {
	case allComplete:
		return StatusComplete
	case anyStarted:
		return StatusInProgress
	default:
		return StatusPlanned
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gormes && go test ./internal/progress/ -run TestSubphase_DerivedStatus -v`
Expected: PASS (5 subtests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): derive subphase status from items

Adds Subphase.DerivedStatus() implementing the bubble-up rule: all
items complete -> complete; any complete/in_progress -> in_progress;
else planned. Falls back to explicit Status when no items.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Derive phase status from subphases

**Files:**
- Modify: `gormes/internal/progress/progress.go` — add `Phase.DerivedStatus()`
- Modify: `gormes/internal/progress/derive_test.go` — add phase-level tests

- [ ] **Step 1: Extend the test file**

Append to `gormes/internal/progress/derive_test.go`:

```go
func TestPhase_DerivedStatus(t *testing.T) {
	tests := []struct {
		name string
		ph   Phase
		want Status
	}{
		{
			name: "all subphases complete",
			ph: Phase{Subphases: map[string]Subphase{
				"A": {Items: []Item{{Status: StatusComplete}}},
				"B": {Status: StatusComplete},
			}},
			want: StatusComplete,
		},
		{
			name: "mix of complete and planned",
			ph: Phase{Subphases: map[string]Subphase{
				"A": {Items: []Item{{Status: StatusComplete}}},
				"B": {Status: StatusPlanned},
			}},
			want: StatusInProgress,
		},
		{
			name: "all planned",
			ph: Phase{Subphases: map[string]Subphase{
				"A": {Status: StatusPlanned},
				"B": {Status: StatusPlanned},
			}},
			want: StatusPlanned,
		},
		{
			name: "no subphases",
			ph:   Phase{Subphases: map[string]Subphase{}},
			want: StatusPlanned,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ph.DerivedStatus()
			if got != tc.want {
				t.Errorf("DerivedStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gormes && go test ./internal/progress/ -run TestPhase_DerivedStatus -v`
Expected: FAIL — `ph.DerivedStatus undefined`.

- [ ] **Step 3: Implement the method**

Append to `gormes/internal/progress/progress.go`:

```go
// DerivedStatus computes phase status from subphases. Empty phase -> planned.
func (p Phase) DerivedStatus() Status {
	if len(p.Subphases) == 0 {
		return StatusPlanned
	}
	allComplete := true
	anyStarted := false
	for _, sp := range p.Subphases {
		st := sp.DerivedStatus()
		if st != StatusComplete {
			allComplete = false
		}
		if st == StatusComplete || st == StatusInProgress {
			anyStarted = true
		}
	}
	switch {
	case allComplete:
		return StatusComplete
	case anyStarted:
		return StatusInProgress
	default:
		return StatusPlanned
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gormes && go test ./internal/progress/ -run TestPhase_DerivedStatus -v`
Expected: PASS (4 subtests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): derive phase status from subphases

Phase.DerivedStatus() applies the same bubble-up rule across its
subphases. Empty phase is treated as planned.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Compute stats

**Files:**
- Modify: `gormes/internal/progress/progress.go` — add `Stats` struct + `(*Progress).Stats()`
- Create: `gormes/internal/progress/stats_test.go`

- [ ] **Step 1: Write the failing test**

Create `gormes/internal/progress/stats_test.go`:

```go
package progress

import "testing"

func TestStats_SingleSubphase(t *testing.T) {
	p := &Progress{
		Phases: map[string]Phase{
			"1": {
				Subphases: map[string]Subphase{
					"1.A": {Items: []Item{{Status: StatusComplete}, {Status: StatusPlanned}}},
				},
			},
		},
	}
	s := p.Stats()
	if s.Subphases.Total != 1 {
		t.Errorf("Subphases.Total = %d, want 1", s.Subphases.Total)
	}
	if s.Subphases.InProgress != 1 {
		t.Errorf("Subphases.InProgress = %d, want 1", s.Subphases.InProgress)
	}
	if s.Items.Total != 2 || s.Items.Complete != 1 || s.Items.Planned != 1 {
		t.Errorf("Items = %+v, want total=2 complete=1 planned=1", s.Items)
	}
}

func TestStats_MixedPhases(t *testing.T) {
	p := &Progress{
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Status: StatusComplete}, {Status: StatusComplete}}},
				"1.B": {Items: []Item{{Status: StatusComplete}}},
			}},
			"2": {Subphases: map[string]Subphase{
				"2.A": {Status: StatusPlanned},
				"2.B": {Items: []Item{{Status: StatusInProgress}}},
			}},
		},
	}
	s := p.Stats()
	if s.Subphases.Total != 4 {
		t.Errorf("Subphases.Total = %d, want 4", s.Subphases.Total)
	}
	if s.Subphases.Complete != 2 {
		t.Errorf("Subphases.Complete = %d, want 2", s.Subphases.Complete)
	}
	if s.Subphases.InProgress != 1 {
		t.Errorf("Subphases.InProgress = %d, want 1", s.Subphases.InProgress)
	}
	if s.Subphases.Planned != 1 {
		t.Errorf("Subphases.Planned = %d, want 1", s.Subphases.Planned)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gormes && go test ./internal/progress/ -run TestStats -v`
Expected: FAIL — `p.Stats undefined`.

- [ ] **Step 3: Implement stats**

Append to `gormes/internal/progress/progress.go`:

```go
type Counts struct {
	Total      int
	Complete   int
	InProgress int
	Planned    int
}

type Stats struct {
	Phases    Counts
	Subphases Counts
	Items     Counts
}

// Stats walks all phases/subphases/items and tallies derived status.
// Computed on demand — never stored in progress.json.
func (p *Progress) Stats() Stats {
	var s Stats
	for _, ph := range p.Phases {
		s.Phases.Total++
		tally(&s.Phases, ph.DerivedStatus())
		for _, sp := range ph.Subphases {
			s.Subphases.Total++
			tally(&s.Subphases, sp.DerivedStatus())
			for _, it := range sp.Items {
				s.Items.Total++
				tally(&s.Items, it.Status)
			}
		}
	}
	return s
}

func tally(c *Counts, st Status) {
	switch st {
	case StatusComplete:
		c.Complete++
	case StatusInProgress:
		c.InProgress++
	case StatusPlanned:
		c.Planned++
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd gormes && go test ./internal/progress/ -run TestStats -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): compute stats from derived status

Progress.Stats() tallies Phases, Subphases, and Items from the bubble-
up result. Counts are computed on demand — nothing is persisted in
progress.json so stats cannot drift.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Validate schema

**Files:**
- Create: `gormes/internal/progress/validate.go`
- Create: `gormes/internal/progress/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `gormes/internal/progress/validate_test.go`:

```go
package progress

import (
	"strings"
	"testing"
)

func TestValidate_OK(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "x", Status: StatusComplete}}},
				"1.B": {Status: StatusPlanned},
			}},
		},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsBadVersion(t *testing.T) {
	p := &Progress{Meta: Meta{Version: "1.0"}}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Validate() = %v, want version error", err)
	}
}

func TestValidate_RejectsBadStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: "done"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("Validate() = %v, want status error", err)
	}
}

func TestValidate_RejectsBothItemsAndStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {
				Items:  []Item{{Name: "x", Status: StatusComplete}},
				Status: StatusComplete,
			},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("Validate() = %v, want exactly-one error", err)
	}
}

func TestValidate_RejectsNeither(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {}, // no items, no status
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("Validate() = %v, want exactly-one error", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd gormes && go test ./internal/progress/ -run TestValidate -v`
Expected: FAIL — `Validate undefined`.

- [ ] **Step 3: Implement validation**

Create `gormes/internal/progress/validate.go`:

```go
package progress

import "fmt"

// Validate enforces schema invariants:
//   - meta.version is "2.0"
//   - every item.status is in {complete, in_progress, planned}
//   - every subphase has exactly one of items or an explicit status
//   - every explicit subphase status is in the allowed set
func Validate(p *Progress) error {
	if p.Meta.Version != "2.0" {
		return fmt.Errorf("progress: meta.version = %q, want %q", p.Meta.Version, "2.0")
	}
	for phKey, ph := range p.Phases {
		for spKey, sp := range ph.Subphases {
			hasItems := len(sp.Items) > 0
			hasStatus := sp.Status != ""
			if hasItems == hasStatus { // both true or both false
				return fmt.Errorf("progress: phase %s subphase %s must have exactly one of items or status", phKey, spKey)
			}
			if hasStatus && !validStatus(sp.Status) {
				return fmt.Errorf("progress: phase %s subphase %s: invalid status %q", phKey, spKey, sp.Status)
			}
			for i, it := range sp.Items {
				if !validStatus(it.Status) {
					return fmt.Errorf("progress: phase %s subphase %s item[%d] (%q): invalid status %q",
						phKey, spKey, i, it.Name, it.Status)
				}
			}
		}
	}
	return nil
}

func validStatus(s Status) bool {
	return s == StatusComplete || s == StatusInProgress || s == StatusPlanned
}
```

- [ ] **Step 4: Run tests**

Run: `cd gormes && go test ./internal/progress/ -run TestValidate -v`
Expected: PASS (5 subtests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): validate v2 schema

Validate() enforces meta.version == 2.0, the exactly-one rule
(items XOR explicit status) on every subphase, and the allowed
status set on both items and subphase overrides.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Migrate `progress.json` to v2 schema

**Files:**
- Modify: `gormes/docs/content/building-gormes/architecture_plan/progress.json`

This task has no code tests — the invariant is that the real file parses + validates.

- [ ] **Step 1: Rewrite the file**

Overwrite `gormes/docs/content/building-gormes/architecture_plan/progress.json` with the v2 structure below. The rule applied to every subphase from the old file: if the old subphase was `complete`, every item becomes `complete`; if `in_progress`, the items listed below are marked individually based on current Phase 3 reality; if `planned`, every item becomes `planned`. Old `stats`, `status` (where items exist), and `shipped`/`ongoing` arrays at the phase level are dropped.

```json
{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-20",
    "links": {
      "github_readme": "https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md",
      "landing_page": "https://gormes.ai",
      "docs_site": "https://docs.gormes.ai/building-gormes/architecture_plan/",
      "source_code": "https://github.com/TrebuchetDynamics/gormes-agent"
    }
  },
  "phases": {
    "1": {
      "name": "Phase 1 — The Dashboard",
      "deliverable": "Tactical bridge: Go TUI over Python's api_server HTTP+SSE boundary",
      "subphases": {
        "1.A": {
          "name": "Core TUI",
          "items": [
            { "name": "Bubble Tea shell", "status": "complete" },
            { "name": "16ms coalescing mailbox", "status": "complete" },
            { "name": "SSE reconnect", "status": "complete" }
          ]
        },
        "1.B": {
          "name": "Wire Doctor",
          "items": [
            { "name": "Offline tool validation", "status": "complete" }
          ]
        }
      }
    },
    "2": {
      "name": "Phase 2 — The Gateway",
      "deliverable": "Go-native tools + Telegram + session resume + wider adapters",
      "subphases": {
        "2.A": {
          "name": "Tool Registry",
          "items": [
            { "name": "In-process Go tool registry", "status": "complete" },
            { "name": "Streamed tool_calls accumulation", "status": "complete" },
            { "name": "Kernel tool loop", "status": "complete" },
            { "name": "Doctor verification", "status": "complete" }
          ]
        },
        "2.B.1": {
          "name": "Telegram Scout",
          "items": [
            { "name": "Telegram adapter", "status": "complete" },
            { "name": "Long-poll ingress", "status": "complete" },
            { "name": "Edit coalescing", "status": "complete" }
          ]
        },
        "2.C": {
          "name": "Thin Mapping Persistence",
          "items": [
            { "name": "bbolt session resume", "status": "complete" },
            { "name": "(platform, chat_id) -> session_id", "status": "complete" }
          ]
        },
        "2.D": {
          "name": "Cron / Scheduled Automations",
          "items": [
            { "name": "Go ticker + bbolt job store", "status": "planned" },
            { "name": "Natural-language cron parsing (Phase 4)", "status": "planned" }
          ]
        },
        "2.B.2": {
          "name": "Wider Gateway Surface",
          "items": [
            { "name": "Discord", "status": "planned" },
            { "name": "Slack", "status": "planned" },
            { "name": "WhatsApp", "status": "planned" },
            { "name": "Signal", "status": "planned" },
            { "name": "Email", "status": "planned" },
            { "name": "SMS", "status": "planned" }
          ]
        },
        "2.E": {
          "name": "Subagent System",
          "priority": "P0",
          "items": [
            { "name": "Execution isolation", "status": "planned" },
            { "name": "Resource boundaries", "status": "planned" },
            { "name": "Context isolation", "status": "planned" },
            { "name": "Cancellation scopes", "status": "planned" }
          ]
        },
        "2.F": {
          "name": "Hooks + Lifecycle",
          "items": [
            { "name": "Per-event extension points", "status": "planned" },
            { "name": "Managed restarts", "status": "planned" }
          ]
        },
        "2.G": {
          "name": "Skills System",
          "priority": "P0",
          "items": [
            { "name": "Learning loop foundation", "status": "planned" },
            { "name": "Pattern extraction", "status": "planned" }
          ]
        }
      }
    },
    "3": {
      "name": "Phase 3 — The Black Box (Memory)",
      "deliverable": "SQLite + FTS5 + ontological graph + semantic fusion in Go",
      "subphases": {
        "3.A": {
          "name": "SQLite + FTS5 Lattice",
          "items": [
            { "name": "SqliteStore", "status": "complete" },
            { "name": "FTS5 triggers", "status": "complete" },
            { "name": "Schema migrations v3a->v3d", "status": "complete" }
          ]
        },
        "3.B": {
          "name": "Ontological Graph + LLM Extractor",
          "items": [
            { "name": "Extractor", "status": "complete" },
            { "name": "Entity/relationship upsert", "status": "complete" },
            { "name": "Dead-letter queue", "status": "complete" }
          ]
        },
        "3.C": {
          "name": "Neural Recall + Context Injection",
          "items": [
            { "name": "RecallProvider", "status": "complete" },
            { "name": "2-layer seed selection", "status": "complete" },
            { "name": "CTE traversal", "status": "complete" },
            { "name": "<memory-context> fence", "status": "complete" }
          ]
        },
        "3.D": {
          "name": "Semantic Fusion + Local Embeddings",
          "items": [
            { "name": "Ollama embeddings", "status": "complete" },
            { "name": "Vector cache", "status": "complete" },
            { "name": "Cosine similarity recall", "status": "complete" },
            { "name": "Hybrid fusion", "status": "complete" }
          ]
        },
        "3.D.5": {
          "name": "Memory Mirror (USER.md sync)",
          "items": [
            { "name": "Async background export", "status": "complete" },
            { "name": "SQLite as source of truth", "status": "complete" }
          ]
        },
        "3.E.1": {
          "name": "Session Index Mirror",
          "items": [
            { "name": "bbolt sessions.yaml export", "status": "planned" }
          ]
        },
        "3.E.2": {
          "name": "Tool Execution Audit Log",
          "items": [
            { "name": "JSONL audit trail", "status": "planned" }
          ]
        },
        "3.E.3": {
          "name": "Transcript Export Command",
          "items": [
            { "name": "Markdown export", "status": "planned" }
          ]
        },
        "3.E.4": {
          "name": "Extraction State Visibility",
          "items": [
            { "name": "gormes memory status", "status": "planned" }
          ]
        },
        "3.E.5": {
          "name": "Insights Audit Log",
          "items": [
            { "name": "Usage JSONL", "status": "planned" }
          ]
        },
        "3.E.6": {
          "name": "Memory Decay",
          "items": [
            { "name": "Weight attenuation", "status": "planned" },
            { "name": "last_seen tracking", "status": "planned" }
          ]
        },
        "3.E.7": {
          "name": "Cross-Chat Synthesis",
          "items": [
            { "name": "Graph unification across chats", "status": "planned" }
          ]
        }
      }
    },
    "4": {
      "name": "Phase 4 — The Brain Transplant",
      "deliverable": "Native Go agent orchestrator + prompt builder",
      "dependency_note": "Build priority: Skills (2.G) -> Subagents (2.E) -> Gateway -> Native Agent Loop",
      "subphases": {
        "4.A": {
          "name": "Provider Adapters",
          "items": [
            { "name": "Anthropic", "status": "planned" },
            { "name": "Bedrock", "status": "planned" },
            { "name": "Gemini", "status": "planned" },
            { "name": "OpenRouter", "status": "planned" },
            { "name": "Google Code Assist", "status": "planned" },
            { "name": "Codex", "status": "planned" }
          ]
        },
        "4.B": { "name": "Context Engine + Compression", "items": [
          { "name": "Long session management", "status": "planned" },
          { "name": "Context compression", "status": "planned" }
        ]},
        "4.C": { "name": "Native Prompt Builder", "items": [
          { "name": "System + memory + tools + history assembly", "status": "planned" }
        ]},
        "4.D": { "name": "Smart Model Routing", "items": [
          { "name": "Per-turn model selection", "status": "planned" }
        ]},
        "4.E": { "name": "Trajectory + Insights", "items": [
          { "name": "Self-monitoring telemetry", "status": "planned" }
        ]},
        "4.F": { "name": "Title Generation", "items": [
          { "name": "Auto-naming sessions", "status": "planned" }
        ]},
        "4.G": { "name": "Credentials + OAuth", "items": [
          { "name": "Token vault", "status": "planned" },
          { "name": "Multi-account auth", "status": "planned" }
        ]},
        "4.H": { "name": "Rate / Retry / Caching", "items": [
          { "name": "Provider-side resilience", "status": "planned" }
        ]}
      }
    },
    "5": {
      "name": "Phase 5 — The Final Purge",
      "deliverable": "Python tool scripts ported to Go or WASM",
      "subphases": {
        "5.A": { "name": "Tool Surface Port", "items": [
          { "name": "61-tool registry port", "status": "planned" }
        ]},
        "5.B": { "name": "Sandboxing Backends", "items": [
          { "name": "Docker", "status": "planned" },
          { "name": "Modal", "status": "planned" },
          { "name": "Daytona", "status": "planned" },
          { "name": "Singularity", "status": "planned" }
        ]},
        "5.C": { "name": "Browser Automation", "items": [
          { "name": "Chromedp", "status": "planned" },
          { "name": "Rod", "status": "planned" }
        ]},
        "5.D": { "name": "Vision + Image Generation", "items": [
          { "name": "Multimodal in/out", "status": "planned" }
        ]},
        "5.E": { "name": "TTS / Voice / Transcription", "items": [
          { "name": "Voice mode port", "status": "planned" }
        ]},
        "5.F": { "name": "Skills System (Remaining)", "items": [
          { "name": "Skills hub", "status": "planned" },
          { "name": "Skill registries", "status": "planned" }
        ]},
        "5.G": { "name": "MCP Integration", "items": [
          { "name": "MCP client", "status": "planned" },
          { "name": "OAuth flows", "status": "planned" }
        ]},
        "5.H": { "name": "ACP Integration", "items": [
          { "name": "ACP server side", "status": "planned" }
        ]},
        "5.I": { "name": "Plugins Architecture", "items": [
          { "name": "Plugin SDK", "status": "planned" },
          { "name": "Third-party extensions", "status": "planned" }
        ]},
        "5.J": { "name": "Approval / Security Guards", "items": [
          { "name": "Dangerous action gating", "status": "planned" }
        ]},
        "5.K": { "name": "Code Execution", "items": [
          { "name": "Sandboxed exec", "status": "planned" }
        ]},
        "5.L": { "name": "File Ops + Patches", "items": [
          { "name": "Atomic checkpoints", "status": "planned" }
        ]},
        "5.M": { "name": "Mixture of Agents", "items": [
          { "name": "Multi-model coordination", "status": "planned" }
        ]},
        "5.N": { "name": "Misc Operator Tools", "items": [
          { "name": "Todo", "status": "planned" },
          { "name": "Clarify", "status": "planned" },
          { "name": "Session search", "status": "planned" },
          { "name": "Debug helpers", "status": "planned" }
        ]},
        "5.O": { "name": "Hermes CLI Parity", "items": [
          { "name": "49-file CLI tree port", "status": "planned" }
        ]},
        "5.P": { "name": "Docker / Packaging", "items": [
          { "name": "OCI image", "status": "planned" },
          { "name": "Homebrew", "status": "planned" }
        ]},
        "5.Q": { "name": "TUI Gateway Streaming", "items": [
          { "name": "SSE streaming to Bubble Tea TUI", "status": "planned" }
        ]}
      }
    },
    "6": {
      "name": "Phase 6 — The Learning Loop (Soul)",
      "deliverable": "Native skill extraction. Compounding intelligence. The feature Hermes doesn't have.",
      "subphases": {
        "6.A": { "name": "Complexity Detector", "items": [
          { "name": "Heuristic or LLM-scored signal", "status": "planned" }
        ]},
        "6.B": { "name": "Skill Extractor", "items": [
          { "name": "LLM-assisted pattern distillation", "status": "planned" }
        ]},
        "6.C": { "name": "Skill Storage Format", "items": [
          { "name": "Portable SKILL.md format", "status": "planned" }
        ]},
        "6.D": { "name": "Skill Retrieval + Matching", "items": [
          { "name": "Hybrid lexical + semantic lookup", "status": "planned" }
        ]},
        "6.E": { "name": "Feedback Loop", "items": [
          { "name": "Skill effectiveness scoring", "status": "planned" }
        ]},
        "6.F": { "name": "Skill Surface", "items": [
          { "name": "TUI + Telegram browsing", "status": "planned" }
        ]}
      }
    }
  }
}
```

- [ ] **Step 2: Write a sanity test that real file parses + validates**

Append to `gormes/internal/progress/progress_test.go`:

```go
func TestLoad_RealFile(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	// Phase 1 must derive as complete (all subphases complete).
	if got := p.Phases["1"].DerivedStatus(); got != StatusComplete {
		t.Errorf("Phase 1 = %q, want complete", got)
	}
	// Phase 2 has one subphase (2.A, 2.B.1, 2.C) complete and more planned -> in_progress.
	if got := p.Phases["2"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 2 = %q, want in_progress", got)
	}
	// Phase 4 is entirely planned.
	if got := p.Phases["4"].DerivedStatus(); got != StatusPlanned {
		t.Errorf("Phase 4 = %q, want planned", got)
	}
}
```

- [ ] **Step 3: Run the test**

Run: `cd gormes && go test ./internal/progress/ -run TestLoad_RealFile -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd gormes && git add internal/progress/ docs/content/building-gormes/architecture_plan/progress.json
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): migrate progress.json to v2 schema

Explicit item objects with per-item status. Per-subphase/phase status
fields removed where items exist. Top-level stats block dropped —
computed on demand by progress.Stats(). Real file is validated by a
new sanity test.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Render README rollup table

**Files:**
- Create: `gormes/internal/progress/render.go`
- Create: `gormes/internal/progress/render_test.go`

- [ ] **Step 1: Write the failing test**

Create `gormes/internal/progress/render_test.go`:

```go
package progress

import (
	"strings"
	"testing"
)

func TestRenderReadmeRollup_Shape(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "Phase 1 — Dashboard", Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Status: StatusComplete}}},
			}},
			"2": {Name: "Phase 2 — Gateway", Subphases: map[string]Subphase{
				"2.A": {Items: []Item{{Status: StatusComplete}}},
				"2.B": {Items: []Item{{Status: StatusPlanned}}},
			}},
		},
	}
	got := RenderReadmeRollup(p)
	// Table has a header row
	if !strings.Contains(got, "| Phase | Status | Shipped |") {
		t.Errorf("rollup missing table header; got:\n%s", got)
	}
	// Phase 1 is complete, renders with check icon and 1/1
	if !strings.Contains(got, "Phase 1 — Dashboard") {
		t.Errorf("rollup missing Phase 1 row; got:\n%s", got)
	}
	if !strings.Contains(got, "1/1") {
		t.Errorf("rollup missing 1/1 count for Phase 1; got:\n%s", got)
	}
	// Phase 2 is in_progress -> 1/2
	if !strings.Contains(got, "1/2") {
		t.Errorf("rollup missing 1/2 count for Phase 2; got:\n%s", got)
	}
}

func TestRenderReadmeRollup_Sorted(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"2": {Name: "Phase 2", Subphases: map[string]Subphase{"2.A": {Status: StatusPlanned}}},
			"1": {Name: "Phase 1", Subphases: map[string]Subphase{"1.A": {Status: StatusComplete}}},
		},
	}
	got := RenderReadmeRollup(p)
	i1 := strings.Index(got, "Phase 1")
	i2 := strings.Index(got, "Phase 2")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("phases not sorted (i1=%d, i2=%d):\n%s", i1, i2, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gormes && go test ./internal/progress/ -run TestRenderReadmeRollup -v`
Expected: FAIL — `RenderReadmeRollup undefined`.

- [ ] **Step 3: Implement the renderer**

Create `gormes/internal/progress/render.go`:

```go
package progress

import (
	"fmt"
	"sort"
	"strings"
)

// statusIcon maps derived status to the glyph shown in markdown tables.
func statusIcon(s Status) string {
	switch s {
	case StatusComplete:
		return "✅"
	case StatusInProgress:
		return "🔨"
	default:
		return "⏳"
	}
}

// RenderReadmeRollup returns the 6-row phase table inserted into the
// README's `## Architecture` section between the PROGRESS markers.
func RenderReadmeRollup(p *Progress) string {
	var b strings.Builder
	b.WriteString("| Phase | Status | Shipped |\n")
	b.WriteString("|-------|--------|---------|\n")
	for _, key := range sortedKeys(p.Phases) {
		ph := p.Phases[key]
		total := len(ph.Subphases)
		complete := 0
		for _, sp := range ph.Subphases {
			if sp.DerivedStatus() == StatusComplete {
				complete++
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d/%d subphases |\n",
			ph.Name, statusIcon(ph.DerivedStatus()), complete, total)
	}
	return b.String()
}

func sortedKeys(m map[string]Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `cd gormes && go test ./internal/progress/ -run TestRenderReadmeRollup -v`
Expected: PASS (2 subtests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): render README rollup table

RenderReadmeRollup() produces a 6-row phase table with derived status
icon and N/M subphase counts, sorted by phase key. Consumed by the
readme-rollup marker region in README.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Render docs full checklist

**Files:**
- Modify: `gormes/internal/progress/render.go` — add `RenderDocsChecklist`
- Modify: `gormes/internal/progress/render_test.go` — add checklist tests

- [ ] **Step 1: Extend the test file**

Append to `gormes/internal/progress/render_test.go`:

```go
func TestRenderDocsChecklist_StatsLine(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "P1", Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "done", Status: StatusComplete}}},
				"1.B": {Items: []Item{{Name: "todo", Status: StatusPlanned}}},
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "**Overall:** 1/2 subphases shipped") {
		t.Errorf("checklist missing overall stats line; got:\n%s", got)
	}
}

func TestRenderDocsChecklist_ItemCheckboxes(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "Phase 1 — Test", Subphases: map[string]Subphase{
				"1.A": {Name: "Alpha", Items: []Item{
					{Name: "done", Status: StatusComplete},
					{Name: "todo", Status: StatusPlanned},
				}},
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "- [x] done") {
		t.Errorf("checklist missing checked item; got:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] todo") {
		t.Errorf("checklist missing unchecked item; got:\n%s", got)
	}
	if !strings.Contains(got, "### 1.A — Alpha") {
		t.Errorf("checklist missing subphase header; got:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd gormes && go test ./internal/progress/ -run TestRenderDocsChecklist -v`
Expected: FAIL — `RenderDocsChecklist undefined`.

- [ ] **Step 3: Implement the renderer**

Append to `gormes/internal/progress/render.go`:

```go
// RenderDocsChecklist returns the full item-level checklist embedded
// in _index.md between the PROGRESS markers. Emits:
//   - an **Overall** stats line
//   - a phase-level table matching the README rollup
//   - a per-subphase section with - [x] / - [ ] checkboxes
func RenderDocsChecklist(p *Progress) string {
	s := p.Stats()
	var b strings.Builder

	fmt.Fprintf(&b, "**Overall:** %d/%d subphases shipped · %d in progress · %d planned\n\n",
		s.Subphases.Complete, s.Subphases.Total, s.Subphases.InProgress, s.Subphases.Planned)

	b.WriteString(RenderReadmeRollup(p))
	b.WriteString("\n---\n\n")

	for _, key := range sortedKeys(p.Phases) {
		ph := p.Phases[key]
		fmt.Fprintf(&b, "## %s %s\n\n", ph.Name, statusIcon(ph.DerivedStatus()))
		if ph.Deliverable != "" {
			fmt.Fprintf(&b, "*%s*\n\n", ph.Deliverable)
		}
		for _, spKey := range sortedSubKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			fmt.Fprintf(&b, "### %s — %s %s\n\n", spKey, sp.Name, statusIcon(sp.DerivedStatus()))
			if len(sp.Items) == 0 {
				fmt.Fprintf(&b, "*(no item breakdown — tracked at subphase level: %s)*\n\n", sp.Status)
				continue
			}
			for _, it := range sp.Items {
				box := "[ ]"
				if it.Status == StatusComplete {
					box = "[x]"
				}
				fmt.Fprintf(&b, "- %s %s\n", box, it.Name)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sortedSubKeys(m map[string]Subphase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `cd gormes && go test ./internal/progress/ -v`
Expected: PASS (all tests — render + earlier suites).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): render docs-full-checklist

RenderDocsChecklist() emits the overall stats line, the phase rollup,
and a per-subphase section with [ ]/[x] checkboxes for each item. This
is the body dropped between PROGRESS markers in _index.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Marker detect + replace helpers

**Files:**
- Create: `gormes/internal/progress/markers.go`
- Create: `gormes/internal/progress/markers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `gormes/internal/progress/markers_test.go`:

```go
package progress

import (
	"strings"
	"testing"
)

func TestReplaceMarker_HappyPath(t *testing.T) {
	input := strings.Join([]string{
		"intro",
		"<!-- PROGRESS:START kind=readme-rollup -->",
		"STALE CONTENT",
		"<!-- PROGRESS:END -->",
		"outro",
	}, "\n")
	out, err := ReplaceMarker(input, "readme-rollup", "NEW CONTENT\n")
	if err != nil {
		t.Fatalf("ReplaceMarker() error = %v", err)
	}
	if !strings.Contains(out, "NEW CONTENT") {
		t.Errorf("output missing new content:\n%s", out)
	}
	if strings.Contains(out, "STALE CONTENT") {
		t.Errorf("output still contains stale content:\n%s", out)
	}
	if !strings.Contains(out, "intro") || !strings.Contains(out, "outro") {
		t.Errorf("output missing surrounding prose:\n%s", out)
	}
}

func TestReplaceMarker_MissingMarkers(t *testing.T) {
	_, err := ReplaceMarker("no markers here", "readme-rollup", "x")
	if err == nil {
		t.Errorf("want error on missing markers, got nil")
	}
}

func TestReplaceMarker_WrongKind(t *testing.T) {
	input := "<!-- PROGRESS:START kind=other -->\nx\n<!-- PROGRESS:END -->"
	_, err := ReplaceMarker(input, "readme-rollup", "x")
	if err == nil {
		t.Errorf("want error on wrong kind, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd gormes && go test ./internal/progress/ -run TestReplaceMarker -v`
Expected: FAIL — `ReplaceMarker undefined`.

- [ ] **Step 3: Implement the helper**

Create `gormes/internal/progress/markers.go`:

```go
package progress

import (
	"fmt"
	"regexp"
)

// startMarker matches <!-- PROGRESS:START kind=<name> --> allowing flexible spacing.
var startMarker = regexp.MustCompile(`<!--\s*PROGRESS:START\s+kind=([a-zA-Z0-9_-]+)\s*-->`)

const endMarker = "<!-- PROGRESS:END -->"

// ReplaceMarker replaces the content between PROGRESS:START kind=<kind>
// and PROGRESS:END with the supplied body. The markers themselves are
// preserved. Returns an error if the markers are missing, unbalanced,
// or the start marker's kind does not match.
func ReplaceMarker(input, kind, body string) (string, error) {
	m := startMarker.FindStringIndex(input)
	if m == nil {
		return "", fmt.Errorf("progress: start marker not found")
	}
	start := m[0]
	bodyStart := m[1]
	kindMatch := startMarker.FindStringSubmatch(input[start:bodyStart])
	if len(kindMatch) < 2 || kindMatch[1] != kind {
		return "", fmt.Errorf("progress: expected kind=%q, found %q", kind, kindMatch[1])
	}
	endIdx := indexAfter(input, bodyStart, endMarker)
	if endIdx < 0 {
		return "", fmt.Errorf("progress: end marker not found after start")
	}
	endStop := endIdx + len(endMarker)
	return input[:bodyStart] + "\n" + body + input[endIdx:endStop] + input[endStop:], nil
}

func indexAfter(s string, off int, sub string) int {
	i := indexFrom(s[off:], sub)
	if i < 0 {
		return -1
	}
	return off + i
}

func indexFrom(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Run tests**

Run: `cd gormes && go test ./internal/progress/ -run TestReplaceMarker -v`
Expected: PASS (3 subtests).

- [ ] **Step 5: Commit**

```bash
cd gormes && git add internal/progress/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress): marker detect + replace helpers

ReplaceMarker() swaps the body inside PROGRESS:START kind=...
/ PROGRESS:END, preserving surrounding prose and the markers
themselves. Fails loudly on missing or mismatched markers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `progress-gen` CLI — validate + write modes

**Files:**
- Create: `gormes/cmd/progress-gen/main.go`

This task has no unit tests — the CLI is a thin glue binary exercised by the Makefile. Integration is smoke-tested manually.

- [ ] **Step 1: Create the CLI**

Create `gormes/cmd/progress-gen/main.go`:

```go
// Command progress-gen validates progress.json and regenerates the
// marker-bounded regions in README.md and the architecture_plan
// _index.md. Run from the main Makefile.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

func main() {
	validate := flag.Bool("validate", false, "validate progress.json only (read-only)")
	write := flag.Bool("write", false, "regenerate marker regions in README.md and _index.md")
	flag.Parse()

	if !*validate && !*write {
		fmt.Fprintln(os.Stderr, "progress-gen: pass -validate or -write")
		os.Exit(2)
	}

	// Resolve paths relative to the main module root (this binary is
	// invoked from gormes/ by the Makefile).
	root, err := os.Getwd()
	if err != nil {
		die(err)
	}
	progressPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	readmePath := filepath.Join(root, "..", "README.md")
	docsIndexPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "_index.md")

	p, err := progress.Load(progressPath)
	if err != nil {
		die(err)
	}
	if err := progress.Validate(p); err != nil {
		die(err)
	}

	if *validate {
		fmt.Printf("progress-gen: validated %d phases\n", len(p.Phases))
		return
	}

	if err := rewrite(readmePath, "readme-rollup", progress.RenderReadmeRollup(p)); err != nil {
		die(err)
	}
	if err := rewrite(docsIndexPath, "docs-full-checklist", progress.RenderDocsChecklist(p)); err != nil {
		die(err)
	}
	fmt.Println("progress-gen: README.md and _index.md regenerated")
}

func rewrite(path, kind, body string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	out, err := progress.ReplaceMarker(string(b), kind, body)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "progress-gen:", err)
	os.Exit(1)
}
```

- [ ] **Step 2: Verify `-validate` mode runs against the real file**

Run: `cd gormes && go run ./cmd/progress-gen -validate`
Expected: `progress-gen: validated 6 phases`

- [ ] **Step 3: Commit (write mode not yet exercised — no markers yet)**

```bash
cd gormes && git add cmd/progress-gen/
cd .. && git commit -m "$(cat <<'EOF'
feat(progress-gen): validate + write CLI

cmd/progress-gen wraps progress.Load + Validate and, in -write mode,
rewrites the marker-bounded regions in README.md and _index.md via
progress.ReplaceMarker. Invoked from the Makefile.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Add markers to `_index.md` + first render

**Files:**
- Modify: `gormes/docs/content/building-gormes/architecture_plan/_index.md`

- [ ] **Step 1: Rewrite `_index.md` with markers**

Overwrite `gormes/docs/content/building-gormes/architecture_plan/_index.md`:

```markdown
---
title: "Architecture Plan"
weight: 10
---

# Gormes — Executive Roadmap

**Single source of truth:** [`progress.json`](progress.json) — machine-readable, validated + regenerated on build.

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + rollup phase table
- [Landing page](https://gormes.ai) — Marketing + roadmap section
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

---

## Progress

<!-- PROGRESS:START kind=docs-full-checklist -->
(generated — `make build`)
<!-- PROGRESS:END -->

---

## Data Format

[`progress.json`](progress.json) is the machine-readable source of truth. Top-level structure:

- `meta` — schema version, last-updated timestamp, canonical URLs
- `phases` — six phases keyed `"1"`..`"6"`, each containing `subphases`
- each subphase carries either `items` (the normal case) or an explicit `status`

Stats (complete/in-progress/planned counts) are **not stored** — they are computed on render. Updated automatically on `make build`.
```

- [ ] **Step 2: Run the write generator**

Run: `cd gormes && go run ./cmd/progress-gen -write`
Expected: `progress-gen: README.md and _index.md regenerated` — and yes, it will fail at `README.md` because its markers don't exist yet. The command must update `_index.md` BEFORE it tries `README.md`. That's why `rewrite()` is called in that order in Task 10's main. **Expect one failure on README.md:** `progress-gen: .../README.md: progress: start marker not found`. `_index.md` should still be updated before the failure. Open `_index.md` and confirm the PROGRESS block is populated.

- [ ] **Step 3: Commit**

```bash
cd gormes && git add docs/content/building-gormes/architecture_plan/_index.md
cd .. && git commit -m "$(cat <<'EOF'
docs(progress): add PROGRESS markers to _index.md + first render

_index.md now frames progress.json as the source of truth and delegates
the checklist body to progress-gen via PROGRESS:START/END markers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Add markers to `README.md` + first render

**Files:**
- Modify: `README.md` (root)

- [ ] **Step 1: Replace the hand-written architecture phase table with a marker block**

Edit the root `README.md`. Find the `## Architecture` section — specifically the block from `Gormes is not a wrapper around Hermes.` down through the end of the phase table (including the line `Each phase ships standalone value while moving the full stack toward pure Go.`). Replace the whole thing with:

```markdown
## Architecture

Gormes is not a wrapper around Hermes. It is a **strangler fig rewrite** — each phase ships standalone value while moving the full stack toward pure Go.

<!-- PROGRESS:START kind=readme-rollup -->
(generated — `make build`)
<!-- PROGRESS:END -->

Full item-level checklist and stats: **[docs.gormes.ai/building-gormes/architecture_plan](https://docs.gormes.ai/building-gormes/architecture_plan/)**
```

- [ ] **Step 2: Run the write generator (now it should succeed for both files)**

Run: `cd gormes && go run ./cmd/progress-gen -write`
Expected: `progress-gen: README.md and _index.md regenerated`

- [ ] **Step 3: Inspect output**

Run: `head -80 README.md | tail -40`
Expected: the `## Architecture` section shows the generated 6-row phase rollup table.

- [ ] **Step 4: Commit**

```bash
git add README.md gormes/docs/content/building-gormes/architecture_plan/_index.md
git commit -m "$(cat <<'EOF'
docs(progress): wire README Architecture table to progress.json

Replace hand-maintained phase table with PROGRESS marker block. The
table is now regenerated by progress-gen on every make build, so the
drift bug (8/52 vs. actual counts) is eliminated.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Wire landing page (www.gormes.ai) to `internal/progress`

**Files:**
- Modify: `gormes/www.gormes.ai/go.mod` — add require + replace
- Create: `gormes/www.gormes.ai/internal/site/progress.go`
- Create: `gormes/www.gormes.ai/internal/site/progress_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/content.go` — swap `RoadmapPhases` to `buildRoadmapPhases()`

- [ ] **Step 1: Add require + replace directives to the landing-page module**

Append to `gormes/www.gormes.ai/go.mod`:

```
require github.com/TrebuchetDynamics/gormes-agent/gormes v0.0.0

replace github.com/TrebuchetDynamics/gormes-agent/gormes => ../
```

- [ ] **Step 2: Write the failing test for tone mapping**

Create `gormes/www.gormes.ai/internal/site/progress_test.go`:

```go
package site

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

func TestToneFor(t *testing.T) {
	tests := []struct {
		st   progress.Status
		key  string
		want string
	}{
		{progress.StatusComplete, "1", "shipped"},
		{progress.StatusInProgress, "2", "progress"},
		{progress.StatusPlanned, "4", "planned"},
		{progress.StatusPlanned, "5", "later"}, // Phase 5 is a "later" tone even when planned
		{progress.StatusPlanned, "6", "planned"},
	}
	for _, tc := range tests {
		got := toneFor(tc.st, tc.key)
		if got != tc.want {
			t.Errorf("toneFor(%q, phase=%s) = %q, want %q", tc.st, tc.key, got, tc.want)
		}
	}
}

func TestBuildRoadmapPhases_Counts(t *testing.T) {
	p := &progress.Progress{
		Meta: progress.Meta{Version: "2.0"},
		Phases: map[string]progress.Phase{
			"1": {Name: "Phase 1 — Dashboard", Subphases: map[string]progress.Subphase{
				"1.A": {Items: []progress.Item{{Status: progress.StatusComplete}}},
				"1.B": {Items: []progress.Item{{Status: progress.StatusComplete}}},
			}},
		},
	}
	got := buildRoadmapPhases(p)
	if len(got) != 1 {
		t.Fatalf("len(phases) = %d, want 1", len(got))
	}
	if got[0].StatusTone != "shipped" {
		t.Errorf("StatusTone = %q, want shipped", got[0].StatusTone)
	}
	if got[0].Title != "Phase 1 — Dashboard" {
		t.Errorf("Title = %q", got[0].Title)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd gormes/www.gormes.ai && go test ./internal/site -run TestToneFor -v`
Expected: FAIL — `toneFor undefined` (and `buildRoadmapPhases undefined`).

- [ ] **Step 4: Implement the presentation helpers**

Create `gormes/www.gormes.ai/internal/site/progress.go`:

```go
package site

import (
	"embed"
	"fmt"
	"html/template"
	"sort"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

//go:embed data/progress.json
var progressFS embed.FS

// loadEmbeddedProgress decodes the progress.json embedded at build time.
// If the embed is missing or invalid, the landing page falls back to an
// empty roadmap rather than refusing to build.
func loadEmbeddedProgress() *progress.Progress {
	b, err := progressFS.ReadFile("data/progress.json")
	if err != nil {
		return nil
	}
	var p progress.Progress
	if err := jsonUnmarshalProgress(b, &p); err != nil {
		return nil
	}
	return &p
}

// toneFor returns the CSS-class suffix used by .roadmap-status-<tone>.
//   - complete  -> "shipped"
//   - in_progress -> "progress"
//   - planned: phase "5" -> "later" (per design), otherwise "planned"
func toneFor(st progress.Status, phaseKey string) string {
	switch st {
	case progress.StatusComplete:
		return "shipped"
	case progress.StatusInProgress:
		return "progress"
	case progress.StatusPlanned:
		if phaseKey == "5" {
			return "later"
		}
		return "planned"
	}
	return "planned"
}

// itemIconFor maps item status to the glyph shown on the landing page.
func itemIconFor(st progress.Status) (icon, tone string) {
	switch st {
	case progress.StatusComplete:
		return "✓", "shipped"
	case progress.StatusInProgress:
		return "◌", "ongoing"
	default:
		return "⏳", "pending"
	}
}

// buildRoadmapPhases turns the progress.json model into the
// []RoadmapPhase slice consumed by the landing-page template.
func buildRoadmapPhases(p *progress.Progress) []RoadmapPhase {
	if p == nil {
		return nil
	}
	keys := sortPhaseKeys(p.Phases)
	out := make([]RoadmapPhase, 0, len(keys))
	for _, key := range keys {
		ph := p.Phases[key]
		items := buildItems(ph)
		total := len(ph.Subphases)
		complete := 0
		for _, sp := range ph.Subphases {
			if sp.DerivedStatus() == progress.StatusComplete {
				complete++
			}
		}
		out = append(out, RoadmapPhase{
			StatusLabel: statusLabelFor(ph.DerivedStatus(), complete, total),
			StatusTone:  toneFor(ph.DerivedStatus(), key),
			Title:       ph.Name,
			Items:       items,
		})
	}
	return out
}

func buildItems(ph progress.Phase) []RoadmapItem {
	subKeys := make([]string, 0, len(ph.Subphases))
	for k := range ph.Subphases {
		subKeys = append(subKeys, k)
	}
	sort.Strings(subKeys)

	items := make([]RoadmapItem, 0)
	for _, spKey := range subKeys {
		sp := ph.Subphases[spKey]
		icon, tone := itemIconFor(sp.DerivedStatus())
		items = append(items, RoadmapItem{
			Icon:  icon,
			Tone:  tone,
			Label: template.HTML(fmt.Sprintf("%s %s", spKey, sp.Name)),
		})
	}
	return items
}

func statusLabelFor(st progress.Status, complete, total int) string {
	switch st {
	case progress.StatusComplete:
		return fmt.Sprintf("SHIPPED · %d/%d", complete, total)
	case progress.StatusInProgress:
		return fmt.Sprintf("IN PROGRESS · %d/%d", complete, total)
	default:
		return fmt.Sprintf("PLANNED · 0/%d", total)
	}
}

func sortPhaseKeys(m map[string]progress.Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 5: Simplify `loadEmbeddedProgress` to call `json.Unmarshal` directly**

Edit the import block and body of `gormes/www.gormes.ai/internal/site/progress.go`. The final import block and loader:

```go
import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

//go:embed data/progress.json
var progressFS embed.FS

func loadEmbeddedProgress() *progress.Progress {
	b, err := progressFS.ReadFile("data/progress.json")
	if err != nil {
		return nil
	}
	var p progress.Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	return &p
}
```

Delete the `jsonUnmarshalProgress` and `jsonDecode` helpers shown in Step 4 — they were scaffolding and are no longer needed.

- [ ] **Step 6: Create the embed source file**

The landing-page module cannot `//go:embed` a path outside its directory. Copy the file into the module's `internal/site/data/` at build time. For now, add a symlink so development stays in sync:

Run:
```bash
mkdir -p gormes/www.gormes.ai/internal/site/data
ln -sf ../../../../docs/content/building-gormes/architecture_plan/progress.json \
       gormes/www.gormes.ai/internal/site/data/progress.json
```

- [ ] **Step 7: Wire `content.go` to call `buildRoadmapPhases`**

In `gormes/www.gormes.ai/internal/site/content.go`, replace the hand-written `RoadmapPhases: []RoadmapPhase{ ... }` slice and the surrounding `ProgressTracker`/`ProgressTrackerURL` lines (line 157 through line 238) with:

```go
			RoadmapLabel:    "§ 03 · SHIPPING STATE",
			RoadmapHeadline: "What ships now, what doesn't.",
			ProgressTracker: progressTrackerLabel(),
			ProgressTrackerURL: "https://docs.gormes.ai/building-gormes/architecture_plan/",
			RoadmapPhases:   buildRoadmapPhases(loadEmbeddedProgress()),
```

Add this helper to `gormes/www.gormes.ai/internal/site/progress.go`:

```go
// progressTrackerLabel returns the "N/M shipped" headline text shown
// above the roadmap. Falls back to an empty string if the embed fails.
func progressTrackerLabel() string {
	p := loadEmbeddedProgress()
	if p == nil {
		return ""
	}
	s := p.Stats()
	return fmt.Sprintf("%d/%d shipped", s.Subphases.Complete, s.Subphases.Total)
}
```

- [ ] **Step 8: Run `go mod tidy` in the landing-page module**

Run: `cd gormes/www.gormes.ai && go mod tidy`
Expected: exit 0, `go.sum` updated if needed.

- [ ] **Step 9: Run all landing-page tests**

Run: `cd gormes/www.gormes.ai && go test ./...`
Expected: PASS — including the new `progress_test.go` and the existing `render_test.go` / `server_smoke_test.go`.

- [ ] **Step 10: Commit**

```bash
cd gormes/www.gormes.ai && git add go.mod go.sum internal/site/
cd ../.. && git commit -m "$(cat <<'EOF'
feat(www): landing page reads progress.json via go:embed

Adds a replace directive so the www.gormes.ai module can import the
gormes/internal/progress package, plus a presentation layer
(buildRoadmapPhases, toneFor, statusLabelFor) that maps derived
status to the existing RoadmapPhase tones. Hand-written roadmap
slice + drifted 8/52 counter removed — both now computed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Wire Makefile

**Files:**
- Modify: `gormes/Makefile`

- [ ] **Step 1: Add validate + generate targets**

Overwrite `gormes/Makefile` with:

```make
.PHONY: build run test test-live lint fmt clean update-readme validate-progress generate-progress

BUILD_FLAGS := -trimpath -ldflags="-s -w"
BINARY_PATH := bin/gormes

build: validate-progress $(BINARY_PATH)
	@$(call record-benchmark)
	@$(call record-progress)
	$(MAKE) -s generate-progress
	@$(call update-readme)

$(BINARY_PATH):
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_PATH) ./cmd/gormes

validate-progress:
	@echo "Validating progress.json..."
	@go run ./cmd/progress-gen -validate

generate-progress:
	@echo "Regenerating progress-driven markdown..."
	@go run ./cmd/progress-gen -write

define record-benchmark
	@echo "Recording benchmark..."
	@bash scripts/record-benchmark.sh
endef

define update-readme
	@echo "Updating README.md..."
	@bash scripts/update-readme.sh
endef

define record-progress
	@echo "Updating progress..."
	@bash scripts/record-progress.sh
endef

update-readme:
	@$(call update-readme)

run: build
	./bin/gormes

test:
	go test ./...

test-live:
	go test -tags=live ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -rf bin/ coverage.out
```

Key changes vs. current Makefile: `validate-progress` is a new prereq of `build` (so a broken `progress.json` fails the build before the binary is produced); `generate-progress` is invoked as a recursive make target in the build recipe (not wrapped in `$(call ...)`, to avoid the self-recursion pitfall); `.PHONY` extended to list both new targets.

- [ ] **Step 2: Run a clean build to exercise the whole pipeline**

Run: `cd gormes && make clean && make build`
Expected output includes:
- `Validating progress.json...`
- `progress-gen: validated 6 phases`
- `Recording benchmark...`
- `Updating progress...`
- `Regenerating progress-driven markdown...`
- `progress-gen: README.md and _index.md regenerated`
- `Updating README.md...`
- binary produced at `bin/gormes`

- [ ] **Step 3: Verify `git status` reflects only expected diffs**

Run: `git status`
Expected: clean tree apart from `benchmarks.json` auto-update and `progress.json` `last_updated` touch (both expected from the build hooks).

- [ ] **Step 4: Commit**

```bash
git add gormes/Makefile
git commit -m "$(cat <<'EOF'
build(progress): run progress-gen from make build

make build now validates progress.json as a prereq and regenerates the
marker regions in README.md and _index.md before the README benchmark
touch-up runs. A broken progress.json fails the build.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

**Spec coverage check:**
- v2 schema → Task 1 (types), Task 6 (migration)
- Bubble-up rule → Tasks 2, 3
- Stats computation (no longer stored) → Task 4, migration in Task 6 drops stats block
- Validation → Task 5
- Renderers (rollup + full checklist) → Tasks 7, 8
- Marker convention → Task 9, consumed by Task 10 CLI
- README wiring → Task 12
- `_index.md` wiring → Task 11
- Landing page wiring via go:embed → Task 13
- Makefile wiring → Task 14
- Out-of-scope items (CLI to mark items, ETAs/owners rendering, phase narrative files) → not touched, as specified.

**Placeholder scan:** Task 13 Step 5 originally contained a bit of hand-waving around the `jsonUnmarshalProgress` wrapper; Step 5's final paragraph now explicitly directs the implementer to consolidate imports and delete the wrapper. No other TBDs.

**Type consistency:** `Status`, `Progress`, `Phase`, `Subphase`, `Item`, `Stats`, `Counts`, `RenderReadmeRollup`, `RenderDocsChecklist`, `ReplaceMarker`, `Load`, `Validate`, `DerivedStatus` — all match between the types task (Task 1), the derivation tasks (2, 3), the validation task (5), the rendering tasks (7, 8), the marker task (9), and the CLI (10). Landing-page `toneFor`, `itemIconFor`, `statusLabelFor`, `buildRoadmapPhases`, `progressTrackerLabel`, `loadEmbeddedProgress` all match between Task 13's test and implementation steps.
