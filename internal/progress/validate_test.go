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

func TestValidate_AcceptsCompleteAutoloopMeta(t *testing.T) {
	p := &Progress{
		Meta: Meta{
			Version: "2.0",
			Autoloop: AutoloopMeta{
				Entrypoint:      "scripts/gormes-auto-codexu-orchestrator.sh",
				Plan:            "docs/superpowers/plans/2026-04-24-autoloop-repoctl-go-port.md",
				AgentQueue:      "docs/content/building-gormes/agent-queue.md",
				ProgressSchema:  "docs/content/building-gormes/progress-schema.md",
				CandidateSource: "docs/content/building-gormes/architecture_plan/progress.json",
				UnitTest:        "scripts/orchestrator/tests/run.sh unit",
				CandidatePolicy: []string{"skip blocked rows"},
			},
		},
		Phases: map[string]Phase{},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsPartialAutoloopMeta(t *testing.T) {
	p := &Progress{
		Meta: Meta{
			Version: "2.0",
			Autoloop: AutoloopMeta{
				Entrypoint:      "scripts/gormes-auto-codexu-orchestrator.sh",
				CandidatePolicy: []string{" "},
			},
		},
		Phases: map[string]Phase{},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want autoloop metadata errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"meta.autoloop missing plan",
		"meta.autoloop missing agent_queue",
		"meta.autoloop missing progress_schema",
		"meta.autoloop missing candidate_source",
		"meta.autoloop missing unit_test",
		"meta.autoloop candidate_policy[0] is blank",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
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

func TestValidate_RejectsBadTrustClass(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: StatusPlanned, TrustClass: []string{"unknown"}}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "trust_class") {
		t.Errorf("Validate() = %v, want trust_class error", err)
	}
}

func TestValidate_RejectsBadPriority(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: StatusPlanned, Priority: "urgent"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "priority") {
		t.Errorf("Validate() = %v, want priority error", err)
	}
}

func TestValidate_RejectsBadContractStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: StatusPlanned, ContractStatus: "almost"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "contract_status") {
		t.Errorf("Validate() = %v, want contract_status error", err)
	}
}

func TestValidate_RejectsBadSliceSize(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: StatusPlanned, SliceSize: "huge"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "slice_size") {
		t.Errorf("Validate() = %v, want slice_size error", err)
	}
}

func TestValidate_RejectsBadExecutionOwner(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: StatusPlanned, ExecutionOwner: "everyone"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "execution_owner") {
		t.Errorf("Validate() = %v, want execution_owner error", err)
	}
}

func TestValidate_AcceptsContractMetadata(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "x",
				Status:         StatusPlanned,
				Priority:       "P0",
				Contract:       "contract",
				ContractStatus: ContractStatusDraft,
				SliceSize:      SliceSizeSmall,
				ExecutionOwner: ExecutionOwnerGateway,
				TrustClass:     []string{"operator", "gateway", "child-agent", "system"},
				DegradedMode:   "doctor reports degraded status",
				Fixture:        "internal/example fixtures",
				SourceRefs:     []string{"docs/content/upstream-hermes/source-study.md"},
				BlockedBy:      []string{"dependency"},
				Unblocks:       []string{"downstream"},
				ReadyWhen:      []string{"dependency is complete"},
				NotReadyWhen:   []string{"live credentials are required"},
				Acceptance:     []string{"fixture passes"},
				WriteScope:     []string{"internal/gateway/"},
				TestCommands:   []string{"go test ./internal/gateway -count=1"},
				DoneSignal:     []string{"fixture passes locally"},
			}}},
		}}},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsActiveOrP0MissingContractMetadata(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{
				{Name: "active", Status: StatusInProgress},
				{Name: "p0", Status: StatusPlanned, Priority: "P0"},
			}},
		}}},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want metadata errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"active/P0 item missing contract",
		"active/P0 item missing contract_status",
		"active/P0 item missing trust_class",
		"active/P0 item missing degraded_mode",
		"active/P0 item missing fixture",
		"active/P0 item missing source_refs",
		"active/P0 item missing acceptance",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestValidate_RejectsContractRowMissingExecutionMetadata(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "contract row",
				Status:         StatusPlanned,
				Contract:       "contract",
				ContractStatus: ContractStatusDraft,
			}}},
		}}},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want execution metadata errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"contract row missing slice_size",
		"contract row missing execution_owner",
		"contract row missing ready_when",
		"contract row missing write_scope",
		"contract row missing test_commands",
		"contract row missing done_signal",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestValidate_RejectsBlankAutonomousHandoffFields(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "contract row",
				Status:         StatusPlanned,
				Contract:       "contract",
				ContractStatus: ContractStatusDraft,
				SliceSize:      SliceSizeSmall,
				ExecutionOwner: ExecutionOwnerGateway,
				ReadyWhen:      []string{"ready"},
				WriteScope:     []string{"internal/gateway/", " "},
				TestCommands:   []string{"go test ./internal/gateway -count=1", ""},
				DoneSignal:     []string{"fixture passes", "\t"},
			}}},
		}}},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want blank field errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"write_scope[1] is blank",
		"test_commands[1] is blank",
		"done_signal[1] is blank",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}

