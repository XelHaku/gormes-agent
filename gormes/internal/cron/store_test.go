package cron

import (
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return s, func() { _ = db.Close() }
}

func TestStore_CreateAndGet(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	j := NewJob("morning", "0 8 * * *", "status")
	if err := s.Create(j); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "morning" || got.Schedule != "0 8 * * *" || got.Prompt != "status" {
		t.Errorf("got = %+v, want name/sched/prompt intact", got)
	}
}

func TestStore_List(t *testing.T) {
	s, done := newTestStore(t)
	defer done()
	_ = s.Create(NewJob("a", "@daily", "x"))
	_ = s.Create(NewJob("b", "@hourly", "y"))
	_ = s.Create(NewJob("c", "@every 1m", "z"))
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestStore_Update(t *testing.T) {
	s, done := newTestStore(t)
	defer done()
	j := NewJob("m", "@daily", "p")
	_ = s.Create(j)
	j.Paused = true
	j.LastRunUnix = 1700000000
	j.LastStatus = "success"
	if err := s.Update(j); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(j.ID)
	if !got.Paused || got.LastRunUnix != 1700000000 || got.LastStatus != "success" {
		t.Errorf("after Update, got = %+v", got)
	}
}

func TestStore_Delete(t *testing.T) {
	s, done := newTestStore(t)
	defer done()
	j := NewJob("x", "@daily", "y")
	_ = s.Create(j)
	if err := s.Delete(j.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(j.ID); err == nil {
		t.Error("Get after Delete returned no error, want ErrJobNotFound")
	}
}

func TestStore_GetMissingReturnsTypedError(t *testing.T) {
	s, done := newTestStore(t)
	defer done()
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if err != ErrJobNotFound {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestStore_CreateRejectsDuplicateName(t *testing.T) {
	s, done := newTestStore(t)
	defer done()
	_ = s.Create(NewJob("same", "@daily", "p1"))
	err := s.Create(NewJob("same", "@hourly", "p2"))
	if err == nil {
		t.Fatal("expected error on duplicate name")
	}
	if err != ErrJobNameTaken {
		t.Errorf("err = %v, want ErrJobNameTaken", err)
	}
}
