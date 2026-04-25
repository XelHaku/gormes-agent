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

func (candidate Candidate) SelectionReason() string {
	switch candidateBucket(candidate) {
	case candidateBucketP0:
		return "P0 handoff"
	case candidateBucketInProgress:
		return "already active"
	case candidateBucketFixtureReady:
		return "fixture ready"
	case candidateBucketUnblocks:
		return "unblocks downstream work"
	case candidateBucketDraft:
		return "draft contract"
	default:
		return "planned row"
	}
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

				blockedBy := trimStringSlice(item.BlockedBy)
				sliceSize := lowerTrim(item.SliceSize)
				if !opts.IncludeBlocked && len(blockedBy) > 0 {
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
					Priority:       strings.TrimSpace(item.Priority),
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
	ID    string         `json:"id"`
	Items []progressItem `json:"items"`
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
		rank += 100
	}
	if activeFirst {
		rank += candidateBucket(candidate)
	}

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
