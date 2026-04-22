package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Recorder captures one tool-execution audit record.
type Recorder interface {
	Record(rec Record) error
}

// Record is one append-only tool-execution audit event.
type Record struct {
	Timestamp       time.Time       `json:"timestamp"`
	Source          string          `json:"source"`
	SessionID       string          `json:"session_id"`
	AgentID         string          `json:"agent_id"`
	Tool            string          `json:"tool"`
	Args            json.RawMessage `json:"args"`
	DurationMs      int64           `json:"duration_ms"`
	Status          string          `json:"status"`
	ResultSizeBytes int             `json:"result_size_bytes"`
	Error           string          `json:"error"`
}

// JSONLWriter appends audit records to a JSONL file.
type JSONLWriter struct {
	path string
	mu   sync.Mutex
}

func NewJSONLWriter(path string) *JSONLWriter {
	return &JSONLWriter{path: strings.TrimSpace(path)}
}

func (w *JSONLWriter) Record(rec Record) error {
	if w == nil || w.path == "" {
		return nil
	}

	rec = normalize(rec)
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func normalize(rec Record) Record {
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	} else {
		rec.Timestamp = rec.Timestamp.UTC()
	}
	rec.Source = strings.TrimSpace(rec.Source)
	rec.SessionID = strings.TrimSpace(rec.SessionID)
	rec.AgentID = strings.TrimSpace(rec.AgentID)
	rec.Tool = strings.TrimSpace(rec.Tool)
	rec.Status = strings.TrimSpace(rec.Status)
	if rec.Status == "" {
		rec.Status = "unknown"
	}
	if rec.DurationMs < 0 {
		rec.DurationMs = 0
	}
	if len(rec.Args) == 0 {
		rec.Args = json.RawMessage(`null`)
	} else {
		rec.Args = append(json.RawMessage(nil), rec.Args...)
	}
	return rec
}
