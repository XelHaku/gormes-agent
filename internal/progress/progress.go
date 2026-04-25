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

type ContractStatus string

const (
	ContractStatusMissing      ContractStatus = "missing"
	ContractStatusDraft        ContractStatus = "draft"
	ContractStatusFixtureReady ContractStatus = "fixture_ready"
	ContractStatusValidated    ContractStatus = "validated"
)

type SliceSize string

const (
	SliceSizeSmall    SliceSize = "small"
	SliceSizeMedium   SliceSize = "medium"
	SliceSizeLarge    SliceSize = "large"
	SliceSizeUmbrella SliceSize = "umbrella"
)

type ExecutionOwner string

const (
	ExecutionOwnerDocs         ExecutionOwner = "docs"
	ExecutionOwnerGateway      ExecutionOwner = "gateway"
	ExecutionOwnerMemory       ExecutionOwner = "memory"
	ExecutionOwnerProvider     ExecutionOwner = "provider"
	ExecutionOwnerTools        ExecutionOwner = "tools"
	ExecutionOwnerSkills       ExecutionOwner = "skills"
	ExecutionOwnerOrchestrator ExecutionOwner = "orchestrator"
)

type AutoloopMeta struct {
	Entrypoint      string   `json:"entrypoint"`
	Plan            string   `json:"plan"`
	AgentQueue      string   `json:"agent_queue"`
	ProgressSchema  string   `json:"progress_schema"`
	CandidateSource string   `json:"candidate_source"`
	UnitTest        string   `json:"unit_test"`
	CandidatePolicy []string `json:"candidate_policy"`
}

type Meta struct {
	Version     string       `json:"version"`
	LastUpdated string       `json:"last_updated"`
	Links       Links        `json:"links"`
	Autoloop    AutoloopMeta `json:"autoloop,omitempty"`
}

type Links struct {
	GitHubReadme string `json:"github_readme"`
	LandingPage  string `json:"landing_page"`
	DocsSite     string `json:"docs_site"`
	SourceCode   string `json:"source_code"`
}

type Item struct {
	Name     string `json:"name"`
	Priority string `json:"priority,omitempty"`
	Status   Status `json:"status"`
	// Optional contract metadata turns roadmap rows into executable architecture
	// requirements without forcing every historical item to be rewritten at once.
	Contract       string         `json:"contract,omitempty"`
	ContractStatus ContractStatus `json:"contract_status,omitempty"`
	SliceSize      SliceSize      `json:"slice_size,omitempty"`
	ExecutionOwner ExecutionOwner `json:"execution_owner,omitempty"`
	TrustClass     []string       `json:"trust_class,omitempty"`
	DegradedMode   string         `json:"degraded_mode,omitempty"`
	Fixture        string         `json:"fixture,omitempty"`
	SourceRefs     []string       `json:"source_refs,omitempty"`
	ReadyWhen      []string       `json:"ready_when,omitempty"`
	NotReadyWhen   []string       `json:"not_ready_when,omitempty"`
	BlockedBy      []string       `json:"blocked_by,omitempty"`
	Unblocks       []string       `json:"unblocks,omitempty"`
	Acceptance     []string       `json:"acceptance,omitempty"`
	Note           string         `json:"note,omitempty"`
	WriteScope     []string       `json:"write_scope,omitempty"`
	TestCommands   []string       `json:"test_commands,omitempty"`
	DoneSignal     []string       `json:"done_signal,omitempty"`
	// Optional, reserved, not rendered yet.
	PR    string `json:"pr,omitempty"`
	Owner string `json:"owner,omitempty"`
	ETA   string `json:"eta,omitempty"`
	// Health is execution-history metadata owned by autoloop. The planner
	// must preserve this block verbatim across regenerations (see
	// docs/superpowers/specs/2026-04-24-reactive-autoloop-design.md).
	Health *RowHealth `json:"health,omitempty"`
	// PlannerVerdict is execution-history metadata owned by the planner
	// runtime. Autoloop reads it (to skip human-escalated rows) and must
	// preserve it verbatim across writes (see
	// docs/superpowers/specs/2026-04-24-planner-self-healing-design.md).
	PlannerVerdict *PlannerVerdict `json:"planner_verdict,omitempty"`
}

type Subphase struct {
	Name     string `json:"name"`
	Priority string `json:"priority,omitempty"`
	// Exactly one of Items or Status is set. Enforced by Validate.
	Items  []Item `json:"items,omitempty"`
	Status Status `json:"status,omitempty"`
}

type Phase struct {
	Name        string `json:"name"`
	Deliverable string `json:"deliverable"`
	// DependencyNote is a free-form string on some phases.
	DependencyNote string              `json:"dependency_note,omitempty"`
	Subphases      map[string]Subphase `json:"subphases"`
}

type Progress struct {
	Meta   Meta             `json:"meta"`
	Phases map[string]Phase `json:"phases"`
}

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

// DerivedStatus computes phase status from subphases. Empty phase -> planned.
func (ph Phase) DerivedStatus() Status {
	if len(ph.Subphases) == 0 {
		return StatusPlanned
	}
	allComplete := true
	anyStarted := false
	for _, sp := range ph.Subphases {
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
