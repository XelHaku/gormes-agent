package autoloop

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

type CandidateOptions struct {
	ActiveFirst     bool
	PriorityBoost   []string
	MaxPhase        int
	IncludeBlocked  bool
	IncludeUmbrella bool
	IncludePaused   bool
	// IncludeQuarantined causes NormalizeCandidates to surface rows whose
	// Health.Quarantine block is current (spec hash matches). Default false:
	// quarantined rows are filtered out so the run loop avoids known-bad
	// targets. Stale quarantines (spec hash mismatch) are always surfaced and
	// flagged with Candidate.StaleQuarantine regardless of this setting.
	IncludeQuarantined bool
	// IncludeNeedsHuman causes NormalizeCandidates to surface rows whose
	// PlannerVerdict.NeedsHuman is true. Default false: such rows are
	// filtered out so the autoloop honors the planner's escalation. Mirrors
	// IncludeQuarantined exactly. Surfaced rows are flagged with
	// Candidate.NeedsHumanFlag for downstream visibility.
	IncludeNeedsHuman bool
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
	// Health is the row's autoloop execution-history block, if any. Surfaced
	// here so the run loop and reporting can consult quarantine / failure
	// counts without re-loading progress.json.
	Health *progress.RowHealth
	// StaleQuarantine is set by Task 5's selection logic when the row's
	// existing Quarantine.SpecHash no longer matches the current ItemSpecHash
	// (planner reshape detected). The run loop forwards this to the health
	// accumulator so Flush clears the stale block atomically with run health.
	StaleQuarantine bool
	// PenaltyApplied is the ranking penalty derived from Health
	// (ConsecutiveFailures + 2*len(BackendsTried)). Recorded so the reason
	// string and downstream tooling can surface why a row sank in priority.
	PenaltyApplied int
	// NeedsHumanFlag is set when the row's PlannerVerdict.NeedsHuman is true
	// AND the candidate was surfaced anyway via IncludeNeedsHuman. Allows
	// reporting / status tooling to highlight the override without re-loading
	// the verdict block. Always false in the default skip-NeedsHuman path.
	NeedsHumanFlag bool
}

// failurePenalty returns the ranking penalty for n consecutive failures.
// 0 -> 0, 1 -> 5, 2 -> 20, 3+ -> 45 (capped). Rows past the quarantine
// threshold should already be filtered by NormalizeCandidates, but the cap
// covers manual-override scenarios where IncludeQuarantined is set.
func failurePenalty(n int) int {
	switch {
	case n <= 0:
		return 0
	case n == 1:
		return 5
	case n == 2:
		return 20
	default:
		return 45
	}
}

func (candidate Candidate) SelectionReason() string {
	var base string
	switch candidateBucket(candidate) {
	case candidateBucketP0:
		base = "P0 handoff"
	case candidateBucketInProgress:
		base = "already active"
	case candidateBucketFixtureReady:
		base = "fixture ready"
	case candidateBucketUnblocks:
		base = "unblocks downstream work"
	case candidateBucketDraft:
		base = "draft contract"
	default:
		base = "planned row"
	}
	if candidate.PenaltyApplied > 0 {
		base += fmt.Sprintf(" penalty=%d", candidate.PenaltyApplied)
	}
	if candidate.StaleQuarantine {
		base += " quarantine_stale_cleared"
	}
	if candidate.NeedsHumanFlag {
		base += " needs_human_visible"
	}
	return base
}

