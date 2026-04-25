package builderloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type LedgerEvent struct {
	TS     time.Time `json:"ts"`
	RunID  string    `json:"run_id,omitempty"`
	Event  string    `json:"event"`
	Worker int       `json:"worker,omitempty"`
	Task   string    `json:"task,omitempty"`
	Branch string    `json:"branch,omitempty"`
	Commit string    `json:"commit,omitempty"`
	Status string    `json:"status,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

func (event *LedgerEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		TS     time.Time       `json:"ts"`
		RunID  string          `json:"run_id,omitempty"`
		Event  string          `json:"event"`
		Worker int             `json:"worker,omitempty"`
		Task   string          `json:"task,omitempty"`
		Branch string          `json:"branch,omitempty"`
		Commit string          `json:"commit,omitempty"`
		Status string          `json:"status,omitempty"`
		Detail json.RawMessage `json:"detail,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*event = LedgerEvent{
		TS:     raw.TS,
		RunID:  raw.RunID,
		Event:  raw.Event,
		Worker: raw.Worker,
		Task:   raw.Task,
		Branch: raw.Branch,
		Commit: raw.Commit,
		Status: raw.Status,
	}
	if len(raw.Detail) == 0 || string(raw.Detail) == "null" {
		return nil
	}
	var detail string
	if err := json.Unmarshal(raw.Detail, &detail); err == nil {
		event.Detail = detail
		return nil
	}
	event.Detail = string(raw.Detail)
	return nil
}

func AppendLedgerEvent(path string, event LedgerEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(event)
}
