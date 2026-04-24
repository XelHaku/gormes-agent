# Autoloop Control Plane Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `cmd/autoloop` easier and more effective by organizing execution-control docs under a focused autoloop section and passing full progress-row metadata to worker agents.

**Architecture:** Keep `docs/content/building-gormes/architecture_plan/progress.json` as the canonical queue for this pass. Move generated execution docs under `docs/content/building-gormes/autoloop/`, then teach `internal/autoloop` to select the same worker-ready rows that generated `agent-queue.md` shows. Autoloop continues reading structured JSON directly; generated markdown remains the human mirror.

**Tech Stack:** Go, Hugo content front matter, `cmd/progress-gen`, `internal/progress`, `internal/autoloop`, `go test`, `make validate-progress`, `make generate-progress`.

---

Run all commands from the repository root
`/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`. Before starting,
check `git status --short` and preserve unrelated unstaged user changes.

## File Structure

- Create: `docs/content/building-gormes/autoloop/_index.md` section landing page.
- Move: `docs/content/building-gormes/autoloop-handoff.md` to `docs/content/building-gormes/autoloop/autoloop-handoff.md`.
- Move: `docs/content/building-gormes/agent-queue.md` to `docs/content/building-gormes/autoloop/agent-queue.md`.
- Move: `docs/content/building-gormes/next-slices.md` to `docs/content/building-gormes/autoloop/next-slices.md`.
- Move: `docs/content/building-gormes/blocked-slices.md` to `docs/content/building-gormes/autoloop/blocked-slices.md`.
- Move: `docs/content/building-gormes/umbrella-cleanup.md` to `docs/content/building-gormes/autoloop/umbrella-cleanup.md`.
- Move: `docs/content/building-gormes/progress-schema.md` to `docs/content/building-gormes/autoloop/progress-schema.md`.
- Modify: `cmd/progress-gen/main.go` so generated execution pages target the new section.
- Modify: `docs/content/building-gormes/architecture_plan/progress.json` `meta.autoloop` paths.
- Modify: `docs/content/building-gormes/_index.md` and `cmd/autoloop/README.md` links.
- Modify: `docs/docs_test.go`, `docs/build_test.go`, and failing docs tests found by `go test ./docs`.
- Modify: `internal/progress/render.go` and `internal/progress/render_test.go` generated path text.
- Modify: `internal/autoloop/candidates.go` and `internal/autoloop/candidates_test.go` candidate metadata and selection.
- Modify: `internal/autoloop/run.go`, `internal/autoloop/run_test.go`, `cmd/autoloop/main.go`, and `cmd/autoloop/main_test.go` prompts and dry-run output.

### Task 1: Move Execution Docs Into An Autoloop Section

**Files:**
- Create: `docs/content/building-gormes/autoloop/_index.md`
- Move: `docs/content/building-gormes/autoloop-handoff.md`
- Move: `docs/content/building-gormes/agent-queue.md`
- Move: `docs/content/building-gormes/next-slices.md`
- Move: `docs/content/building-gormes/blocked-slices.md`
- Move: `docs/content/building-gormes/umbrella-cleanup.md`
- Move: `docs/content/building-gormes/progress-schema.md`
- Modify: `docs/docs_test.go`
- Modify: `docs/build_test.go`

- [ ] **Step 1: Update docs coverage tests first**

In `docs/docs_test.go`, replace these `nativeHugoPages` entries:

```go
	"building-gormes/autoloop-handoff.md":                            {},
	"building-gormes/agent-queue.md":                                 {},
	"building-gormes/next-slices.md":                                 {},
	"building-gormes/blocked-slices.md":                              {},
	"building-gormes/umbrella-cleanup.md":                            {},
	"building-gormes/progress-schema.md":                             {},
```

with:

```go
	"building-gormes/autoloop/_index.md":                              {},
	"building-gormes/autoloop/autoloop-handoff.md":                    {},
	"building-gormes/autoloop/agent-queue.md":                         {},
	"building-gormes/autoloop/next-slices.md":                         {},
	"building-gormes/autoloop/blocked-slices.md":                      {},
	"building-gormes/autoloop/umbrella-cleanup.md":                    {},
	"building-gormes/autoloop/progress-schema.md":                     {},
```

In `docs/build_test.go`, replace these `wantPages` entries:

```go
		"building-gormes/contract-readiness/index.html",
		"building-gormes/autoloop-handoff/index.html",
		"building-gormes/agent-queue/index.html",
		"building-gormes/next-slices/index.html",
		"building-gormes/blocked-slices/index.html",
		"building-gormes/umbrella-cleanup/index.html",
		"building-gormes/progress-schema/index.html",
```

with:

```go
		"building-gormes/contract-readiness/index.html",
		"building-gormes/autoloop/index.html",
		"building-gormes/autoloop/autoloop-handoff/index.html",
		"building-gormes/autoloop/agent-queue/index.html",
		"building-gormes/autoloop/next-slices/index.html",
		"building-gormes/autoloop/blocked-slices/index.html",
		"building-gormes/autoloop/umbrella-cleanup/index.html",
		"building-gormes/autoloop/progress-schema/index.html",
```

