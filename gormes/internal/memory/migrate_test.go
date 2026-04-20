package memory

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSqlite_FreshDBIsV3b(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3c" {
		t.Errorf("schema version = %q, want 3c", v)
	}
}

func TestMigrate_TurnsGainsExtractedColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	for _, col := range []string{"extracted", "extraction_attempts", "extraction_error"} {
		var name string
		row := s.db.QueryRow(
			`SELECT name FROM pragma_table_info('turns') WHERE name = ?`, col)
		if err := row.Scan(&name); err != nil {
			t.Errorf("column %q missing from turns: %v", col, err)
		}
	}
}

func TestMigrate_EntitiesAndRelationshipsExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	for _, table := range []string{"entities", "relationships"} {
		var n int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(context.Background())

	// Re-open — migration runs against v3c, should no-op.
	s2, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer s2.Close(context.Background())

	var v string
	_ = s2.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3c" {
		t.Errorf("version = %q after re-open, want 3c", v)
	}
}

func TestMigrate_UnknownVersionRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	_, _ = s.db.Exec(`UPDATE schema_meta SET v = '3z' WHERE k = 'version'`)
	s.Close(context.Background())

	_, err := OpenSqlite(path, 0, nil)
	if !errors.Is(err, ErrSchemaUnknown) {
		t.Errorf("err = %v, want errors.Is(err, ErrSchemaUnknown)", err)
	}
}

func TestOpenSqlite_FreshDBIsV3c(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3c" {
		t.Errorf("schema version = %q, want 3c", v)
	}
}

func TestMigrate_3bTo3c_AddsChatIDColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var name string
	var notNull int
	var dflt sql.NullString
	row := s.db.QueryRow(
		`SELECT name, "notnull", dflt_value
		 FROM pragma_table_info('turns') WHERE name = 'chat_id'`)
	if err := row.Scan(&name, &notNull, &dflt); err != nil {
		t.Fatalf("turns.chat_id missing: %v", err)
	}
	if notNull != 1 {
		t.Errorf("chat_id NOT NULL = %d, want 1", notNull)
	}
	// default is typically "''" — quoted empty string.
	if !dflt.Valid || strings.Trim(dflt.String, "'") != "" {
		t.Errorf("chat_id default = %v, want empty string", dflt)
	}
}

func TestMigrate_ChatIDBackfillsEmptyOnExistingTurns(t *testing.T) {
	// Open a fresh DB (which migrates all the way to v3c), insert a turn
	// via raw SQL (no chat_id specified), reopen, verify chat_id == "".
	path := filepath.Join(t.TempDir(), "memory.db")
	s1, _ := OpenSqlite(path, 0, nil)
	_, _ = s1.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES('s','user','hi',1)`)
	s1.Close(context.Background())

	s2, _ := OpenSqlite(path, 0, nil)
	defer s2.Close(context.Background())
	var cid string
	_ = s2.db.QueryRow(`SELECT chat_id FROM turns WHERE content = 'hi'`).Scan(&cid)
	if cid != "" {
		t.Errorf("chat_id of pre-existing turn = %q, want empty", cid)
	}
}

func TestMigrate_3cHasIndexOnChatID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_turns_chat_id'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("idx_turns_chat_id missing: %v", err)
	}
}
