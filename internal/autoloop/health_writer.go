package autoloop

import (
	"strconv"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// SpecHashProvider returns the current spec hash for a row, used when a
// Flush triggers a new quarantine (the SpecHash is stamped at quarantine
// creation so future selection passes can detect spec drift). Pass nil
// when the caller has no live progress doc to hash from.
type SpecHashProvider func(phaseID, subphaseID, itemName string) string

type rowKey struct {
	phaseID    string
	subphaseID string
	itemName   string
}

type pendingHealth struct {
	successes    int
	failures     int
	lastCategory progress.FailureCategory
	lastBackend  string
	lastStderr   string
	staleClear   bool
	contract     string // captured for diagnostics
}

type healthAccumulator struct {
	runID     string
	now       func() time.Time
	threshold int
	rows      map[rowKey]*pendingHealth
}

func newHealthAccumulator(runID string, now func() time.Time, threshold int) *healthAccumulator {
	if threshold <= 0 {
		threshold = 3
	}
	return &healthAccumulator{
		runID:     runID,
		now:       now,
		threshold: threshold,
		rows:      map[rowKey]*pendingHealth{},
	}
}

func (a *healthAccumulator) get(c Candidate) *pendingHealth {
	key := rowKey{c.PhaseID, c.SubphaseID, c.ItemName}
	p, ok := a.rows[key]
	if !ok {
		p = &pendingHealth{contract: c.Contract}
		a.rows[key] = p
	}
	return p
}

// RecordSuccess marks one successful worker outcome for the candidate.
func (a *healthAccumulator) RecordSuccess(c Candidate) {
	a.get(c).successes++
}

// RecordFailure marks one failed worker outcome for the candidate.
func (a *healthAccumulator) RecordFailure(c Candidate, cat progress.FailureCategory, backend, stderrTail string) {
	p := a.get(c)
	p.failures++
	p.lastCategory = cat
	p.lastBackend = backend
	p.lastStderr = capStderrTail(stderrTail, 2048)
}

// MarkStaleQuarantine records that L3 selection treated this candidate as
// stale-quarantined (spec hash mismatch). Used at flush to clear the block
// and reset ConsecutiveFailures so the planner-repaired row gets fresh runway.
func (a *healthAccumulator) MarkStaleQuarantine(c Candidate) {
	a.get(c).staleClear = true
}

// Flush applies all accumulated mutations to progress.json in one batched
// write. The mutate closures own all the quarantine math; this is the single
// place where ConsecutiveFailures is incremented or reset. Pass hashOf=nil
// when no live progress doc is available (Quarantine.SpecHash will be empty
// in that case; selection's stale-clear logic will then mark such quarantines
// stale on the next pass since SpecHash="" rarely matches a real ItemSpecHash).
func (a *healthAccumulator) Flush(progressPath string, hashOf SpecHashProvider) error {
	if len(a.rows) == 0 {
		return nil
	}

	now := a.now().UTC().Format(time.RFC3339)
	updates := make([]progress.HealthUpdate, 0, len(a.rows))
	for key, pending := range a.rows {
		p := pending
		k := key
		updates = append(updates, progress.HealthUpdate{
			PhaseID:    k.phaseID,
			SubphaseID: k.subphaseID,
			ItemName:   k.itemName,
			Mutate: func(h *progress.RowHealth) {
				a.applyMutation(h, p, k, now, hashOf)
			},
		})
	}

	return progress.ApplyHealthUpdates(progressPath, updates)
}

func (a *healthAccumulator) applyMutation(h *progress.RowHealth, p *pendingHealth, k rowKey, now string, hashOf SpecHashProvider) {
	// Stale-quarantine clear: reset both block and counter, do NOT touch LastSuccess.
	if p.staleClear && h.Quarantine != nil {
		h.Quarantine = nil
		h.ConsecutiveFailures = 0
	}

	if p.failures > 0 {
		h.AttemptCount += p.failures
		h.LastAttempt = now
		h.LastFailure = &progress.FailureSummary{
			RunID:      a.runID,
			Category:   p.lastCategory,
			Backend:    p.lastBackend,
			StderrTail: p.lastStderr,
		}
		if p.lastBackend != "" && !containsBackend(h.BackendsTried, p.lastBackend) {
			h.BackendsTried = append(h.BackendsTried, p.lastBackend)
		}
	}

	if p.successes > 0 {
		h.AttemptCount += p.successes
		h.LastAttempt = now
		h.LastSuccess = now
		h.ConsecutiveFailures = 0
		h.Quarantine = nil
		return
	}

	if p.failures > 0 {
		h.ConsecutiveFailures += p.failures
	}

	if h.Quarantine == nil && h.ConsecutiveFailures >= a.threshold && p.failures > 0 {
		specHash := ""
		if hashOf != nil {
			specHash = hashOf(k.phaseID, k.subphaseID, k.itemName)
		}
		h.Quarantine = &progress.Quarantine{
			Reason:       quarantineReason(h.ConsecutiveFailures, p.lastCategory),
			Since:        now,
			AfterRunID:   a.runID,
			Threshold:    a.threshold,
			SpecHash:     specHash,
			LastCategory: p.lastCategory,
		}
	}
}

func containsBackend(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func capStderrTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func quarantineReason(consecutive int, cat progress.FailureCategory) string {
	if cat == "" {
		return "auto: " + strconv.Itoa(consecutive) + " consecutive failures"
	}
	return "auto: " + strconv.Itoa(consecutive) + " consecutive failures, last category " + string(cat)
}
