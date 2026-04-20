package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSqlite_CreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n); err != nil {
		t.Errorf("turns table missing: %v", err)
	}
	if n != 0 {
		t.Errorf("turns count at startup = %d, want 0", n)
	}

	if err := s.db.QueryRow("SELECT COUNT(*) FROM turns_fts").Scan(&n); err != nil {
		t.Errorf("turns_fts virtual table missing: %v", err)
	}
}

func TestOpenSqlite_SchemaMetaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	err := s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if err != nil {
		t.Fatalf("schema_meta missing: %v", err)
	}
	if v != "3a" {
		t.Errorf("schema version = %q, want %q", v, "3a")
	}
}

func TestOpenSqlite_AutoCreatesParentDir(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "newsubdir")
	path := filepath.Join(parent, "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite (missing parent dir): %v", err)
	}
	defer s.Close(context.Background())

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("parent dir should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("parent dir perm = %o, want 0700", perm)
	}
}

func TestOpenSqlite_SetsWALMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}
