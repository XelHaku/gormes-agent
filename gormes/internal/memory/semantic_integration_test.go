// Package memory — Phase 3.D semantic recall crucible against local Ollama.
//
// Gated by the same skipIfNoOllama helper from extractor_integration_test.go.
// Proves the gap closure: a query that lexically matches nothing
// ("tell me about my projects") nevertheless surfaces Acme via
// the semantic seed layer.
//
// Skips cleanly if Ollama or the chosen embedding model aren't available.
// Environment:
//
//	GORMES_RUN_OLLAMA_INTEGRATION  set to 1 to opt into live Ollama coverage
//	GORMES_EXTRACTOR_ENDPOINT  (default http://localhost:11434)
//	GORMES_EXTRACTOR_MODEL     (chat model for extractor; see 3.B)
//	GORMES_SEMANTIC_MODEL      (embedding model; default nomic-embed-text)
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/testutil/ollama"
)

func semanticModel() string {
	if v := os.Getenv("GORMES_SEMANTIC_MODEL"); v != "" {
		return v
	}
	return "nomic-embed-text"
}

func skipIfNoEmbeddingModel(t *testing.T) {
	t.Helper()
	// Reuse the extractor-ready gate for base reachability, then probe the embed
	// endpoint specifically.
	ollama.SkipUnlessExtractorReady(t)
	ec := newEmbedClient(integrationEndpoint(), "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ec.Embed(ctx, semanticModel(), "probe")
	if err != nil {
		t.Skipf("embedding model %q not available at %s: %v\n"+
			"  Pull with: ollama pull %s",
			semanticModel(), integrationEndpoint(), err, semanticModel())
	}
}

func TestRecall_Integration_Ollama_MyProjectsFindsAcme(t *testing.T) {
	skipIfNoEmbeddingModel(t)

	endpoint := integrationEndpoint()
	chatModel := integrationModel()
	embedModel := semanticModel()
	t.Logf("=== 3.D crucible: extractor=%s, embedder=%s @ %s ===",
		chatModel, embedModel, endpoint)

	path := filepath.Join(t.TempDir(), "semantic.db")
	store, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	// ── Seed 3 entity-rich turns ──────────────────────────────────────
	turns := []string{
		"I am setting up the Acme project in Springfield.",
		"Vania is helping me test the Neovim configuration.",
		"Juan works on the Go backend of Acme every day.",
	}
	for i, content := range turns {
		_, err := store.db.Exec(
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id)
			 VALUES(?, 'user', ?, ?, ?)`,
			"sem-session", content, time.Now().Unix()+int64(i), "telegram:42")
		if err != nil {
			t.Fatal(err)
		}
	}

	// ── Phase A: run extractor to populate entities ───────────────────
	hc := hermes.NewHTTPClient(endpoint, "")
	ext := NewExtractor(store, hc, ExtractorConfig{
		Model:        chatModel,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    3,
		CallTimeout:  180 * time.Second,
	}, nil)
	extCtx, extCancel := context.WithTimeout(context.Background(), 4*time.Minute)
	go ext.Run(extCtx)
	for extCtx.Err() == nil {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 0`).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	extCancel()
	_ = ext.Close(context.Background())

	var entCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entCount)
	t.Logf("extractor populated %d entities", entCount)
	if entCount == 0 {
		t.Fatal("no entities extracted — cannot proceed")
	}

	// ── Phase B: run embedder to populate entity_embeddings ───────────
	cache := newSemanticCache()
	ec := newEmbedClient(endpoint, "")
	embedder := NewEmbedder(store, ec, EmbedderConfig{
		Model:        embedModel,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  30 * time.Second,
	}, nil, cache)
	embCtx, embCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	go embedder.Run(embCtx)
	for embCtx.Err() == nil {
		var n int
		_ = store.db.QueryRow(
			`SELECT COUNT(*) FROM entities e
			 LEFT JOIN entity_embeddings ee ON ee.entity_id = e.id AND ee.model = ?
			 WHERE ee.entity_id IS NULL`, embedModel).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	embCancel()
	_ = embedder.Close(context.Background())

	var embCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&embCount)
	t.Logf("embedder populated %d embeddings", embCount)

	// ── Phase C: run recall with "tell me about my projects" ──────────
	prov := NewRecall(store, RecallConfig{
		WeightThreshold:       1.0,
		MaxFacts:              10,
		Depth:                 2,
		MaxSeeds:              5,
		SemanticModel:         embedModel,
		SemanticTopK:          3,
		SemanticMinSimilarity: 0.35,
		QueryEmbedTimeout:     5 * time.Second, // generous for the crucible
	}, nil).WithEmbedClient(ec, cache)

	recallCtx, recallCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer recallCancel()
	block := prov.GetContext(recallCtx, RecallInput{
		UserMessage: "tell me about my projects",
		ChatKey:     "telegram:42",
	})

	t.Logf("=== FENCE FOR 'tell me about my projects' ===")
	if block == "" {
		t.Logf("  (empty — semantic layer did not bridge the gap)")
	} else {
		for _, line := range strings.Split(block, "\n") {
			t.Logf("  %s", line)
		}
	}
	t.Logf("=== END FENCE ===")

	fmt.Printf("\n[3.D] memory.db: %s\n", path)
	fmt.Printf("[3.D] chat_model=%s embed_model=%s entities=%d embeddings=%d\n\n",
		chatModel, embedModel, entCount, embCount)

	// Core assertion: the fence for the non-lexical query contains Acme.
	// This is the Phase 3.D ship criterion.
	if block == "" {
		t.Errorf("block empty for non-lexical query — semantic layer didn't fire")
	}
	if !strings.Contains(block, "Acme") {
		t.Errorf("fence missing Acme on non-lexical query; got %q", block)
	}
}
