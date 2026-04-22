package memory

// schemaVersion is the canonical target version for this binary. OpenSqlite
// migrates any earlier supported version up to this value, and refuses to
// open DBs with an unknown version (future schemas).
const schemaVersion = "3f"

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

// migration3cTo3d extends v3c with Phase 3.D semantic fusion:
//   - entity_embeddings table holds L2-normalized float32 vectors
//     (little-endian BLOB) alongside model name + dim for mismatch
//     detection. FK cascade cleans up if the entity is deleted.
//   - idx_entity_embeddings_model makes model-filtered scans cheap.
const migration3cTo3d = `
CREATE TABLE IF NOT EXISTS entity_embeddings (
	entity_id   INTEGER PRIMARY KEY,
	model       TEXT    NOT NULL,
	dim         INTEGER NOT NULL CHECK(dim > 0 AND dim <= 4096),
	vec         BLOB    NOT NULL,
	updated_at  INTEGER NOT NULL,
	FOREIGN KEY(entity_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entity_embeddings_model
	ON entity_embeddings(model);

UPDATE schema_meta SET v = '3d' WHERE k = 'version' AND v = '3c';
`

// migration3dTo3e extends v3d with Phase 2.D cron fields:
//   - turns gains cron / cron_job_id columns; default 0/NULL so
//     existing rows (non-cron) are unaffected.
//   - cron_runs table is the per-run audit trail: one row per
//     scheduled fire, capturing outcome + delivery decision.
//   - CHECK constraints lock the allowed status / suppression_reason
//     values so garbage data can't enter the audit log.
const migration3dTo3e = `
ALTER TABLE turns ADD COLUMN cron INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN cron_job_id TEXT;

CREATE TABLE IF NOT EXISTS cron_runs (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id              TEXT    NOT NULL,
	started_at          INTEGER NOT NULL,
	finished_at         INTEGER,
	prompt_hash         TEXT    NOT NULL,
	status              TEXT    NOT NULL CHECK(status IN (
	                        'success','timeout','error','suppressed'
	                    )),
	delivered           INTEGER NOT NULL DEFAULT 0 CHECK(delivered IN (0,1)),
	suppression_reason  TEXT    CHECK(suppression_reason IS NULL OR
	                                  suppression_reason IN ('silent','empty')),
	output_preview      TEXT,
	error_msg           TEXT
);
CREATE INDEX IF NOT EXISTS idx_cron_runs_job_started
	ON cron_runs(job_id, started_at DESC);

UPDATE schema_meta SET v = '3e' WHERE k = 'version' AND v = '3d';
`

// migration3eTo3f adds the first Goncho-owned persistence surface:
//   - goncho_peer_cards stores the global card per peer
//   - goncho_conclusions stores durable manual or derived facts
//   - goncho_conclusions_fts indexes conclusion content for lexical search
const migration3eTo3f = `
CREATE TABLE IF NOT EXISTS goncho_peer_cards (
	workspace_id TEXT NOT NULL,
	peer_id      TEXT NOT NULL,
	card_json    TEXT NOT NULL,
	updated_at   INTEGER NOT NULL,
	PRIMARY KEY(workspace_id, peer_id)
);

CREATE TABLE IF NOT EXISTS goncho_conclusions (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id     TEXT NOT NULL,
	observer_peer_id TEXT NOT NULL,
	peer_id          TEXT NOT NULL,
	session_key      TEXT,
	content          TEXT NOT NULL,
	kind             TEXT NOT NULL DEFAULT 'manual',
	status           TEXT NOT NULL CHECK(status IN ('pending','processed','dead_letter')),
	source           TEXT NOT NULL DEFAULT 'manual',
	idempotency_key  TEXT NOT NULL,
	evidence_json    TEXT NOT NULL DEFAULT '[]',
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_goncho_conclusions_idempotency
	ON goncho_conclusions(workspace_id, observer_peer_id, peer_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_goncho_conclusions_peer
	ON goncho_conclusions(workspace_id, peer_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_goncho_conclusions_session
	ON goncho_conclusions(workspace_id, session_key, updated_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS goncho_conclusions_fts USING fts5(
	content,
	content='goncho_conclusions',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS goncho_conclusions_ai AFTER INSERT ON goncho_conclusions BEGIN
	INSERT INTO goncho_conclusions_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS goncho_conclusions_ad AFTER DELETE ON goncho_conclusions BEGIN
	INSERT INTO goncho_conclusions_fts(goncho_conclusions_fts, rowid, content) VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS goncho_conclusions_au AFTER UPDATE ON goncho_conclusions BEGIN
	INSERT INTO goncho_conclusions_fts(goncho_conclusions_fts, rowid, content) VALUES('delete', old.id, old.content);
	INSERT INTO goncho_conclusions_fts(rowid, content) VALUES (new.id, new.content);
END;

UPDATE schema_meta SET v = '3f' WHERE k = 'version' AND v = '3e';
`
