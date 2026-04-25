package gateway

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPairingStore_PersistsPendingAndApprovedUnderXDGStateRoot(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgRoot)

	wantPath := filepath.Join(xdgRoot, "gormes", "pairing.json")
	if got := DefaultPairingStorePath(); got != wantPath {
		t.Fatalf("DefaultPairingStorePath() = %q, want %q", got, wantPath)
	}

	store := NewXDGPairingStore()
	now := time.Date(2026, 4, 25, 18, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := store.RecordPendingPairing(context.Background(), PairingPendingRecord{
		Platform:  "telegram",
		Code:      "TG-READY",
		UserID:    "telegram-user-42",
		UserName:  "Ada",
		CreatedAt: now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("record pending: %v", err)
	}
	if err := store.RecordApprovedPairing(context.Background(), PairingApprovedRecord{
		Platform:   "discord",
		UserID:     "discord-user-7",
		UserName:   "Grace",
		ApprovedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("record approved: %v", err)
	}

	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("stat pairing read model: %v", err)
	}
	assertOwnerOnlyMode(t, wantPath)

	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("read pairing status: %v", err)
	}
	if len(status.Degraded) != 0 {
		t.Fatalf("degraded evidence = %+v, want none", status.Degraded)
	}
	if got := pendingKeys(status.Pending); !reflect.DeepEqual(got, []string{"telegram/telegram-user-42/TG-READY/300"}) {
		t.Fatalf("pending = %v", got)
	}
	if got := approvedKeys(status.Approved); !reflect.DeepEqual(got, []string{"discord/discord-user-7"}) {
		t.Fatalf("approved = %v", got)
	}
	if got := platformKeys(status.Platforms); !reflect.DeepEqual(got, []string{"discord/paired/0/1", "telegram/unpaired/1/0"}) {
		t.Fatalf("platform statuses = %v", got)
	}
}

func TestPairingStore_DeterministicReadoutOrdering(t *testing.T) {
	store := NewPairingStore(filepath.Join(t.TempDir(), "pairing.json"))
	now := time.Date(2026, 4, 25, 19, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	for _, record := range []PairingPendingRecord{
		{Platform: "telegram", UserID: "user-b", Code: "NEW", CreatedAt: now.Add(-10 * time.Minute)},
		{Platform: "telegram", UserID: "user-a", Code: "YOUNG", CreatedAt: now.Add(-1 * time.Minute)},
		{Platform: "discord", UserID: "user-c", Code: "DISCORD", CreatedAt: now.Add(-20 * time.Minute)},
		{Platform: "telegram", UserID: "user-a", Code: "OLD", CreatedAt: now.Add(-30 * time.Minute)},
	} {
		if err := store.RecordPendingPairing(context.Background(), record); err != nil {
			t.Fatalf("record pending %s: %v", record.Code, err)
		}
	}
	for _, record := range []PairingApprovedRecord{
		{Platform: "telegram", UserID: "user-z", ApprovedAt: now.Add(-1 * time.Minute)},
		{Platform: "discord", UserID: "user-b", ApprovedAt: now.Add(-2 * time.Minute)},
		{Platform: "discord", UserID: "user-a", ApprovedAt: now.Add(-3 * time.Minute)},
	} {
		if err := store.RecordApprovedPairing(context.Background(), record); err != nil {
			t.Fatalf("record approved %s/%s: %v", record.Platform, record.UserID, err)
		}
	}

	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("read pairing status: %v", err)
	}

	wantPending := []string{
		"discord/user-c/DISCORD/1200",
		"telegram/user-a/OLD/1800",
		"telegram/user-a/YOUNG/60",
		"telegram/user-b/NEW/600",
	}
	if got := pendingKeys(status.Pending); !reflect.DeepEqual(got, wantPending) {
		t.Fatalf("pending order = %v, want %v", got, wantPending)
	}

	wantApproved := []string{
		"discord/user-a",
		"discord/user-b",
		"telegram/user-z",
	}
	if got := approvedKeys(status.Approved); !reflect.DeepEqual(got, wantApproved) {
		t.Fatalf("approved order = %v, want %v", got, wantApproved)
	}

	wantPlatforms := []string{
		"discord/paired/1/2",
		"telegram/paired/3/1",
	}
	if got := platformKeys(status.Platforms); !reflect.DeepEqual(got, wantPlatforms) {
		t.Fatalf("platform order = %v, want %v", got, wantPlatforms)
	}
}