Add alias expectations after the new paths in the same `wantPages` slice:

```go
		"building-gormes/autoloop-handoff/index.html",
		"building-gormes/agent-queue/index.html",
		"building-gormes/next-slices/index.html",
		"building-gormes/blocked-slices/index.html",
		"building-gormes/umbrella-cleanup/index.html",
		"building-gormes/progress-schema/index.html",
```

- [ ] **Step 2: Run tests to verify they fail before the move**

Run:

```bash
go test ./docs -run 'TestMirroredDocsCoverage|TestHugoBuild' -count=1
```

Expected: FAIL. `TestMirroredDocsCoverage` should report the old top-level execution docs as unexpected or the new autoloop files as missing. `TestHugoBuild` may also report missing `building-gormes/autoloop/...` pages.

- [ ] **Step 3: Move files and create the section index**

Run:

```bash
mkdir -p docs/content/building-gormes/autoloop
git mv docs/content/building-gormes/autoloop-handoff.md docs/content/building-gormes/autoloop/autoloop-handoff.md
git mv docs/content/building-gormes/agent-queue.md docs/content/building-gormes/autoloop/agent-queue.md
git mv docs/content/building-gormes/next-slices.md docs/content/building-gormes/autoloop/next-slices.md
git mv docs/content/building-gormes/blocked-slices.md docs/content/building-gormes/autoloop/blocked-slices.md
git mv docs/content/building-gormes/umbrella-cleanup.md docs/content/building-gormes/autoloop/umbrella-cleanup.md
git mv docs/content/building-gormes/progress-schema.md docs/content/building-gormes/autoloop/progress-schema.md
```

Create `docs/content/building-gormes/autoloop/_index.md`:

```markdown
---
title: "Autoloop Control Plane"
weight: 30
---

# Autoloop Control Plane

Autoloop is the unattended execution control plane for the building-gormes
roadmap. These pages mirror the structured rows in
`docs/content/building-gormes/architecture_plan/progress.json` so operators,
contributors, and worker agents use the same queue.

## Start Here

- [Autoloop Handoff](./autoloop-handoff/) explains the shared entrypoint, queue
  source, generated docs, tests, and candidate policy.
- [Agent Queue](./agent-queue/) lists rows that are ready for autonomous worker
  execution.
- [Next Slices](./next-slices/) shows the short ranking of high-leverage work.
- [Blocked Slices](./blocked-slices/) keeps blocked rows visible without making
  them assignable.
- [Umbrella Cleanup](./umbrella-cleanup/) lists broad rows that need to be split
  before assignment.
- [Progress Schema](./progress-schema/) defines the row fields autoloop expects.
```

- [ ] **Step 4: Add Hugo aliases to moved pages**

Set the front matter in `docs/content/building-gormes/autoloop/autoloop-handoff.md` to:

```markdown
---
title: "Autoloop Handoff"
weight: 10
aliases:
  - /building-gormes/autoloop-handoff/
---
```

Set the front matter in `docs/content/building-gormes/autoloop/agent-queue.md` to:

```markdown
---
title: "Agent Queue"
weight: 20
aliases:
  - /building-gormes/agent-queue/
---
```

Set the front matter in `docs/content/building-gormes/autoloop/next-slices.md` to:

```markdown
---
title: "Next Slices"
weight: 30
aliases:
  - /building-gormes/next-slices/
---
```

Set the front matter in `docs/content/building-gormes/autoloop/blocked-slices.md` to:

```markdown
---
title: "Blocked Slices"
weight: 40
aliases:
  - /building-gormes/blocked-slices/
---
```

Set the front matter in `docs/content/building-gormes/autoloop/umbrella-cleanup.md` to:

```markdown
---
title: "Umbrella Cleanup"
weight: 50
aliases:
  - /building-gormes/umbrella-cleanup/
---
```

Set the front matter in `docs/content/building-gormes/autoloop/progress-schema.md` to:

```markdown
---
title: "Progress Schema"
weight: 60
aliases:
  - /building-gormes/progress-schema/
---
```

- [ ] **Step 5: Run docs tests**

Run:

