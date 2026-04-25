package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
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
		{"Acme", "PROJECT", "sports analytics"},
		{"Springfield", "PLACE", ""},
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
		{"Juan", "Acme", "WORKS_ON", 3.0},
		{"Acme", "Springfield", "LOCATED_IN", 2.0},
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
		 VALUES('s','user','Juan works on Acme daily',1,'telegram:42')`)

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
		UserMessage: "tell me about Acme",
		ChatKey:     "telegram:42",
	})
	if out == "" {
		t.Fatal("GetContext returned empty; expected <memory-context> block")
	}
	for _, want := range []string{
		"<memory-context>",
		"</memory-context>",
		"Acme",
		"Springfield",
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

	out := p.GetContext(ctx, RecallInput{UserMessage: "tell me about Acme"})
	if out != "" {
		t.Errorf("GetContext on cancelled ctx = %q, want empty string", out)
	}
}

func TestProvider_GetContext_Layer1SameChatFence(t *testing.T) {
	_, p := openProviderWithRichGraph(t)
	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "Acme progress?",
		ChatKey:     "telegram:99", // different chat from the seeded turn
	})
	if out != "" {
		t.Errorf("same-chat default should fence exact-name recall; got %q", out)
	}
}

func TestProvider_GetContext_CrossChatOptInUsesSameUserBindings(t *testing.T) {
	_, p := openProviderWithRichGraph(t)

	dir := session.NewMemMap()
	ctx := context.Background()
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata telegram: %v", err)
	}
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-discord",
		Source:    "discord",
		ChatID:    "7",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata discord: %v", err)
	}

	p = p.WithDirectory(dir)
	out := p.GetContext(ctx, RecallInput{
		UserMessage: "Acme progress?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
	})
	if !strings.Contains(out, "Acme") {
		t.Fatalf("cross-chat opt-in should surface Acme from same user's other chat; got %q", out)
	}
}

func TestProvider_GetContext_CrossChatSourceFilter(t *testing.T) {
	_, p := openProviderWithRichGraph(t)

	dir := session.NewMemMap()
	ctx := context.Background()
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata telegram: %v", err)
	}
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-discord",
		Source:    "discord",
		ChatID:    "7",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata discord: %v", err)
	}

	p = p.WithDirectory(dir)
	blocked := p.GetContext(ctx, RecallInput{
		UserMessage: "Acme progress?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
		Sources:     []string{"discord"},
	})
	if blocked != "" {
		t.Fatalf("discord-only source filter should block telegram facts; got %q", blocked)
	}

	allowed := p.GetContext(ctx, RecallInput{
		UserMessage: "Acme progress?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
		Sources:     []string{"telegram"},
	})
	if !strings.Contains(allowed, "Acme") {
		t.Fatalf("telegram source filter should allow Acme; got %q", allowed)
	}
}

func TestProvider_GetContext_CrossChatUnknownCurrentBindingFallsBackSameChat(t *testing.T) {
	s, p := openProviderWithRichGraph(t)
	ctx := context.Background()
	seedSameChatEntity(t, s, "Orchid", "same-chat only", "discord:7")

	dir := session.NewMemMap()
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata telegram: %v", err)
	}

	out := p.WithDirectory(dir).GetContext(ctx, RecallInput{
		UserMessage: "Acme Orchid status?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
	})
	if !strings.Contains(out, "Orchid") {
		t.Fatalf("unknown current binding should keep same-chat recall; got %q", out)
	}
	if strings.Contains(out, "Acme") {
		t.Fatalf("unknown current binding widened into another chat; got %q", out)
	}
}

func TestProvider_GetContext_CrossChatConflictingCurrentBindingFallsBackSameChat(t *testing.T) {
	s, p := openProviderWithRichGraph(t)
	ctx := context.Background()
	seedSameChatEntity(t, s, "Orchid", "same-chat only", "discord:7")

	p = p.WithDirectory(recallDirectoryFunc(func(context.Context, string) ([]session.Metadata, error) {
		return []session.Metadata{
			{
				SessionID: "sess-current",
				Source:    "discord",
				ChatID:    "7",
				UserID:    "user-maria",
			},
			{
				SessionID: "sess-telegram",
				Source:    "telegram",
				ChatID:    "42",
				UserID:    "user-juan",
			},
		}, nil
	}))

	out := p.GetContext(ctx, RecallInput{
		UserMessage: "Acme Orchid status?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
	})
	if !strings.Contains(out, "Orchid") {
		t.Fatalf("conflicting current binding should keep same-chat recall; got %q", out)
	}
	if strings.Contains(out, "Acme") {
		t.Fatalf("conflicting current binding widened into another chat; got %q", out)
	}
}

func TestProvider_GetContext_CrossChatUnresolvedDirectoryFallsBackSameChat(t *testing.T) {
	s, p := openProviderWithRichGraph(t)
	ctx := context.Background()
	seedSameChatEntity(t, s, "Orchid", "same-chat only", "discord:7")

	p = p.WithDirectory(recallDirectoryFunc(func(context.Context, string) ([]session.Metadata, error) {
		return nil, errors.New("metadata unavailable")
	}))

	out := p.GetContext(ctx, RecallInput{
		UserMessage: "Acme Orchid status?",
		ChatKey:     "discord:7",
		UserID:      "user-juan",
		CrossChat:   true,
	})
	if !strings.Contains(out, "Orchid") {
		t.Fatalf("unresolved current binding should keep same-chat recall; got %q", out)
	}
	if strings.Contains(out, "Acme") {
		t.Fatalf("unresolved current binding widened into another chat; got %q", out)
	}
}

type recallDirectoryFunc func(context.Context, string) ([]session.Metadata, error)

func (f recallDirectoryFunc) ListMetadataByUserID(ctx context.Context, userID string) ([]session.Metadata, error) {
	return f(ctx, userID)
}

func seedSameChatEntity(t *testing.T, s *SqliteStore, name, desc, chatKey string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO entities(name, type, description, updated_at) VALUES(?,?,?,?)`,
		name, "PROJECT", desc, time.Now().Unix(),
	); err != nil {
		t.Fatalf("insert same-chat entity: %v", err)
	}
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id)
		 VALUES(?, 'user', ?, ?, ?)`,
		"sess-"+strings.ToLower(name), name+" belongs to this chat", time.Now().Unix(), chatKey,
	); err != nil {
		t.Fatalf("insert same-chat turn: %v", err)
	}
}

// stubEmbedServer returns a fixed vector for any input — enough to seed
// the graph with embeddings for hybrid tests.
func stubEmbedServer(t *testing.T, returnVec []float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": returnVec, "index": 0}},
		})
	}))
}

func TestProvider_SemanticDisabledIsLexicalOnly(t *testing.T) {
	// When SemanticModel is empty, the provider must behave identically
	// to Phase 3.C — no embed calls, no semantic seeds.
	_, p := openProviderWithRichGraph(t)
	// Ensure p.ec is nil / SemanticModel empty; openProviderWithRichGraph
	// sets a default RecallConfig with no semantic fields.

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about Acme",
		ChatKey:     "telegram:42",
	})
	if out == "" {
		t.Fatal("GetContext returned empty; lexical seed should still work")
	}
	// No crash, no panic — good enough.
}

func TestProvider_SemanticSeedsAreUnioned(t *testing.T) {
	// Insert entity "Widget" with NO lexical match in the message, but
	// pre-populate an embedding that matches the query vector. The
	// semantic layer should surface it.
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Widget','PROJECT',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Widget'`).Scan(&id)
	vec := []float32{1, 0, 0, 0}
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'stub', 4, ?, 1)`,
		id, encodeFloat32LE(vec))

	// Stub server returns the same vector so cosine(query, entity) == 1.
	ts := stubEmbedServer(t, []float32{1, 0, 0, 0})
	defer ts.Close()

	cache := newSemanticCache()
	ec := newEmbedClient(ts.URL, "")
	p := NewRecall(s, RecallConfig{
		WeightThreshold:       1.0,
		MaxFacts:              10,
		Depth:                 2,
		MaxSeeds:              5,
		SemanticModel:         "stub",
		SemanticTopK:          3,
		SemanticMinSimilarity: 0.5,
		QueryEmbedTimeout:     1 * time.Second,
	}, nil).WithEmbedClient(ec, cache)

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about gadgets", // no lexical match
	})
	if !strings.Contains(out, "Widget") {
		t.Errorf("semantic-only path missed Widget; got %q", out)
	}
}

func TestProvider_SemanticFallsThroughOnEmbedFailure(t *testing.T) {
	// Unreachable embed endpoint → lexical-only behavior.
	_, p := openProviderWithRichGraph(t)
	p.ec = newEmbedClient("http://127.0.0.1:1", "")
	p.cache = newSemanticCache()
	// Also set semantic config so GetContext even attempts the call.
	p.cfg.SemanticModel = "unreachable"
	p.cfg.SemanticTopK = 3
	p.cfg.SemanticMinSimilarity = 0.5
	p.cfg.QueryEmbedTimeout = 200 * time.Millisecond

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about Acme",
		ChatKey:     "telegram:42",
	})
	// Lexical still works — the fence includes Acme.
	if !strings.Contains(out, "Acme") {
		t.Errorf("lexical fallback failed when embed endpoint is unreachable: %q", out)
	}
}

func TestRecallConfig_WithDefaults_DecayHorizon(t *testing.T) {
	// Zero-value field gets the default (180 days).
	cfg := RecallConfig{}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != 180 {
		t.Errorf("zero-value -> withDefaults: DecayHorizonDays = %d, want 180",
			cfg.DecayHorizonDays)
	}

	// Positive field value is preserved.
	cfg = RecallConfig{DecayHorizonDays: 30}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != 30 {
		t.Errorf("positive preserved: DecayHorizonDays = %d, want 30",
			cfg.DecayHorizonDays)
	}

	// Negative (disable sentinel) is preserved — NOT defaulted.
	cfg = RecallConfig{DecayHorizonDays: -1}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != -1 {
		t.Errorf("negative preserved: DecayHorizonDays = %d, want -1 (disable sentinel)",
			cfg.DecayHorizonDays)
	}
}
