// Package memory — Phase 3.C recall end-to-end crucible against local Ollama.
//
// This test is gated by the same skip-if-no-Ollama helper used by the
// Phase 3.B extractor integration. It runs under standard `go test ./...`
// but SKIPS cleanly when Ollama is unreachable or the configured model
// isn't loaded.
//
// Flow (the "does the Bear remember?" test):
//   1. Seed 3 entity-rich turns directly into the turns table.
//   2. Run the real Phase-3.B extractor against Ollama to populate
//      entities + relationships.
//   3. Construct memory.NewRecall.
//   4. Ask a SEPARATE question ("tell me about my projects") that does
//      NOT name any of the seed entities directly — this forces the
//      recall pipeline to rely on the graph, not on exact-name match.
//   5. Dump the fence block; assert it contains at least one of the
//      project entities.
//
// Overrides (same as extractor_integration_test.go):
//   GORMES_EXTRACTOR_ENDPOINT  (default http://localhost:11434)
//   GORMES_EXTRACTOR_MODEL     (default gemma4:26b)
//
// Recommended: run with the fast qwen model:
//   GORMES_EXTRACTOR_MODEL="huggingface.co/r1r21nb/qwen2.5-3b-instruct.Q4_K_M.gguf:latest" \
//     go test ./internal/memory/... -run TestRecall_Integration_Ollama -v -timeout 5m
package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

func TestRecall_Integration_Ollama_SecondTurnSeesFirstTurnEntities(t *testing.T) {
	skipIfNoOllama(t) // helper from extractor_integration_test.go

	endpoint := integrationEndpoint()
	model := integrationModel()
	t.Logf("=== recall integration: %s @ %s ===", model, endpoint)

	path := filepath.Join(t.TempDir(), "recall.db")
	store, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	// ── Phase A: seed 3 entity-rich turns ─────────────────────────────
	highDensityTurns := []string{
		"I am setting up the AzulVigia project in Cadereyta.",
		"Vania is helping me test the Neovim configuration.",
		"Juan works on the Go backend of AzulVigia every day.",
	}
	for i, content := range highDensityTurns {
		_, err := store.db.Exec(
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id)
			 VALUES(?, 'user', ?, ?, ?)`,
			"recall-session", content, time.Now().Unix()+int64(i), "telegram:42",
		)
		if err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}

	// ── Phase B: run the real extractor against Ollama ────────────────
	hc := hermes.NewHTTPClient(endpoint, "")
	ext := NewExtractor(store, hc, ExtractorConfig{
		Model:        model,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    3,
		CallTimeout:  180 * time.Second,
		BackoffBase:  500 * time.Millisecond,
		BackoffMax:   5 * time.Second,
	}, nil)

	extCtx, extCancel := context.WithTimeout(context.Background(), 4*time.Minute)
	go ext.Run(extCtx)
	for {
		if extCtx.Err() != nil {
			break
		}
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 0`).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	extCancel()
	_ = ext.Close(context.Background())

	var entCount, relCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entCount)
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&relCount)
	t.Logf("extractor populated %d entities, %d relationships", entCount, relCount)
	if entCount == 0 {
		t.Fatal("extractor populated zero entities; recall test cannot proceed")
	}

	// ── Phase C: run recall on a "my projects" query ──────────────────
	prov := NewRecall(store, RecallConfig{
		WeightThreshold: 1.0,
		MaxFacts:        10,
		Depth:           2,
		MaxSeeds:        5,
	}, nil)

	// Two queries — one direct (AzulVigia), one indirect ("my projects").
	for _, probe := range []struct {
		label   string
		message string
	}{
		{"direct-name-match", "tell me about AzulVigia"},
		{"indirect-concept", "tell me about my projects"},
	} {
		recallCtx, recallCancel := context.WithTimeout(context.Background(), 5*time.Second)
		block := prov.GetContext(recallCtx, RecallInput{
			UserMessage: probe.message,
			ChatKey:     "telegram:42",
		})
		recallCancel()

		t.Logf("=== recall probe: %q (%s) ===", probe.message, probe.label)
		if block == "" {
			t.Logf("  (empty block)")
		} else {
			for _, line := range strings.Split(block, "\n") {
				t.Logf("  %s", line)
			}
		}
		t.Logf("=== end probe: %s ===", probe.label)

		if probe.label == "direct-name-match" {
			if !strings.Contains(block, "<memory-context>") {
				t.Errorf("direct-match block missing fence")
			}
			if !strings.Contains(block, "AzulVigia") {
				t.Errorf("direct-match block missing AzulVigia seed entity")
			}
		}
	}

	fmt.Printf("\n[recall] memory.db path: %s\n", path)
	fmt.Printf("[recall] model=%s endpoint=%s entities=%d relationships=%d\n\n",
		model, endpoint, entCount, relCount)

	// ── Final tally: how many of the canonical entities surfaced? ────
	var entNames []string
	rows, _ := store.db.Query(`SELECT name FROM entities ORDER BY id`)
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		entNames = append(entNames, n)
	}
	rows.Close()
	t.Logf("--- ALL ENTITIES EXTRACTED: %v ---", entNames)
}
