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
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %s", v, schemaVersion)
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

	// Re-open — migration runs against the current schema, should no-op.
	s2, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer s2.Close(context.Background())

	var v string
	_ = s2.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != schemaVersion {
		t.Errorf("version = %q after re-open, want %s", v, schemaVersion)
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
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %s", v, schemaVersion)
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
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %s", v, schemaVersion)
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

func TestOpenSqlite_FreshDBIsV3e(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %s", v, schemaVersion)
	}
}

func TestOpenSqlite_FreshDBIsV3f(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %s", v, schemaVersion)
	}
}

func TestMigrate_3eTo3f_AddsGonchoTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	for _, table := range []string{"goncho_peer_cards", "goncho_conclusions"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestMigrate_3eTo3f_AddsGonchoConclusionsFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	var name string
	err = s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='goncho_conclusions_fts'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("goncho_conclusions_fts missing: %v", err)
	}
}

func TestMigrate_3fTo3g_AddsMemorySyncColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	for _, col := range []string{"turn_key", "memory_sync_status", "memory_sync_reason"} {
		var name string
		row := s.db.QueryRow(
			`SELECT name FROM pragma_table_info('turns') WHERE name = ?`, col)
		if err := row.Scan(&name); err != nil {
			t.Errorf("column %q missing from turns: %v", col, err)
		}
	}
}

func TestMigrate_3gTo3h_PreservesFlatPeerCardsAsGormesObserver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	fixture, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite fixture: %v", err)
	}
	if err := fixture.Close(context.Background()); err != nil {
		t.Fatalf("close fixture store: %v", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := applyPragmas(db); err != nil {
		t.Fatalf("applyPragmas: %v", err)
	}
	if _, err := db.Exec(`
		DROP TABLE goncho_peer_cards;
		CREATE TABLE goncho_peer_cards (
			workspace_id TEXT NOT NULL,
			peer_id      TEXT NOT NULL,
			card_json    TEXT NOT NULL,
			updated_at   INTEGER NOT NULL,
			PRIMARY KEY(workspace_id, peer_id)
		);
		INSERT INTO goncho_peer_cards(workspace_id, peer_id, card_json, updated_at)
		VALUES('default', 'bob', '["Legacy Bob card"]', 123);
		UPDATE schema_meta SET v = '3g' WHERE k = 'version';
	`); err != nil {
		t.Fatalf("build legacy peer card fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close 3g fixture: %v", err)
	}

	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite migrated fixture: %v", err)
	}
	defer s.Close(context.Background())

	var observer string
	var card string
	err = s.db.QueryRow(`
		SELECT observer_peer_id, card_json
		FROM goncho_peer_cards
		WHERE workspace_id = 'default' AND peer_id = 'bob'
	`).Scan(&observer, &card)
	if err != nil {
		t.Fatalf("query migrated peer card: %v", err)
	}
	if observer != "gormes" {
		t.Fatalf("observer_peer_id = %q, want gormes", observer)
	}
	if card != `["Legacy Bob card"]` {
		t.Fatalf("card_json = %s, want legacy card", card)
	}

	var pkCount int
	if err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('goncho_peer_cards')
		WHERE name IN ('workspace_id', 'observer_peer_id', 'peer_id') AND pk > 0
	`).Scan(&pkCount); err != nil {
		t.Fatalf("query peer-card primary key: %v", err)
	}
	if pkCount != 3 {
		t.Fatalf("directional primary key column count = %d, want 3", pkCount)
	}
}

func TestMigrate_3dTo3e_AddsCronColumnsToTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	rows, err := s.db.Query(`PRAGMA table_info(turns)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	has := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		_ = rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		has[name] = true
	}
	if !has["cron"] {
		t.Error("turns is missing 'cron' column")
	}
	if !has["cron_job_id"] {
		t.Error("turns is missing 'cron_job_id' column")
	}
}

func TestMigrate_3dTo3e_AddsCronRunsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cron_runs`).Scan(&n)
	if err != nil {
		t.Errorf("cron_runs table missing: %v", err)
	}
}

func TestMigrate_3dTo3e_StatusCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	for _, status := range []string{"success", "timeout", "error", "suppressed"} {
		_, err := s.db.Exec(
			`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status) VALUES(?, ?, ?, ?)`,
			"j", 1, "h", status)
		if err != nil {
			t.Errorf("status=%q rejected: %v", status, err)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status) VALUES('j', 1, 'h', 'nope')`)
	if err == nil {
		t.Error("status='nope' should trip CHECK constraint")
	}
}

func TestMigrate_3dTo3e_SuppressionReasonCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, err := s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
		 VALUES('j', 1, 'h', 'success', NULL)`)
	if err != nil {
		t.Errorf("suppression_reason NULL rejected: %v", err)
	}
	for _, r := range []string{"silent", "empty"} {
		_, err := s.db.Exec(
			`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
			 VALUES('j', 1, 'h', 'suppressed', ?)`, r)
		if err != nil {
			t.Errorf("suppression_reason=%q rejected: %v", r, err)
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
		 VALUES('j', 1, 'h', 'suppressed', 'bogus')`)
	if err == nil {
		t.Error("suppression_reason='bogus' should trip CHECK")
	}
}

func TestMigrate_3dTo3e_DeliveredCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// Valid delivered values.
	for _, d := range []int{0, 1} {
		_, err := s.db.Exec(
			`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, delivered)
			 VALUES('j', 1, 'h', 'success', ?)`, d)
		if err != nil {
			t.Errorf("delivered=%d rejected: %v", d, err)
		}
	}
	// Invalid delivered value must trip CHECK.
	_, err := s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, delivered)
		 VALUES('j', 1, 'h', 'success', 2)`)
	if err == nil {
		t.Error("delivered=2 should trip CHECK constraint")
	}
}