func NormalizeCandidates(path string, opts CandidateOptions) ([]Candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var progressDoc progressJSON
	if err := json.Unmarshal(data, &progressDoc); err != nil {
		return nil, err
	}

	completed := completedItemSet(progressDoc)
	var candidates []Candidate
	seen := make(map[string]struct{})
	for _, phase := range progressDoc.Phases {
		if phaseAboveMax(phase.ID, opts.MaxPhase) {
			continue
		}
		if phasePaused(phase.ID) && !opts.IncludePaused {
			continue
		}
		for _, subphase := range phase.allSubphases() {
			for _, item := range subphase.Items {
				name := firstNonEmpty(item.ItemName, item.Name, item.Title, item.ID)
				if name == "" {
					continue
				}

				status := lowerTrim(item.Status)
				if status == "" {
					status = "unknown"
				}
				if status == "complete" {
					continue
				}

				blockedBy := trimStringSlice(item.BlockedBy)
				sliceSize := lowerTrim(item.SliceSize)
				if len(blockedBy) > 0 && !opts.IncludeBlocked && !blockersComplete(blockedBy, completed) {
					continue
				}
				if !opts.IncludeUmbrella && sliceSize == "umbrella" {
					continue
				}

				candidate := Candidate{
					PhaseID:        strings.TrimSpace(phase.ID),
					SubphaseID:     strings.TrimSpace(subphase.ID),
					ItemName:       name,
					Status:         status,
					Priority:       strings.TrimSpace(firstNonEmpty(item.Priority, subphase.Priority)),
					Contract:       strings.TrimSpace(item.Contract),
					ContractStatus: lowerTrim(item.ContractStatus),
					SliceSize:      sliceSize,
					ExecutionOwner: lowerTrim(item.ExecutionOwner),
					TrustClass:     trimStringSlice(item.TrustClass),
					DegradedMode:   strings.TrimSpace(item.DegradedMode),
					Fixture:        strings.TrimSpace(item.Fixture),
					SourceRefs:     trimStringSlice(item.SourceRefs),
					BlockedBy:      blockedBy,
					Unblocks:       trimStringSlice(item.Unblocks),
					ReadyWhen:      trimStringSlice(item.ReadyWhen),
					NotReadyWhen:   trimStringSlice(item.NotReadyWhen),
					Acceptance:     trimStringSlice(item.Acceptance),
					WriteScope:     trimStringSlice(item.WriteScope),
					TestCommands:   trimStringSlice(item.TestCommands),
					DoneSignal:     trimStringSlice(item.DoneSignal),
					Note:           strings.TrimSpace(item.Note),
				}
				if !agentQueueCandidate(candidate) {
					continue
				}

				// Honor row health (Task 5):
				//   - Active quarantine (spec hash matches current spec) is
				//     filtered out unless IncludeQuarantined is set.
				//   - Stale quarantine (spec hash mismatch) surfaces the row
				//     with StaleQuarantine=true so the run loop can clear the
				//     block atomically with this run's health updates.
				//   - Consecutive-failure / backends-tried penalty is recorded
				//     on the candidate so the sort below can demote it.
				candidate.Health = item.Health
				if item.Health != nil && item.Health.Quarantine != nil {
					currentHash := progress.ItemSpecHash(itemPtr(item))
					if currentHash != item.Health.Quarantine.SpecHash {
						candidate.StaleQuarantine = true
					} else if !opts.IncludeQuarantined {
						continue
					}
				}
				// L5 PlannerVerdict skip: rows the planner has escalated to
				// "needs human" are removed from selection by default. Mirror
				// of the quarantine-skip pattern above. IncludeNeedsHuman
				// surfaces the row (flagged) for status / debug paths.
				if item.PlannerVerdict != nil && item.PlannerVerdict.NeedsHuman {
					if !opts.IncludeNeedsHuman {
						continue
					}
					candidate.NeedsHumanFlag = true
				}
				if item.Health != nil {
					pen := failurePenalty(item.Health.ConsecutiveFailures)
					pen += 2 * len(item.Health.BackendsTried)
					candidate.PenaltyApplied = pen
				}

				seenKey := candidateSortKey(candidate)
				if _, ok := seen[seenKey]; ok {
					continue
				}
				seen[seenKey] = struct{}{}

				candidates = append(candidates, candidate)
			}
		}
	}

	boosts := priorityBoostSet(opts.PriorityBoost)
	sort.Slice(candidates, func(i, j int) bool {
		left := candidateRank(candidates[i], opts.ActiveFirst, boosts) + candidates[i].PenaltyApplied
		right := candidateRank(candidates[j], opts.ActiveFirst, boosts) + candidates[j].PenaltyApplied
		if left != right {
			return left < right
		}

		return candidateSortKey(candidates[i]) < candidateSortKey(candidates[j])
	})

	return candidates, nil
}

