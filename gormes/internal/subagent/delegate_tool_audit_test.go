package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestDelegateTool_AppendsAuditRecordForDelegatedChildTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools", "audit.jsonl")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})

	client := hermes.NewMockClient()
	client.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "tool_calls", ToolCalls: []hermes.ToolCall{{
			ID:        "call_1",
			Name:      "echo",
			Arguments: json.RawMessage(`{"text":"hi from child"}`),
		}}},
	}, "sid_child_1")
	client.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "done"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sid_child_2")

	mgr := NewManager(ManagerOpts{
		ParentCtx:            context.Background(),
		ParentID:             "parent_test",
		Depth:                0,
		Registry:             NewRegistry(),
		ToolExecutor:         tools.NewInProcessToolExecutor(reg),
		ToolAudit:            audit.NewJSONLWriter(path),
		DefaultTimeout:       time.Second,
		DefaultMaxIterations: 4,
		NewRunner: func() Runner {
			return NewHermesRunner(client, "test-model", []hermes.ToolDescriptor{{
				Name:        "echo",
				Description: "echo",
				Schema:      json.RawMessage(`{"type":"object"}`),
			}})
		},
	})
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"use delegated tool","toolsets":"echo"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	records := readDelegatedAuditRecords(t, path)
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Source != "delegate_task" {
		t.Fatalf("record source = %q, want %q", records[0].Source, "delegate_task")
	}
	if !strings.HasPrefix(records[0].AgentID, "sa_") {
		t.Fatalf("record agent_id = %q, want sa_ prefix", records[0].AgentID)
	}
	if records[0].Tool != "echo" {
		t.Fatalf("record tool = %q, want %q", records[0].Tool, "echo")
	}
	if records[0].Status != "completed" {
		t.Fatalf("record status = %q, want %q", records[0].Status, "completed")
	}
	if records[0].DurationMs < 0 {
		t.Fatalf("record duration_ms = %d, want >= 0", records[0].DurationMs)
	}
	if records[0].Error != "" {
		t.Fatalf("record error = %q, want empty", records[0].Error)
	}
}

func readDelegatedAuditRecords(t *testing.T, path string) []audit.Record {
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
