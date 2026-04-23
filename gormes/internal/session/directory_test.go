package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestBoltMap_MetadataRoundTripAndListByUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
		UpdatedAt: 10,
	}); err != nil {
		t.Fatalf("PutMetadata telegram: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-discord",
		Source:    "discord",
		ChatID:    "chan-9",
		UserID:    "user-juan",
		UpdatedAt: 20,
	}); err != nil {
		t.Fatalf("PutMetadata discord: %v", err)
	}

	got, ok, err := m.GetMetadata(ctx, "sess-telegram")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata() ok = false, want true")
	}
	if got.Source != "telegram" || got.ChatID != "42" || got.UserID != "user-juan" {
		t.Fatalf("GetMetadata() = %+v, want telegram/42/user-juan", got)
	}

	userID, ok, err := m.ResolveUserID(ctx, "telegram", "42")
	if err != nil {
		t.Fatalf("ResolveUserID: %v", err)
	}
	if !ok {
		t.Fatal("ResolveUserID() ok = false, want true")
	}
	if userID != "user-juan" {
		t.Fatalf("ResolveUserID() = %q, want user-juan", userID)
	}

	listed, err := m.ListMetadataByUserID(ctx, "user-juan")
	if err != nil {
		t.Fatalf("ListMetadataByUserID: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListMetadataByUserID() len = %d, want 2", len(listed))
	}
	if listed[0].SessionID != "sess-discord" || listed[1].SessionID != "sess-telegram" {
		t.Fatalf("ListMetadataByUserID() = %+v, want UpdatedAt-desc deterministic order", listed)
	}
}

func TestBoltMap_PutMetadataInheritsUserBindingFromChat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-1",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata first session: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-2",
		Source:    "telegram",
		ChatID:    "42",
	}); err != nil {
		t.Fatalf("PutMetadata second session: %v", err)
	}

	got, ok, err := m.GetMetadata(ctx, "sess-2")
	if err != nil {
		t.Fatalf("GetMetadata second session: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata second session ok = false, want true")
	}
	if got.UserID != "user-juan" {
		t.Fatalf("GetMetadata second session user_id = %q, want inherited user-juan", got.UserID)
	}
}

func TestBoltMap_PutMetadataRejectsConflictingUserBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-1",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
	}); err != nil {
		t.Fatalf("PutMetadata first binding: %v", err)
	}

	err = m.PutMetadata(ctx, Metadata{
		SessionID: "sess-2",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-maria",
	})
	if !errors.Is(err, ErrUserBindingConflict) {
		t.Fatalf("PutMetadata conflicting binding err = %v, want ErrUserBindingConflict", err)
	}
}

func TestBoltMap_MetadataRoundTripIncludesParentSessionAndLineageKind(t *testing.T) {
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
		UserID:          "user-juan",
		ParentSessionID: "sess-root",
		LineageKind:     LineageKindCompressionSplit,
		UpdatedAt:       20,
	}); err != nil {
		t.Fatalf("PutMetadata child: %v", err)
	}

	root, ok, err := m.GetMetadata(ctx, "sess-root")
	if err != nil {
		t.Fatalf("GetMetadata root: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata(root) ok = false, want true")
	}
	if root.ParentSessionID != "" {
		t.Fatalf("root ParentSessionID = %q, want empty", root.ParentSessionID)
	}
	if root.LineageKind != LineageKindPrimary {
		t.Fatalf("root LineageKind = %q, want %q", root.LineageKind, LineageKindPrimary)
	}

	child, ok, err := m.GetMetadata(ctx, "sess-child")
	if err != nil {
		t.Fatalf("GetMetadata child: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata(child) ok = false, want true")
	}
	if child.ParentSessionID != "sess-root" || child.LineageKind != LineageKindCompressionSplit {
		t.Fatalf("GetMetadata(child) = %+v, want parent sess-root with compression_split lineage", child)
	}
}

func TestBoltMap_PutMetadataRejectsLineageLoops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()

	err = m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-self",
		ParentSessionID: "sess-self",
		LineageKind:     LineageKindCompressionSplit,
	})
	if !errors.Is(err, ErrLineageCycle) {
		t.Fatalf("PutMetadata self-parent err = %v, want ErrLineageCycle", err)
	}

	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-root",
		UpdatedAt: 10,
	}); err != nil {
		t.Fatalf("PutMetadata root: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-child",
		ParentSessionID: "sess-root",
		LineageKind:     LineageKindCompressionSplit,
		UpdatedAt:       20,
	}); err != nil {
		t.Fatalf("PutMetadata child: %v", err)
	}

	err = m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-root",
		ParentSessionID: "sess-child",
		LineageKind:     LineageKindFork,
		UpdatedAt:       30,
	})
	if !errors.Is(err, ErrLineageCycle) {
		t.Fatalf("PutMetadata trivial loop err = %v, want ErrLineageCycle", err)
	}
}

func TestMemMap_MetadataRoundTripAndConflictRules(t *testing.T) {
	m := NewMemMap()
	ctx := context.Background()

	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-telegram",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-juan",
		UpdatedAt: 10,
	}); err != nil {
		t.Fatalf("PutMetadata telegram: %v", err)
	}
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-discord",
		Source:    "discord",
		ChatID:    "chan-9",
		UserID:    "user-juan",
		UpdatedAt: 20,
	}); err != nil {
		t.Fatalf("PutMetadata discord: %v", err)
	}

	listed, err := m.ListMetadataByUserID(ctx, "user-juan")
	if err != nil {
		t.Fatalf("ListMetadataByUserID: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("ListMetadataByUserID() len = %d, want 2", len(listed))
	}

	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-telegram-2",
		Source:    "telegram",
		ChatID:    "42",
	}); err != nil {
		t.Fatalf("PutMetadata inherited binding: %v", err)
	}
	got, ok, err := m.GetMetadata(ctx, "sess-telegram-2")
	if err != nil {
		t.Fatalf("GetMetadata inherited binding: %v", err)
	}
	if !ok || got.UserID != "user-juan" {
		t.Fatalf("GetMetadata inherited binding = %+v, %v, want user-juan", got, ok)
	}

	err = m.PutMetadata(ctx, Metadata{
		SessionID: "sess-conflict",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "user-maria",
	})
	if !errors.Is(err, ErrUserBindingConflict) {
		t.Fatalf("PutMetadata conflicting binding err = %v, want ErrUserBindingConflict", err)
	}
}