// itemPtr returns a pointer to a progress.Item view of the given progressItem
// suitable for passing to progress.ItemSpecHash. Lifted to a helper so the
// conversion happens in one place.
func itemPtr(item progressItem) *progress.Item {
	view := item.toProgressItem()
	return &view
}

func phaseAboveMax(phaseID string, maxPhase int) bool {
	if maxPhase < 1 {
		return false
	}

	phaseNum, err := strconv.Atoi(strings.TrimSpace(phaseID))
	if err != nil {
		return false
	}

	return phaseNum > maxPhase
}

func phasePaused(phaseID string) bool {
	return strings.TrimSpace(phaseID) == "7"
}

func firstNonEmpty(vals ...string) string {
	for _, val := range vals {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

type progressJSON struct {
	Phases progressPhases `json:"phases"`
}

type progressPhases []progressPhase

func (phases *progressPhases) UnmarshalJSON(data []byte) error {
	var keyed map[string]progressPhase
	if err := json.Unmarshal(data, &keyed); err == nil {
		*phases = make([]progressPhase, 0, len(keyed))
		for id, phase := range keyed {
			phase.ID = firstNonEmpty(id, phase.ID)
			*phases = append(*phases, phase)
		}

		return nil
	}

	var listed []progressPhase
	if err := json.Unmarshal(data, &listed); err != nil {
		return err
	}
	*phases = listed

	return nil
}

type progressPhase struct {
	ID        string            `json:"id"`
	Subphases progressSubphases `json:"subphases"`
	SubPhases progressSubphases `json:"sub_phases"`
}

func (phase progressPhase) allSubphases() []progressSubphase {
	if len(phase.Subphases) > 0 {
		return phase.Subphases
	}

	return phase.SubPhases
}

type progressSubphases []progressSubphase

func (subphases *progressSubphases) UnmarshalJSON(data []byte) error {
	var keyed map[string]progressSubphase
	if err := json.Unmarshal(data, &keyed); err == nil {
		*subphases = make([]progressSubphase, 0, len(keyed))
		for id, subphase := range keyed {
			subphase.ID = firstNonEmpty(id, subphase.ID)
			*subphases = append(*subphases, subphase)
		}

		return nil
	}

	var listed []progressSubphase
	if err := json.Unmarshal(data, &listed); err != nil {
		return err
	}
	*subphases = listed

	return nil
}

type progressSubphase struct {
	ID       string         `json:"id"`
	Priority string         `json:"priority"`
	Items    []progressItem `json:"items"`
}

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
	// Health mirrors progress.Item.Health so candidate selection can honor
	// quarantine and ranking penalties without re-loading the file through
	// the canonical progress.Load path.
	Health *progress.RowHealth `json:"health,omitempty"`
	// PlannerVerdict mirrors progress.Item.PlannerVerdict so candidate
	// selection can honor planner-set NeedsHuman escalations without
	// re-loading the file through progress.Load. Mirrors Health exactly.
	PlannerVerdict *progress.PlannerVerdict `json:"planner_verdict,omitempty"`
}

// toProgressItem builds a progress.Item view containing only the fields used
// by progress.ItemSpecHash. Values are passed through verbatim so the digest
// matches the one progress.Load + progress.ItemSpecHash would produce against
// the same file.
func (item progressItem) toProgressItem() progress.Item {
	return progress.Item{
		Contract:       item.Contract,
		ContractStatus: progress.ContractStatus(item.ContractStatus),
		BlockedBy:      append([]string(nil), item.BlockedBy...),
		WriteScope:     append([]string(nil), item.WriteScope...),
		Fixture:        item.Fixture,
	}
}

func priorityBoostSet(boosts []string) map[string]struct{} {
	set := make(map[string]struct{}, len(boosts))
	for _, boost := range boosts {
		key := strings.ToLower(strings.TrimSpace(boost))
		if key != "" {
			set[key] = struct{}{}
		}
	}

	return set
}

func lowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func trimStringSlice(values []string) []string {
	var trimmed []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}

	return trimmed
}

