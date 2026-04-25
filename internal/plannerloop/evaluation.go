package plannerloop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// DefaultEvaluationWindow is the default lookback for self-evaluation.
// Seven days mirrors AutoloopAudit's window so the planner sees a
// consistent horizon across both correlation surfaces.
const DefaultEvaluationWindow = 7 * 24 * time.Hour

// ReshapeOutcome correlates one planner-recorded RowChange{Kind:"spec_changed"}
// with what autoloop did to that row in subsequent runs. The planner threads
// these into its next prompt as observational signal: rows that resisted
// reshape are candidates for a different decomposition or escalation to
// "needs_human" via PlannerVerdict (L5).
type ReshapeOutcome struct {
	PhaseID            string `json:"phase_id"`
	SubphaseID         string `json:"subphase_id"`
	ItemName           string `json:"item_name"`
	ReshapedAt         string `json:"reshaped_at"`
	ReshapedBy         string `json:"reshaped_by"`
	Outcome            string `json:"outcome"` // "unstuck" | "still_failing" | "no_attempts_yet"
	AutoloopRuns       int    `json:"autoloop_runs"`
	LastFailure        string `json:"last_failure,omitempty"`
	LastSuccess        string `json:"last_success,omitempty"`
	StaleClearObserved bool   `json:"stale_clear_observed"`
}

// Evaluate walks the planner ledger window, collects every row reshape, and
// correlates each with the autoloop ledger to determine the outcome. When
// the autoloop ledger is missing, every reshape is reported as
// "no_attempts_yet" rather than failing the evaluation — the planner would
// rather see partial signal than abort.
func Evaluate(plannerLedgerPath, autoloopLedgerPath string, window time.Duration, now time.Time) ([]ReshapeOutcome, error) {
	plannerEvents, err := LoadLedgerWindow(plannerLedgerPath, window, now)
	if err != nil {
		return nil, fmt.Errorf("evaluate: load planner ledger: %w", err)
	}

	autoloopEvents, err := loadAutoloopLedgerLite(autoloopLedgerPath, window, now)
	if err != nil {
		// Don't fail evaluation if autoloop ledger is missing or unreadable;
		// treat as no attempts.
		autoloopEvents = nil
	}

	type latestReshape struct {
		event  LedgerEvent
		change RowChange
	}
	// If a row was reshaped multiple times in the window, only the latest
	// reshape is reported (older ones are superseded). Keyed by the
	// canonical "phase/subphase/item" string used by autoloop's task field.
	latest := map[string]latestReshape{}
	for _, ev := range plannerEvents {
		evTS, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil {
			continue
		}
		for _, rc := range ev.RowsChanged {
			if rc.Kind != "spec_changed" {
				continue
			}
			key := rc.PhaseID + "/" + rc.SubphaseID + "/" + rc.ItemName
			if existing, ok := latest[key]; ok {
				existingTS, err := time.Parse(time.RFC3339, existing.event.TS)
				if err == nil && !evTS.After(existingTS) {
					continue
				}
			}
			latest[key] = latestReshape{event: ev, change: rc}
		}
	}

	var out []ReshapeOutcome
	for _, reshape := range latest {
		reshapeTS, err := time.Parse(time.RFC3339, reshape.event.TS)
		if err != nil {
			continue
		}
		taskKey := reshape.change.PhaseID + "/" + reshape.change.SubphaseID + "/" + reshape.change.ItemName
		outcome := classifyOutcome(taskKey, reshapeTS, autoloopEvents)
		out = append(out, ReshapeOutcome{
			PhaseID:            reshape.change.PhaseID,
			SubphaseID:         reshape.change.SubphaseID,
			ItemName:           reshape.change.ItemName,
			ReshapedAt:         reshape.event.TS,
			ReshapedBy:         reshape.event.RunID,
			Outcome:            outcome.kind,
			AutoloopRuns:       outcome.runs,
			LastFailure:        outcome.lastFailure,
			LastSuccess:        outcome.lastSuccess,
			StaleClearObserved: outcome.staleClearObserved,
		})
	}
	return out, nil
}

// autoloopEventLite is a local subset of autoloop's LedgerEvent decoded only
// for evaluation. The lite struct keeps this package decoupled from
// internal/builderloop and tolerant of schema drift: encoding/json silently
// ignores unknown fields.
type autoloopEventLite struct {
	TS     string `json:"ts"`
	Event  string `json:"event"`
	Task   string `json:"task"`
	Status string `json:"status"`
}

type outcomeClass struct {
	kind               string
	runs               int
	lastFailure        string
	lastSuccess        string
	staleClearObserved bool
}

// classifyOutcome inspects every autoloop event recorded for taskKey AFTER
// the reshape timestamp and decides:
//   - any worker_promoted -> "unstuck" (LastSuccess = that event's ts)
//   - else any worker_failed/worker_error -> "still_failing"
//     (count = AutoloopRuns; LastFailure = status from latest failure)
//   - else -> "no_attempts_yet"
//
// quarantine_stale_cleared (or status="stale_quarantine_cleared") sets
// StaleClearObserved as a side signal regardless of the bucket.
func classifyOutcome(taskKey string, reshapeTS time.Time, events []autoloopEventLite) outcomeClass {
	var runs int
	var lastFailure, lastSuccess string
	var staleClear bool
	var promoted bool
	for _, ev := range events {
		if !taskMatches(ev.Task, taskKey) {
			continue
		}
		evTS, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil || !evTS.After(reshapeTS) {
			continue
		}
		runs++
		switch ev.Event {
		case "worker_promoted":
			promoted = true
			lastSuccess = ev.TS
		case "worker_failed", "worker_error":
			lastFailure = ev.Status
		case "backend_degraded":
			// not row-level; ignore
		}
		if ev.Status == "stale_quarantine_cleared" || ev.Event == "quarantine_stale_cleared" {
			staleClear = true
		}
	}
	if promoted {
		return outcomeClass{kind: "unstuck", runs: runs, lastSuccess: lastSuccess, staleClearObserved: staleClear}
	}
	if runs > 0 {
		return outcomeClass{kind: "still_failing", runs: runs, lastFailure: lastFailure, staleClearObserved: staleClear}
	}
	return outcomeClass{kind: "no_attempts_yet"}
}

// taskMatches returns true when autoloop's "task" field refers to the same
// row as the planner's "phase/subphase/item" key. Autoloop encodes the task
// in several historical formats (bare key, prefixed with "phase/sub/"), so
// we accept exact match OR substring containment to stay tolerant.
func taskMatches(autoloopTask, taskKey string) bool {
	if autoloopTask == taskKey {
		return true
	}
	return strings.Contains(autoloopTask, taskKey)
}

// loadAutoloopLedgerLite reads autoloop's runs.jsonl, decoding only the
// fields evaluation cares about. Schema differences from the planner
// ledger are tolerated (unknown fields are ignored by encoding/json).
// Returns events within [now-window, now] inclusive; bad timestamps are
// skipped silently.
func loadAutoloopLedgerLite(path string, window time.Duration, now time.Time) ([]autoloopEventLite, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cutoff := now.Add(-window)
	var out []autoloopEventLite
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev autoloopEventLite
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil {
			continue
		}
		if t.Before(cutoff) || t.After(now) {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}
