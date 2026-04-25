package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestGonchoDreamMigrationAddsSchedulerTable(t *testing.T) {
	store, err := OpenSqlite(filepath.Join(t.TempDir(), "memory.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	for _, col := range []string{
		"workspace_id",
		"observer_peer_id",
		"observed_peer_id",
		"work_unit_key",
		"dream_type",
		"status",
		"manual",
		"reason",
		"new_conclusions",
		"min_conclusions",
		"last_conclusion_id",
		"scheduled_for",
		"started_at",
		"completed_at",
		"cancelled_at",
		"stale_at",
		"last_activity_at",
		"cooldown_until",
		"idle_until",
		"created_at",
		"updated_at",
	} {
		var name string
		err := store.db.QueryRow(`SELECT name FROM pragma_table_info('goncho_dreams') WHERE name = ?`, col).Scan(&name)
		if err != nil {
			t.Fatalf("goncho_dreams column %q missing: %v", col, err)
		}
	}

	insert := func(status string) error {
		_, err := store.db.Exec(`
			INSERT INTO goncho_dreams(
				workspace_id, observer_peer_id, observed_peer_id, work_unit_key, dream_type,
				status, manual, reason, new_conclusions, min_conclusions, last_conclusion_id,
				scheduled_for, last_activity_at, cooldown_until, idle_until, created_at, updated_at
			)
			VALUES('default', 'gormes', 'user-1', 'dream:consolidation:default:gormes:user-1',
				'consolidation', ?, 0, 'test', 50, 50, 50, 1, 1, 0, 0, 1, 1)`, status)
		return err
	}
	if err := insert("pending"); err != nil {
		t.Fatalf("pending dream rejected: %v", err)
	}
	if err := insert("in_progress"); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate active dream err = %v, want partial unique constraint", err)
	}
	if _, err := store.db.Exec(`UPDATE goncho_dreams SET status = 'completed', completed_at = 2, updated_at = 2`); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if err := insert("pending"); err != nil {
		t.Fatalf("new pending after completed history rejected: %v", err)
	}
}
