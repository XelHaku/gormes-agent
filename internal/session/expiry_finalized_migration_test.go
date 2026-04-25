package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestExpiryFinalizedMigration_LoadsLegacyMemoryFlushedAsFinalizedEvidence(t *testing.T) {
	m := openExpiryMigrationBolt(t)
	defer m.Close()
	ctx := context.Background()

	seedRawMetadata(t, m, "sess-legacy", []byte(`{
		"session_id": "sess-legacy",
		"source": "telegram",
		"chat_id": "42",
		"user_id": "u-42",
		"updated_at": 10,
		"memory_flushed": true
	}`))

	got, ok, err := m.GetMetadata(ctx, "sess-legacy")
	if err != nil {
		t.Fatalf("GetMetadata legacy: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata legacy ok = false, want true")
	}
	if !got.ExpiryFinalized {
		t.Fatalf("ExpiryFinalized = false, want true after memory_flushed migration: %+v", got)
	}
	if !got.MigratedMemoryFlushed {
		t.Fatalf("MigratedMemoryFlushed = false, want legacy evidence: %+v", got)
	}
}

func TestExpiryFinalizedMigration_NewMetadataWritesUseExpiryFinalizedOnly(t *testing.T) {
	m := openExpiryMigrationBolt(t)
	defer m.Close()
	ctx := context.Background()

	if err := m.PutMetadata(ctx, Metadata{
		SessionID:       "sess-new",
		Source:          "telegram",
		ChatID:          "42",
		UserID:          "u-42",
		UpdatedAt:       20,
		ExpiryFinalized: true,
	}); err != nil {
		t.Fatalf("PutMetadata new finalized session: %v", err)
	}

	raw := rawMetadataObject(t, m, "sess-new")
	if got := rawBool(t, raw, "expiry_finalized"); !got {
		t.Fatalf("expiry_finalized = false, want true in raw metadata: %s", rawMetadataBytes(t, m, "sess-new"))
	}
	assertRawMetadataKeyMissing(t, raw, "memory_flushed")
	assertRawMetadataKeyMissing(t, raw, "migrated_memory_flushed")
}

func TestExpiryFinalizedMigration_RewritesLegacyResumeMetadataWithoutMemoryFlushedField(t *testing.T) {
	m := openExpiryMigrationBolt(t)
	defer m.Close()
	ctx := context.Background()

	seedRawMetadata(t, m, "sess-legacy", []byte(`{
		"session_id": "sess-legacy",
		"source": "telegram",
		"chat_id": "42",
		"user_id": "u-42",
		"updated_at": 10,
		"resume_pending": true,
		"resume_reason": "restart_timeout",
		"resume_marked_at": 11,
		"memory_flushed": true
	}`))

	cleared, err := m.ClearResumePending(ctx, "sess-legacy")
	if err != nil {
		t.Fatalf("ClearResumePending legacy: %v", err)
	}
	if !cleared {
		t.Fatal("ClearResumePending cleared = false, want true")
	}

	raw := rawMetadataObject(t, m, "sess-legacy")
	assertRawMetadataKeyMissing(t, raw, "memory_flushed")
	if got := rawBool(t, raw, "expiry_finalized"); !got {
		t.Fatalf("expiry_finalized = false, want true after rewrite: %s", rawMetadataBytes(t, m, "sess-legacy"))
	}
	if got := rawBool(t, raw, "migrated_memory_flushed"); !got {
		t.Fatalf("migrated_memory_flushed = false, want legacy evidence after rewrite: %s", rawMetadataBytes(t, m, "sess-legacy"))
	}

	got, ok, err := m.GetMetadata(ctx, "sess-legacy")
	if err != nil {
		t.Fatalf("GetMetadata rewritten legacy: %v", err)
	}
	if !ok {
		t.Fatal("GetMetadata rewritten legacy ok = false, want true")
	}
	if got.ResumePending || got.ResumeReason != "" || got.ResumeMarkedAt != 0 {
		t.Fatalf("resume fields were not cleared: %+v", got)
	}
	if !got.ExpiryFinalized || !got.MigratedMemoryFlushed {
		t.Fatalf("migration evidence not preserved after rewrite: %+v", got)
	}
}

func openExpiryMigrationBolt(t *testing.T) *BoltMap {
	t.Helper()
	m, err := OpenBolt(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	return m
}

func seedRawMetadata(t *testing.T, m *BoltMap, sessionID string, raw []byte) {
	t.Helper()
	if !json.Valid(raw) {
		t.Fatalf("invalid raw metadata fixture: %s", raw)
	}
	err := m.DB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		return b.Put([]byte(sessionID), raw)
	})
	if err != nil {
		t.Fatalf("seed raw metadata %s: %v", sessionID, err)
	}
}

func rawMetadataBytes(t *testing.T, m *BoltMap, sessionID string) []byte {
	t.Helper()
	var out []byte
	err := m.DB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		raw := b.Get([]byte(sessionID))
		if raw != nil {
			out = append([]byte(nil), raw...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read raw metadata %s: %v", sessionID, err)
	}
	if len(out) == 0 {
		t.Fatalf("raw metadata %s missing", sessionID)
	}
	return out
}

func rawMetadataObject(t *testing.T, m *BoltMap, sessionID string) map[string]json.RawMessage {
	t.Helper()
	raw := rawMetadataBytes(t, m, sessionID)
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode raw metadata %s: %v\n%s", sessionID, err, raw)
	}
	return out
}

func rawBool(t *testing.T, raw map[string]json.RawMessage, key string) bool {
	t.Helper()
	value, ok := raw[key]
	if !ok {
		t.Fatalf("raw metadata missing %q", key)
	}
	var got bool
	if err := json.Unmarshal(value, &got); err != nil {
		t.Fatalf("raw metadata %q is not bool: %v", key, err)
	}
	return got
}

func assertRawMetadataKeyMissing(t *testing.T, raw map[string]json.RawMessage, key string) {
	t.Helper()
	if _, ok := raw[key]; ok {
		t.Fatalf("raw metadata unexpectedly contains %q: %+v", key, raw)
	}
}