func TestPairingStore_DegradedReadsReturnEmptyDeterministicStatus(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		store := NewPairingStore(filepath.Join(t.TempDir(), "pairing.json"))

		status, err := store.ReadPairingStatus(context.Background())
		if err != nil {
			t.Fatalf("read missing pairing status: %v", err)
		}
		assertEmptyPairingStatus(t, status, PairingDegradedMissing)
	})

	t.Run("corrupt json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pairing.json")
		if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
			t.Fatalf("write corrupt pairing state: %v", err)
		}
		store := NewPairingStore(path)

		status, err := store.ReadPairingStatus(context.Background())
		if err != nil {
			t.Fatalf("read corrupt pairing status: %v", err)
		}
		assertEmptyPairingStatus(t, status, PairingDegradedCorrupt)
		if !strings.Contains(status.Degraded[0].Message, "decode") {
			t.Fatalf("degraded message = %q, want decode evidence", status.Degraded[0].Message)
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		store := NewPairingStore(filepath.Join(t.TempDir(), "pairing.json"))
		store.readFile = func(string) ([]byte, error) {
			return nil, fs.ErrPermission
		}

		status, err := store.ReadPairingStatus(context.Background())
		if err != nil {
			t.Fatalf("read permission-denied pairing status: %v", err)
		}
		assertEmptyPairingStatus(t, status, PairingDegradedPermissionDenied)
	})
}

func TestPairingStore_AtomicWriteFailurePreservesOldState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pairing.json")
	store := NewPairingStore(path)
	now := time.Date(2026, 4, 25, 20, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := store.RecordApprovedPairing(context.Background(), PairingApprovedRecord{
		Platform:   "telegram",
		UserID:     "old-user",
		ApprovedAt: now,
	}); err != nil {
		t.Fatalf("record old approved user: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read old pairing state: %v", err)
	}

	store.writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("simulated atomic replace failure")
	}
	err = store.RecordApprovedPairing(context.Background(), PairingApprovedRecord{
		Platform:   "telegram",
		UserID:     "new-user",
		ApprovedAt: now.Add(time.Minute),
	})
	if err == nil {
		t.Fatal("RecordApprovedPairing() error = nil, want simulated write failure")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pairing state after failed write: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("pairing state changed after failed write\nbefore: %s\nafter: %s", before, after)
	}

	store.writeFile = atomicWritePairingFile
	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("read pairing status after failed write: %v", err)
	}
	if got := approvedKeys(status.Approved); !reflect.DeepEqual(got, []string{"telegram/old-user"}) {
		t.Fatalf("approved after failed write = %v, want old state only", got)
	}
}

func assertOwnerOnlyMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode(%s) = %o, want 600", path, got)
	}
}

func assertEmptyPairingStatus(t *testing.T, status PairingStatus, reason PairingDegradedReason) {
	t.Helper()
	if len(status.Pending) != 0 || len(status.Approved) != 0 || len(status.Platforms) != 0 {
		t.Fatalf("status = %+v, want deterministic empty read model", status)
	}
	if got := status.Kind; got != pairingStatusKind {
		t.Fatalf("Kind = %q, want %q", got, pairingStatusKind)
	}
	if got := status.Version; got != 1 {
		t.Fatalf("Version = %d, want 1", got)
	}
	if len(status.Degraded) != 1 {
		t.Fatalf("degraded evidence = %+v, want one %q entry", status.Degraded, reason)
	}
	if status.Degraded[0].Reason != reason {
		t.Fatalf("degraded reason = %q, want %q", status.Degraded[0].Reason, reason)
	}
	if status.Degraded[0].Path == "" {
		t.Fatalf("degraded path is empty")
	}
}

func pendingKeys(records []PairingPendingRecord) []string {
	keys := make([]string, 0, len(records))
	for _, record := range records {
		keys = append(keys, record.Platform+"/"+record.UserID+"/"+record.Code+"/"+formatInt64(record.AgeSeconds))
	}
	return keys
}

func approvedKeys(records []PairingApprovedRecord) []string {
	keys := make([]string, 0, len(records))
	for _, record := range records {
		keys = append(keys, record.Platform+"/"+record.UserID)
	}
	return keys
}

func platformKeys(records []PairingPlatformStatus) []string {
	keys := make([]string, 0, len(records))
	for _, record := range records {
		keys = append(keys, record.Platform+"/"+string(record.State)+"/"+formatInt(record.PendingCount)+"/"+formatInt(record.ApprovedCount))
	}
	return keys
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

func formatInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}
