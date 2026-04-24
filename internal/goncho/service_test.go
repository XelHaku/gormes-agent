package goncho

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

// failingDirectory is a SessionDirectory that always errors. Used to prove the
// goncho service falls back to same-chat search when the cross-chat directory
// is unreachable instead of leaking the error to callers.
type failingDirectory struct{ err error }

func (f failingDirectory) ListMetadataByUserID(_ context.Context, _ string) ([]session.Metadata, error) {
	return nil, f.err
}

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

func TestService_SearchUserScopeRespectsSourceFilter(t *testing.T) {
	store, dir, svc, cleanup := newTestServiceWithDirectory(t)
	defer cleanup()

	ctx := context.Background()
	for _, meta := range []session.Metadata{
		{SessionID: "sess-telegram", Source: "telegram", ChatID: "42", UserID: "user-juan"},
		{SessionID: "sess-discord", Source: "discord", ChatID: "chan-9", UserID: "user-juan"},
	} {
		if err := dir.PutMetadata(ctx, meta); err != nil {
			t.Fatalf("PutMetadata(%s): %v", meta.SessionID, err)
		}
	}
	now := time.Now().Unix()
	for _, turn := range []struct {
		sessionID string
		chatID    string
		content   string
		ts        int64
	}{
		{
			sessionID: "sess-telegram",
			chatID:    "telegram:42",
			content:   "Atlas planning stayed in Telegram.",
			ts:        now - 20,
		},
		{
			sessionID: "sess-discord",
			chatID:    "discord:chan-9",
			content:   "Atlas execution moved to Discord.",
			ts:        now - 10,
		},
	} {
		if _, err := store.DB().ExecContext(ctx,
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
			turn.sessionID, "user", turn.content, turn.ts, turn.chatID,
		); err != nil {
			t.Fatalf("insert turn %s: %v", turn.sessionID, err)
		}
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:      "user-juan",
		Query:     "Atlas",
		MaxTokens: 200,
		Scope:     "user",
		Sources:   []string{"discord"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("Search results len = %d, want 1", len(got.Results))
	}
	if got.Results[0].Source != "turn" || got.Results[0].SessionKey != "sess-discord" {
		t.Fatalf("Search result = %+v, want discord turn bound to sess-discord", got.Results[0])
	}
}

// TestService_SearchUserScopeUnknownUserFallsBackToSameChat pins a deny-path
// fixture: when scope=user is requested with a peer that has no canonical
// session bindings, the search must fall back to the caller's same-chat
// turns instead of returning empty (which would silently drop legitimate
// in-chat results) or widening to all chats.
func TestService_SearchUserScopeUnknownUserFallsBackToSameChat(t *testing.T) {
	store, _, svc, cleanup := newTestServiceWithDirectory(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-known", "user", "Atlas same-chat reminder.", time.Now().Unix(), "telegram:42",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:       "ghost-user",
		Query:      "Atlas",
		MaxTokens:  200,
		SessionKey: "telegram:42",
		Scope:      "user",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("Search results len = %d, want 1 (same-chat fallback for unbound user)", len(got.Results))
	}
	if got.Results[0].Source != "turn" || got.Results[0].SessionKey != "telegram:42" {
		t.Fatalf("Search result = %+v, want turn bound to telegram:42", got.Results[0])
	}
}

// TestService_SearchUserScopeDirectoryErrorFallsBackToSameChat pins a
// deny-path fixture: a transient directory error must not leak through the
// search surface as a hard failure. The user-scope branch must collapse to
// same-chat behavior so callers keep getting in-chat results.
func TestService_SearchUserScopeDirectoryErrorFallsBackToSameChat(t *testing.T) {
	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if cerr := store.Close(context.Background()); cerr != nil {
			t.Fatalf("Close: %v", cerr)
		}
	}()

	svc := NewService(store.DB(), Config{
		WorkspaceID:      "default",
		ObserverPeerID:   "gormes",
		RecentMessages:   4,
		SessionDirectory: failingDirectory{err: errors.New("session: directory offline")},
	}, nil)

	ctx := context.Background()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-known", "user", "Atlas continues in chat.", time.Now().Unix(), "telegram:42",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:       "user-juan",
		Query:      "Atlas",
		MaxTokens:  200,
		SessionKey: "telegram:42",
		Scope:      "user",
	})
	if err != nil {
		t.Fatalf("Search returned directory error to caller: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("Search results len = %d, want 1 (same-chat fallback for dir error)", len(got.Results))
	}
	if got.Results[0].Source != "turn" || got.Results[0].SessionKey != "telegram:42" {
		t.Fatalf("Search result = %+v, want turn bound to telegram:42", got.Results[0])
	}
}

// TestService_SearchUserScopeSourceFilterDeniesAllFallsBackToSameChat pins a
// deny-path fixture: when a source allow-list excludes every binding the
// user owns, the user-scope branch collapses to same-chat search. The
// allow-list must not act as a wide cross-chat passthrough.
func TestService_SearchUserScopeSourceFilterDeniesAllFallsBackToSameChat(t *testing.T) {
	store, dir, svc, cleanup := newTestServiceWithDirectory(t)
	defer cleanup()

	ctx := context.Background()
	if err := dir.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata: %v", err)
	}
	now := time.Now().Unix()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-telegram", "user", "Atlas same-chat fallback line.", now, "telegram:42",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	got, err := svc.Search(ctx, SearchParams{
		Peer:       "user-juan",
		Query:      "Atlas",
		MaxTokens:  200,
		SessionKey: "telegram:42",
		Scope:      "user",
		Sources:    []string{"slack"},
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("Search results len = %d, want 1 (same-chat fallback when source filter denies all)", len(got.Results))
	}
	if got.Results[0].Source != "turn" || got.Results[0].SessionKey != "telegram:42" {
		t.Fatalf("Search result = %+v, want turn bound to telegram:42", got.Results[0])
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

func newTestServiceWithDirectory(t *testing.T) (*memory.SqliteStore, *session.MemMap, *Service, func()) {
	t.Helper()

	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	dir := session.NewMemMap()
	svc := NewService(store.DB(), Config{
		WorkspaceID:      "default",
		ObserverPeerID:   "gormes",
		RecentMessages:   4,
		SessionDirectory: dir,
	}, nil)
	return store, dir, svc, func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
