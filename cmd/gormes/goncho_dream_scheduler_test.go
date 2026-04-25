package main

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

func TestGonchoDreamDoctorAndMemoryStatusExposeDreamEvidence(t *testing.T) {
	seedGonchoDoctorZeroStateDB(t)
	t.Setenv("GORMES_GONCHO_DREAM_ENABLED", "true")
	now := time.Now().UTC().Truncate(time.Second)
	db := openDreamCommandDB(t)
	seedCommandDreamIntent(t, db.DB(), "user-pending", "pending", now.Add(-time.Hour), 0)
	seedCommandDreamIntent(t, db.DB(), "user-running", "in_progress", now.Add(-time.Hour), 0)
	seedCommandDreamIntent(t, db.DB(), "user-cooldown", "completed", now.Add(-2*time.Hour), now.Add(6*time.Hour).Unix())
	if err := db.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
	if err != nil {
		t.Fatalf("doctor Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("doctor stderr = %q, want empty", stderr)
	}
	for _, want := range []string{"dream_status: active", "dream_pending", "dream_in_progress", "dream_cooldown"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor stdout missing %q:\n%s", want, stdout)
		}
	}

	statusOut := runMemoryStatusCommand(t)
	for _, want := range []string{"dream_status: active", "dream_pending", "dream_in_progress", "dream_cooldown"} {
		if !strings.Contains(statusOut, want) {
			t.Fatalf("memory status output missing %q:\n%s", want, statusOut)
		}
	}
}

func TestGonchoDreamDoctorAndStatusReportDisabledAndUnavailableEvidence(t *testing.T) {
	seedGonchoDoctorZeroStateDB(t)

	stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
	if err != nil {
		t.Fatalf("doctor disabled Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "dream_status: dream_disabled") || !strings.Contains(stdout, "dream_disabled") {
		t.Fatalf("doctor disabled stdout missing dream_disabled evidence:\n%s", stdout)
	}
	statusOut := runMemoryStatusCommand(t)
	if !strings.Contains(statusOut, "dream_status: dream_disabled") {
		t.Fatalf("memory status disabled output missing dream_disabled:\n%s", statusOut)
	}

	t.Setenv("GORMES_GONCHO_DREAM_ENABLED", "true")
	db := openDreamCommandDB(t)
	if _, err := db.DB().Exec(`DROP TABLE goncho_dreams`); err != nil {
		t.Fatalf("drop goncho_dreams: %v", err)
	}
	if err := db.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stdout, stderr, err = runGonchoDoctorCommand(t, "goncho", "doctor")
	if err != nil {
		t.Fatalf("doctor unavailable Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "dream_status: dream_unavailable") {
		t.Fatalf("doctor unavailable stdout missing dream_unavailable:\n%s", stdout)
	}
	statusOut = runMemoryStatusCommand(t)
	if !strings.Contains(statusOut, "dream_status: dream_unavailable") {
		t.Fatalf("memory status unavailable output missing dream_unavailable:\n%s", statusOut)
	}
}

func openDreamCommandDB(t *testing.T) *memory.SqliteStore {
	t.Helper()
	store, err := memory.OpenSqlite(config.MemoryDBPath(), 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	return store
}

func seedCommandDreamIntent(t *testing.T, db *sql.DB, observedPeerID, status string, at time.Time, cooldownUntil int64) {
	t.Helper()
	completedAt := int64(0)
	if status == "completed" {
		completedAt = at.Unix()
	}
	_, err := db.Exec(`
		INSERT INTO goncho_dreams(
			workspace_id, observer_peer_id, observed_peer_id, work_unit_key, dream_type,
			status, manual, reason, new_conclusions, min_conclusions, last_conclusion_id,
			scheduled_for, completed_at, cooldown_until, idle_until, last_activity_at,
			created_at, updated_at
		)
		VALUES('gormes', 'gormes', ?, ?, 'consolidation', ?, 0, 'doctor seed',
			50, 50, 50, ?, NULLIF(?, 0), ?, 0, ?, ?, ?)`,
		observedPeerID,
		"dream:consolidation:gormes:gormes:"+observedPeerID,
		status,
		at.Unix(),
		completedAt,
		cooldownUntil,
		at.Unix(),
		at.Unix(),
		at.Unix(),
	)
	if err != nil {
		t.Fatalf("seed command dream intent: %v", err)
	}
}

func runMemoryStatusCommand(t *testing.T) string {
	t.Helper()
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"memory", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("memory status Execute: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("memory status stderr = %q, want empty", stderr.String())
	}
	return stdout.String()
}
