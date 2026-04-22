package subagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type runLogRecord struct {
	ID         string       `json:"id"`
	ParentID   string       `json:"parent_id"`
	Depth      int          `json:"depth"`
	Goal       string       `json:"goal"`
	Status     ResultStatus `json:"status"`
	ExitReason string       `json:"exit_reason"`
	DurationMs int64        `json:"duration_ms"`
	Iterations int          `json:"iterations"`
	Error      string       `json:"error,omitempty"`
	FinishedAt time.Time    `json:"finished_at"`
}

type runLogger struct {
	path string
	mu   sync.Mutex
}

func newRunLogger(path string) *runLogger {
	if path == "" {
		return nil
	}
	return &runLogger{path: path}
}

func (l *runLogger) append(sa *Subagent, result *SubagentResult) error {
	record := runLogRecord{
		ID:         result.ID,
		ParentID:   sa.ParentID,
		Depth:      sa.Depth,
		Goal:       sa.cfg.Goal,
		Status:     result.Status,
		ExitReason: result.ExitReason,
		DurationMs: result.Duration.Milliseconds(),
		Iterations: result.Iterations,
		Error:      result.Error,
		FinishedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}
