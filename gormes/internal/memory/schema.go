package memory

// schemaVersion is the canonical target version for this binary. OpenSqlite
// migrates any earlier supported version up to this value, and refuses to
// open DBs with an unknown version (future schemas).
const schemaVersion = "3c"

// schemaV3a is the baseline schema installed on a fresh DB. It matches
// exactly what Phase 3.A shipped — any change to this string is a schema
// migration and must go through the version-gated migrate() path.
const schemaV3a = `
CREATE TABLE IF NOT EXISTS schema_meta (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta(k, v) VALUES ('version', '3a');

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

// migration3aTo3b extends v3a with the Ontological Graph:
//   - turns gains extracted / extraction_attempts / extraction_error columns
//   - partial index idx_turns_unextracted for O(log n) polling
//   - entities + relationships tables with type/predicate CHECK whitelists
const migration3aTo3b = `
ALTER TABLE turns ADD COLUMN extracted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_error TEXT;
CREATE INDEX IF NOT EXISTS idx_turns_unextracted
	ON turns(id) WHERE extracted = 0;

CREATE TABLE IF NOT EXISTS entities (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT    NOT NULL,
	type        TEXT    NOT NULL CHECK(type IN (
	                'PERSON','PROJECT','CONCEPT','PLACE','ORGANIZATION','TOOL','OTHER'
	            )),
	description TEXT,
	updated_at  INTEGER NOT NULL,
	UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);

CREATE TABLE IF NOT EXISTS relationships (
	source_id   INTEGER NOT NULL,
	target_id   INTEGER NOT NULL,
	predicate   TEXT    NOT NULL CHECK(predicate IN (
	                'WORKS_ON','KNOWS','LIKES','DISLIKES',
	                'HAS_SKILL','LOCATED_IN','PART_OF','RELATED_TO'
	            )),
	weight      REAL    NOT NULL DEFAULT 1.0,
	updated_at  INTEGER NOT NULL,
	PRIMARY KEY(source_id, target_id, predicate),
	FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE,
	FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);
CREATE INDEX IF NOT EXISTS idx_relationships_predicate ON relationships(predicate);

UPDATE schema_meta SET v = '3b' WHERE k = 'version' AND v = '3a';
`

// migration3bTo3c extends v3b with Phase 3.C seed-scoping:
//   - turns gains chat_id column for per-chat seed selection
//   - idx_turns_chat_id makes the scoped-seed FTS5 join cheap
const migration3bTo3c = `
ALTER TABLE turns ADD COLUMN chat_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_turns_chat_id ON turns(chat_id, id);

UPDATE schema_meta SET v = '3c' WHERE k = 'version' AND v = '3b';
`
