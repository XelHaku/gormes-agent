package goncho

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
)

func TestService_ProfileRoundTrip(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	if err := svc.SetProfile(ctx, "telegram:6586915095", []string{"Blind", "Prefers exact outputs"}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Profile(ctx, "telegram:6586915095")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Blind", "Prefers exact outputs"}
	if !slices.Equal(got.Card, want) {
		t.Fatalf("card = %#v, want %#v", got.Card, want)
	}
}

func TestService_ConcludeAndSearchIsIdempotent(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	first, err := svc.Conclude(ctx, ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user prefers exact evidence-first reports.",
		SessionKey: "telegram:6586915095",
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Conclude(ctx, ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user prefers exact evidence-first reports.",
		SessionKey: "telegram:6586915095",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent conclude returned ids %d and %d", first.ID, second.ID)
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:       "telegram:6586915095",
		Query:      "evidence-first",
		MaxTokens:  200,
		SessionKey: "telegram:6586915095",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) == 0 {
		t.Fatal("want at least one search result")
	}
	if got.Results[0].Source != "conclusion" {
		t.Fatalf("first result source = %q, want conclusion", got.Results[0].Source)
	}
}

func TestService_ContextIncludesPeerCardConclusionsAndRecentMessages(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	if err := svc.SetProfile(ctx, "telegram:6586915095", []string{"Blind", "Prefers exact outputs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Conclude(ctx, ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user prefers exact evidence-first reports.",
		SessionKey: "telegram:6586915095",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.db.ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-1", "user", "Please keep reports exact and evidence-first.", time.Now().Unix(), "telegram:6586915095",
	); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Context(ctx, ContextParams{
		Peer:       "telegram:6586915095",
		Query:      "exact",
		MaxTokens:  400,
		SessionKey: "telegram:6586915095",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.PeerCard) != 2 {
		t.Fatalf("peer card len = %d, want 2", len(got.PeerCard))
	}
	if len(got.Conclusions) == 0 {
		t.Fatal("want conclusions in context")
	}
	if len(got.RecentMessages) == 0 {
		t.Fatal("want recent messages in context")
	}
	if got.Representation == "" {
		t.Fatal("want non-empty representation")
	}
}

func TestService_DeleteConclusion(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	created, err := svc.Conclude(ctx, ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user dislikes vague status reports.",
	})
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := svc.Conclude(ctx, ConcludeParams{
		Peer:     "telegram:6586915095",
		DeleteID: created.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Deleted {
		t.Fatal("expected deleted=true")
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:      "telegram:6586915095",
		Query:     "vague",
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 0 {
		t.Fatalf("expected 0 search results after delete, got %d", len(got.Results))
	}
}

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()

	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}

	svc := NewService(store.DB(), Config{
		WorkspaceID:    "default",
		ObserverPeerID: "gormes",
		RecentMessages: 4,
	}, nil)

	return svc, func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
