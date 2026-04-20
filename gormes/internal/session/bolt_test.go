package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBolt_PutGetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-abc"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := m.Get(ctx, "telegram:42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sess-abc" {
		t.Errorf("Get = %q, want %q", got, "sess-abc")
	}
}

func TestBolt_GetMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, _ := OpenBolt(path)
	defer m.Close()
	got, err := m.Get(context.Background(), "does-not-exist")
	if err != nil {
		t.Errorf("Get on missing key should not error, got %v", err)
	}
	if got != "" {
		t.Errorf("Get on missing key = %q, want \"\"", got)
	}
}

func TestBolt_AutoCreatesParentDir(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "newsubdir")
	path := filepath.Join(parent, "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt (missing parent dir): %v", err)
	}
	defer m.Close()

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("parent dir should exist after OpenBolt: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("parent is not a dir")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("parent dir perm = %o, want 0700", perm)
	}
}

func TestBolt_CloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, _ := OpenBolt(path)
	if err := m.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

func TestBolt_ConcurrentPutGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, _ := OpenBolt(path)
	defer m.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = m.Put(ctx, "k", "v") }()
		go func() { defer wg.Done(); _, _ = m.Get(ctx, "k") }()
	}
	wg.Wait()

	got, _ := m.Get(ctx, "k")
	if got != "v" {
		t.Errorf("after concurrent writes, Get = %q, want %q", got, "v")
	}
}
