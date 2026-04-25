package memory

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

func TestSessionCatalog_SearchMessagesFiltersBySource(t *testing.T) {
	store, err := OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	ctx := context.Background()
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
			content:   "Project Atlas started in Telegram.",
			ts:        now - 20,
		},
		{
			sessionID: "sess-discord",
			chatID:    "discord:chan-9",
			content:   "Project Atlas got a Discord follow-up.",
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

	metas := []session.Metadata{
		{SessionID: "sess-telegram", Source: "telegram", ChatID: "42", UserID: "user-juan"},
		{SessionID: "sess-discord", Source: "discord", ChatID: "chan-9", UserID: "user-juan"},
	}

	hits, err := SearchMessages(ctx, store.DB(), metas, SearchFilter{
		UserID:  "user-juan",
		Sources: []string{"discord"},
		Query:   "Atlas",
	}, 10)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("SearchMessages len = %d, want 1", len(hits))
	}
	if hits[0].Source != "discord" || hits[0].SessionID != "sess-discord" {
		t.Fatalf("SearchMessages hit = %+v, want discord/sess-discord", hits[0])
	}
}

func TestSessionCatalog_SearchMessagesSkipsInterruptedSyncRows(t *testing.T) {
	store, err := OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	ctx := context.Background()
	now := time.Now().Unix()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, memory_sync_status, memory_sync_reason)
		 VALUES
		 ('sess-ready', 'user', 'Atlas stable note', ?, 'telegram:42', 'ready', NULL),
		 ('sess-skip', 'user', 'Atlas interrupted note', ?, 'telegram:42', 'skipped', 'interrupted')`,
		now, now+1,
	); err != nil {
		t.Fatal(err)
	}

	hits, err := SearchMessages(ctx, store.DB(), []session.Metadata{
		{SessionID: "sess-ready", Source: "telegram", ChatID: "42", UserID: "user-juan"},
		{SessionID: "sess-skip", Source: "telegram", ChatID: "42", UserID: "user-juan"},
	}, SearchFilter{UserID: "user-juan", Query: "Atlas"}, 10)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "Atlas stable note" {
		t.Fatalf("SearchMessages hits = %+v, want only ready row", hits)
	}
}

func TestSessionCatalog_SearchSessionsOrdersByLatestTurn(t *testing.T) {
	store, err := OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	ctx := context.Background()
	now := time.Now().Unix()
	for _, turn := range []struct {
		sessionID string
		chatID    string
		content   string
		ts        int64
	}{
		{
			sessionID: "sess-older",
			chatID:    "telegram:42",
			content:   "older project note",
			ts:        now - 40,
		},
		{
			sessionID: "sess-newer",
			chatID:    "discord:chan-9",
			content:   "newer project note",
			ts:        now - 5,
		},
		{
			sessionID: "sess-middle",
			chatID:    "slack:C123",
			content:   "middle project note",
			ts:        now - 15,
		},
	} {
		if _, err := store.DB().ExecContext(ctx,
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
			turn.sessionID, "assistant", turn.content, turn.ts, turn.chatID,
		); err != nil {
			t.Fatalf("insert turn %s: %v", turn.sessionID, err)
		}
	}

	metas := []session.Metadata{
		{SessionID: "sess-older", Source: "telegram", ChatID: "42", UserID: "user-juan"},
		{SessionID: "sess-newer", Source: "discord", ChatID: "chan-9", UserID: "user-juan"},
		{SessionID: "sess-middle", Source: "slack", ChatID: "C123", UserID: "user-juan"},
	}

	sessions, err := SearchSessions(ctx, store.DB(), metas, SearchFilter{
		UserID: "user-juan",
		Query:  "project",
	}, 10)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("SearchSessions len = %d, want 3", len(sessions))
	}
	got := []string{sessions[0].SessionID, sessions[1].SessionID, sessions[2].SessionID}
	want := []string{"sess-newer", "sess-middle", "sess-older"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SearchSessions order = %v, want %v", got, want)
		}
	}
}
