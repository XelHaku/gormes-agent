package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageLoggerAppendsOneJSONLRecordPerSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	logger := NewUsageLogger(path)

	if err := logger.Record(context.Background(), []string{"careful-review"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], `"skill_name":"careful-review"`) {
		t.Fatalf("usage line = %q, want skill name", lines[0])
	}
}

func TestUsageLoggerSkipsEmptySelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	logger := NewUsageLogger(path)

	if err := logger.Record(context.Background(), nil); err != nil {
		t.Fatalf("Record(nil) error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("usage file exists after empty record, stat err = %v", err)
	}
}
