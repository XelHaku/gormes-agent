// Package plannertriggers is the shared bridge between the builderloop
// package (which appends trigger events when row health state transitions
// matter to the planner) and the plannerloop package (which consumes
// those events on its next run via a persisted cursor).
//
// The package exists as a separate, lower-level package on purpose:
// plannerloop already imports builderloop (for the LedgerEvent type and
// the Runner/ExecRunner shapes), so builderloop cannot import plannerloop
// without creating an import cycle. Both packages can safely depend on
// plannertriggers, which depends on neither.
package plannertriggers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// TriggerEvent is one event in the builder-loop->planner triggers.jsonl
// ledger. The builder loop appends; the planner reads on its next run and
// advances a TriggerCursor past the consumed IDs.
type TriggerEvent struct {
	ID            string `json:"id"`     // ULID-ish: timestamp + monotonic counter
	TS            string `json:"ts"`     // RFC3339 emission timestamp
	Source        string `json:"source"` // "builder-loop" (legacy "autoloop" still accepted by readers)
	Kind          string `json:"kind"`   // "quarantine_added" | "quarantine_stale_cleared" | "manual"
	PhaseID       string `json:"phase_id,omitempty"`
	SubphaseID    string `json:"subphase_id,omitempty"`
	ItemName      string `json:"item_name,omitempty"`
	Reason        string `json:"reason,omitempty"`
	AutoloopRunID string `json:"autoloop_run_id,omitempty"`
}

// TriggerCursor is the planner's bookmark in triggers.jsonl. After
// consuming events the planner persists the last-seen ID so the next run
// only processes newer events.
type TriggerCursor struct {
	LastConsumedID string `json:"last_consumed_id"`
	LastReadAt     string `json:"last_read_at"`
}

// triggerIDCounter is a process-monotonic sequence used to disambiguate
// IDs created within the same millisecond. The format combines a
// millisecond-precision UTC timestamp with this counter so a sort by ID
// matches append order across processes well enough for the cursor
// equality check below.
var triggerIDCounter atomic.Uint64

// AppendTriggerEvent atomically appends one TriggerEvent to the JSONL
// ledger at path. If event.ID is empty a process-monotonic ID is
// generated; if event.TS is empty it defaults to time.Now in UTC; if
// event.Source is empty it defaults to "builder-loop". The parent
// directory is created if missing.
func AppendTriggerEvent(path string, event TriggerEvent) error {
	if event.ID == "" {
		now := time.Now().UTC()
		seq := triggerIDCounter.Add(1)
		event.ID = fmt.Sprintf("%s-%06d", now.Format("20060102T150405.000Z"), seq)
	}
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Source == "" {
		event.Source = "builder-loop"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir trigger dir: %w", err)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal trigger event: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open triggers: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		return fmt.Errorf("write trigger event: %w", err)
	}
	return nil
}

// ReadTriggersSinceCursor returns events strictly after
// cursor.LastConsumedID in append order. A zero cursor (empty
// LastConsumedID) returns every event. Bad JSON lines are skipped so a
// single corrupt entry can't strand the planner. If the cursor's ID is
// not present in the current file the cursor is treated as stale and all
// events are returned (the file may have been rotated; safer to re-emit
// than to drop signal).
func ReadTriggersSinceCursor(path string, cursor TriggerCursor) ([]TriggerEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []TriggerEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev TriggerEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		all = append(all, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if cursor.LastConsumedID == "" {
		return all, nil
	}
	for i, ev := range all {
		if ev.ID == cursor.LastConsumedID {
			return all[i+1:], nil
		}
	}
	// Cursor not found in current file; treat as stale and return all.
	return all, nil
}

// LoadCursor reads a TriggerCursor from path. A missing file is not an
// error; the zero-value cursor is returned (planner treats this as
// "consume everything").
func LoadCursor(path string) (TriggerCursor, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TriggerCursor{}, nil
		}
		return TriggerCursor{}, err
	}
	var c TriggerCursor
	if err := json.Unmarshal(body, &c); err != nil {
		return TriggerCursor{}, err
	}
	return c, nil
}

// SaveCursor atomically writes the cursor via temp + rename so a crash
// mid-write can't leave a partial cursor on disk.
func SaveCursor(path string, cursor TriggerCursor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cursor-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
