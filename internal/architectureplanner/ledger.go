package architectureplanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LedgerEvent is one entry in the planner runs.jsonl ledger.
type LedgerEvent struct {
	TS            string        `json:"ts"` // RFC3339
	RunID         string        `json:"run_id"`
	Trigger       string        `json:"trigger"` // "scheduled" | "event" | "manual" | "retry"
	TriggerEvents []string      `json:"trigger_events,omitempty"`
	Backend       string        `json:"backend"`
	Mode          string        `json:"mode"`
	Status        string        `json:"status"` // "ok" | "validation_rejected" | "backend_failed" | "no_changes" | "needs_human_set"
	Detail        string        `json:"detail,omitempty"`
	BeforeStats   ProgressStats `json:"before_stats,omitempty"`
	AfterStats    ProgressStats `json:"after_stats,omitempty"`
	RowsChanged   []RowChange   `json:"rows_changed,omitempty"`
	RetryAttempt  int           `json:"retry_attempt,omitempty"`
	// Attempts records every LLM call lifecycle within this RunOnce
	// invocation. Populated by the L3 retry-with-feedback loop so
	// operators can audit which attempt failed and which rows were
	// dropped at each step. The final entry's Index matches RetryAttempt.
	// Empty for runs that never reached the LLM (dry-run, sync-only,
	// pre-L3 single-attempt code paths kept for backward compatibility).
	Attempts []retryAttempt `json:"attempts,omitempty"`
	Keywords []string       `json:"keywords,omitempty"` // L6 topical focus
}

// RowChange records one mutation to a progress.json row in a planner run.
type RowChange struct {
	PhaseID    string `json:"phase_id"`
	SubphaseID string `json:"subphase_id"`
	ItemName   string `json:"item_name"`
	Kind       string `json:"kind"` // "added" | "deleted" | "spec_changed" | "verdict_set"
	Detail     string `json:"detail,omitempty"`
}

// ProgressStats is a snapshot of progress.json composition at a point in time.
type ProgressStats struct {
	Shipped     int `json:"shipped,omitempty"`
	InProgress  int `json:"in_progress,omitempty"`
	Planned     int `json:"planned,omitempty"`
	Quarantined int `json:"quarantined,omitempty"`
	NeedsHuman  int `json:"needs_human,omitempty"`
}

// AppendLedgerEvent atomically appends one event as a single JSON line.
// Uses O_APPEND|O_CREATE|O_WRONLY for POSIX-atomic line writes (lines under
// PIPE_BUF (4096 bytes on Linux) are atomic per the syscall contract).
func AppendLedgerEvent(path string, event LedgerEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir ledger dir: %w", err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal ledger event: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()
	_, err = f.Write(body)
	return err
}

// LoadLedger reads all events from the ledger file. Bad lines are logged
// and skipped; they do not abort the load.
func LoadLedger(path string) ([]LedgerEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []LedgerEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1 MiB per line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event LedgerEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// Skip corrupt lines; do not propagate.
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return events, err
	}
	return events, nil
}

// LoadLedgerWindow returns events within [now-window, now] inclusive. Bad
// timestamps are skipped.
func LoadLedgerWindow(path string, window time.Duration, now time.Time) ([]LedgerEvent, error) {
	all, err := LoadLedger(path)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-window)
	out := []LedgerEvent{}
	for _, ev := range all {
		t, err := time.Parse(time.RFC3339, ev.TS)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) && !t.After(now) {
			out = append(out, ev)
		}
	}
	return out, nil
}
