package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
)

func CheckDurableLedger(ctx context.Context, ledger *subagent.DurableLedger, runLogPath string) CheckResult {
	result := CheckResult{Name: "Durable jobs"}
	if ledger == nil {
		result.Status = StatusWarn
		if strings.TrimSpace(runLogPath) != "" {
			result.Summary = "append-only run logs configured; durable restart/replay unavailable"
			result.Items = []ItemInfo{
				{Name: "run_log", Status: StatusPass, Note: runLogPath},
				{Name: "ledger", Status: StatusWarn, Note: "SQLite durable ledger is not configured"},
			}
			return result
		}
		result.Summary = "durable restart/replay unavailable; append-only run logs not configured"
		result.Items = []ItemInfo{
			{Name: "ledger", Status: StatusWarn, Note: "SQLite durable ledger is not configured"},
		}
		return result
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		result.Status = StatusFail
		result.Summary = "durable ledger status unavailable"
		result.Items = []ItemInfo{
			{Name: "ledger", Status: StatusFail, Note: err.Error()},
		}
		return result
	}

	result.Status = StatusPass
	prefix := "restart/replay available"
	workerDegraded := status.Worker.Liveness != subagent.DurableWorkerHealthy
	supervisorDegraded := status.Worker.Supervisor != subagent.DurableSupervisorAvailable
	restartIntent := status.Worker.RestartIntent.Requested
	if status.QueueFull || status.TimedOut > 0 || status.StaleWaiting > 0 ||
		workerDegraded || supervisorDegraded || restartIntent {
		result.Status = StatusWarn
		if status.QueueFull {
			prefix = "queue full; restart/replay available"
		} else if workerDegraded || supervisorDegraded {
			prefix = "durable worker degraded; restart/replay available"
		} else if restartIntent {
			prefix = "restart intent recorded; restart/replay available"
		}
	}
	supervisorNote := string(status.Worker.Supervisor)
	if strings.TrimSpace(status.Worker.SupervisorReason) != "" {
		supervisorNote += ":" + status.Worker.SupervisorReason
	}
	restartReason := status.Worker.RestartIntent.Reason
	if restartReason == "" {
		restartReason = "none"
	}
	result.Summary = fmt.Sprintf(
		"%s (%d total, %d waiting, %d claimed, %d stalled, %d timeout-at, %d timed-out, %d stale waiting, %d backpressure-denied, worker=%s, supervisor=%s, restart_intent=%d reason=%s)",
		prefix, status.Total, status.Waiting, status.Claimed, status.Stalled,
		status.TimeoutScheduled, status.TimedOut, status.StaleWaiting, status.BackpressureDenied,
		status.Worker.Liveness, supervisorNote, status.Worker.RestartIntent.AuditEvents, restartReason,
	)
	queueStatus := StatusPass
	if status.QueueFull || status.TimedOut > 0 || status.StaleWaiting > 0 {
		queueStatus = StatusWarn
	}
	backpressureStatus := StatusPass
	if status.BackpressureDenied > 0 {
		backpressureStatus = StatusWarn
	}
	workerStatus := StatusPass
	if workerDegraded {
		workerStatus = StatusWarn
	}
	supervisorStatus := StatusPass
	if supervisorDegraded {
		supervisorStatus = StatusWarn
	}
	restartStatus := StatusPass
	if restartIntent {
		restartStatus = StatusWarn
	}
	result.Items = []ItemInfo{
		{Name: "ledger", Status: StatusPass, Note: "SQLite durable job ledger configured"},
		{Name: "queue_health", Status: queueStatus, Note: fmt.Sprintf(
			"waiting=%d claimed=%d stalled=%d timeout_at=%d timed_out=%d stale_waiting=%d queue_full=%t max_waiting=%d",
			status.Waiting, status.Claimed, status.Stalled, status.TimeoutScheduled,
			status.TimedOut, status.StaleWaiting, status.QueueFull, status.MaxWaiting,
		)},
		{Name: "backpressure", Status: backpressureStatus, Note: fmt.Sprintf("%d denied", status.BackpressureDenied)},
		{Name: "cancel_intent", Status: StatusPass, Note: fmt.Sprintf("%d requested", status.CancelRequested)},
		{Name: "durable_worker", Status: workerStatus, Note: fmt.Sprintf(
			"liveness=%s worker_id=%s last_heartbeat=%s stale_after=%s",
			status.Worker.Liveness, status.Worker.WorkerID,
			formatDurableTime(status.Worker.LastHeartbeat), status.Worker.HeartbeatStaleAfter,
		)},
		{Name: "supervisor", Status: supervisorStatus, Note: supervisorNote},
		{Name: "restart_intent", Status: restartStatus, Note: fmt.Sprintf(
			"requested=%t worker_id=%s supervisor_id=%s reason=%s audit_events=%d requested_at=%s",
			status.Worker.RestartIntent.Requested,
			status.Worker.RestartIntent.WorkerID,
			status.Worker.RestartIntent.SupervisorID,
			restartReason,
			status.Worker.RestartIntent.AuditEvents,
			formatDurableTime(status.Worker.RestartIntent.RequestedAt),
		)},
	}
	return result
}

func formatDurableTime(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.UTC().Format(time.RFC3339)
}