func TestValidate_RejectsInProgressUmbrella(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "bad active umbrella",
				Status:         StatusInProgress,
				Priority:       "P1",
				Contract:       "contract",
				ContractStatus: ContractStatusDraft,
				SliceSize:      SliceSizeUmbrella,
				ExecutionOwner: ExecutionOwnerTools,
				TrustClass:     []string{"system"},
				DegradedMode:   "visible",
				Fixture:        "internal/tools fixtures",
				SourceRefs:     []string{"docs/content/building-gormes/progress-schema.md"},
				ReadyWhen:      []string{"split first"},
				NotReadyWhen:   []string{"still broad"},
				Acceptance:     []string{"split"},
				WriteScope:     []string{"internal/tools/"},
				TestCommands:   []string{"go test ./internal/tools -count=1"},
				DoneSignal:     []string{"row is split before implementation"},
			}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "in_progress item cannot have slice_size") {
		t.Errorf("Validate() = %v, want in_progress umbrella error", err)
	}
}

func TestValidate_RejectsBlockedWithoutReadyWhen(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "blocked", Status: StatusPlanned, BlockedBy: []string{"x"}}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "blocked item missing ready_when") {
		t.Errorf("Validate() = %v, want blocked ready_when error", err)
	}
}

func TestValidate_RejectsFixtureReadyWithoutConcreteFixture(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "fixture ready",
				Status:         StatusPlanned,
				Contract:       "contract",
				ContractStatus: ContractStatusFixtureReady,
				SliceSize:      SliceSizeSmall,
				ExecutionOwner: ExecutionOwnerProvider,
				ReadyWhen:      []string{"ready"},
				Fixture:        "fixtures",
				WriteScope:     []string{"internal/hermes/"},
				TestCommands:   []string{"go test ./internal/hermes -count=1"},
				DoneSignal:     []string{"fixtures replay"},
			}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "fixture_ready item needs concrete fixture") {
		t.Errorf("Validate() = %v, want concrete fixture error", err)
	}
}

func TestValidate_RejectsCompleteContractWithoutValidatedStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{
				Name:           "complete",
				Status:         StatusComplete,
				Contract:       "contract",
				ContractStatus: ContractStatusDraft,
				SliceSize:      SliceSizeSmall,
				ExecutionOwner: ExecutionOwnerProvider,
				ReadyWhen:      []string{"ready"},
				WriteScope:     []string{"internal/hermes/"},
				TestCommands:   []string{"go test ./internal/hermes -count=1"},
				DoneSignal:     []string{"fixtures replay"},
			}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "complete contract row must use contract_status") {
		t.Errorf("Validate() = %v, want complete validated error", err)
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

func TestValidate_AccumulatesMultipleErrors(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "bad", Status: "nope"}}},
				"1.B": {Items: []Item{{Name: "x", Status: StatusComplete}}, Status: StatusComplete}, // both
			}},
			"2": {Subphases: map[string]Subphase{
				"2.A": {}, // neither
			}},
		},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want multiple errors")
	}
	// errors.Join formats as one error per line separated by \n.
	msg := err.Error()
	for _, want := range []string{
		"phase 1 subphase 1.A", // bad item status
		"phase 1 subphase 1.B", // both items and status
		"phase 2 subphase 2.A", // neither
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}
