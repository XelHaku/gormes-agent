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
//   GORMES_EXTRACTOR_ENDPOINT   base URL (default http://localhost:11434)
//   GORMES_EXTRACTOR_MODEL      model tag to use (default gemma4:26b)
//
// Run just this test against a real Ollama:
//   GORMES_EXTRACTOR_MODEL=qwen2.5:3b \
//     go test ./internal/memory/... -run TestExtractor_Integration_Ollama -v -timeout 3m
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
)

// ollamaDefaultEndpoint is the conventional Ollama localhost port.
// Must NOT include /v1 — hermes.HTTPClient appends /v1/chat/completions.
const ollamaDefaultEndpoint = "http://localhost:11434"
const ollamaDefaultModel = "gemma4:26b"

func integrationEndpoint() string {
	if v := os.Getenv("GORMES_EXTRACTOR_ENDPOINT"); v != "" {
		return v
	}
	return ollamaDefaultEndpoint
}

func integrationModel() string {
	if v := os.Getenv("GORMES_EXTRACTOR_MODEL"); v != "" {
		return v
	}
	return ollamaDefaultModel
}

// skipIfNoOllama pings the /v1/models endpoint. Any connection failure,
// 4xx, or 5xx → t.Skip with a helpful message. Also verifies the
// configured model is listed; skips with a clear message if not.
func skipIfNoOllama(t *testing.T) {
	t.Helper()
	endpoint := integrationEndpoint()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(endpoint + "/v1/models")
	if err != nil {
		t.Skipf("LLM endpoint %s not reachable (connection refused?): %v\n"+
			"  To run this test: start Ollama (or any OpenAI-compatible server)\n"+
			"  and optionally set GORMES_EXTRACTOR_ENDPOINT / GORMES_EXTRACTOR_MODEL.",
			endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Skipf("LLM endpoint %s returned HTTP %d: %s", endpoint, resp.StatusCode, string(body))
	}

	// Verify the model is available.
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		t.Skipf("could not decode /v1/models response: %v", err)
	}
	want := integrationModel()
	for _, m := range models.Data {
		if m.ID == want {
			return
		}
	}
	available := make([]string, 0, len(models.Data))
	for _, m := range models.Data {
		available = append(available, m.ID)
	}
	t.Skipf("model %q not loaded on %s; available: %v\n"+
		"  Pull with: ollama pull %s\n"+
		"  Or override with GORMES_EXTRACTOR_MODEL=<one of the above>.",
		want, endpoint, available, want)
}

// TestExtractor_Integration_Ollama is the 100%-Go-native end-to-end
// crucible: no Python api_server, no Telegram, no fakes — just a local
// LLM (Ollama by default). Direct-inserts 3 entity-rich turns into the
// turns table, runs a real Extractor pointed at the local LLM, waits for
// the polling loop to drain the batch, then dumps entities + relationships
// + turn state via t.Logf.
func TestExtractor_Integration_Ollama(t *testing.T) {
	skipIfNoOllama(t)

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
		"I am setting up the AzulVigia project in Cadereyta.",
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
	expected := []string{"AzulVigia", "Cadereyta", "Vania", "Neovim", "Go", "Trebuchet Dynamics"}
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
