package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func openGraph(t *testing.T) *SqliteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

func TestGraph_UpsertEntityInsertsThenUpdates(t *testing.T) {
	s := openGraph(t)
	v := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "Jose", Type: "PERSON", Description: "first"},
		},
	}
	if err := writeGraphBatch(context.Background(), s.db, v, nil); err != nil {
		t.Fatal(err)
	}
	v.Entities[0].Description = "second"
	if err := writeGraphBatch(context.Background(), s.db, v, nil); err != nil {
		t.Fatal(err)
	}

	var n int
	var desc string
	_ = s.db.QueryRow("SELECT COUNT(*), MAX(description) FROM entities").Scan(&n, &desc)
	if n != 1 {
		t.Errorf("entities count = %d, want 1 (upsert should dedupe)", n)
	}
	if desc != "second" {
		t.Errorf("description = %q, want 'second' (non-empty must override)", desc)
	}
}

func TestGraph_UpsertEntityEmptyDescDoesNotOverwrite(t *testing.T) {
	s := openGraph(t)
	_ = writeGraphBatch(context.Background(), s.db, ValidatedOutput{
		Entities: []ValidatedEntity{{Name: "X", Type: "CONCEPT", Description: "original"}},
	}, nil)
	_ = writeGraphBatch(context.Background(), s.db, ValidatedOutput{
		Entities: []ValidatedEntity{{Name: "X", Type: "CONCEPT", Description: ""}},
	}, nil)

	var desc string
	_ = s.db.QueryRow(
		`SELECT description FROM entities WHERE name = 'X' AND type = 'CONCEPT'`,
	).Scan(&desc)
	if desc != "original" {
		t.Errorf("description = %q, want 'original' (empty must NOT overwrite)", desc)
	}
}

func TestGraph_UpsertRelationshipAccumulatesWeight(t *testing.T) {
	s := openGraph(t)
	batch := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "A", Type: "PERSON"},
			{Name: "B", Type: "PROJECT"},
		},
		Relationships: []ValidatedRelationship{
			{Source: "A", Target: "B", Predicate: "WORKS_ON", Weight: 0.5},
		},
	}
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)

	var w float64
	_ = s.db.QueryRow(
		`SELECT weight FROM relationships WHERE predicate = 'WORKS_ON'`,
	).Scan(&w)
	if w != 1.5 {
		t.Errorf("weight = %v, want 1.5 (0.5 * 3)", w)
	}
}

func TestGraph_UpsertRelationshipWeightCapAt10(t *testing.T) {
	s := openGraph(t)
	batch := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "A", Type: "PERSON"},
			{Name: "B", Type: "PROJECT"},
		},
		Relationships: []ValidatedRelationship{
			{Source: "A", Target: "B", Predicate: "WORKS_ON", Weight: 1.0},
		},
	}
	for i := 0; i < 15; i++ {
		_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	}

	var w float64
	_ = s.db.QueryRow(
		`SELECT weight FROM relationships WHERE predicate = 'WORKS_ON'`,
	).Scan(&w)
	if w != 10.0 {
		t.Errorf("weight = %v, want 10.0 (capped)", w)
	}
}

func TestGraph_UpsertRelationshipBumpsLastSeenWithoutRewritingUpdatedAt(t *testing.T) {
	s := openGraph(t)
	batch := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "A", Type: "PERSON"},
			{Name: "B", Type: "PROJECT"},
		},
		Relationships: []ValidatedRelationship{
			{Source: "A", Target: "B", Predicate: "WORKS_ON", Weight: 1.0},
		},
	}

	if err := writeGraphBatch(context.Background(), s.db, batch, nil); err != nil {
		t.Fatalf("first writeGraphBatch: %v", err)
	}
	if _, err := s.db.Exec(
		`UPDATE relationships
		 SET updated_at = 100, last_seen = 100
		 WHERE predicate = 'WORKS_ON'`,
	); err != nil {
		t.Fatalf("pin relationship timestamps: %v", err)
	}

	if err := writeGraphBatch(context.Background(), s.db, batch, nil); err != nil {
		t.Fatalf("second writeGraphBatch: %v", err)
	}

	var updatedAt, lastSeen int64
	if err := s.db.QueryRow(
		`SELECT updated_at, last_seen FROM relationships WHERE predicate = 'WORKS_ON'`,
	).Scan(&updatedAt, &lastSeen); err != nil {
		t.Fatalf("read relationship timestamps: %v", err)
	}
	if updatedAt != 100 {
		t.Fatalf("updated_at = %d, want preserved structural timestamp 100", updatedAt)
	}
	if lastSeen <= 100 {
		t.Fatalf("last_seen = %d, want observation freshness > 100", lastSeen)
	}
}

func TestGraph_MarkTurnsExtracted(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES
		('s','user','a',1),('s','user','b',2),('s','assistant','c',3)`)

	if err := writeGraphBatch(context.Background(), s.db, ValidatedOutput{}, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}

	var extracted1, extracted3 int
	_ = s.db.QueryRow(`SELECT extracted FROM turns WHERE id = 1`).Scan(&extracted1)
	_ = s.db.QueryRow(`SELECT extracted FROM turns WHERE id = 3`).Scan(&extracted3)
	if extracted1 != 1 {
		t.Errorf("turn 1 extracted = %d, want 1", extracted1)
	}
	if extracted3 != 0 {
		t.Errorf("turn 3 extracted = %d, want 0 (not in batch)", extracted3)
	}
}

func TestGraph_CheckConstraintRejectsBadPredicate(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1),('B','PROJECT',1)`)

	_, err := s.db.Exec(
		`INSERT INTO relationships(source_id,target_id,predicate,updated_at) VALUES(1,2,'NOT_WHITELISTED',1)`)
	if err == nil {
		t.Error("expected CHECK constraint to reject NOT_WHITELISTED predicate")
	}
}

func TestGraph_IncrementAttempts(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES
		('s','user','a',1),('s','user','b',2)`)

	if err := incrementAttempts(context.Background(), s.db, []int64{1, 2}, "boom"); err != nil {
		t.Fatal(err)
	}

	var attempts int
	var errMsg string
	_ = s.db.QueryRow(`SELECT extraction_attempts, extraction_error FROM turns WHERE id = 1`).Scan(&attempts, &errMsg)
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if errMsg != "boom" {
		t.Errorf("error = %q, want 'boom'", errMsg)
	}
}

func TestGraph_MarkDeadLetter(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES('s','user','a',1)`)

	if err := markDeadLetter(context.Background(), s.db, []int64{1}, "final"); err != nil {
		t.Fatal(err)
	}

	var extracted int
	var errMsg string
	_ = s.db.QueryRow(`SELECT extracted, extraction_error FROM turns WHERE id = 1`).Scan(&extracted, &errMsg)
	if extracted != 2 {
		t.Errorf("extracted = %d, want 2 (dead-letter)", extracted)
	}
	if errMsg != "final" {
		t.Errorf("error = %q, want 'final'", errMsg)
	}
}
