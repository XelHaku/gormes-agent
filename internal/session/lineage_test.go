package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoltMap_MetadataPersistsLineageWithoutRewritingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-root",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
		UpdatedAt: 10,
	}); err != nil {
		t.Fatalf("PutMetadata root: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-child",
		Source:          "telegram",
		ChatID:          "42",
		ParentSessionID: "sess-root",
		LineageKind:     LineageKindCompression,
		UpdatedAt:       20,
	}); err != nil {
		t.Fatalf("PutMetadata child: %v", err)
	}

	root, ok, err := m.GetMetadata(ctx, "sess-root")
	if err != nil {
		t.Fatalf("GetMetadata root: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata root ok = false, want true")
	}
	if root.ParentSessionID != "" {
		t.Fatalf("root parent_session_id = %q, want empty", root.ParentSessionID)
	}
	if root.LineageKind != LineageKindPrimary {
		t.Fatalf("root lineage_kind = %q, want %q", root.LineageKind, LineageKindPrimary)
	}

	child, ok, err := m.GetMetadata(ctx, "sess-child")
	if err != nil {
		t.Fatalf("GetMetadata child: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata child ok = false, want true")
	}
	if child.ParentSessionID != "sess-root" || child.LineageKind != LineageKindCompression {
		t.Fatalf("child lineage = parent %q kind %q, want sess-root/%s", child.ParentSessionID, child.LineageKind, LineageKindCompression)
	}
	if child.UserID != "user-juan" {
		t.Fatalf("child inherited user_id = %q, want user-juan", child.UserID)
	}
}

func TestLineageRejectsSelfParentAndLoops(t *testing.T) {
	ctx := context.Background()

	t.Run("bolt self parent", func(t *testing.T) {
		m := openBoltForLineageTest(t)
		err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-a",
			ParentSessionID: "sess-a",
			LineageKind:     LineageKindCompression,
		})
		if !errors.Is(err, ErrLineageLoop) {
			t.Fatalf("PutMetadata self parent err = %v, want ErrLineageLoop", err)
		}
	})

	t.Run("bolt two node loop", func(t *testing.T) {
		m := openBoltForLineageTest(t)
		if err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-b",
			ParentSessionID: "sess-a",
			LineageKind:     LineageKindCompression,
		}); err != nil {
			t.Fatalf("PutMetadata first edge: %v", err)
		}
		err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-a",
			ParentSessionID: "sess-b",
			LineageKind:     LineageKindCompression,
		})
		if !errors.Is(err, ErrLineageLoop) {
			t.Fatalf("PutMetadata two node loop err = %v, want ErrLineageLoop", err)
		}
	})

	t.Run("mem self parent", func(t *testing.T) {
		m := NewMemMap()
		err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-a",
			ParentSessionID: "sess-a",
			LineageKind:     LineageKindCompression,
		})
		if !errors.Is(err, ErrLineageLoop) {
			t.Fatalf("PutMetadata self parent err = %v, want ErrLineageLoop", err)
		}
	})

	t.Run("mem two node loop", func(t *testing.T) {
		m := NewMemMap()
		if err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-b",
			ParentSessionID: "sess-a",
			LineageKind:     LineageKindCompression,
		}); err != nil {
			t.Fatalf("PutMetadata first edge: %v", err)
		}
		err := m.PutMetadata(ctx, Metadata{
			SessionID:       "sess-a",
			ParentSessionID: "sess-b",
			LineageKind:     LineageKindCompression,
		})
		if !errors.Is(err, ErrLineageLoop) {
			t.Fatalf("PutMetadata two node loop err = %v, want ErrLineageLoop", err)
		}
	})
}

func TestSessionIndexMirror_ExposesLineageAudit(t *testing.T) {
	m := openBoltForLineageTest(t)
	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-root"); err != nil {
		t.Fatalf("Put root mapping: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-root",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
		UpdatedAt: 10,
	}); err != nil {
		t.Fatalf("PutMetadata root: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-child",
		Source:          "telegram",
		ChatID:          "42",
		ParentSessionID: "sess-root",
		LineageKind:     LineageKindCompression,
		UpdatedAt:       20,
	}); err != nil {
		t.Fatalf("PutMetadata child: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-orphan",
		Source:          "telegram",
		ChatID:          "99",
		UserID:          "user-orphan",
		ParentSessionID: "sess-missing",
		LineageKind:     LineageKindFork,
		UpdatedAt:       30,
	}); err != nil {
		t.Fatalf("PutMetadata orphan: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "sessions", "index.yaml")
	mirror := NewSessionIndexMirror(m, outPath)
	mirror.now = func() time.Time {
		return time.Date(2026, 4, 25, 1, 40, 0, 0, time.UTC)
	}

	if err := mirror.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outPath, err)
	}
	got := string(raw)
	for _, want := range []string{
		"lineage:\n",
		"  sess-root:\n",
		"    lineage_kind: primary\n",
		"    children:\n",
		"      - sess-child\n",
		"    lineage_status: ok\n",
		"  sess-child:\n",
		"    parent_session_id: sess-root\n",
		"    lineage_kind: compression\n",
		"    lineage_status: ok\n",
		"  sess-orphan:\n",
		"    parent_session_id: sess-missing\n",
		"    lineage_kind: fork\n",
		"    lineage_status: orphan\n",
		"updated_at: 2026-04-25T01:40:00Z\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mirror YAML missing %q:\n%s", want, got)
		}
	}
}

func TestBoltMap_ResolveLineageTipFollowsCompressionChildrenOnly(t *testing.T) {
	m := openBoltForLineageTest(t)
	ctx := context.Background()
	for _, meta := range []Metadata{
		{SessionID: "sess-root", UpdatedAt: 10},
		{SessionID: "sess-fork", ParentSessionID: "sess-root", LineageKind: LineageKindFork, UpdatedAt: 40},
		{SessionID: "sess-child", ParentSessionID: "sess-root", LineageKind: LineageKindCompression, UpdatedAt: 20},
		{SessionID: "sess-grandchild", ParentSessionID: "sess-child", LineageKind: LineageKindCompression, UpdatedAt: 30},
	} {
		if err := m.PutMetadata(ctx, meta); err != nil {
			t.Fatalf("PutMetadata(%s): %v", meta.SessionID, err)
		}
	}

	resolved, err := m.ResolveLineageTip(ctx, "sess-root")
	if err != nil {
		t.Fatalf("ResolveLineageTip: %v", err)
	}
	if resolved.LiveSessionID != "sess-grandchild" {
		t.Fatalf("ResolveLineageTip live = %q, want sess-grandchild", resolved.LiveSessionID)
	}
	if strings.Join(resolved.Path, " -> ") != "sess-root -> sess-child -> sess-grandchild" {
		t.Fatalf("ResolveLineageTip path = %v", resolved.Path)
	}
	if resolved.Status != LineageStatusOK {
		t.Fatalf("ResolveLineageTip status = %q, want %q", resolved.Status, LineageStatusOK)
	}
}

func openBoltForLineageTest(t *testing.T) *BoltMap {
	t.Helper()
	m, err := OpenBolt(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}
