package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openProviderWithRichGraph(t *testing.T) (*SqliteStore, *Provider) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	// Entities.
	for _, e := range []struct{ name, typ, desc string }{
		{"AzulVigia", "PROJECT", "sports analytics"},
		{"Cadereyta", "PLACE", ""},
		{"Juan", "PERSON", "the user"},
		{"Vania", "PERSON", "partner"},
		{"Go", "TOOL", ""},
	} {
		_, _ = s.db.Exec(
			`INSERT INTO entities(name, type, description, updated_at) VALUES(?,?,?,?)`,
			e.name, e.typ, e.desc, time.Now().Unix())
	}
	// Relationships.
	type rel struct {
		src, tgt, pred string
		w              float64
	}
	rels := []rel{
		{"Juan", "AzulVigia", "WORKS_ON", 3.0},
		{"AzulVigia", "Cadereyta", "LOCATED_IN", 2.0},
		{"Vania", "Juan", "KNOWS", 5.0},
		{"Juan", "Go", "HAS_SKILL", 4.0},
	}
	for _, r := range rels {
		_, _ = s.db.Exec(`
			INSERT INTO relationships(source_id, target_id, predicate, weight, updated_at)
			SELECT (SELECT id FROM entities WHERE name = ?),
			       (SELECT id FROM entities WHERE name = ?),
			       ?, ?, ?`,
			r.src, r.tgt, r.pred, r.w, time.Now().Unix())
	}
	// Turn seeds for FTS5 fallback.
	_, _ = s.db.Exec(
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id)
		 VALUES('s','user','Juan works on AzulVigia daily',1,'telegram:42')`)

	p := NewRecall(s, RecallConfig{
		WeightThreshold: 1.0,
		MaxFacts:        10,
		Depth:           2,
		MaxSeeds:        5,
	}, nil)
	return s, p
}

func TestProvider_GetContext_HappyPath(t *testing.T) {
	_, p := openProviderWithRichGraph(t)
	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about AzulVigia",
		ChatKey:     "telegram:42",
	})
	if out == "" {
		t.Fatal("GetContext returned empty; expected <memory-context> block")
	}
	for _, want := range []string{
		"<memory-context>",
		"</memory-context>",
		"AzulVigia",
		"Cadereyta",
		"## Entities",
		"## Relationships",
		"do not acknowledge",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("block missing %q", want)
		}
	}
}

func TestProvider_GetContext_EmptyGraphReturnsEmptyString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())
	p := NewRecall(s, RecallConfig{WeightThreshold: 1.0, MaxFacts: 10, Depth: 2, MaxSeeds: 5}, nil)

	out := p.GetContext(context.Background(), RecallInput{UserMessage: "hello world"})
	if out != "" {
		t.Errorf("GetContext on empty graph = %q, want empty string", out)
	}
}

func TestProvider_GetContext_NoMatchReturnsEmptyString(t *testing.T) {
	_, p := openProviderWithRichGraph(t)
	// Message with no proper nouns that match any seeded entity AND no
	// meaningful FTS5 overlap with existing turn content.
	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "xyzzy plover blergh",
	})
	if out != "" {
		t.Errorf("GetContext with no match = %q, want empty string", out)
	}
}

func TestProvider_GetContext_RespectsContextDeadline(t *testing.T) {
	_, p := openProviderWithRichGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled ctx

	out := p.GetContext(ctx, RecallInput{UserMessage: "tell me about AzulVigia"})
	if out != "" {
		t.Errorf("GetContext on cancelled ctx = %q, want empty string", out)
	}
}

func TestProvider_GetContext_Layer1FindsEntityRegardlessOfChat(t *testing.T) {
	// Layer-1 (exact-name match) doesn't scope by chat_id — entities are
	// global. So a query from any chat that NAMES the entity finds it.
	_, p := openProviderWithRichGraph(t)
	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "AzulVigia progress?",
		ChatKey:     "telegram:99", // different chat from the seeded turn
	})
	if !strings.Contains(out, "AzulVigia") {
		t.Errorf("Layer-1 exact match should still find AzulVigia regardless of chat; got %q", out)
	}
}
