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
	if !strings.Contains(got, "| Phase | Status | Shipped |") {
		t.Errorf("rollup missing table header; got:\n%s", got)
	}
	if !strings.Contains(got, "Phase 1 — Dashboard") {
		t.Errorf("rollup missing Phase 1 row; got:\n%s", got)
	}
	if !strings.Contains(got, "1/1") {
		t.Errorf("rollup missing 1/1 count for Phase 1; got:\n%s", got)
	}
	if !strings.Contains(got, "1/2") {
		t.Errorf("rollup missing 1/2 count for Phase 2; got:\n%s", got)
	}
	// Guard against statusIcon() silently returning "".
	if !strings.Contains(got, "✅") {
		t.Errorf("rollup missing shipped icon ✅; got:\n%s", got)
	}
	if !strings.Contains(got, "🔨") {
		t.Errorf("rollup missing in-progress icon 🔨; got:\n%s", got)
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
	// Match on the table-cell leader "| Phase 1 " (with trailing space) so
	// "Phase 1" does not accidentally match "Phase 10".
	i1 := strings.Index(got, "| Phase 1 ")
	i2 := strings.Index(got, "| Phase 2 ")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("phases not sorted (i1=%d, i2=%d):\n%s", i1, i2, got)
	}
}

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

func TestRenderDocsChecklist_EmptyItemsFallback(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"5": {Name: "P5", Subphases: map[string]Subphase{
				"5.A": {Name: "Later", Status: StatusPlanned},
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "no item breakdown") {
		t.Errorf("missing fallback line; got:\n%s", got)
	}
	if !strings.Contains(got, "tracked at subphase level: planned") {
		t.Errorf("fallback missing status echo; got:\n%s", got)
	}
}

func TestRenderDocsChecklist_EmptyItemsBlankStatus(t *testing.T) {
	// If Validate is bypassed and a subphase has neither items nor status,
	// the renderer must not emit a dangling blank. This guards against
	// silent UI corruption if the invariant is ever violated.
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"5": {Name: "P5", Subphases: map[string]Subphase{
				"5.A": {Name: "Later"}, // no items, no status
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "tracked at subphase level: unspecified") {
		t.Errorf("missing unspecified fallback; got:\n%s", got)
	}
}

func TestRenderContractReadiness(t *testing.T) {
	p := &Progress{Phases: map[string]Phase{
		"2": {Name: "Phase 2", Subphases: map[string]Subphase{
			"2.F": {Name: "Gateway", Items: []Item{{
				Name:           "Steer",
				Status:         StatusPlanned,
				Contract:       "Active turn steering",
				ContractStatus: ContractStatusDraft,
				SliceSize:      SliceSizeSmall,
				ExecutionOwner: ExecutionOwnerGateway,
				TrustClass:     []string{"operator", "gateway"},
				DegradedMode:   "busy status",
				Fixture:        "internal/gateway fixtures",
			}}},
		}},
	}}

	got := RenderContractReadiness(p)
	for _, want := range []string{
		"| Phase | Progress item | Contract status | Owner | Size | Trust class | Fixture | Degraded mode |",
		"2 / 2.F",
		"Steer — Active turn steering",
		"`draft`",
		"`gateway`",
		"`small`",
		"operator, gateway",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("contract readiness missing %q:\n%s", want, got)
		}
	}
}

func TestRenderNextSlicesOrdersUnblockedP0ThenActiveAndSkipsBlockedUmbrella(t *testing.T) {
	p := &Progress{Phases: map[string]Phase{
		"1": {Name: "Phase 1", Subphases: map[string]Subphase{
			"1.A": {Name: "Alpha", Items: []Item{
				{Name: "planned", Status: StatusPlanned, Contract: "planned contract", ContractStatus: ContractStatusDraft, SliceSize: SliceSizeSmall, TrustClass: []string{"system"}, Fixture: "f"},
				{Name: "active", Status: StatusInProgress, Contract: "active contract", ContractStatus: ContractStatusFixtureReady, SliceSize: SliceSizeSmall, TrustClass: []string{"system"}, Fixture: "f"},
				{Name: "p0", Status: StatusPlanned, Priority: "P0", Contract: "p0 contract", ContractStatus: ContractStatusDraft, SliceSize: SliceSizeSmall, TrustClass: []string{"operator"}, Fixture: "f"},
				{Name: "blocked", Status: StatusPlanned, Contract: "blocked contract", ContractStatus: ContractStatusFixtureReady, SliceSize: SliceSizeSmall, TrustClass: []string{"system"}, Fixture: "f", BlockedBy: []string{"dependency"}},
				{Name: "umbrella", Status: StatusPlanned, Contract: "umbrella contract", ContractStatus: ContractStatusDraft, SliceSize: SliceSizeUmbrella, TrustClass: []string{"system"}, Fixture: "f"},
				{Name: "complete", Status: StatusComplete, Contract: "complete contract", ContractStatus: ContractStatusValidated, SliceSize: SliceSizeSmall, TrustClass: []string{"system"}, Fixture: "f"},
			}},
		}},
	}}

	got := RenderNextSlices(p, 10)
	active := strings.Index(got, "active contract")
	p0 := strings.Index(got, "p0 contract")
	planned := strings.Index(got, "planned contract")
	if active < 0 || p0 < 0 || planned < 0 {
		t.Fatalf("next slices missing expected rows:\n%s", got)
	}
	if strings.Contains(got, "blocked contract") || strings.Contains(got, "umbrella contract") || strings.Contains(got, "complete contract") {
		t.Fatalf("next slices included blocked, umbrella, or complete row:\n%s", got)
	}
	if !(p0 < active && active < planned) {
		t.Fatalf("next slices order wrong; active=%d p0=%d planned=%d:\n%s", active, p0, planned, got)
	}
}

func TestRenderAgentQueueIncludesExecutionCardAndSkipsBlockedUmbrella(t *testing.T) {
	p := &Progress{Phases: map[string]Phase{
		"4": {Name: "Phase 4", Subphases: map[string]Subphase{
			"4.A": {Name: "Providers", Items: []Item{
				{
					Name:           "Provider harness",
					Status:         StatusInProgress,
					Contract:       "Provider-neutral transcript contract",
					ContractStatus: ContractStatusFixtureReady,
					SliceSize:      SliceSizeMedium,
					ExecutionOwner: ExecutionOwnerProvider,
					TrustClass:     []string{"system"},
					DegradedMode:   "provider status reports gaps",
					Fixture:        "internal/hermes/testdata/provider_transcripts",
					SourceRefs:     []string{"docs/content/upstream-hermes/source-study.md"},
					ReadyWhen:      []string{"fixtures replay"},
					NotReadyWhen:   []string{"live provider call required"},
					Acceptance:     []string{"go test ./internal/hermes passes"},
					WriteScope:     []string{"internal/hermes"},
					TestCommands:   []string{"go test ./internal/hermes"},
					DoneSignal:     []string{"provider transcripts replay"},
					Unblocks:       []string{"Bedrock"},
				},
				{
					Name:      "Blocked provider",
					Status:    StatusPlanned,
					Contract:  "blocked",
					SliceSize: SliceSizeSmall,
					BlockedBy: []string{"Provider harness"},
				},
				{
					Name:      "Umbrella provider",
					Status:    StatusPlanned,
					Contract:  "umbrella",
					SliceSize: SliceSizeUmbrella,
				},
				{
					Name:           "Completed provider",
					Status:         StatusComplete,
					Contract:       "complete",
					ContractStatus: ContractStatusValidated,
					SliceSize:      SliceSizeSmall,
				},
			}},
		}},
	}}

	got := RenderAgentQueue(p, 10)
	for _, want := range []string{
		"## 1. Provider harness",
		"- Owner: `provider`",
		"- Size: `medium`",
		"- Contract: Provider-neutral transcript contract",
		"- Ready when: fixtures replay",
		"- Not ready when: live provider call required",
		"- Write scope: `internal/hermes`",
		"- Test commands: `go test ./internal/hermes`",
		"- Fixture: `internal/hermes/testdata/provider_transcripts`",
		"- Acceptance: go test ./internal/hermes passes",
		"- Done signal: provider transcripts replay",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("agent queue missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Blocked provider") || strings.Contains(got, "Umbrella provider") || strings.Contains(got, "Completed provider") {
		t.Fatalf("agent queue included blocked, umbrella, or complete row:\n%s", got)
	}
}

func TestRenderBlockedSlices(t *testing.T) {
	p := &Progress{Phases: map[string]Phase{
		"3": {Name: "Phase 3", Subphases: map[string]Subphase{
			"3.E": {Name: "Memory", Items: []Item{
				{
					Name:      "Cross-chat",
					Contract:  "Scoped recall",
					BlockedBy: []string{"schema"},
					ReadyWhen: []string{"schema ships"},
					Unblocks:  []string{"operator evidence"},
				},
				{
					Name:           "Completed blocked row",
					Status:         StatusComplete,
					Contract:       "complete",
					ContractStatus: ContractStatusValidated,
					BlockedBy:      []string{"old schema"},
					ReadyWhen:      []string{"old schema shipped"},
				},
			}},
		}},
	}}

	got := RenderBlockedSlices(p)
	for _, want := range []string{"Cross-chat", "schema ships", "operator evidence"} {
		if !strings.Contains(got, want) {
			t.Fatalf("blocked slices missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Completed blocked row") {
		t.Fatalf("blocked slices included complete row:\n%s", got)
	}
}

func TestRenderUmbrellaCleanup(t *testing.T) {
	p := &Progress{Phases: map[string]Phase{
		"5": {Name: "Phase 5", Subphases: map[string]Subphase{
			"5.A": {Name: "Tools", Items: []Item{{
				Name:           "61-tool registry port",
				SliceSize:      SliceSizeUmbrella,
				ExecutionOwner: ExecutionOwnerTools,
				NotReadyWhen:   []string{"not split"},
				Unblocks:       []string{"schema parity"},
			}}},
		}},
	}}

	got := RenderUmbrellaCleanup(p)
	for _, want := range []string{"61-tool registry port", "`tools`", "not split", "schema parity"} {
		if !strings.Contains(got, want) {
			t.Fatalf("umbrella cleanup missing %q:\n%s", want, got)
		}
	}
}

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

func TestRenderProgressSchema(t *testing.T) {
	got := RenderProgressSchema()
	for _, want := range []string{
		"`meta.autoloop.entrypoint`",
		"`meta.autoloop.candidate_policy`",
		"`slice_size`",
		"`execution_owner`",
		"`ready_when`",
		"`write_scope`",
		"`test_commands`",
		"`done_signal`",
		"`in_progress` rows cannot use `slice_size: umbrella`",
		"`docs/content/building-gormes/autoloop/autoloop-handoff.md`",
		"`docs/content/building-gormes/autoloop/agent-queue.md`",
		"`docs/content/building-gormes/autoloop/blocked-slices.md`",
		"`docs/content/building-gormes/autoloop/umbrella-cleanup.md`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress schema missing %q:\n%s", want, got)
		}
	}
}