```bash
go test ./docs -run 'TestMirroredDocsCoverage|TestHugoBuild' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the docs move**

Run:

```bash
git add docs/content/building-gormes/autoloop docs/docs_test.go docs/build_test.go
git add -u docs/content/building-gormes
git commit -m "docs: group autoloop control plane pages"
```

### Task 2: Point Progress Generation At The New Section

**Files:**
- Modify: `cmd/progress-gen/main.go`
- Modify: `internal/progress/render.go`
- Modify: `internal/progress/render_test.go`
- Modify: `docs/content/building-gormes/architecture_plan/progress.json`
- Modify: `docs/content/building-gormes/autoloop/autoloop-handoff.md`
- Modify: `docs/content/building-gormes/autoloop/agent-queue.md`
- Modify: `docs/content/building-gormes/autoloop/next-slices.md`
- Modify: `docs/content/building-gormes/autoloop/blocked-slices.md`
- Modify: `docs/content/building-gormes/autoloop/umbrella-cleanup.md`
- Modify: `docs/content/building-gormes/autoloop/progress-schema.md`

- [ ] **Step 1: Update progress render tests first**

In `internal/progress/render_test.go`, update `TestRenderAutoloopHandoff` to use new generated docs paths:

```go
func TestRenderAutoloopHandoff(t *testing.T) {
	p := &Progress{Meta: Meta{Autoloop: AutoloopMeta{
		Entrypoint:      "scripts/gormes-auto-codexu-orchestrator.sh",
		Plan:            "docs/superpowers/plans/plan.md",
		AgentQueue:      "docs/content/building-gormes/autoloop/agent-queue.md",
		ProgressSchema:  "docs/content/building-gormes/autoloop/progress-schema.md",
		CandidateSource: "docs/content/building-gormes/architecture_plan/progress.json",
		UnitTest:        "go test ./internal/autoloop ./cmd/autoloop -count=1",
		CandidatePolicy: []string{"Skip blocked rows.", "Skip umbrella rows."},
	}}}

	got := RenderAutoloopHandoff(p)
	for _, want := range []string{
		"## Control Plane",
		"- Entrypoint: `scripts/gormes-auto-codexu-orchestrator.sh`",
		"- Plan: `docs/superpowers/plans/plan.md`",
		"- Candidate source: `docs/content/building-gormes/architecture_plan/progress.json`",
		"- Agent queue: `docs/content/building-gormes/autoloop/agent-queue.md`",
		"- Progress schema: `docs/content/building-gormes/autoloop/progress-schema.md`",
		"- Unit tests: `go test ./internal/autoloop ./cmd/autoloop -count=1`",
		"## Candidate Policy",
		"- Skip blocked rows.",
		"- Skip umbrella rows.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("autoloop handoff missing %q:\n%s", want, got)
		}
	}
}
```

In `TestRenderProgressSchema`, add these expected strings to the `want` list:

```go
		"`docs/content/building-gormes/autoloop/autoloop-handoff.md`",
		"`docs/content/building-gormes/autoloop/agent-queue.md`",
		"`docs/content/building-gormes/autoloop/blocked-slices.md`",
		"`docs/content/building-gormes/autoloop/umbrella-cleanup.md`",
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/progress -run 'TestRenderAutoloopHandoff|TestRenderProgressSchema' -count=1
```

Expected: FAIL because `RenderProgressSchema` still names the old top-level generated pages.

- [ ] **Step 3: Update progress generator target paths**

In `cmd/progress-gen/main.go`, replace the individual top-level execution page path declarations with:

```go
	autoloopDir := filepath.Join(root, "docs", "content", "building-gormes", "autoloop")
	autoloopHandoffPath := filepath.Join(autoloopDir, "autoloop-handoff.md")
	agentQueuePath := filepath.Join(autoloopDir, "agent-queue.md")
	nextSlicesPath := filepath.Join(autoloopDir, "next-slices.md")
	blockedSlicesPath := filepath.Join(autoloopDir, "blocked-slices.md")
	umbrellaCleanupPath := filepath.Join(autoloopDir, "umbrella-cleanup.md")
	progressSchemaPath := filepath.Join(autoloopDir, "progress-schema.md")
```

Keep these existing paths unchanged:

```go
	progressPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	readmePath := filepath.Join(root, "README.md")
	docsIndexPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "_index.md")
	contractReadinessPath := filepath.Join(root, "docs", "content", "building-gormes", "contract-readiness.md")
	siteProgressPath := filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json")
```

- [ ] **Step 4: Update generated schema text**

In `internal/progress/render.go`, replace the `Generated Agent Surfaces` section inside `RenderProgressSchema` with:

```go
## Generated Agent Surfaces

- `+"`docs/content/building-gormes/autoloop/autoloop-handoff.md`"+` lists shared unattended-loop entrypoint, plan, candidate source, generated docs, test command, and candidate policy.
- `+"`docs/content/building-gormes/autoloop/agent-queue.md`"+` lists only unblocked, non-umbrella contract rows with owner, size, readiness, degraded mode, fixture, write scope, test commands, done signal, acceptance, and source references.
- `+"`docs/content/building-gormes/autoloop/blocked-slices.md`"+` keeps blocked rows out of the execution queue while preserving their unblock condition.
- `+"`docs/content/building-gormes/autoloop/umbrella-cleanup.md`"+` lists broad inventory rows that must be split before assignment.
```

- [ ] **Step 5: Update canonical autoloop metadata paths**

In `docs/content/building-gormes/architecture_plan/progress.json`, update `meta.autoloop` path values to:

```json
      "agent_queue": "docs/content/building-gormes/autoloop/agent-queue.md",
      "progress_schema": "docs/content/building-gormes/autoloop/progress-schema.md",
      "candidate_source": "docs/content/building-gormes/architecture_plan/progress.json",
