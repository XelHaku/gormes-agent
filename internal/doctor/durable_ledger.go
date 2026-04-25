package doctor

import (
	"context"
	"fmt"
	"strings"

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
	if status.QueueFull || status.TimedOut > 0 || status.StaleWaiting > 0 {
		result.Status = StatusWarn
		if status.QueueFull {
			prefix = "queue full; restart/replay available"
		}
	}
	result.Summary = fmt.Sprintf(
		"%s (%d total, %d waiting, %d claimed, %d stalled, %d timeout-at, %d timed-out, %d stale waiting, %d backpressure-denied)",
		prefix, status.Total, status.Waiting, status.Claimed, status.Stalled,
		status.TimeoutScheduled, status.TimedOut, status.StaleWaiting, status.BackpressureDenied,
	)
	queueStatus := StatusPass
	if status.QueueFull || status.TimedOut > 0 || status.StaleWaiting > 0 {
		queueStatus = StatusWarn
	}
	backpressureStatus := StatusPass
	if status.BackpressureDenied > 0 {
		backpressureStatus = StatusWarn
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
	}
	return result
}
