package memory

// schemaVersion is the string stored in schema_meta.v. Bump on every
// incompatible migration; Phase 3.B will introduce "3b" alongside.
const schemaVersion = "3a"

// schemaDDL is applied idempotently on every OpenSqlite. CREATE IF NOT
// EXISTS everywhere so re-open on an existing DB is a cheap no-op.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS schema_meta (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta(k, v) VALUES ('version', '` + schemaVersion + `');

CREATE TABLE IF NOT EXISTS turns (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id  TEXT    NOT NULL,
	role        TEXT    NOT NULL CHECK(role IN ('user','assistant')),
	content     TEXT    NOT NULL,
	ts_unix     INTEGER NOT NULL,
	meta_json   TEXT
);

CREATE INDEX IF NOT EXISTS idx_turns_session_ts
	ON turns(session_id, ts_unix);

CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(
	content,
	content='turns',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS turns_ai AFTER INSERT ON turns BEGIN
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_ad AFTER DELETE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_au AFTER UPDATE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;
`
