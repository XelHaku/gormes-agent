package memory

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrSchemaUnknown is returned by OpenSqlite when the DB's schema version
// is neither the current target nor any known predecessor. Callers should
// exit 1 with a clear message; the DB may have been written by a future
// binary.
var ErrSchemaUnknown = errors.New("memory: schema version unknown to this binary")

// migrate installs or upgrades the DB schema to schemaVersion. Safe to
// call on a fresh DB (installs v3a then migrates to current) or on any
// previously-migrated DB (runs only the needed steps). Single transaction
// per migration step so a failure leaves the DB in a consistent state.
func migrate(db *sql.DB) error {
	// Ensure schema_meta + v3a baseline exist. Idempotent on re-run.
	if _, err := db.Exec(schemaV3a); err != nil {
		return fmt.Errorf("memory: apply v3a baseline: %w", err)
	}

	var v string
	if err := db.QueryRow(`SELECT v FROM schema_meta WHERE k = 'version'`).Scan(&v); err != nil {
		return fmt.Errorf("memory: read schema version: %w", err)
	}

	switch v {
	case "3a":
		if err := runMigrationTx(db, migration3aTo3b); err != nil {
			return fmt.Errorf("memory: migrate 3a->3b: %w", err)
		}
		return migrate(db)
	case "3b":
		if err := runMigrationTx(db, migration3bTo3c); err != nil {
			return fmt.Errorf("memory: migrate 3b->3c: %w", err)
		}
		return migrate(db)
	case "3c":
		if err := runMigrationTx(db, migration3cTo3d); err != nil {
			return fmt.Errorf("memory: migrate 3c->3d: %w", err)
		}
		return migrate(db)
	case "3d":
		if err := runMigrationTx(db, migration3dTo3e); err != nil {
			return fmt.Errorf("memory: migrate 3d->3e: %w", err)
		}
		return migrate(db)
	case "3e":
		if err := runMigrationTx(db, migration3eTo3f); err != nil {
			return fmt.Errorf("memory: migrate 3e->3f: %w", err)
		}
		return migrate(db)
	case "3f":
		return nil
	default:
		return fmt.Errorf("%w: got %q, want %q", ErrSchemaUnknown, v, schemaVersion)
	}
}

// runMigrationTx applies a multi-statement DDL script in a single
// transaction. If any statement fails, the transaction rolls back and the
// DB stays at its previous version.
func runMigrationTx(db *sql.DB, ddl string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after successful Commit

	if _, err := tx.Exec(ddl); err != nil {
		return err
	}
	return tx.Commit()
}
