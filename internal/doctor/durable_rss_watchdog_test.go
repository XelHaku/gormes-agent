package doctor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
)

func TestDoctorDurableWorkerRSSWatchdog_DisabledAndUnavailable(t *testing.T) {
	ctx := context.Background()
	ledger, _, cleanup := newDoctorRSSWatchdogLedger(t, ctx)
	defer cleanup()

	disabledAt := time.Date(2026, 4, 27, 13, 30, 0, 0, time.UTC)
	if err := ledger.RecordWorkerRSSWatchdogEvent(ctx, subagent.DurableWorkerRSSWatchdogEvent{
		WorkerID: "worker-a",
		Reason:   subagent.DurableWorkerRSSWatchdogDisabled,
		Evidence: subagent.DurableWorkerRSSWatchdogEvidence{
			Reason:    subagent.DurableWorkerRSSWatchdogDisabled,
			CheckedAt: disabledAt,
		},
		CreatedAt: disabledAt,
	}); err != nil {
		t.Fatalf("Record disabled RSS watchdog event: %v", err)
	}
	unavailableAt := disabledAt.Add(time.Minute)
	if err := ledger.RecordWorkerRSSWatchdogEvent(ctx, subagent.DurableWorkerRSSWatchdogEvent{
		JobID:    "cron:rss",
		WorkerID: "worker-a",
		Reason:   subagent.DurableWorkerRSSWatchdogUnavailable,
		Evidence: subagent.DurableWorkerRSSWatchdogEvidence{
			Reason:    subagent.DurableWorkerRSSWatchdogUnavailable,
			CheckedAt: unavailableAt,
			ErrorText: "rss reader is nil",
		},
		CreatedAt: unavailableAt,
	}); err != nil {
		t.Fatalf("Record unavailable RSS watchdog event: %v", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN for unavailable RSS evidence: %+v", result.Status, result)
	}
	if !strings.Contains(result.Summary, "rss_watchdog_unavailable=1") {
		t.Fatalf("Summary = %q, want unavailable count", result.Summary)
	}
	item := findDoctorItem(result.Items, "rss_watchdog")
	if item.Name == "" {
		t.Fatalf("Items = %+v, want rss_watchdog item", result.Items)
	}
	if item.Status != StatusWarn {
		t.Fatalf("rss_watchdog status = %v, want WARN", item.Status)
	}
	for _, want := range []string{
		"rss_watchdog_disabled=1",
		"rss_watchdog_unavailable=1",
		"last_reason=rss_watchdog_unavailable",
		"checked_at=2026-04-27T13:31:00Z",
	} {
		if !strings.Contains(item.Note, want) {
			t.Fatalf("rss_watchdog note = %q, want %q", item.Note, want)
		}
	}
}

func TestDoctorDurableWorkerRSSWatchdog_ThresholdAndDrainEvidence(t *testing.T) {
	ctx := context.Background()
	ledger, _, cleanup := newDoctorRSSWatchdogLedger(t, ctx)
	defer cleanup()

	thresholdAt := time.Date(2026, 4, 27, 13, 35, 0, 0, time.UTC)
	if err := ledger.RecordWorkerRSSWatchdogEvent(ctx, subagent.DurableWorkerRSSWatchdogEvent{
		JobID:    "cron:threshold",
		WorkerID: "worker-a",
		Reason:   subagent.DurableWorkerRSSThresholdExceeded,
		Evidence: subagent.DurableWorkerRSSWatchdogEvidence{
			Reason:     subagent.DurableWorkerRSSThresholdExceeded,
			ObservedMB: 151,
			MaxMB:      100,
			CheckedAt:  thresholdAt,
		},
		CreatedAt: thresholdAt,
	}); err != nil {
		t.Fatalf("Record threshold RSS watchdog event: %v", err)
	}
	drainAt := thresholdAt.Add(time.Minute)
	if err := ledger.RecordWorkerRSSWatchdogEvent(ctx, subagent.DurableWorkerRSSWatchdogEvent{
		JobID:    "cron:threshold",
		WorkerID: "worker-a",
		Reason:   subagent.DurableWorkerRSSDrainStarted,
		Evidence: subagent.DurableWorkerRSSWatchdogEvidence{
			Reason:     subagent.DurableWorkerRSSThresholdExceeded,
			ObservedMB: 151,
			MaxMB:      100,
			CheckedAt:  drainAt,
			ErrorText:  "token=super-secret-value",
		},
		CreatedAt: drainAt,
	}); err != nil {
		t.Fatalf("Record drain RSS watchdog event: %v", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN for RSS drain evidence: %+v", result.Status, result)
	}
	item := findDoctorItem(result.Items, "rss_watchdog")
	if item.Name == "" {
		t.Fatalf("Items = %+v, want rss_watchdog item", result.Items)
	}
	if item.Status != StatusWarn {
		t.Fatalf("rss_watchdog status = %v, want WARN", item.Status)
	}
	for _, want := range []string{
		"rss_threshold_exceeded=1",
		"rss_drain_started=1",
		"last_reason=rss_drain_started",
		"evidence_reason=rss_threshold_exceeded",
		"max_mb=100",
		"observed_mb=151",
		"checked_at=2026-04-27T13:36:00Z",
		"error=[redacted]",
	} {
		if !strings.Contains(item.Note, want) {
			t.Fatalf("rss_watchdog note = %q, want %q", item.Note, want)
		}
	}
	if strings.Contains(result.Format(), "super-secret-value") {
		t.Fatalf("doctor output leaked secret:\n%s", result.Format())
	}
}

func TestDoctorDurableWorkerRSSWatchdog_StableRestartEvidence(t *testing.T) {
	ctx := context.Background()
	ledger, ms, cleanup := newDoctorRSSWatchdogLedger(t, ctx)
	defer cleanup()

	restartedAt := time.Date(2026, 4, 27, 13, 40, 0, 0, time.UTC)
	payload := map[string]any{
		"type":        string(subagent.DurableWorkerStableWatchdogRestart),
		"worker_id":   "worker-a",
		"reason":      string(subagent.DurableWorkerStableWatchdogRestart),
		"crash_count": 1,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal stable restart payload: %v", err)
	}
	if _, err := ms.DB().ExecContext(ctx, `
		INSERT INTO durable_worker_events
			(type, worker_id, reason, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		string(subagent.DurableWorkerStableWatchdogRestart),
		"worker-a",
		string(subagent.DurableWorkerStableWatchdogRestart),
		string(raw),
		restartedAt.UnixNano(),
	); err != nil {
		t.Fatalf("Insert stable restart event: %v", err)
	}

	result := CheckDurableLedger(ctx, ledger, "")

	if result.Status != StatusPass {
		t.Fatalf("Status = %v, want PASS because stable restart is evidence only: %+v", result.Status, result)
	}
	if strings.Contains(result.Summary, "restart_intent=1") {
		t.Fatalf("Summary = %q, stable restart should not increment restart intent/crash-loop degradation", result.Summary)
	}
	item := findDoctorItem(result.Items, "rss_watchdog")
	if item.Name == "" {
		t.Fatalf("Items = %+v, want rss_watchdog item", result.Items)
	}
	if item.Status != StatusPass {
		t.Fatalf("rss_watchdog status = %v, want PASS", item.Status)
	}
	for _, want := range []string{
		"stable_watchdog_restart=1",
		"last_reason=stable_watchdog_restart",
		"crash_loop=0",
	} {
		if !strings.Contains(item.Note, want) {
			t.Fatalf("rss_watchdog note = %q, want %q", item.Note, want)
		}
	}
}

func newDoctorRSSWatchdogLedger(t *testing.T, ctx context.Context) (*subagent.DurableLedger, *memory.SqliteStore, func()) {
	t.Helper()
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		_ = ms.Close(ctx)
		t.Fatalf("NewDurableLedger: %v", err)
	}
	now := time.Date(2026, 4, 27, 13, 29, 0, 0, time.UTC)
	if err := ledger.RecordSupervisorStatus(ctx, subagent.DurableSupervisorReport{
		Available:  true,
		ReportedAt: now,
	}); err != nil {
		_ = ms.Close(ctx)
		t.Fatalf("RecordSupervisorStatus: %v", err)
	}
	if err := ledger.RecordWorkerHeartbeat(ctx, subagent.DurableWorkerHeartbeat{
		WorkerID:    "worker-a",
		HeartbeatAt: time.Now().UTC(),
	}); err != nil {
		_ = ms.Close(ctx)
		t.Fatalf("RecordWorkerHeartbeat: %v", err)
	}
	return ledger, ms, func() {
		_ = ms.Close(ctx)
	}
}
