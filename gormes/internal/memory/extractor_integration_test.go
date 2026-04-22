// Package memory — extractor integration test against a real local LLM.
//
// This test is INTENTIONALLY not gated by a build tag. It runs under
// standard `go test ./...` but SKIPS cleanly when no OpenAI-compatible
// LLM is reachable at the configured endpoint. Philosophy: missing
// optional infrastructure must never fail the suite.
//
// Default endpoint: http://localhost:11434 (Ollama). Ollama exposes
// an OpenAI-compatible /v1/chat/completions endpoint that the existing
// hermes.HTTPClient consumes without modification — Gormes is "Go-native
// + bring-your-own-LLM" out of the box.
//
// Overrides (operator):
//
//	GORMES_RUN_OLLAMA_INTEGRATION  set to 1 to opt into live Ollama coverage
//	GORMES_EXTRACTOR_ENDPOINT   base URL (default http://localhost:11434)
//	GORMES_EXTRACTOR_MODEL      model tag to use (default gemma4:26b)
//
// Run just this test against a real Ollama:
//
//	GORMES_RUN_OLLAMA_INTEGRATION=1 GORMES_EXTRACTOR_MODEL=qwen2.5:3b \
//	  go test ./internal/memory/... -run TestExtractor_Integration_Ollama -v -timeout 3m
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

func integrationEndpoint() string {
	return ollama.Endpoint()
}

func integrationModel() string {
	return ollama.Model()
}

