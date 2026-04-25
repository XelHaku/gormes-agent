package subagent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

func newTestDurableLedger(t *testing.T) (*DurableLedger, *memory.SqliteStore, func()) {
	t.Helper()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "memory.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	ledger, err := NewDurableLedger(ms.DB())
	if err != nil {
		_ = ms.Close(context.Background())
		t.Fatalf("NewDurableLedger: %v", err)
	}
	return ledger, ms, func() { _ = ms.Close(context.Background()) }
}

func TestDurableLedgerRecordsRestartableJobLifecycle(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	_, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:       "cron:daily:1700000000",
		Kind:     WorkKindCronJob,
		Depth:    0,
		Progress: json.RawMessage(`{"phase":"queued"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	lockUntil := time.Now().UTC().Add(time.Minute)
	claimed, ok, err := ledger.Claim(ctx, DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: lockUntil,
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !ok {
		t.Fatal("Claim ok = false, want true")
	}
	if claimed.ID != "cron:daily:1700000000" || claimed.Status != DurableJobActive {
		t.Fatalf("claimed = %+v, want active cron job", claimed)
	}

	if ok, err := ledger.UpdateProgress(ctx, claimed.ID, "worker-a", json.RawMessage(`{"phase":"rendered","pct":50}`)); err != nil || !ok {
		t.Fatalf("UpdateProgress ok=%v err=%v, want ok", ok, err)
	}
	if ok, err := ledger.Renew(ctx, claimed.ID, "worker-a", lockUntil.Add(time.Minute)); err != nil || !ok {
		t.Fatalf("Renew ok=%v err=%v, want ok", ok, err)
	}
	completed, ok, err := ledger.Complete(ctx, claimed.ID, "worker-a", json.RawMessage(`{"delivered":true}`))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !ok {
		t.Fatal("Complete ok = false, want true")
	}

	got, err := ledger.Get(ctx, completed.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != WorkKindCronJob || got.Status != DurableJobCompleted {
		t.Fatalf("got kind/status = %q/%q, want cron_job/completed", got.Kind, got.Status)
	}
	if got.ParentID != "" || got.Depth != 0 {
		t.Fatalf("parent/depth = %q/%d, want empty/0", got.ParentID, got.Depth)
	}
	assertJSONEqual(t, "progress", got.Progress, `{"phase":"rendered","pct":50}`)
	assertJSONEqual(t, "result", got.Result, `{"delivered":true}`)
	if got.ErrorText != "" {
		t.Fatalf("ErrorText = %q, want empty", got.ErrorText)
	}
	if got.CancelRequested {
		t.Fatal("CancelRequested = true, want false")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() || got.StartedAt.IsZero() || got.FinishedAt.IsZero() {
		t.Fatalf("timestamps not fully populated: %+v", got)
	}
}

func TestDurableLedgerReclaimsExpiredActiveJob(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:   "cron:stale",
		Kind: WorkKindCronJob,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	first, ok, err := ledger.Claim(ctx, DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(-time.Minute),
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if !ok || first.ID != "cron:stale" {
		t.Fatalf("first Claim = %+v ok=%v, want cron:stale", first, ok)
	}

	second, ok, err := ledger.Claim(ctx, DurableClaim{
		WorkerID:  "worker-b",
		LockUntil: time.Now().UTC().Add(time.Minute),
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if !ok || second.ID != "cron:stale" || second.LockOwner != "worker-b" {
		t.Fatalf("second Claim = %+v ok=%v, want worker-b reclaim", second, ok)
	}

	if _, ok, err := ledger.Complete(ctx, "cron:stale", "worker-a", json.RawMessage(`{"stale":true}`)); err != nil || ok {
		t.Fatalf("stale Complete ok=%v err=%v, want false nil", ok, err)
	}
	if _, ok, err := ledger.Complete(ctx, "cron:stale", "worker-b", json.RawMessage(`{"reclaimed":true}`)); err != nil || !ok {
		t.Fatalf("reclaim Complete ok=%v err=%v, want true nil", ok, err)
	}
}

func TestDurableLedgerCanClaimSpecificJobWithoutStealingOlderWork(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "older", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit older: %v", err)
	}
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "target", Kind: WorkKindLLMSubagent}); err != nil {
		t.Fatalf("Submit target: %v", err)
	}

	target, ok, err := ledger.ClaimJob(ctx, "target", DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	if !ok || target.ID != "target" || target.LockOwner != "worker-a" {
		t.Fatalf("ClaimJob = %+v ok=%v, want target owned by worker-a", target, ok)
	}
	older, err := ledger.Get(ctx, "older")
	if err != nil {
		t.Fatalf("Get older: %v", err)
	}
	if older.Status != DurableJobWaiting {
		t.Fatalf("older status = %q, want waiting", older.Status)
	}
}

func TestDurableLedgerRecordsParentChildFailureAndCancellationIntent(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:    "parent-1",
		Kind:  WorkKindLLMSubagent,
		Depth: 0,
	}); err != nil {
		t.Fatalf("Submit parent: %v", err)
	}
	child, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:       "child-1",
		Kind:     WorkKindCronJob,
		ParentID: "parent-1",
	})
	if err != nil {
		t.Fatalf("Submit child: %v", err)
	}
	if child.ParentID != "parent-1" || child.Depth != 1 {
		t.Fatalf("child parent/depth = %q/%d, want parent-1/1", child.ParentID, child.Depth)
	}

	claimed, ok, err := ledger.Claim(ctx, DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Minute),
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !ok || claimed.ID != "child-1" {
		t.Fatalf("Claim = %+v ok=%v, want child-1", claimed, ok)
	}
	failed, ok, err := ledger.Fail(ctx, "child-1", "worker-a", "boom")
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if !ok || failed.Status != DurableJobFailed || failed.ErrorText != "boom" {
		t.Fatalf("failed = %+v ok=%v, want failed boom", failed, ok)
	}

	events, err := ledger.ChildEvents(ctx, "parent-1")
	if err != nil {
		t.Fatalf("ChildEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ChildID != "child-1" || events[0].Outcome != DurableChildFailed || events[0].ErrorText != "boom" {
		t.Fatalf("event = %+v, want failed child_done for child-1", events[0])
	}

	_, err = ledger.Submit(ctx, DurableJobSubmission{ID: "cancel-me", Kind: WorkKindCronJob})
	if err != nil {
		t.Fatalf("Submit cancel-me: %v", err)
	}
	claimed, ok, err = ledger.Claim(ctx, DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Minute),
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("Claim cancel-me: %v", err)
	}
	if !ok || claimed.ID != "cancel-me" {
		t.Fatalf("Claim cancel-me = %+v ok=%v", claimed, ok)
	}
	cancelled, ok, err := ledger.Cancel(ctx, "cancel-me", "operator requested stop")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !ok || cancelled.Status != DurableJobCancelled || !cancelled.CancelRequested {
		t.Fatalf("cancelled = %+v ok=%v, want cancelled with intent", cancelled, ok)
	}
	if cancelled.CancelReason != "operator requested stop" || cancelled.LockOwner != "" {
		t.Fatalf("cancel fields = reason %q owner %q, want reason and cleared owner", cancelled.CancelReason, cancelled.LockOwner)
	}
	if ok, err := ledger.Renew(ctx, "cancel-me", "worker-a", time.Now().UTC().Add(time.Minute)); err != nil || ok {
		t.Fatalf("Renew after cancel ok=%v err=%v, want false nil", ok, err)
	}

	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "parent-cancel", Kind: WorkKindLLMSubagent}); err != nil {
		t.Fatalf("Submit parent-cancel: %v", err)
	}
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "child-cancel", Kind: WorkKindCronJob, ParentID: "parent-cancel"}); err != nil {
		t.Fatalf("Submit child-cancel: %v", err)
	}
	claimed, ok, err = ledger.ClaimJob(ctx, "child-cancel", DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Claim child-cancel: %v", err)
	}
	if !ok || claimed.ID != "child-cancel" {
		t.Fatalf("Claim child-cancel = %+v ok=%v", claimed, ok)
	}
	if _, ok, err := ledger.Cancel(ctx, "child-cancel", "cancel child"); err != nil || !ok {
		t.Fatalf("Cancel child-cancel ok=%v err=%v, want true nil", ok, err)
	}
	cancelEvents, err := ledger.ChildEvents(ctx, "parent-cancel")
	if err != nil {
		t.Fatalf("ChildEvents parent-cancel: %v", err)
	}
	if len(cancelEvents) != 1 || cancelEvents[0].Outcome != DurableChildCancelled {
		t.Fatalf("cancel events = %+v, want one cancelled child_done", cancelEvents)
	}
	parentCancel, err := ledger.Get(ctx, "parent-cancel")
	if err != nil {
		t.Fatalf("Get parent-cancel: %v", err)
	}
	if parentCancel.Status != DurableJobWaiting {
		t.Fatalf("parent-cancel status = %q, want waiting after cancelled child is terminal", parentCancel.Status)
	}
}

func TestManagerRecordsSubagentRunInDurableLedger(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	mgr := NewManager(ManagerOpts{
		ParentCtx:     context.Background(),
		ParentID:      "parent-test",
		Depth:         2,
		MaxDepth:      4,
		Registry:      NewRegistry(),
		NewRunner:     func() Runner { return StubRunner{} },
		DurableLedger: ledger,
	})
	defer mgr.Close()

	sa, err := mgr.Spawn(context.Background(), SubagentConfig{Goal: "ledger me"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	result, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	got, err := ledger.Get(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("ledger Get: %v", err)
	}
	if got.Kind != WorkKindLLMSubagent || got.Status != DurableJobCompleted {
		t.Fatalf("ledger kind/status = %q/%q, want llm_subagent/completed", got.Kind, got.Status)
	}
	if got.ParentID != "parent-test" || got.Depth != 3 {
		t.Fatalf("parent/depth = %q/%d, want parent-test/3", got.ParentID, got.Depth)
	}
	assertJSONEqual(t, "result", got.Result, `{"exit_reason":"stub_runner_no_llm_yet","iterations":0,"status":"completed","summary":"ledger me"}`)
}

func assertJSONEqual(t *testing.T, name string, got json.RawMessage, want string) {
	t.Helper()
	var gotAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatalf("%s unmarshal got: %v\nraw=%s", name, err, got)
	}
	var wantAny any
	if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
		t.Fatalf("%s unmarshal want: %v", name, err)
	}
	gotJSON, _ := json.Marshal(gotAny)
	wantJSON, _ := json.Marshal(wantAny)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("%s JSON = %s, want %s", name, gotJSON, wantJSON)
	}
}
