package insights

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// UsageWriter appends daily insights records to usage.jsonl.
type UsageWriter struct {
	path string
	mu   sync.Mutex
}

func NewUsageWriter(path string) *UsageWriter {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &UsageWriter{path: path}
}

func (w *UsageWriter) Record(rollup DailyRollup) error {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}

	raw, err := json.Marshal(normalizeRollup(rollup))
	if err != nil {
		return err
	}

	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func normalizeRollup(rollup DailyRollup) DailyRollup {
	rollup.Date = strings.TrimSpace(rollup.Date)
	if rollup.SessionCount < 0 {
		rollup.SessionCount = 0
	}
	if rollup.TotalTokensIn < 0 {
		rollup.TotalTokensIn = 0
	}
	if rollup.TotalTokensOut < 0 {
		rollup.TotalTokensOut = 0
	}
	rollup.EstimatedCostUSD = roundUSD(rollup.EstimatedCostUSD)
	if rollup.ModelBreakdown == nil {
		rollup.ModelBreakdown = map[string]int{}
	}
	return rollup
}
