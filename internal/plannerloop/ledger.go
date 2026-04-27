package plannerloop

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LedgerEvent is one entry in the planner runs.jsonl ledger.
type LedgerEvent struct {
	TS            string        `json:"ts"` // RFC3339
	RunID         string        `json:"run_id"`
	Event         string        `json:"event,omitempty"` // optional non-terminal observation, e.g. PR intake or backend progress
	Trigger       string        `json:"trigger"`         // "scheduled" | "event" | "manual" | "retry"
	TriggerEvents []string      `json:"trigger_events,omitempty"`
	Backend       string        `json:"backend"`
	Mode          string        `json:"mode"`
	Status        string        `json:"status"` // "ok" | "validation_rejected" | "validation_failed" | "backend_failed" | "no_changes" | "needs_human_set"
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
	// DriftPromotions records subphase DriftState forward transitions (Phase D
	// Task 5). Only forward edges are recorded — porting→converged,
	// porting→owned, converged→owned. Backward transitions are logged but not
	// emitted here (humans demote via direct edit; planner runtime never
	// demotes). Empty when no subphase DriftState changed in the run.
	DriftPromotions []DriftPromotion `json:"drift_promotions,omitempty"`
}

// RowChange records one mutation to a progress.json row in a planner run.
type RowChange struct {
	PhaseID    string `json:"phase_id"`
	SubphaseID string `json:"subphase_id"`
	ItemName   string `json:"item_name"`
	Kind       string `json:"kind"` // "added" | "deleted" | "spec_changed" | "verdict_set"
	Detail     string `json:"detail,omitempty"`
}

// DriftPromotion records one forward transition of a subphase's DriftState
// status in a planner run. Forward edges only: porting→converged,
// porting→owned, converged→owned. Used by the L5 status surface to show
// "Recent drift promotions" and by ledger forensics for the convergence
// lifecycle audit. From and To are the DriftState.Status values; SubphaseID
// is "phaseID.subphaseID" (e.g. "2.B").
type DriftPromotion struct {
	SubphaseID string `json:"subphase_id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Reason     string `json:"reason,omitempty"`
}

// ProgressStats is a snapshot of progress.json composition at a point in time.
type ProgressStats struct {
	Shipped     int `json:"shipped,omitempty"`
	InProgress  int `json:"in_progress,omitempty"`
	Planned     int `json:"planned,omitempty"`
	Quarantined int `json:"quarantined,omitempty"`
	NeedsHuman  int `json:"needs_human,omitempty"`
}

// AppendLedgerEvent appends one event as a single JSON line.
//
// The lock and newline repair make appends robust across multiple planner
// goroutines/processes and after hard kills that can leave a trailing partial
// line. Corrupt partial lines move to a .corrupt sidecar for forensics so the
// main ledger remains parseable by readers and shell tooling.
func AppendLedgerEvent(path string, event LedgerEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir ledger dir: %w", err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal ledger event: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ledger: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()
	if err := ensureLedgerLineBoundary(path, f); err != nil {
		return err
	}
	return writeAll(f, body)
}

func ensureLedgerLineBoundary(path string, f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat ledger: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], info.Size()-1); err != nil {
		return fmt.Errorf("read ledger tail: %w", err)
	}
	if last[0] == '\n' {
		return nil
	}
	start, fragment, err := trailingLedgerFragment(f, info.Size())
	if err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(fragment)
	if len(trimmed) > 0 && !json.Valid(trimmed) {
		if err := appendCorruptLedgerFragment(path+".corrupt", fragment); err != nil {
			return err
		}
		if err := f.Truncate(start); err != nil {
			return fmt.Errorf("truncate corrupt ledger tail: %w", err)
		}
		return nil
	}
	if err := writeAll(f, []byte("\n")); err != nil {
		return fmt.Errorf("repair ledger line boundary: %w", err)
	}
	return nil
}

func trailingLedgerFragment(f *os.File, size int64) (int64, []byte, error) {
	const chunkSize int64 = 64 * 1024
	end := size
	for end > 0 {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil {
			return 0, nil, fmt.Errorf("read ledger fragment: %w", err)
		}
		if idx := bytes.LastIndexByte(buf, '\n'); idx >= 0 {
			fragmentStart := start + int64(idx) + 1
			return fragmentStart, buf[idx+1:], nil
		}
		end = start
	}
	buf := make([]byte, size)
	if size > 0 {
		if _, err := f.ReadAt(buf, 0); err != nil {
			return 0, nil, fmt.Errorf("read ledger fragment: %w", err)
		}
	}
	return 0, buf, nil
}

func appendCorruptLedgerFragment(path string, fragment []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir corrupt ledger dir: %w", err)
	}
	event := struct {
		TS       string `json:"ts"`
		Reason   string `json:"reason"`
		Fragment string `json:"fragment"`
	}{
		TS:       time.Now().UTC().Format(time.RFC3339Nano),
		Reason:   "trailing_partial_line",
		Fragment: string(fragment),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal corrupt ledger fragment: %w", err)
	}
	body = append(body, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open corrupt ledger: %w", err)
	}
	defer f.Close()
	return writeAll(f, body)
}

func writeAll(f *os.File, body []byte) error {
	for len(body) > 0 {
		n, err := f.Write(body)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		body = body[n:]
	}
	return nil
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
