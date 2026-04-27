package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
)

func TestCheckDurableLedgerReportsAppendOnlyRunLogDegradedMode(t *testing.T) {
	result := CheckDurableLedger(context.Background(), nil, "/tmp/gormes-subagents.jsonl")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN", result.Status)
	}
	if !strings.Contains(result.Summary, "append-only run logs") {
		t.Fatalf("Summary = %q, want append-only run logs", result.Summary)
	}
	if !strings.Contains(result.Summary, "restart/replay unavailable") {
		t.Fatalf("Summary = %q, want restart/replay unavailable", result.Summary)
	}
}

func TestCheckDurableLedgerReportsReplayAvailable(t *testing.T) {
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(context.Background())
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}

	if _, err := ledger.Submit(context.Background(), subagent.DurableJobSubmission{
		ID:   "job-1",
		Kind: subagent.WorkKindCronJob,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(context.Background(), subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(context.Background(), subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	result := CheckDurableLedger(context.Background(), ledger, "")

	if result.Status != StatusPass {
		t.Fatalf("Status = %v, want PASS: %+v", result.Status, result)
	}
	if !strings.Contains(result.Summary, "restart/replay available") {
		t.Fatalf("Summary = %q, want restart/replay available", result.Summary)
	}
}

func TestCheckDurableLedgerReportsWorkerSupervisorDegradedModes(t *testing.T) {
	ctx := context.Background()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(ctx)
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}

	noWorker := CheckDurableLedger(ctx, ledger, "")
	if noWorker.Status != StatusWarn {
		t.Fatalf("no-worker status = %v, want WARN: %+v", noWorker.Status, noWorker)
	}
	for _, want := range []string{"worker=no-worker", "supervisor=supervisor-unavailable"} {
		if !strings.Contains(noWorker.Summary, want) {
			t.Fatalf("no-worker summary = %q, want %q", noWorker.Summary, want)
		}
	}

	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus available: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat stale: %v", err)
	}
	stale := CheckDurableLedger(ctx, ledger, "")
	if stale.Status != StatusWarn {
		t.Fatalf("stale status = %v, want WARN: %+v", stale.Status, stale)
	}
	if !strings.Contains(stale.Summary, "worker=stale-heartbeat") {
		t.Fatalf("stale summary = %q, want worker=stale-heartbeat", stale.Summary)
	}

	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat healthy: %v", err)
	}
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  false,
		Reason:     "pid-file-unreadable",
		ReportedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus unavailable: %v", err)
	}
	unavailable := CheckDurableLedger(ctx, ledger, "")
	if unavailable.Status != StatusWarn {
		t.Fatalf("unavailable status = %v, want WARN: %+v", unavailable.Status, unavailable)
	}
	for _, want := range []string{"worker=healthy", "supervisor=supervisor-unavailable", "pid-file-unreadable"} {
		if !strings.Contains(unavailable.Summary, want) {
			t.Fatalf("unavailable summary = %q, want %q", unavailable.Summary, want)
		}
	}
}

func TestCheckDurableLedgerReportsDurableWorkerHeartbeatEvidence(t *testing.T) {
	ctx := context.Background()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(ctx)
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}

	noHeartbeat := CheckDurableLedger(ctx, ledger, "")
	workerItem := findDoctorItem(noHeartbeat.Items, "durable_worker")
	if workerItem.Name == "" {
		t.Fatalf("Items = %+v, want durable_worker item", noHeartbeat.Items)
	}
	if !strings.Contains(workerItem.Note, "heartbeat=heartbeat_unavailable") {
		t.Fatalf("worker note = %q, want heartbeat_unavailable evidence", workerItem.Note)
	}

	heartbeatAt := time.Now().UTC().Add(-time.Minute)
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: heartbeatAt,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	withHeartbeat := CheckDurableLedger(ctx, ledger, "")
	workerItem = findDoctorItem(withHeartbeat.Items, "durable_worker")
	if !strings.Contains(workerItem.Note, "heartbeat="+heartbeatAt.Format(time.RFC3339)) {
		t.Fatalf("worker note = %q, want last heartbeat %s", workerItem.Note, heartbeatAt.Format(time.RFC3339))
	}
}