const (
	candidateBucketP0 = iota
	candidateBucketInProgress
	candidateBucketFixtureReady
	candidateBucketUnblocks
	candidateBucketDraft
	candidateBucketPlanned
	candidateBucketOther
)

func candidateRank(candidate Candidate, activeFirst bool, boosts map[string]struct{}) int {
	rank := 0
	if _, ok := boosts[strings.ToLower(strings.TrimSpace(candidate.SubphaseID))]; !ok {
		rank += 1000
	}
	if activeFirst {
		rank += candidateBucket(candidate) * 10
	}
	rank += candidatePriorityTie(candidate.Priority)

	return rank
}

func candidateBucket(candidate Candidate) int {
	switch {
	case strings.EqualFold(strings.TrimSpace(candidate.Priority), "P0"):
		return candidateBucketP0
	case candidate.Status == "in_progress":
		return candidateBucketInProgress
	case candidate.ContractStatus == "fixture_ready":
		return candidateBucketFixtureReady
	case len(candidate.Unblocks) > 0:
		return candidateBucketUnblocks
	case candidate.ContractStatus == "draft":
		return candidateBucketDraft
	case candidate.Status == "planned":
		return candidateBucketPlanned
	default:
		return candidateBucketOther
	}
}

func agentQueueCandidate(candidate Candidate) bool {
	return strings.TrimSpace(candidate.Contract) != "" && candidateBucket(candidate) <= candidateBucketDraft
}

func candidateSortKey(candidate Candidate) string {
	return candidate.PhaseID + "/" + candidate.SubphaseID + "/" + candidate.ItemName
}

func candidatePriorityTie(priority string) int {
	normalized := strings.ToUpper(strings.TrimSpace(priority))
	if len(normalized) < 2 || normalized[0] != 'P' {
		return 9
	}
	value, err := strconv.Atoi(normalized[1:])
	if err != nil || value < 0 || value > 9 {
		return 9
	}
	return value
}

func completedItemSet(progress progressJSON) map[string]struct{} {
	completed := make(map[string]struct{})
	for _, phase := range progress.Phases {
		for _, subphase := range phase.allSubphases() {
			for _, item := range subphase.Items {
				if !strings.EqualFold(strings.TrimSpace(item.Status), "complete") {
					continue
				}
				name := firstNonEmpty(item.ItemName, item.Name, item.Title, item.ID)
				for _, key := range blockerKeys(phase.ID, subphase.ID, name) {
					completed[key] = struct{}{}
				}
			}
		}
	}
	return completed
}

func blockersComplete(blockers []string, completed map[string]struct{}) bool {
	for _, blocker := range blockers {
		key := strings.ToLower(strings.TrimSpace(blocker))
		if key == "" {
			continue
		}
		if _, ok := completed[key]; !ok {
			return false
		}
	}
	return true
}

func blockerKeys(phaseID, subphaseID, itemName string) []string {
	phaseID = strings.TrimSpace(phaseID)
	subphaseID = strings.TrimSpace(subphaseID)
	itemName = strings.TrimSpace(itemName)
	if itemName == "" {
		return nil
	}

	var keys []string
	for _, key := range []string{itemName} {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized != "" {
			keys = append(keys, normalized)
		}
	}
	if subphaseID != "" {
		keys = append(keys, strings.ToLower(subphaseID+"/"+itemName))
	}
	if phaseID != "" && subphaseID != "" {
		keys = append(keys, strings.ToLower(phaseID+"/"+subphaseID+"/"+itemName))
	}
	return keys
}
