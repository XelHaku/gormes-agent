package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// seedEmbeddedGraph inserts N entities with fabricated embeddings for the
// given model. The embedding for each entity is a unit vector with a 1 at
// position (id-1) mod dim, where id is the entity's autoincrement primary
// key. Because SQLite assigns IDs sequentially across calls, entities
// inserted in separate seedEmbeddedGraph invocations get distinct positions
// (and thus orthogonal vectors), making cosine = 0 for non-matching queries.
func seedEmbeddedGraph(t *testing.T, s *SqliteStore, model string, dim int, names []string) map[string]int64 {
	t.Helper()
	ids := make(map[string]int64)
	now := time.Now().Unix()
	for _, name := range names {
		_, err := s.db.Exec(
			`INSERT INTO entities(name, type, updated_at) VALUES(?, 'PERSON', ?)`,
			name, now)
		if err != nil {
			t.Fatal(err)
		}
		var id int64
		_ = s.db.QueryRow(`SELECT id FROM entities WHERE name = ?`, name).Scan(&id)
		ids[name] = id

		vec := make([]float32, dim)
		vec[(id-1)%int64(dim)] = 1.0
		l2Normalize(vec) // stays unit because only one nonzero
		blob := encodeFloat32LE(vec)
		_, err = s.db.Exec(
			`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
			 VALUES(?, ?, ?, ?, ?)`,
			id, model, dim, blob, now)
		if err != nil {
			t.Fatalf("insert embedding for %s: %v", name, err)
		}
	}
	return ids
}

func TestSemanticSeeds_MatchesExactPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	ids := seedEmbeddedGraph(t, s, "test-model", 4,
		[]string{"A", "B", "C", "D"}) // positions 0, 1, 2, 3

	// Query vector matches position 2: [0, 0, 1, 0]
	query := []float32{0, 0, 1, 0}
	l2Normalize(query)

	got, err := semanticSeeds(context.Background(), s.db, cache, "test-model", query, 1, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0] != ids["C"] {
		t.Errorf("got = %d, want %d (C)", got[0], ids["C"])
	}
}

func TestSemanticSeeds_ThresholdFiltersLowScores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	seedEmbeddedGraph(t, s, "test-model", 4, []string{"A", "B"})

	// Query orthogonal to both seeded entities.
	query := []float32{0, 0, 0, 1}
	l2Normalize(query)

	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5) // threshold 0.5
	// Neither "A" nor "B" has any overlap with position 3; all cosines are 0.
	if len(got) != 0 {
		t.Errorf("threshold 0.5 should filter; got %d seeds", len(got))
	}
}

func TestSemanticSeeds_FiltersByModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	seedEmbeddedGraph(t, s, "model-A", 4, []string{"X"})
	seedEmbeddedGraph(t, s, "model-B", 4, []string{"Y"})

	// Query with model-A; should only see X, not Y (same vector).
	query := []float32{1, 0, 0, 0}
	l2Normalize(query)
	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"model-A", query, 5, 0.5)
	for _, id := range got {
		var name string
		_ = s.db.QueryRow(`SELECT name FROM entities WHERE id = ?`, id).Scan(&name)
		if name == "Y" {
			t.Errorf("model-B's Y leaked into model-A scan")
		}
	}
}

func TestSemanticSeeds_CacheInvalidatesOnGraphBump(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	ids1 := seedEmbeddedGraph(t, s, "test-model", 4, []string{"A"})

	query := []float32{1, 0, 0, 0}
	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5)
	if len(got) != 1 || got[0] != ids1["A"] {
		t.Fatalf("first call: got %v, want [%d]", got, ids1["A"])
	}

	// Add a new embedding + bump the cache version.
	seedEmbeddedGraph(t, s, "test-model", 4, []string{"B"})
	cache.bump()

	query2 := []float32{0, 1, 0, 0}
	got, _ = semanticSeeds(context.Background(), s.db, cache,
		"test-model", query2, 5, 0.5)
	if len(got) == 0 {
		t.Error("expected B to surface after cache bump; got nothing")
	}
}

func TestSemanticSeeds_EmptyDatabaseReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	query := []float32{1, 0, 0, 0}
	got, err := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5)
	if err != nil {
		t.Errorf("err = %v, want nil on empty DB", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestSemanticSeeds_SkipsCorruptRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	// Good entity with correct embedding.
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Good','PERSON',1)`)
	var goodID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Good'`).Scan(&goodID)
	goodVec := []float32{1, 0, 0, 0}
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		 VALUES(?, 'm', 4, ?, 1)`,
		goodID, encodeFloat32LE(goodVec))

	// Corrupt entity — dim says 4 but BLOB has 3 bytes. CHECK constraint
	// on dim passes (4 > 0 and <= 4096), but decode should fail.
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Bad','PERSON',1)`)
	var badID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Bad'`).Scan(&badID)
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		 VALUES(?, 'm', 4, ?, 1)`,
		badID, []byte{1, 2, 3}) // not a multiple of 4

	query := []float32{1, 0, 0, 0}
	got, err := semanticSeeds(context.Background(), s.db, cache,
		"m", query, 5, 0.5)
	if err != nil {
		t.Errorf("scan should tolerate corrupt rows: %v", err)
	}
	// Good should still be found.
	if len(got) == 0 || got[0] != goodID {
		t.Errorf("got = %v, want [%d]", got, goodID)
	}
}
