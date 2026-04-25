package cron

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
)

func TestExecutorRecordsCronRunInDurableLedger(t *testing.T) {
	fk := newFakeKernel("durable report", 0)
	e, _, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(context.Background())
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}
	e.cfg.DurableLedger = ledger

	job := NewJob("morning", "0 8 * * *", "status summary")
	if err := e.cfg.JobStore.Create(job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	e.Run(context.Background(), job)

	fk.mu.Lock()
	if len(fk.events) != 1 {
		t.Fatalf("kernel events = %d, want 1", len(fk.events))
	}
	ledgerID := fk.events[0].SessionID
	fk.mu.Unlock()

	got, err := ledger.Get(context.Background(), ledgerID)
	if err != nil {
		t.Fatalf("ledger Get(%q): %v", ledgerID, err)
	}
	if got.Kind != subagent.WorkKindCronJob || got.Status != subagent.DurableJobCompleted {
		t.Fatalf("ledger kind/status = %q/%q, want cron_job/completed", got.Kind, got.Status)
	}
	assertCronJSONEqual(t, "progress", got.Progress, `{"cron_job_id":"`+job.ID+`","job_name":"morning","phase":"submitted","prompt_hash":"`+shortHash(job.Prompt)+`"}`)
	assertCronJSONEqual(t, "result", got.Result, `{"delivered":true,"status":"success"}`)
	if got.StartedAt.IsZero() || got.FinishedAt.IsZero() {
		t.Fatalf("durable timestamps not populated: %+v", got)
	}
}

func assertCronJSONEqual(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	var gotAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatalf("%s unmarshal got: %v\nraw=%s", name, err, got)
	}
	var wantAny any
	if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
		t.Fatalf("%s unmarshal want: %v", name, err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		gotJSON, _ := json.Marshal(gotAny)
		wantJSON, _ := json.Marshal(wantAny)
		t.Fatalf("%s JSON = %s, want %s", name, gotJSON, wantJSON)
	}
}
