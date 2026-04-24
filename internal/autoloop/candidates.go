package autoloop

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

type CandidateOptions struct {
	ActiveFirst     bool
	PriorityBoost   []string
	MaxPhase        int
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

func NormalizeCandidates(path string, opts CandidateOptions) ([]Candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var progress progressJSON
	if err := json.Unmarshal(data, &progress); err != nil {
		return nil, err
	}

	completed := completedItemSet(progress)
	var candidates []Candidate
	seen := make(map[string]struct{})
	for _, phase := range progress.Phases {
		if phaseAboveMax(phase.ID, opts.MaxPhase) {
			continue
		}
		for _, subphase := range phase.allSubphases() {
			for _, item := range subphase.Items {
				name := firstNonEmpty(item.ItemName, item.Name, item.Title, item.ID)
				if name == "" {
					continue
				}

				status := strings.ToLower(strings.TrimSpace(item.Status))
				if status == "" {
					status = "unknown"
				}
				if status == "complete" {
					continue
				}
				if len(item.BlockedBy) > 0 && !opts.IncludeBlocked && !blockersComplete(item.BlockedBy, completed) {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(item.SliceSize), "umbrella") && !opts.IncludeUmbrella {
					continue
				}

				candidate := Candidate{
					PhaseID:        strings.TrimSpace(phase.ID),
					SubphaseID:     strings.TrimSpace(subphase.ID),
					ItemName:       name,
					Status:         status,
					Priority:       strings.TrimSpace(firstNonEmpty(item.Priority, subphase.Priority)),
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
		left := candidateRank(candidates[i], opts.ActiveFirst, boosts)
		right := candidateRank(candidates[j], opts.ActiveFirst, boosts)
		if left != right {
			return left < right
		}

		return candidateSortKey(candidates[i]) < candidateSortKey(candidates[j])
	})

	return candidates, nil
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

func firstNonEmpty(vals ...string) string {
	for _, val := range vals {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

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

func candidateRank(candidate Candidate, activeFirst bool, boosts map[string]struct{}) int {
	rank := 0
	if _, ok := boosts[strings.ToLower(strings.TrimSpace(candidate.SubphaseID))]; !ok {
		rank += 1000
	}

	if activeFirst {
		rank += candidateSelectionBucket(candidate) * 10
	}

	rank += candidatePriorityTie(candidate.Priority)

	return rank
}

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
