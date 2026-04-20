package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEmbedServer returns deterministic vectors for each input.
// Entity "A" maps to [1,0,0,0]; "B" to [0,1,0,0]; everything else to zeros.
func fakeEmbedServer(t *testing.T, callCount *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		var vec []float32
		switch {
		case contains(req.Input, "A"):
			vec = []float32{1, 0, 0, 0}
		case contains(req.Input, "B"):
			vec = []float32{0, 1, 0, 0}
		default:
			vec = []float32{0, 0, 0, 0}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": vec, "index": 0},
			},
		})
	}))
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEmbedder_EmbedsMissingEntities(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	// Seed 2 entities, no embeddings.
	_, _ = store.db.Exec(`
		INSERT INTO entities(name, type, description, updated_at) VALUES
			('A','PERSON','',1),
			('B','PERSON','',2)
	`)

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
		if n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
	if n != 2 {
		t.Errorf("embeddings count = %d, want 2", n)
	}
}

func TestEmbedder_SkipsAlreadyEmbeddedForCurrentModel(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)
	vec := []float32{1, 0, 0, 0}
	_, _ = store.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'test-model', 4, ?, 1)`,
		id, encodeFloat32LE(vec))

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go e.Run(ctx)
	<-ctx.Done()
	_ = e.Close(context.Background())

	// No new embedding calls (entity already covered for this model).
	if calls.Load() != 0 {
		t.Errorf("embed calls = %d, want 0 (already embedded for current model)", calls.Load())
	}
}

func TestEmbedder_ReplacesOnModelChange(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)
	// Pre-populate with a different model's embedding.
	_, _ = store.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'OLD-model', 4, ?, 1)`,
		id, encodeFloat32LE([]float32{0, 0, 1, 0}))

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id=? AND model='test-model'`, id).Scan(&n)
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&n)
	if n != 1 {
		t.Errorf("after model switch, entity has %d embedding rows; want 1 (REPLACE)", n)
	}
	var model string
	_ = store.db.QueryRow(`SELECT model FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&model)
	if model != "test-model" {
		t.Errorf("model = %q, want test-model", model)
	}
}

func TestEmbedder_NormalizesBeforeStorage(t *testing.T) {
	// Fake server returns a non-unit vector; the embedder must L2-normalize
	// before storage so semanticSeeds can use dot product directly.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{3, 4, 0, 0}, "index": 0}, // magnitude 5
			},
		})
	}))
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    1,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go e.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
		if n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var blob []byte
	_ = store.db.QueryRow(`SELECT vec FROM entity_embeddings WHERE entity_id=?`, id).Scan(&blob)
	stored, _ := decodeFloat32LE(blob)
	// Should be [0.6, 0.8, 0, 0] — (3, 4) / 5.
	if len(stored) != 4 {
		t.Fatalf("len = %d, want 4", len(stored))
	}
	if abs32(stored[0]-0.6) > 1e-5 || abs32(stored[1]-0.8) > 1e-5 {
		t.Errorf("stored = %v, want [0.6, 0.8, 0, 0] (normalized)", stored)
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func TestEmbedder_CloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	e := NewEmbedder(store, newEmbedClient("http://127.0.0.1:1", ""), EmbedderConfig{}, nil, newSemanticCache())

	if err := e.Close(context.Background()); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestEmbedder_RunExitsOnCtxCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	e := NewEmbedder(store, newEmbedClient("http://127.0.0.1:1", ""), EmbedderConfig{
		PollInterval: 50 * time.Millisecond,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s of cancel")
	}
}
