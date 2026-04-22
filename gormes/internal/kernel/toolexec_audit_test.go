package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestExecuteToolCalls_AppendsAuditRecordsForSuccessAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools", "audit.jsonl")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr: "echo",
		ExecuteFn: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"text":"hi"}`), nil
		},
	})
	reg.MustRegister(&tools.MockTool{
		NameStr: "boom",
		ExecuteFn: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("synthetic failure")
		},
	})

	k := New(Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		InitialSessionID:  "sess_123",
		ToolAudit:         audit.NewJSONLWriter(path),
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)

	res := k.executeToolCalls(context.Background(), []hermes.ToolCall{
		{ID: "call_1", Name: "echo", Arguments: json.RawMessage(`{"text":"hi"}`)},
		{ID: "call_2", Name: "boom", Arguments: json.RawMessage(`{"task":"fail"}`)},
	})
	if len(res) != 2 {
		t.Fatalf("result count = %d, want 2", len(res))
	}
	if !strings.Contains(res[1].Content, "synthetic failure") {
		t.Fatalf("failed tool content = %q, want synthetic failure", res[1].Content)
	}

	records := readAuditRecords(t, path)
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}

	if records[0].Source != "kernel" {
		t.Fatalf("records[0].source = %q, want %q", records[0].Source, "kernel")
	}
	if records[0].SessionID != "sess_123" {
		t.Fatalf("records[0].session_id = %q, want %q", records[0].SessionID, "sess_123")
	}
	if records[0].Tool != "echo" {
		t.Fatalf("records[0].tool = %q, want %q", records[0].Tool, "echo")
	}
	if records[0].Status != "completed" {
		t.Fatalf("records[0].status = %q, want %q", records[0].Status, "completed")
	}
	if records[0].DurationMs < 0 {
		t.Fatalf("records[0].duration_ms = %d, want >= 0", records[0].DurationMs)
	}
	if records[0].Error != "" {
		t.Fatalf("records[0].error = %q, want empty", records[0].Error)
	}

	if records[1].Tool != "boom" {
		t.Fatalf("records[1].tool = %q, want %q", records[1].Tool, "boom")
	}
	if records[1].Status != "failed" {
		t.Fatalf("records[1].status = %q, want %q", records[1].Status, "failed")
	}
	if records[1].DurationMs < 0 {
		t.Fatalf("records[1].duration_ms = %d, want >= 0", records[1].DurationMs)
	}
	if !strings.Contains(records[1].Error, "synthetic failure") {
		t.Fatalf("records[1].error = %q, want synthetic failure", records[1].Error)
	}
}

func readAuditRecords(t *testing.T, path string) []audit.Record {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	out := make([]audit.Record, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var rec audit.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("Unmarshal(%s): %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}