func TestCheckDurableLedgerReportsRestartIntentAuditEvidence(t *testing.T) {
	ctx := context.Background()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(ctx)
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	if err := ledger.RecordWorkerRestartIntent(ctx, subagent.DurableWorkerRestartIntent{
		WorkerID:     "worker-a",
		Reason:       "operator-requested-restart",
		RequestedAt:  now.Add(time.Second),
		SupervisorID: "supervisor-a",
	}); err != nil {
		t.Fatalf("RecordWorkerRestartIntent: %v", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")
	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN for restart intent audit evidence: %+v", result.Status, result)
	}
	for _, want := range []string{"restart_intent=1", "operator-requested-restart"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("Summary = %q, want %q", result.Summary, want)
		}
	}
}

func findDoctorItem(items []ItemInfo, name string) ItemInfo {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return ItemInfo{}
}

func TestCheckDurableLedgerReportsLifecycleControlEvidence(t *testing.T) {
	ctx := context.Background()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(ctx)
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "pause-me", Kind: subagent.WorkKindCronJob}); err != nil {
		t.Fatalf("Submit pause-me: %v", err)
	}
	if _, ok, err := ledger.Pause(ctx, "pause-me", subagent.DurableLifecycleIntent{
		Trust:       subagent.TrustOperator,
		Actor:       "operator:ada",
		Reason:      "maintenance",
		RequestedAt: now.Add(time.Second),
	}); err != nil || !ok {
		t.Fatalf("Pause ok=%v err=%v, want true nil", ok, err)
	}
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "resume-me", Kind: subagent.WorkKindCronJob}); err != nil {
		t.Fatalf("Submit resume-me: %v", err)
	}
	if _, ok, err := ledger.Pause(ctx, "resume-me", subagent.DurableLifecycleIntent{
		Trust:       subagent.TrustOperator,
		Actor:       "operator:ada",
		Reason:      "maintenance",
		RequestedAt: now.Add(2 * time.Second),
	}); err != nil || !ok {
		t.Fatalf("Pause resume-me ok=%v err=%v, want true nil", ok, err)
	}
	if _, ok, err := ledger.Resume(ctx, "resume-me", subagent.DurableLifecycleIntent{
		Trust:       subagent.TrustSystem,
		Actor:       "supervisor-a",
		Reason:      "resume requested",
		RequestedAt: now.Add(3 * time.Second),
	}); err != nil || !ok {
		t.Fatalf("Resume ok=%v err=%v, want true nil", ok, err)
	}
	if _, ok, err := ledger.Pause(ctx, "resume-me", subagent.DurableLifecycleIntent{
		Trust:  subagent.TrustChildAgent,
		Actor:  "child-agent:7",
		Reason: "unsupported",
	}); !errors.Is(err, subagent.ErrDurableLifecycleDenied) || ok {
		t.Fatalf("child-agent Pause ok=%v err=%v, want ErrDurableLifecycleDenied and false", ok, err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN for lifecycle evidence: %+v", result.Status, result)
	}
	for _, want := range []string{"1 paused", "1 resume-pending", "1 lifecycle-unsupported"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("Summary = %q, want %q", result.Summary, want)
		}
	}
	var lifecycle ItemInfo
	for _, item := range result.Items {
		if item.Name == "lifecycle_control" {
			lifecycle = item
			break
		}
	}
	if lifecycle.Name == "" {
		t.Fatalf("Items = %+v, want lifecycle_control item", result.Items)
	}
	if lifecycle.Status != StatusWarn {
		t.Fatalf("lifecycle status = %v, want WARN", lifecycle.Status)
	}
	for _, want := range []string{"paused=1", "resume_pending=1", "unsupported=1"} {
		if !strings.Contains(lifecycle.Note, want) {
			t.Fatalf("lifecycle note = %q, want %q", lifecycle.Note, want)
		}
	}
}

