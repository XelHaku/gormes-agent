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
	if v != "3d" {
		t.Errorf("schema version = %q, want 3d", v)
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

	// Re-open — migration runs against v3d, should no-op.
	s2, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer s2.Close(context.Background())

	var v string
	_ = s2.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3d" {
		t.Errorf("version = %q after re-open, want 3d", v)
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
	if v != "3d" {
		t.Errorf("schema version = %q, want 3d", v)
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

func TestOpenSqlite_FreshDBIsV3d(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3d" {
		t.Errorf("schema version = %q, want 3d", v)
	}
}

func TestMigrate_3cTo3d_AddsEntityEmbeddingsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
	if err != nil {
		t.Errorf("entity_embeddings table missing: %v", err)
	}
}

func TestMigrate_3cTo3d_HasModelIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_entity_embeddings_model'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("idx_entity_embeddings_model missing: %v", err)
	}
}

func TestMigrate_3cTo3d_DimCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// Must reject dim=0 and dim>4096 per the CHECK constraint.
	_, _ = s.db.Exec(`INSERT INTO entities(name, type, updated_at) VALUES('X','PERSON',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='X'`).Scan(&id)

	_, err := s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 0, x'00', 1)`,
		id)
	if err == nil {
		t.Error("dim=0 should trip CHECK constraint")
	}

	_, err = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 5000, x'00', 1)`,
		id)
	if err == nil {
		t.Error("dim=5000 should trip CHECK(dim <= 4096)")
	}
}

func TestMigrate_3cTo3d_FKCascadeOnEntityDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, _ = s.db.Exec(`INSERT INTO entities(name, type, updated_at) VALUES('Y','PERSON',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Y'`).Scan(&id)

	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 4, x'00000000', 1)`,
		id)

	res, err := s.db.Exec(`DELETE FROM entities WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("DELETE entity: %v", err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		t.Fatalf("DELETE affected %d rows, want 1", affected)
	}

	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&n)
	if n != 0 {
		t.Errorf("entity_embeddings not cascaded on entity delete; found %d rows", n)
	}
}
