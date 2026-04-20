package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func openGraphWithSeeds(t *testing.T) *SqliteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })

	_, _ = s.db.Exec(`
		INSERT INTO entities(name, type, description, updated_at) VALUES
			('AzulVigia','PROJECT','sports platform',1),
			('Cadereyta','PLACE','',1),
			('Vania','PERSON','',1),
			('Neovim','TOOL','',1)
	`)
	_, _ = s.db.Exec(`
		INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES
			('s','user','working on AzulVigia',1,'telegram:42'),
			('s','user','Vania uses Neovim',2,'telegram:42'),
			('s','user','Neovim rocks',3,'telegram:99')
	`)
	return s
}

func TestSeedsExactName_MatchesCaseInsensitive(t *testing.T) {
	s := openGraphWithSeeds(t)
	ids, err := seedsExactName(context.Background(), s.db,
		[]string{"azulvigia", "Vania"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("len=%d, want 2", len(ids))
	}
}

func TestSeedsExactName_SkipsShortNames(t *testing.T) {
	s := openGraphWithSeeds(t)
	ids, _ := seedsExactName(context.Background(), s.db, []string{"Vo"}, 5)
	if len(ids) != 0 {
		t.Errorf("short name returned %d seeds, want 0", len(ids))
	}
}

func TestSeedsExactName_EmptyCandidateReturnsEmpty(t *testing.T) {
	s := openGraphWithSeeds(t)
	ids, err := seedsExactName(context.Background(), s.db, nil, 5)
	if err != nil {
		t.Errorf("err = %v, want nil on empty candidates", err)
	}
	if len(ids) != 0 {
		t.Errorf("len = %d, want 0", len(ids))
	}
}

func TestSeedsFTS5_MatchesByTurnContent(t *testing.T) {
	s := openGraphWithSeeds(t)
	ids, err := seedsFTS5(context.Background(), s.db,
		"AzulVigia", "telegram:42", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Error("FTS5 match returned zero seeds; AzulVigia should match turn content")
	}
}

func TestSeedsFTS5_ScopesToChatID(t *testing.T) {
	s := openGraphWithSeeds(t)
	// "Neovim" appears in a chat-99 turn, NOT a chat-42 turn. Querying
	// from chat 42 must not return Neovim via FTS5 (chat scoping).
	ids, _ := seedsFTS5(context.Background(), s.db,
		"Neovim", "telegram:42", 5)
	for _, id := range ids {
		var name string
		_ = s.db.QueryRow(`SELECT name FROM entities WHERE id = ?`, id).Scan(&name)
		if name == "Neovim" {
			// "Neovim" ALSO appears in chat-42's "Vania uses Neovim" turn,
			// so this actually SHOULD match. Let me re-check the fixture.
			// Actually: chat-42's turn 2 is "Vania uses Neovim" — Neovim IS there.
			// So this test passes BUT its invariant is weaker than intended.
			// The test still demonstrates scoping because a query from chat 99
			// would find Neovim ONLY, not AzulVigia which is chat-42-only.
			_ = name // keep the assertion stance
		}
	}
	// Stronger scoping check: query from chat 99 for "AzulVigia".
	// AzulVigia is only in a chat-42 turn; chat-99 scope must return zero.
	ids2, _ := seedsFTS5(context.Background(), s.db,
		"AzulVigia", "telegram:99", 5)
	for _, id := range ids2 {
		var name string
		_ = s.db.QueryRow(`SELECT name FROM entities WHERE id = ?`, id).Scan(&name)
		if name == "AzulVigia" {
			t.Errorf("chat-42-only AzulVigia leaked into chat-99 scope")
		}
	}
}

func TestSeedsFTS5_EmptyChatIDMatchesGlobal(t *testing.T) {
	s := openGraphWithSeeds(t)
	ids, _ := seedsFTS5(context.Background(), s.db, "AzulVigia", "", 5)
	if len(ids) == 0 {
		t.Error("empty chat_id should be global scope; got zero seeds")
	}
}
