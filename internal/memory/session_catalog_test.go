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

// TestSessionCatalog_SearchMessagesEmptyUserIDDeniesAll pins a deny-path
// fixture: an unresolved user_id (empty filter.UserID) must never reach the
// turns table, regardless of how many rows the caller passes in metas.
func TestSessionCatalog_SearchMessagesEmptyUserIDDeniesAll(t *testing.T) {
	store, err := OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if cerr := store.Close(context.Background()); cerr != nil {
			t.Fatalf("Close: %v", cerr)
		}
	}()

	ctx := context.Background()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-x", "user", "Atlas confidential note.", time.Now().Unix(), "telegram:42",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	metas := []session.Metadata{
		{SessionID: "sess-x", Source: "telegram", ChatID: "42", UserID: "user-juan"},
	}
	hits, err := SearchMessages(ctx, store.DB(), metas, SearchFilter{
		UserID: "",
		Query:  "Atlas",
	}, 10)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("SearchMessages with empty user_id len = %d, want 0 (deny-path)", len(hits))
	}
}

// TestSessionCatalog_SearchMessagesIgnoresCrossUserMetadata pins a deny-path
// fixture: metadata rows whose UserID does not match the filter must be
// dropped before the SQL query runs. A caller cannot smuggle another user's
// session through the metadata slice.
func TestSessionCatalog_SearchMessagesIgnoresCrossUserMetadata(t *testing.T) {
	store, err := OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer func() {
		if cerr := store.Close(context.Background()); cerr != nil {
			t.Fatalf("Close: %v", cerr)
		}
	}()

	ctx := context.Background()
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, ?, ?, ?, ?)`,
		"sess-other", "user", "Atlas note from another user.", time.Now().Unix(), "telegram:99",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	metas := []session.Metadata{
		// Only one row, and it belongs to user-other — cross-user smuggling.
		{SessionID: "sess-other", Source: "telegram", ChatID: "99", UserID: "user-other"},
	}
	hits, err := SearchMessages(ctx, store.DB(), metas, SearchFilter{
		UserID: "user-juan",
		Query:  "Atlas",
	}, 10)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("SearchMessages cross-user metas len = %d, want 0 (deny-path)", len(hits))
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