```

Keep `candidate_source` unchanged.

- [ ] **Step 6: Run render tests**

Run:

```bash
go test ./internal/progress ./cmd/progress-gen -count=1
```

Expected: PASS.

- [ ] **Step 7: Regenerate progress-driven markdown**

Run:

```bash
make generate-progress
```

Expected output includes:

```text
Regenerating progress-driven markdown...
progress-gen: autoloop handoff regenerated
progress-gen: agent queue regenerated
progress-gen: next slices regenerated
progress-gen: blocked slices regenerated
progress-gen: umbrella cleanup regenerated
progress-gen: progress schema regenerated
progress-gen: site progress data refreshed
```

- [ ] **Step 8: Commit generator path changes**

Run:

```bash
git add cmd/progress-gen/main.go internal/progress/render.go internal/progress/render_test.go
git add docs/content/building-gormes/architecture_plan/progress.json
git add docs/content/building-gormes/autoloop
git add www.gormes.ai/internal/site/data/progress.json
git commit -m "docs: generate autoloop control plane pages"
```

### Task 3: Preserve Progress Row Metadata In Autoloop Candidates

**Files:**
- Modify: `internal/autoloop/candidates.go`
- Modify: `internal/autoloop/candidates_test.go`

- [ ] **Step 1: Add candidate metadata tests**

Append these tests to `internal/autoloop/candidates_test.go` before `writeProgressJSON`:

```go
func TestNormalizeCandidatesPreservesExecutionMetadataAndSkipsBlockedUmbrella(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"7": {
				"subphases": {
					"7.A": {
						"items": [
							{
								"name": "ready candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "Provider-neutral transcript contract",
								"contract_status": "fixture_ready",
								"slice_size": "medium",
								"execution_owner": "provider",
								"trust_class": ["system"],
								"degraded_mode": "provider status reports missing fixtures",
								"fixture": "internal/hermes/testdata/provider_transcripts",
								"source_refs": ["docs/content/upstream-hermes/source-study.md"],
								"ready_when": ["fixtures replay"],
								"not_ready_when": ["live provider call required"],
								"acceptance": ["go test ./internal/hermes passes"],
								"write_scope": ["internal/hermes/"],
								"test_commands": ["go test ./internal/hermes -count=1"],
								"done_signal": ["provider transcript replay passes"],
								"unblocks": ["Bedrock adapter"],
								"note": "Use captured transcript fixtures."
							},
							{
								"name": "blocked candidate",
								"status": "planned",
								"contract": "blocked",
								"slice_size": "small",
								"blocked_by": ["ready candidate"],
								"ready_when": ["ready candidate completes"]
							},
							{
								"name": "umbrella candidate",
								"status": "planned",
								"contract": "umbrella",
								"slice_size": "umbrella"
							}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}
	if gotLen := len(got); gotLen != 1 {
		t.Fatalf("NormalizeCandidates() length = %d, want 1: %#v", gotLen, got)
	}

	candidate := got[0]
	if candidate.ItemName != "ready candidate" {
		t.Fatalf("ItemName = %q, want ready candidate", candidate.ItemName)
	}
	for _, want := range []string{
		candidate.Priority,
		candidate.Contract,
		candidate.ContractStatus,
		candidate.SliceSize,
		candidate.ExecutionOwner,
		candidate.DegradedMode,
		candidate.Fixture,
		candidate.Note,
	} {
		if strings.TrimSpace(want) == "" {
			t.Fatalf("candidate lost scalar metadata: %#v", candidate)
		}
	}
	if !reflect.DeepEqual(candidate.TrustClass, []string{"system"}) {
		t.Fatalf("TrustClass = %#v, want system", candidate.TrustClass)
	}
	if !reflect.DeepEqual(candidate.WriteScope, []string{"internal/hermes/"}) {
		t.Fatalf("WriteScope = %#v, want internal/hermes/", candidate.WriteScope)
	}
	if !reflect.DeepEqual(candidate.TestCommands, []string{"go test ./internal/hermes -count=1"}) {
		t.Fatalf("TestCommands = %#v, want provider test", candidate.TestCommands)
	}
	if got, want := candidate.SelectionReason(), "P0 handoff"; got != want {
		t.Fatalf("SelectionReason() = %q, want %q", got, want)
	}
}

func TestNormalizeCandidatesUsesExecutionBuckets(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"8": {
				"subphases": {
					"8.A": {
						"items": [
							{"name": "draft candidate", "status": "planned", "contract": "draft", "contract_status": "draft", "slice_size": "small"},
							{"name": "fixture candidate", "status": "planned", "contract": "fixture", "contract_status": "fixture_ready", "slice_size": "small"},
							{"name": "active candidate", "status": "in_progress", "contract": "active", "contract_status": "draft", "slice_size": "small"},
							{"name": "p0 candidate", "status": "planned", "priority": "P0", "contract": "p0", "contract_status": "draft", "slice_size": "small"},
							{"name": "unblocking candidate", "status": "planned", "contract": "unblock", "contract_status": "missing", "slice_size": "small", "unblocks": ["next row"]}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	var names []string
	for _, candidate := range got {
		names = append(names, candidate.ItemName)
	}
	want := []string{
		"p0 candidate",
		"active candidate",
		"fixture candidate",
		"unblocking candidate",
		"draft candidate",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("candidate order = %#v, want %#v", names, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestNormalizeCandidatesPreservesExecutionMetadataAndSkipsBlockedUmbrella|TestNormalizeCandidatesUsesExecutionBuckets' -count=1
```

Expected: FAIL because `Candidate` does not have the metadata fields or `SelectionReason`.

- [ ] **Step 3: Enrich candidate structs and parsing**

In `internal/autoloop/candidates.go`, replace `CandidateOptions`, `Candidate`, and `progressItem` with:

```go
type CandidateOptions struct {
	ActiveFirst     bool
	PriorityBoost   []string
	IncludeBlocked  bool
	IncludeUmbrella bool
}

type Candidate struct {
	PhaseID        string
	SubphaseID     string
	ItemName       string
	Status         string
	Priority       string
	Contract       string
	ContractStatus string
	SliceSize      string
	ExecutionOwner string
	TrustClass     []string
	DegradedMode   string
	Fixture        string
	SourceRefs     []string
	BlockedBy      []string
	Unblocks       []string
	ReadyWhen      []string
	NotReadyWhen   []string
	Acceptance     []string
	WriteScope     []string
	TestCommands   []string
	DoneSignal     []string
	Note           string
}
```

```go
type progressItem struct {
	ItemName       string   `json:"item_name"`
	Name           string   `json:"name"`
	Title          string   `json:"title"`
	ID             string   `json:"id"`
	Status         string   `json:"status"`
	Priority       string   `json:"priority"`
	Contract       string   `json:"contract"`
	ContractStatus string   `json:"contract_status"`
	SliceSize      string   `json:"slice_size"`
	ExecutionOwner string   `json:"execution_owner"`
	TrustClass     []string `json:"trust_class"`
	DegradedMode   string   `json:"degraded_mode"`
	Fixture        string   `json:"fixture"`
	SourceRefs     []string `json:"source_refs"`
	BlockedBy      []string `json:"blocked_by"`
	Unblocks       []string `json:"unblocks"`
	ReadyWhen      []string `json:"ready_when"`
	NotReadyWhen   []string `json:"not_ready_when"`
	Acceptance     []string `json:"acceptance"`
	WriteScope     []string `json:"write_scope"`
	TestCommands   []string `json:"test_commands"`
	DoneSignal     []string `json:"done_signal"`
	Note           string   `json:"note"`
}
```

In `NormalizeCandidates`, after status normalization and before constructing the candidate, add:

```go
				if len(item.BlockedBy) > 0 && !opts.IncludeBlocked {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(item.SliceSize), "umbrella") && !opts.IncludeUmbrella {
					continue
				}
```

Construct candidates with:

```go
				candidate := Candidate{
					PhaseID:        strings.TrimSpace(phase.ID),
					SubphaseID:     strings.TrimSpace(subphase.ID),
					ItemName:       name,
					Status:         status,
					Priority:       strings.TrimSpace(item.Priority),
					Contract:       strings.TrimSpace(item.Contract),
					ContractStatus: strings.ToLower(strings.TrimSpace(item.ContractStatus)),
					SliceSize:      strings.ToLower(strings.TrimSpace(item.SliceSize)),
					ExecutionOwner: strings.ToLower(strings.TrimSpace(item.ExecutionOwner)),
					TrustClass:     trimStringSlice(item.TrustClass),
					DegradedMode:   strings.TrimSpace(item.DegradedMode),
					Fixture:        strings.TrimSpace(item.Fixture),
					SourceRefs:     trimStringSlice(item.SourceRefs),
					BlockedBy:      trimStringSlice(item.BlockedBy),
					Unblocks:       trimStringSlice(item.Unblocks),
					ReadyWhen:      trimStringSlice(item.ReadyWhen),
					NotReadyWhen:   trimStringSlice(item.NotReadyWhen),
					Acceptance:     trimStringSlice(item.Acceptance),
					WriteScope:     trimStringSlice(item.WriteScope),
					TestCommands:   trimStringSlice(item.TestCommands),
					DoneSignal:     trimStringSlice(item.DoneSignal),
					Note:           strings.TrimSpace(item.Note),
				}
```

Add these helpers near `firstNonEmpty`:

```go
func trimStringSlice(values []string) []string {
	var out []string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
```

```go
func (candidate Candidate) SelectionReason() string {
	switch candidateSelectionBucket(candidate) {
	case 0:
		return "P0 handoff"
	case 1:
		return "already active"
	case 2:
		return "fixture ready"
	case 3:
		return "unblocks downstream work"
	case 4:
		return "draft contract"
	default:
		return "planned row"
	}
}
```

- [ ] **Step 4: Update ranking logic**

Replace `candidateRank` with:

```go
func candidateRank(candidate Candidate, activeFirst bool, boosts map[string]struct{}) int {
	rank := 0
	if _, ok := boosts[strings.ToLower(strings.TrimSpace(candidate.SubphaseID))]; !ok {
		rank += 100
	}
	if activeFirst {
		rank += candidateSelectionBucket(candidate)
	}
	return rank
}
```

Add:

```go
func candidateSelectionBucket(candidate Candidate) int {
	switch {
	case strings.EqualFold(candidate.Priority, "P0"):
		return 0
	case candidate.Status == "in_progress":
		return 1
	case candidate.ContractStatus == "fixture_ready":
		return 2
	case len(candidate.Unblocks) > 0:
		return 3
	case candidate.ContractStatus == "draft":
		return 4
	case candidate.Status == "planned":
		return 5
	default:
		return 6
	}
}
```

- [ ] **Step 5: Run candidate tests**

Run:

```bash
go test ./internal/autoloop -run TestNormalizeCandidates -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit candidate metadata**

Run:

```bash
git add internal/autoloop/candidates.go internal/autoloop/candidates_test.go
git commit -m "feat: preserve autoloop candidate metadata"
```

### Task 4: Pass Rich Metadata Into Worker Prompts And Dry Runs

**Files:**
- Modify: `internal/autoloop/run.go`
- Modify: `internal/autoloop/run_test.go`
- Modify: `cmd/autoloop/main.go`
- Modify: `cmd/autoloop/main_test.go`

- [ ] **Step 1: Add prompt test first**

In `internal/autoloop/run_test.go`, replace `TestRunOncePassesSelectedTaskPromptToBackend` with:

```go
func TestRunOncePassesExecutionMetadataPromptToBackend(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{
								"name": "prompted candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "Provider-neutral transcript contract",
								"contract_status": "fixture_ready",
								"slice_size": "medium",
								"execution_owner": "provider",
								"trust_class": ["system"],
								"degraded_mode": "provider status reports missing fixtures",
								"fixture": "internal/hermes/testdata/provider_transcripts",
								"source_refs": ["docs/content/upstream-hermes/source-study.md"],
								"ready_when": ["fixtures replay"],
								"not_ready_when": ["live provider call required"],
								"acceptance": ["go test ./internal/hermes passes"],
								"write_scope": ["internal/hermes/"],
								"test_commands": ["go test ./internal/hermes -count=1"],
								"done_signal": ["provider transcript replay passes"],
								"note": "Use captured transcript fixtures."
							}
						]
					}
				}
			}
		}
	}`)
	runner := &FakeRunner{
		Results: []Result{{}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 1; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	args := runner.Commands[0].Args
	if len(args) == 0 {
		t.Fatal("Command args are empty, want backend flags plus prompt")
	}
	prompt := args[len(args)-1]
	for _, want := range []string{
		"Mission:",
		"Selected task:",
		"12 / 12.A / prompted candidate",
		"Current status: planned",
		"Priority: P0",
		"Execution owner: provider",
		"Slice size: medium",
		"Contract: Provider-neutral transcript contract",
		"Trust class:",
		"- system",
		"Allowed write scope:",
		"- internal/hermes/",
		"Required test commands:",
		"- go test ./internal/hermes -count=1",
		"Done signal:",
		"- provider transcript replay passes",
		"Source references:",
		"- docs/content/upstream-hermes/source-study.md",
		"Degraded mode: provider status reports missing fixtures",
		"Note: Use captured transcript fixtures.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}
```

- [ ] **Step 2: Run prompt test to verify it fails**

Run:

```bash
go test ./internal/autoloop -run TestRunOncePassesExecutionMetadataPromptToBackend -count=1
```

Expected: FAIL because the prompt still only contains the short task label and status.

- [ ] **Step 3: Replace worker prompt builder**

In `internal/autoloop/run.go`, replace `BuildWorkerPrompt` with:

```go
func BuildWorkerPrompt(candidate Candidate) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Mission:")
	fmt.Fprintln(&b, "Complete the selected Gormes progress task with strict Test-Driven Development (TDD).")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Selected task:")
	fmt.Fprintf(&b, "- Phase/Subphase/Item: %s / %s / %s\n", candidate.PhaseID, candidate.SubphaseID, candidate.ItemName)
	fmt.Fprintf(&b, "- Current status: %s\n", valueOrDash(candidate.Status))
	fmt.Fprintf(&b, "- Priority: %s\n", valueOrDash(candidate.Priority))
	fmt.Fprintf(&b, "- Execution owner: %s\n", valueOrDash(candidate.ExecutionOwner))
	fmt.Fprintf(&b, "- Slice size: %s\n", valueOrDash(candidate.SliceSize))
	fmt.Fprintf(&b, "- Selection reason: %s\n", candidate.SelectionReason())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Execution contract:")
	fmt.Fprintf(&b, "- Contract: %s\n", valueOrDash(candidate.Contract))
	fmt.Fprintf(&b, "- Contract status: %s\n", valueOrDash(candidate.ContractStatus))
	fmt.Fprintf(&b, "- Fixture: %s\n", valueOrDash(candidate.Fixture))
	fmt.Fprintf(&b, "- Degraded mode: %s\n", valueOrDash(candidate.DegradedMode))
	fmt.Fprintln(&b, "- Trust class:")
	writePromptList(&b, candidate.TrustClass)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Readiness:")
	fmt.Fprintln(&b, "- Ready when:")
	writePromptList(&b, candidate.ReadyWhen)
	fmt.Fprintln(&b, "- Not ready when:")
	writePromptList(&b, candidate.NotReadyWhen)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Worker boundaries:")
	fmt.Fprintln(&b, "- Allowed write scope:")
	writePromptList(&b, candidate.WriteScope)
	fmt.Fprintln(&b, "- Required test commands:")
	writePromptList(&b, candidate.TestCommands)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Completion evidence:")
	fmt.Fprintln(&b, "- Done signal:")
	writePromptList(&b, candidate.DoneSignal)
	fmt.Fprintln(&b, "- Acceptance:")
	writePromptList(&b, candidate.Acceptance)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Source references:")
	writePromptList(&b, candidate.SourceRefs)
	if candidate.Note != "" {
		fmt.Fprintf(&b, "\nNote: %s\n", candidate.Note)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Requirements:")
	fmt.Fprintln(&b, "- Read the repository context before editing.")
	fmt.Fprintln(&b, "- Keep changes scoped to the selected task and its allowed write scope.")
	fmt.Fprintln(&b, "- Run the required test commands before reporting completion.")
	fmt.Fprintln(&b, "- Report against the done signal and acceptance criteria.")
	return b.String()
}
```

Add these helpers below `BuildWorkerPrompt`:

```go
func writePromptList(b *strings.Builder, values []string) {
	if len(values) == 0 {
		fmt.Fprintln(b, "- (none declared)")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func valueOrDash(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}
```

- [ ] **Step 4: Add dry-run output test**

In `cmd/autoloop/main_test.go`, update `TestRunCommandDryRunPrintsSummary` progress JSON item to:

```json
							{
								"name": "planned CLI candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "CLI execution contract",
								"contract_status": "draft",
								"slice_size": "small",
								"execution_owner": "orchestrator",
								"write_scope": ["cmd/autoloop/"],
								"test_commands": ["go test ./cmd/autoloop -count=1"],
								"done_signal": ["dry-run output names metadata"]
							}
```

Update the output expectations in that test to:

```go
	for _, want := range []string{
		"candidates: 1",
		"selected: 1",
		"planned CLI candidate",
		"owner=orchestrator",
		"size=small",
		"reason=P0 handoff",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
```

- [ ] **Step 5: Run dry-run test to verify it fails**

Run:

```bash
go test ./cmd/autoloop -run TestRunCommandDryRunPrintsSummary -count=1
```

Expected: FAIL because `runAutoloop` does not print owner, size, or selection reason yet.

- [ ] **Step 6: Update dry-run summary output**

In `cmd/autoloop/main.go`, replace the selected candidate line in `runAutoloop`:

```go
		fmt.Fprintf(commandStdout, "- %s/%s %s [%s]\n", candidate.PhaseID, candidate.SubphaseID, candidate.ItemName, candidate.Status)
```

with:

```go
		fmt.Fprintf(commandStdout, "- %s/%s %s [%s] owner=%s size=%s reason=%s\n",
			candidate.PhaseID,
			candidate.SubphaseID,
			candidate.ItemName,
			candidate.Status,
			dashIfEmpty(candidate.ExecutionOwner),
			dashIfEmpty(candidate.SliceSize),
			candidate.SelectionReason(),
		)
```

Add this helper near `runAutoloop`:

```go
func dashIfEmpty(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}
```

Add `strings` to the imports in `cmd/autoloop/main.go`:

```go
	"strings"
```

- [ ] **Step 7: Run autoloop command tests**

Run:

```bash
go test ./internal/autoloop ./cmd/autoloop -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit prompt and dry-run changes**

Run:

```bash
git add internal/autoloop/run.go internal/autoloop/run_test.go cmd/autoloop/main.go cmd/autoloop/main_test.go
git commit -m "feat: pass autoloop execution metadata to workers"
```

### Task 5: Clean Links, Regenerate Docs, And Verify End To End

**Files:**
- Modify: `docs/content/building-gormes/_index.md`
- Modify: `docs/content/building-gormes/autoloop/*.md`
- Modify: `docs/content/building-gormes/architecture_plan/_index.md`
- Modify: `cmd/autoloop/README.md`
- Modify: `cmd/README.md`
- Modify: any files found by path search below

- [ ] **Step 1: Find stale execution-page paths**

Run:

```bash
rg -n "building-gormes/(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)|\\./(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)/|\\.\\./(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)/" docs cmd README.md
```

Expected: matches remain in docs and README files before cleanup.

- [ ] **Step 2: Update human-authored links**

In `docs/content/building-gormes/_index.md`, use these target links in the section map and contributor path:

```markdown
| Choose implementation work | [Autoloop Agent Queue](./autoloop/agent-queue/) | [Next Slices](./autoloop/next-slices/), [Blocked Slices](./autoloop/blocked-slices/), [Umbrella Cleanup](./autoloop/umbrella-cleanup/) |
| Prepare an autonomous-worker handoff | [Contract Readiness](./contract-readiness/) | [Progress Schema](./autoloop/progress-schema/), [Autoloop Handoff](./autoloop/autoloop-handoff/) |
```

Use these reference groups:

```markdown
**Execution queue:** [Contract Readiness](./contract-readiness/), [Autoloop Handoff](./autoloop/autoloop-handoff/), [Agent Queue](./autoloop/agent-queue/), [Next Slices](./autoloop/next-slices/), [Blocked Slices](./autoloop/blocked-slices/), [Umbrella Cleanup](./autoloop/umbrella-cleanup/), [Progress Schema](./autoloop/progress-schema/).
```

In `cmd/autoloop/README.md`, use this control-plane list:

```markdown
- Canonical queue: `docs/content/building-gormes/architecture_plan/progress.json`
- Human handoff: `docs/content/building-gormes/`
- Autoloop handoff: `docs/content/building-gormes/autoloop/autoloop-handoff.md`
- Worker-ready rows: `docs/content/building-gormes/autoloop/agent-queue.md`
- Schema contract: `docs/content/building-gormes/autoloop/progress-schema.md`
```

In `cmd/README.md`, keep the canonical queue path unchanged and update references to generated pages so they point to `docs/content/building-gormes/autoloop/`.

- [ ] **Step 3: Regenerate progress docs**

Run:

```bash
make generate-progress
```

Expected: generated autoloop pages under `docs/content/building-gormes/autoloop/` are rewritten, and the website progress copy is refreshed.

- [ ] **Step 4: Search again for stale links**

Run:

```bash
rg -n "docs/content/building-gormes/(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)|\\]\\(\\./(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)/\\)|\\]\\(\\.\\./(autoloop-handoff|agent-queue|next-slices|blocked-slices|umbrella-cleanup|progress-schema)/\\)" docs cmd README.md
```

Expected: no matches, except old alias strings in front matter if the search pattern is broadened manually to include `/building-gormes/.../`.

- [ ] **Step 5: Run focused verification**

Run:

```bash
go test ./internal/progress ./internal/autoloop ./cmd/autoloop ./docs -count=1
```

Expected: PASS.

- [ ] **Step 6: Validate progress**

Run:

```bash
make validate-progress
```

Expected:

```text
Validating progress.json...
progress-gen: validated 6 phases
```

- [ ] **Step 7: Check generated docs are stable**

Run:

```bash
make generate-progress
git diff -- docs/content/building-gormes/autoloop docs/content/building-gormes/architecture_plan/progress.json www.gormes.ai/internal/site/data/progress.json
```

Expected: `make generate-progress` exits 0. The diff either stays empty after the previous generation or shows only deterministic generated content that belongs in this change.

- [ ] **Step 8: Run formatting and diff checks**

Run:

```bash
gofmt -w cmd/progress-gen/main.go internal/progress/render.go internal/autoloop/candidates.go internal/autoloop/run.go cmd/autoloop/main.go
git diff --check
```

Expected: `git diff --check` exits 0.

- [ ] **Step 9: Commit cleanup and verification updates**

Run:

```bash
git add docs/content/building-gormes docs/docs_test.go docs/build_test.go
git add cmd/autoloop/README.md cmd/README.md cmd/progress-gen/main.go
git add internal/progress/render.go internal/progress/render_test.go
git add internal/autoloop/candidates.go internal/autoloop/candidates_test.go internal/autoloop/run.go internal/autoloop/run_test.go
git add cmd/autoloop/main.go cmd/autoloop/main_test.go
git add www.gormes.ai/internal/site/data/progress.json
git commit -m "docs: finish autoloop control plane refactor"
```

- [ ] **Step 10: Final full test command**

Run:

```bash
go test ./...
```

Expected: PASS. If unrelated existing failures appear, capture the failing package, test name, and first error line in the handoff instead of masking the failure.
