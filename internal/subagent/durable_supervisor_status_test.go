package subagent

import (
	"context"
	"testing"
	"time"
)

func TestDurableLedgerStatusDistinguishesWorkerHeartbeatModes(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status no worker: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerNoWorker {
		t.Fatalf("worker liveness = %q, want %q", status.Worker.Liveness, DurableWorkerNoWorker)
	}
	if status.Worker.Supervisor != DurableSupervisorAvailable {
		t.Fatalf("supervisor = %q, want %q", status.Worker.Supervisor, DurableSupervisorAvailable)
	}

	heartbeatAt := now.Add(-time.Minute)
	if err := ledger.RecordWorkerHeartbeat(ctx, DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: heartbeatAt,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat healthy: %v", err)
	}
	status, err = ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status healthy: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerHealthy {
		t.Fatalf("worker liveness = %q, want %q", status.Worker.Liveness, DurableWorkerHealthy)
	}
	if status.Worker.WorkerID != "worker-a" || !status.Worker.LastHeartbeat.Equal(heartbeatAt) {
		t.Fatalf("worker heartbeat = id %q at %v, want worker-a at %v", status.Worker.WorkerID, status.Worker.LastHeartbeat, heartbeatAt)
	}

	staleAt := now.Add(-10 * time.Minute)
	if err := ledger.RecordWorkerHeartbeat(ctx, DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: staleAt,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat stale: %v", err)
	}
	status, err = ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status stale: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerStaleHeartbeat {
		t.Fatalf("worker liveness = %q, want %q", status.Worker.Liveness, DurableWorkerStaleHeartbeat)
	}
	if status.Worker.DegradedReason != "stale-heartbeat" {
		t.Fatalf("degraded reason = %q, want stale-heartbeat", status.Worker.DegradedReason)
	}
}

func TestDurableLedgerStatusReportsSupervisorUnavailableSeparatelyFromHeartbeat(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := ledger.RecordWorkerHeartbeat(ctx, DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status before supervisor report: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerHealthy {
		t.Fatalf("worker liveness = %q, want %q", status.Worker.Liveness, DurableWorkerHealthy)
	}
	if status.Worker.Supervisor != DurableSupervisorUnavailable {
		t.Fatalf("supervisor = %q, want %q when no supervisor status has been recorded", status.Worker.Supervisor, DurableSupervisorUnavailable)
	}

	if err := ledger.RecordSupervisorStatus(ctx, DurableSupervisorReport{
		Available:  false,
		Reason:     "pid-file-unreadable",
		ReportedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus unavailable: %v", err)
	}
	status, err = ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status unavailable supervisor: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerHealthy {
		t.Fatalf("worker liveness = %q, want heartbeat state to remain healthy", status.Worker.Liveness)
	}
	if status.Worker.Supervisor != DurableSupervisorUnavailable {
		t.Fatalf("supervisor = %q, want %q", status.Worker.Supervisor, DurableSupervisorUnavailable)
	}
	if status.Worker.SupervisorReason != "pid-file-unreadable" {
		t.Fatalf("supervisor reason = %q, want pid-file-unreadable", status.Worker.SupervisorReason)
	}
}

func TestDurableLedgerRecordsRestartIntentAsAuditEvidence(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := ledger.RecordSupervisorStatus(ctx, DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: now,
	}); err != nil {
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}

	requestedAt := now.Add(time.Second)
	if err := ledger.RecordWorkerRestartIntent(ctx, DurableWorkerRestartIntent{
		WorkerID:     "worker-a",
		Reason:       "stale-heartbeat",
		RequestedAt:  requestedAt,
		SupervisorID: "supervisor-a",
	}); err != nil {
		t.Fatalf("RecordWorkerRestartIntent: %v", err)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Worker.RestartIntent.Requested {
		t.Fatal("restart intent requested = false, want true")
	}
	if status.Worker.RestartIntent.WorkerID != "worker-a" {
		t.Fatalf("restart worker id = %q, want worker-a", status.Worker.RestartIntent.WorkerID)
	}
	if status.Worker.RestartIntent.Reason != "stale-heartbeat" {
		t.Fatalf("restart reason = %q, want stale-heartbeat", status.Worker.RestartIntent.Reason)
	}
	if status.Worker.RestartIntent.SupervisorID != "supervisor-a" {
		t.Fatalf("restart supervisor id = %q, want supervisor-a", status.Worker.RestartIntent.SupervisorID)
	}
	if !status.Worker.RestartIntent.RequestedAt.Equal(requestedAt) {
		t.Fatalf("restart requested at = %v, want %v", status.Worker.RestartIntent.RequestedAt, requestedAt)
	}
	if status.Worker.RestartIntent.AuditEvents != 1 {
		t.Fatalf("restart audit events = %d, want 1", status.Worker.RestartIntent.AuditEvents)
	}
}