// TestExtractor_Integration_Ollama is the 100%-Go-native end-to-end
// crucible: no Python api_server, no Telegram, no fakes — just a local
// LLM (Ollama by default). Direct-inserts 3 entity-rich turns into the
// turns table, runs a real Extractor pointed at the local LLM, waits for
// the polling loop to drain the batch, then dumps entities + relationships
// + turn state via t.Logf.
func TestExtractor_Integration_Ollama(t *testing.T) {
	ollama.SkipUnlessExtractorReady(t)

	endpoint := integrationEndpoint()
	model := integrationModel()
	t.Logf("=== integration crucible: %s @ %s ===", model, endpoint)

	path := filepath.Join(t.TempDir(), "crucible.db")
	store, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	hc := hermes.NewHTTPClient(endpoint, os.Getenv("GORMES_EXTRACTOR_API_KEY"))

	// Direct-insert 3 entity-rich turns, bypassing the kernel + persistence
	// worker. Synthetic substitute for real Telegram DMs.
	highDensityTurns := []string{
		"I am setting up the Acme project in Springfield.",
		"Vania is helping me test the Neovim configuration.",
		"We need to optimize the Go backend for Trebuchet Dynamics.",
	}
	for i, content := range highDensityTurns {
		_, err := store.db.Exec(
			`INSERT INTO turns(session_id, role, content, ts_unix)
			 VALUES(?, 'user', ?, ?)`,
			"crucible-session", content, time.Now().Unix()+int64(i),
		)
		if err != nil {
			t.Fatalf("direct insert %d: %v", i, err)
		}
	}

	ext := NewExtractor(store, hc, ExtractorConfig{
		Model:        model,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    3,
		MaxAttempts:  3,
		CallTimeout:  180 * time.Second, // 26B models on CPU are slow
		BackoffBase:  500 * time.Millisecond,
		BackoffMax:   5 * time.Second,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go ext.Run(ctx)
	defer func() {
		shutdownCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		_ = ext.Close(shutdownCtx)
	}()

	// Wait for all 3 turns to leave extracted=0 (either ext=1 or ext=2).
	start := time.Now()
	deadline := time.Now().Add(4 * time.Minute)
	var lastUnprocessed int
	for time.Now().Before(deadline) {
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 0`).Scan(&lastUnprocessed)
		if lastUnprocessed == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}
	elapsed := time.Since(start)

	// ── Telemetry ────────────────────────────────────────────────────────
	t.Logf("=== CRUCIBLE TELEMETRY (elapsed=%v) ===", elapsed.Truncate(time.Second))

	// 1. Turn state distribution (the Queue State query).
	stateRows, _ := store.db.Query(`SELECT extracted, COUNT(*) FROM turns GROUP BY extracted ORDER BY extracted`)
	t.Logf("--- Queue state (turns grouped by extracted) ---")
	for stateRows.Next() {
		var state, count int
		_ = stateRows.Scan(&state, &count)
		label := map[int]string{0: "unprocessed", 1: "extracted", 2: "dead-letter"}[state]
		t.Logf("  extracted=%d (%-11s): %d turns", state, label, count)
	}
	stateRows.Close()

	// 2. Entities.
	entRows, _ := store.db.Query(`SELECT id, name, type, description FROM entities ORDER BY id`)
	t.Logf("--- Entities ---")
	entCount := 0
	for entRows.Next() {
		var id int64
		var name, typ, desc string
		_ = entRows.Scan(&id, &name, &typ, &desc)
		t.Logf("  [%d] %-25s %-15s %q", id, name, typ, desc)
		entCount++
	}
	entRows.Close()

	// 3. Relationships joined through entity names (the Edges query).
	relRows, _ := store.db.Query(`
		SELECT e1.name, r.predicate, e2.name, r.weight
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		ORDER BY r.weight DESC, e1.name, e2.name
	`)
	t.Logf("--- Relationships ---")
	relCount := 0
	for relRows.Next() {
		var src, pred, tgt string
		var w float64
		_ = relRows.Scan(&src, &pred, &tgt, &w)
		t.Logf("  %-25s --[%-10s %.2f]--> %s", src, pred, w, tgt)
		relCount++
	}
	relRows.Close()

	// 4. Extractor state per-turn.
	errRows, _ := store.db.Query(`SELECT id, extracted, extraction_attempts, COALESCE(extraction_error,'') FROM turns ORDER BY id`)
	t.Logf("--- Per-turn extractor state ---")
	var totalAttempts int
	for errRows.Next() {
		var id int64
		var state, n int
		var msg string
		_ = errRows.Scan(&id, &state, &n, &msg)
		totalAttempts += n
		if msg != "" && len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		t.Logf("  [%d] extracted=%d attempts=%d err=%q", id, state, n, msg)
	}
	errRows.Close()
	t.Logf("  total attempts across all turns: %d", totalAttempts)

	// 5. Expected-entity name match diagnostic.
	expected := []string{"Acme", "Springfield", "Vania", "Neovim", "Go", "Trebuchet Dynamics"}
	var matched []string
	for _, want := range expected {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entities WHERE name = ?`, want).Scan(&n)
		if n > 0 {
			matched = append(matched, want)
		}
	}
	t.Logf("--- Expected-entity name matches: %d / %d ---", len(matched), len(expected))
	t.Logf("  matched: %v", matched)
	t.Logf("=== END CRUCIBLE TELEMETRY ===")

	fmt.Printf("\n[crucible] memory.db path for external inspection: %s\n", path)
	fmt.Printf("[crucible] model=%s endpoint=%s elapsed=%v\n\n", model, endpoint, elapsed.Truncate(time.Second))

	// ── Correctness assertions ─────────────────────────────────────────
	if lastUnprocessed > 0 {
		t.Errorf("still %d unprocessed turns after 4min — extractor did not drain them", lastUnprocessed)
	}
	if entCount == 0 {
		t.Errorf("entities table empty — LLM returned no entities (or JSON validation rejected everything)")
	}
	// Rough sanity: for three entity-rich turns, we expect AT LEAST 2 entities
	// from the visibly-named proper nouns. A stricter assertion is inadvisable
	// because smaller models may drop some. The telemetry log above tells the
	// whole story.
	if entCount < 2 && lastUnprocessed == 0 {
		t.Errorf("only %d entities extracted from 3 entity-rich turns — model quality concern (not a pipeline bug unless all turns dead-lettered)", entCount)
	}
	// Warn but don't fail if we extracted zero "Canonical" names.
	if len(matched) == 0 && entCount > 0 {
		t.Logf("WARNING: extracted %d entities but zero match the canonical set %v — model may have paraphrased or hallucinated names", entCount, expected)
	}

	// Trim `strings` unused-warning — we may want it in future diagnostics.
	_ = strings.Contains
}
