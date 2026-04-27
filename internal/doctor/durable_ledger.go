package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unsafe"

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
	rssStatus, rssStatusErr := durableRSSWatchdogStatus(ctx, ledger)

	result.Status = StatusPass
	prefix := "restart/replay available"
	workerDegraded := status.Worker.Liveness != subagent.DurableWorkerHealthy
	supervisorDegraded := status.Worker.Supervisor != subagent.DurableSupervisorAvailable
	restartIntent := status.Worker.RestartIntent.Requested
	lifecycleDegraded := status.Paused > 0 || status.ResumePending > 0 || status.LifecycleControlUnsupported > 0
	replayDegraded := status.ReplayUnavailable > 0
	inboxDegraded := status.InboxUnread > 0
	protectedSubmitDegraded := status.ProtectedSubmitDenied > 0
	rssWatchdogDegraded := rssStatusErr != nil || rssStatus.degraded()
	abortRecoveryDegraded := status.Worker.AbortRecovery.AbortSignalSent > 0 ||
		status.Worker.AbortRecovery.AbortSlotRecovered > 0 ||
		status.Worker.AbortRecovery.HandlerIgnoredAbort > 0 ||
		status.Worker.AbortRecovery.AbortRecoveryUnavailable > 0
	if status.QueueFull || status.TimedOut > 0 || status.StaleWaiting > 0 ||
		workerDegraded || supervisorDegraded || restartIntent || lifecycleDegraded ||
		replayDegraded || inboxDegraded || protectedSubmitDegraded || abortRecoveryDegraded ||
		rssWatchdogDegraded {
		result.Status = StatusWarn
		if status.QueueFull {
			prefix = "queue full; restart/replay available"
		} else if abortRecoveryDegraded {
			prefix = "durable worker abort recovery evidence; restart/replay available"
		} else if rssWatchdogDegraded {
			prefix = "durable worker RSS watchdog evidence; restart/replay available"
		} else if workerDegraded || supervisorDegraded {
			prefix = "durable worker degraded; restart/replay available"
		} else if restartIntent {
			prefix = "restart intent recorded; restart/replay available"
		} else if lifecycleDegraded {
			prefix = "durable lifecycle control pending; restart/replay available"
		} else if replayDegraded {
			prefix = "replay unavailable evidence recorded; restart/replay available"
		} else if inboxDegraded {
			prefix = "inbox unread; restart/replay available"
		} else if protectedSubmitDegraded {
			prefix = "protected submit denied; restart/replay available"
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
		"%s (%d total, %d waiting, %d claimed, %d stalled, %d timeout-at, %d timed-out, %d stale waiting, %d backpressure-denied, %d replay-unavailable, %d inbox-unread, %d protected-submit-denied, %d paused, %d resume-pending, %d lifecycle-unsupported, worker=%s, supervisor=%s, restart_intent=%d reason=%s, abort_signal_sent=%d abort_slot_recovered=%d handler_ignored_abort=%d abort_recovery_unavailable=%d)",
		prefix, status.Total, status.Waiting, status.Claimed, status.Stalled,
		status.TimeoutScheduled, status.TimedOut, status.StaleWaiting, status.BackpressureDenied,
		status.ReplayUnavailable, status.InboxUnread, status.ProtectedSubmitDenied,
		status.Paused, status.ResumePending, status.LifecycleControlUnsupported,
		status.Worker.Liveness, supervisorNote, status.Worker.RestartIntent.AuditEvents, restartReason,
		status.Worker.AbortRecovery.AbortSignalSent,
		status.Worker.AbortRecovery.AbortSlotRecovered,
		status.Worker.AbortRecovery.HandlerIgnoredAbort,
		status.Worker.AbortRecovery.AbortRecoveryUnavailable,
	)
	if rssStatusErr != nil {
		result.Summary += fmt.Sprintf(" rss_watchdog_status_unavailable=%q", rssStatusErr.Error())
	} else {
		result.Summary += fmt.Sprintf(
			" rss_watchdog_disabled=%d rss_watchdog_unavailable=%d rss_threshold_exceeded=%d rss_drain_started=%d stable_watchdog_restart=%d rss_last_reason=%s",
			rssStatus.Disabled,
			rssStatus.Unavailable,
			rssStatus.ThresholdExceeded,
			rssStatus.DrainStarted,
			rssStatus.StableRestart,
			rssStatus.lastReason(),
		)
	}
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
	abortRecoveryStatus := StatusPass
	if abortRecoveryDegraded {
		abortRecoveryStatus = StatusWarn
	}
	lifecycleStatus := StatusPass
	if lifecycleDegraded {
		lifecycleStatus = StatusWarn
	}
	replayStatus := StatusPass
	if replayDegraded {
		replayStatus = StatusWarn
	}
	inboxStatus := StatusPass
	if inboxDegraded {
		inboxStatus = StatusWarn
	}
	protectedSubmitStatus := StatusPass
	if protectedSubmitDegraded {
		protectedSubmitStatus = StatusWarn
	}
	rssItemStatus := StatusPass
	if rssWatchdogDegraded {
		rssItemStatus = StatusWarn
	}
	workerHeartbeat := formatDurableTime(status.Worker.LastHeartbeat)
	workerHeartbeatEvidence := workerHeartbeat
	if status.Worker.LastHeartbeat.IsZero() {
		workerHeartbeatEvidence = "heartbeat_unavailable"
	}
	result.Items = []ItemInfo{
		{Name: "ledger", Status: StatusPass, Note: "SQLite durable job ledger configured"},
		{Name: "replay", Status: replayStatus, Note: fmt.Sprintf(
			"available=%t unavailable=%d",
			status.ReplayAvailable, status.ReplayUnavailable,
		)},
		{Name: "queue_health", Status: queueStatus, Note: fmt.Sprintf(
			"waiting=%d claimed=%d stalled=%d timeout_at=%d timed_out=%d stale_waiting=%d queue_full=%t max_waiting=%d",
			status.Waiting, status.Claimed, status.Stalled, status.TimeoutScheduled,
			status.TimedOut, status.StaleWaiting, status.QueueFull, status.MaxWaiting,
		)},
		{Name: "backpressure", Status: backpressureStatus, Note: fmt.Sprintf("%d denied", status.BackpressureDenied)},
		{Name: "inbox", Status: inboxStatus, Note: fmt.Sprintf("%d unread", status.InboxUnread)},
		{Name: "protected_submit", Status: protectedSubmitStatus, Note: fmt.Sprintf("%d denied", status.ProtectedSubmitDenied)},
		{Name: "cancel_intent", Status: StatusPass, Note: fmt.Sprintf("%d requested", status.CancelRequested)},
		{Name: "lifecycle_control", Status: lifecycleStatus, Note: fmt.Sprintf(
			"paused=%d resume_pending=%d unsupported=%d",
			status.Paused, status.ResumePending, status.LifecycleControlUnsupported,
		)},
		{Name: "durable_worker", Status: workerStatus, Note: fmt.Sprintf(
			"liveness=%s worker_id=%s heartbeat=%s last_heartbeat=%s stale_after=%s",
			status.Worker.Liveness, status.Worker.WorkerID,
			workerHeartbeatEvidence, workerHeartbeat, status.Worker.HeartbeatStaleAfter,
		)},
		{Name: "rss_watchdog", Status: rssItemStatus, Note: formatDurableRSSWatchdogStatus(rssStatus, rssStatusErr)},
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
		{Name: "abort_recovery", Status: abortRecoveryStatus, Note: fmt.Sprintf(
			"abort_signal_sent=%d abort_slot_recovered=%d handler_ignored_abort=%d abort_recovery_unavailable=%d last_event=%s job_id=%s worker_id=%s reason=%s at=%s",
			status.Worker.AbortRecovery.AbortSignalSent,
			status.Worker.AbortRecovery.AbortSlotRecovered,
			status.Worker.AbortRecovery.HandlerIgnoredAbort,
			status.Worker.AbortRecovery.AbortRecoveryUnavailable,
			status.Worker.AbortRecovery.LastEvent,
			status.Worker.AbortRecovery.LastJobID,
			status.Worker.AbortRecovery.LastWorkerID,
			status.Worker.AbortRecovery.LastReason,
			formatDurableTime(status.Worker.AbortRecovery.LastEventAt),
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

type durableRSSWatchdogLedgerStatus struct {
	Disabled          int
	Unavailable       int
	ThresholdExceeded int
	DrainStarted      int
	StableRestart     int
	Latest            durableRSSWatchdogEventStatus
}

type durableRSSWatchdogEventStatus struct {
	Type           string
	Reason         string
	WorkerID       string
	JobID          string
	EvidenceReason string
	ObservedMB     int64
	MaxMB          int64
	CheckedAt      time.Time
	ErrorText      string
	CrashCount     int
	CreatedAt      time.Time
}

func (s durableRSSWatchdogLedgerStatus) degraded() bool {
	return s.Unavailable > 0 || s.ThresholdExceeded > 0 || s.DrainStarted > 0
}

func (s durableRSSWatchdogLedgerStatus) lastReason() string {
	if s.Latest.Reason != "" {
		return s.Latest.Reason
	}
	if s.Latest.Type != "" {
		return s.Latest.Type
	}
	return "none"
}

func durableRSSWatchdogStatus(ctx context.Context, ledger *subagent.DurableLedger) (durableRSSWatchdogLedgerStatus, error) {
	db, ok := durableLedgerDB(ledger)
	if !ok {
		return durableRSSWatchdogLedgerStatus{}, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT type, worker_id, reason, payload_json, created_at
		FROM durable_worker_events
		WHERE type IN (?, ?, ?, ?, ?)
		ORDER BY id ASC`,
		string(subagent.DurableWorkerRSSWatchdogDisabled),
		string(subagent.DurableWorkerRSSWatchdogUnavailable),
		string(subagent.DurableWorkerRSSThresholdExceeded),
		string(subagent.DurableWorkerRSSDrainStarted),
		string(subagent.DurableWorkerStableWatchdogRestart),
	)
	if err != nil {
		return durableRSSWatchdogLedgerStatus{}, fmt.Errorf("rss watchdog status: %w", err)
	}
	defer rows.Close()

	var status durableRSSWatchdogLedgerStatus
	for rows.Next() {
		var eventType, workerID, reason, payloadJSON string
		var createdAt int64
		if err := rows.Scan(&eventType, &workerID, &reason, &payloadJSON, &createdAt); err != nil {
			return durableRSSWatchdogLedgerStatus{}, fmt.Errorf("rss watchdog status row: %w", err)
		}
		switch eventType {
		case string(subagent.DurableWorkerRSSWatchdogDisabled):
			status.Disabled++
		case string(subagent.DurableWorkerRSSWatchdogUnavailable):
			status.Unavailable++
		case string(subagent.DurableWorkerRSSThresholdExceeded):
			status.ThresholdExceeded++
		case string(subagent.DurableWorkerRSSDrainStarted):
			status.DrainStarted++
		case string(subagent.DurableWorkerStableWatchdogRestart):
			status.StableRestart++
		}
		status.Latest = parseDurableRSSWatchdogEvent(eventType, workerID, reason, payloadJSON, durableTimeFromUnixNano(createdAt))
	}
	if err := rows.Err(); err != nil {
		return durableRSSWatchdogLedgerStatus{}, fmt.Errorf("rss watchdog status rows: %w", err)
	}
	return status, nil
}

func parseDurableRSSWatchdogEvent(eventType, workerID, reason, payloadJSON string, createdAt time.Time) durableRSSWatchdogEventStatus {
	event := durableRSSWatchdogEventStatus{
		Type:      strings.TrimSpace(eventType),
		Reason:    strings.TrimSpace(reason),
		WorkerID:  strings.TrimSpace(workerID),
		CreatedAt: createdAt,
	}
	var payload struct {
		JobID      string `json:"job_id"`
		WorkerID   string `json:"worker_id"`
		Reason     string `json:"reason"`
		CrashCount int    `json:"crash_count"`
		Evidence   struct {
			Reason     string    `json:"reason"`
			ObservedMB int64     `json:"observed_mb"`
			MaxMB      int64     `json:"max_mb"`
			CheckedAt  time.Time `json:"checked_at"`
			ErrorText  string    `json:"error"`
		} `json:"evidence"`
	}
	if json.Unmarshal([]byte(payloadJSON), &payload) == nil {
		if strings.TrimSpace(payload.WorkerID) != "" {
			event.WorkerID = strings.TrimSpace(payload.WorkerID)
		}
		if strings.TrimSpace(payload.Reason) != "" {
			event.Reason = strings.TrimSpace(payload.Reason)
		}
		event.JobID = strings.TrimSpace(payload.JobID)
		event.CrashCount = payload.CrashCount
		event.EvidenceReason = strings.TrimSpace(payload.Evidence.Reason)
		event.ObservedMB = payload.Evidence.ObservedMB
		event.MaxMB = payload.Evidence.MaxMB
		event.CheckedAt = payload.Evidence.CheckedAt
		event.ErrorText = payload.Evidence.ErrorText
	}
	if event.Reason == "" {
		event.Reason = event.Type
	}
	if event.EvidenceReason == "" {
		event.EvidenceReason = event.Reason
	}
	if event.CheckedAt.IsZero() {
		event.CheckedAt = createdAt
	}
	return event
}

func formatDurableRSSWatchdogStatus(status durableRSSWatchdogLedgerStatus, statusErr error) string {
	if statusErr != nil {
		return "status_unavailable=" + statusErr.Error()
	}
	latest := status.Latest
	return fmt.Sprintf(
		"rss_watchdog_disabled=%d rss_watchdog_unavailable=%d rss_threshold_exceeded=%d rss_drain_started=%d stable_watchdog_restart=%d last_reason=%s evidence_reason=%s worker_id=%s job_id=%s max_mb=%d observed_mb=%d checked_at=%s error=%s crash_loop=%d",
		status.Disabled,
		status.Unavailable,
		status.ThresholdExceeded,
		status.DrainStarted,
		status.StableRestart,
		status.lastReason(),
		latest.EvidenceReason,
		latest.WorkerID,
		latest.JobID,
		latest.MaxMB,
		latest.ObservedMB,
		formatDurableTime(latest.CheckedAt),
		redactDurableRSSWatchdogEvidence(latest.ErrorText),
		0,
	)
}

func redactDurableRSSWatchdogEvidence(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "none"
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"token", "secret", "password", "api_key", "apikey", "authorization", "bearer"} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	return trimmed
}

func durableLedgerDB(ledger *subagent.DurableLedger) (*sql.DB, bool) {
	if ledger == nil {
		return nil, false
	}
	field := reflect.ValueOf(ledger).Elem().FieldByName("db")
	if !field.IsValid() || field.IsNil() {
		return nil, false
	}
	return *(**sql.DB)(unsafe.Pointer(field.UnsafeAddr())), true
}

func durableTimeFromUnixNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}
