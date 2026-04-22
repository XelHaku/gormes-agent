package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type UsageLogger struct {
	path string
	mu   sync.Mutex
}

type usageRecord struct {
	SkillName string    `json:"skill_name"`
	UsedAt    time.Time `json:"used_at"`
}

func NewUsageLogger(path string) *UsageLogger {
	if path == "" {
		return nil
	}
	return &UsageLogger{path: path}
}

func (l *UsageLogger) Record(ctx context.Context, skillNames []string) error {
	if l == nil || len(skillNames) == 0 {
		return nil
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

	for _, name := range skillNames {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		raw, err := json.Marshal(usageRecord{
			SkillName: name,
			UsedAt:    time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return err
		}
	}
	return nil
}