func TestCheckDurableLedgerReportsReplayInboxAndProtectedSubmitEvidenceSeparately(t *testing.T) {
	ctx := context.Background()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(ctx)
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "replay-waiting", Kind: subagent.WorkKindCronJob}); err != nil {
		t.Fatalf("Submit replay-waiting: %v", err)
	}
	if _, ok, err := ledger.Replay(ctx, "replay-waiting", subagent.DurableReplayRequest{ID: "replay-waiting:again"}); !errors.Is(err, subagent.ErrDurableReplayUnavailable) || ok {
		t.Fatalf("Replay waiting ok=%v err=%v, want ErrDurableReplayUnavailable and false", ok, err)
	}
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "agent:target", Kind: subagent.WorkKindLLMSubagent}); err != nil {
		t.Fatalf("Submit agent:target: %v", err)
	}
	if _, err := ledger.SendInboxMessage(ctx, subagent.DurableInboxMessageSubmission{
		JobID:       "agent:target",
		Sender:      "operator:ada",
		SenderTrust: subagent.TrustOperator,
		Payload:     json.RawMessage(`{"directive":"audit me"}`),
	}); err != nil {
		t.Fatalf("SendInboxMessage: %v", err)
	}
	if _, err := ledger.SubmitWithTrust(ctx, subagent.TrustChildAgent, subagent.DurableJobSubmission{
		ID:   "shell:denied",
		Kind: subagent.WorkKindShellCommand,
	}); !errors.Is(err, subagent.ErrDurableRouteDenied) {
		t.Fatalf("SubmitWithTrust child shell err=%v, want ErrDurableRouteDenied", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN: %+v", result.Status, result)
	}
	for _, want := range []string{"1 replay-unavailable", "1 inbox-unread", "1 protected-submit-denied"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("Summary = %q, want %q", result.Summary, want)
		}
	}
	warnItems := map[string]bool{}
	for _, item := range result.Items {
		if item.Status == StatusWarn {
			warnItems[item.Name] = true
		}
	}
	for _, name := range []string{"replay", "inbox", "protected_submit"} {
		if !warnItems[name] {
			t.Fatalf("warn items = %+v, want %s item WARN in %+v", warnItems, name, result.Items)
		}
	}
}

func TestCheckDurableLedgerReportsQueueHealthDegraded(t *testing.T) {
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(context.Background())
	ledger, err := subagent.NewDurableLedgerWithOptions(ms.DB(), subagent.DurableLedgerOptions{
		MaxWaiting: 3,
	})
	if err != nil {
		t.Fatalf("NewDurableLedgerWithOptions: %v", err)
	}

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "timed", Kind: subagent.WorkKindCronJob}); err != nil {
		t.Fatalf("Submit timed: %v", err)
	}
	if _, ok, err := ledger.ClaimJob(ctx, "timed", subagent.DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Hour),
		TimeoutAt: time.Now().UTC().Add(-time.Minute),
	}); err != nil || !ok {
		t.Fatalf("ClaimJob timed ok=%v err=%v, want true nil", ok, err)
	}
	for _, id := range []string{"waiting-1", "waiting-2", "stale"} {
		if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: id, Kind: subagent.WorkKindCronJob}); err != nil {
			t.Fatalf("Submit %s: %v", id, err)
		}
	}
	staleCreatedAt := time.Now().UTC().Add(-2 * time.Hour).UnixNano()
	if _, err := ms.DB().ExecContext(ctx,
		`UPDATE durable_jobs SET created_at = ?, updated_at = ? WHERE id = ?`,
		staleCreatedAt, staleCreatedAt, "stale"); err != nil {
		t.Fatalf("mark stale waiting: %v", err)
	}
	if _, err := ledger.Submit(ctx, subagent.DurableJobSubmission{ID: "denied", Kind: subagent.WorkKindCronJob}); !errors.Is(err, subagent.ErrDurableBackpressure) {
		t.Fatalf("Submit denied err = %v, want ErrDurableBackpressure", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN: %+v", result.Status, result)
	}
	for _, want := range []string{
		"queue full",
		"3 waiting",
		"1 claimed",
		"1 timed-out",
		"1 backpressure-denied",
		"1 stale waiting",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("Summary = %q, want %q", result.Summary, want)
		}
	}
}
