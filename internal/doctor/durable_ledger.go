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
	result.Summary = fmt.Sprintf(
		"restart/replay available (%d total, %d waiting, %d active, %d stalled)",
		status.Total, status.Waiting, status.Active, status.Stalled,
	)
	result.Items = []ItemInfo{
		{Name: "ledger", Status: StatusPass, Note: "SQLite durable job ledger configured"},
		{Name: "cancel_intent", Status: StatusPass, Note: fmt.Sprintf("%d requested", status.CancelRequested)},
	}
	return result
}
