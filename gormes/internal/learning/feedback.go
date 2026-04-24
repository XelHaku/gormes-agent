package learning

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// neutralPrior is the Laplace-smoothed score for a skill with no observations
// yet: (0+1)/(0+2) = 0.5. Ranking code uses it as the default weight.
const neutralPrior = 0.5

// Outcome is a single feedback sample captured after a turn in which a
// previously-selected skill was exposed to the agent. Success reflects whether
// the turn itself completed successfully while the skill was in scope.
type Outcome struct {
	SkillName  string    `json:"skill_name"`
	Success    bool      `json:"success"`
	SessionID  string    `json:"session_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EffectivenessScore is the aggregate feedback signal for one skill.
//
// Score uses Laplace smoothing (successes+1)/(uses+2) so a brand-new skill
// starts at 0.5 and converges to the observed success ratio as samples grow.
type EffectivenessScore struct {
	SkillName string  `json:"skill_name"`
	Uses      int     `json:"uses"`
	Successes int     `json:"successes"`
	Failures  int     `json:"failures"`
	Score     float64 `json:"score"`
}

// FeedbackStore appends per-skill outcomes as JSONL and derives aggregate
// effectiveness scores from the replayed log. It shares the same append-only,
// operator-auditable shape as the Phase 6.A complexity signal.
type FeedbackStore struct {
	path string
	mu   sync.Mutex
}

// NewFeedbackStore constructs a store that persists to path. An empty path
// makes RecordOutcome a no-op but keeps Weight usable with the neutral prior.
func NewFeedbackStore(path string) *FeedbackStore {
	return &FeedbackStore{path: strings.TrimSpace(path)}
}

// RecordOutcome appends one outcome as a JSONL line. Empty skill names and
// nil receivers are silently ignored so callers can wire the store
// unconditionally.
func (f *FeedbackStore) RecordOutcome(ctx context.Context, out Outcome) error {
	if f == nil {
		return nil
	}
	name := strings.TrimSpace(out.SkillName)
	if name == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if out.OccurredAt.IsZero() {
		out.OccurredAt = time.Now().UTC()
	} else {
		out.OccurredAt = out.OccurredAt.UTC()
	}
	out.SkillName = name

	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	fp, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fp.Close()
	_, err = fp.Write(append(raw, '\n'))
	return err
}

// Scores replays the JSONL log and returns one EffectivenessScore per skill,
// sorted by Score descending with SkillName as a stable tiebreaker. A missing
// log file yields an empty slice without error.
func (f *FeedbackStore) Scores(ctx context.Context) ([]EffectivenessScore, error) {
	aggregates, err := f.aggregate(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]EffectivenessScore, 0, len(aggregates))
	for _, a := range aggregates {
		result = append(result, *a)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].SkillName < result[j].SkillName
	})
	return result, nil
}

// Weight returns the Laplace-smoothed score for one skill name, or the
// neutral prior (0.5) when the skill is unknown, the name is blank, or the
// log cannot be read. Ranking code can multiply this into relevance scores
// without needing to special-case fresh skills.
func (f *FeedbackStore) Weight(ctx context.Context, skillName string) float64 {
	name := strings.TrimSpace(skillName)
	if name == "" {
		return neutralPrior
	}
	aggregates, err := f.aggregate(ctx)
	if err != nil {
		return neutralPrior
	}
	if agg, ok := aggregates[name]; ok {
		return agg.Score
	}
	return neutralPrior
}

func (f *FeedbackStore) aggregate(ctx context.Context) (map[string]*EffectivenessScore, error) {
	if f == nil || f.path == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	file, err := os.Open(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	aggregates := map[string]*EffectivenessScore{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Outcome
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		name := strings.TrimSpace(rec.SkillName)
		if name == "" {
			continue
		}
		agg := aggregates[name]
		if agg == nil {
			agg = &EffectivenessScore{SkillName: name}
			aggregates[name] = agg
		}
		agg.Uses++
		if rec.Success {
			agg.Successes++
		} else {
			agg.Failures++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for _, a := range aggregates {
		a.Score = laplaceSmoothed(a.Successes, a.Uses)
	}
	return aggregates, nil
}

func laplaceSmoothed(successes, uses int) float64 {
	if uses < 0 {
		uses = 0
	}
	if successes < 0 {
		successes = 0
	}
	return float64(successes+1) / float64(uses+2)
}
