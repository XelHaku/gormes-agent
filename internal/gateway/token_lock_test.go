package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
)

func TestTokenScopedGatewayLockPathUsesXDGStateHash(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	credential := "123456:ABC-raw-token"
	store := newTokenLockTestStore(t, config.GatewayLockDir(), 1001, 501, nil)

	lock, evidence, err := store.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: credential,
	})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	hash := TokenCredentialHash(credential)
	if len(hash) != 64 {
		t.Fatalf("TokenCredentialHash length = %d, want full sha256 hex", len(hash))
	}
	wantPath := filepath.Join(stateRoot, "gormes", "gateway-locks", "telegram-"+hash+".lock")
	if lock.Path() != wantPath {
		t.Fatalf("lock path = %q, want %q", lock.Path(), wantPath)
	}
	if evidence.CredentialHash != hash || evidence.Platform != "telegram" {
		t.Fatalf("evidence = %+v, want platform telegram and credential hash %s", evidence, hash)
	}
	raw, err := os.ReadFile(lock.Path())
	if err != nil {
		t.Fatalf("read lock record: %v", err)
	}
	for _, leak := range []string{credential, "123456", "ABC-raw-token"} {
		if strings.Contains(lock.Path(), leak) {
			t.Fatalf("lock path %q leaks credential material %q", lock.Path(), leak)
		}
		if strings.Contains(string(raw), leak) {
			t.Fatalf("lock record leaks credential material %q:\n%s", leak, raw)
		}
		if strings.Contains(evidence.Message, leak) {
			t.Fatalf("lock evidence message leaks credential material %q: %+v", leak, evidence)
		}
	}
}

func TestTokenScopedGatewayLockRejectsSameTokenAndAllowsDifferentScopes(t *testing.T) {
	dir := t.TempDir()
	first := newTokenLockTestStore(t, dir, 1001, 501, nil)
	if _, _, err := first.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "shared-token",
	}); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	second := newTokenLockTestStore(t, dir, 2002, 602, fakeRuntimeProcessTable{
		1001: {startTime: 501, command: "gormes gateway"},
	})
	_, evidence, err := second.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "shared-token",
	})
	if !errors.Is(err, ErrTokenLockHeld) {
		t.Fatalf("same token err = %v, want ErrTokenLockHeld", err)
	}
	if evidence.Status != TokenLockStatusHeld || evidence.OwnerPID != 1001 {
		t.Fatalf("same token evidence = %+v, want held by pid 1001", evidence)
	}

	if _, _, err := second.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "other-token",
	}); err != nil {
		t.Fatalf("different token acquire: %v", err)
	}
	if _, _, err := second.Acquire(context.Background(), TokenLockRequest{
		Platform:   "discord",
		Credential: "shared-token",
	}); err != nil {
		t.Fatalf("different platform acquire: %v", err)
	}

	locks, err := filepath.Glob(filepath.Join(dir, "*.lock"))
	if err != nil {
		t.Fatalf("glob locks: %v", err)
	}
	if len(locks) != 3 {
		t.Fatalf("lock count = %d, want same-token rejection plus two independent locks to leave 3 files: %v", len(locks), locks)
	}
}

func TestTokenScopedGatewayLockClearsStaleStoppedOwnerWithoutDeletingUnrelatedLocks(t *testing.T) {
	dir := t.TempDir()
	owner := newTokenLockTestStore(t, dir, 1001, 501, nil)
	ownerLock, _, err := owner.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "shared-token",
	})
	if err != nil {
		t.Fatalf("owner acquire: %v", err)
	}
	unrelated, _, err := newTokenLockTestStore(t, dir, 1002, 502, nil).Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "unrelated-token",
	})
	if err != nil {
		t.Fatalf("unrelated acquire: %v", err)
	}

	contender := newTokenLockTestStore(t, dir, 2002, 602, fakeRuntimeProcessTable{
		1001: {startTime: 501, command: "gormes gateway", stopped: true},
		1002: {startTime: 502, command: "gormes gateway"},
	})
	newLock, evidence, err := contender.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "shared-token",
	})
	if err != nil {
		t.Fatalf("contender acquire after stale owner: %v", err)
	}
	if evidence.Status != TokenLockStatusStaleCleared {
		t.Fatalf("stale evidence status = %q, want %q", evidence.Status, TokenLockStatusStaleCleared)
	}
	if evidence.ProcessValidation.Status != RuntimeProcessValidationStopped {
		t.Fatalf("stale validation = %+v, want stopped process evidence", evidence.ProcessValidation)
	}
	if newLock.Path() != ownerLock.Path() {
		t.Fatalf("new lock path = %q, want reused scoped path %q", newLock.Path(), ownerLock.Path())
	}
	if _, err := os.Stat(unrelated.Path()); err != nil {
		t.Fatalf("unrelated lock was removed: %v", err)
	}
	record := readTokenLockRecordFixture(t, ownerLock.Path())
	if record.PID != 2002 {
		t.Fatalf("reacquired lock pid = %d, want contender pid 2002", record.PID)
	}
}

func TestTokenScopedGatewayLockCredentialHashMismatchIsReportedWithoutDeletingFile(t *testing.T) {
	dir := t.TempDir()
	credential := "shared-token"
	store := newTokenLockTestStore(t, dir, 2002, 602, fakeRuntimeProcessTable{
		1001: {startTime: 501, command: "gormes gateway", stopped: true},
	})
	path := store.LockPath("telegram", credential)
	writeTokenLockJSONFixture(t, path, map[string]any{
		"kind":            "gormes-gateway-token-lock",
		"platform":        "telegram",
		"credential_hash": "not-" + TokenCredentialHash(credential),
		"pid":             1001,
		"start_time":      501,
		"updated_at":      "2026-04-25T17:00:00Z",
	})

	_, evidence, err := store.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: credential,
	})
	if !errors.Is(err, ErrTokenLockCredentialHashMismatch) {
		t.Fatalf("Acquire err = %v, want ErrTokenLockCredentialHashMismatch", err)
	}
	if evidence.Status != TokenLockStatusCredentialHashMismatch {
		t.Fatalf("evidence status = %q, want credential-hash-mismatch", evidence.Status)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mismatched lock: %v", err)
	}
	if !strings.Contains(string(raw), "not-"+TokenCredentialHash(credential)) {
		t.Fatalf("mismatched lock was overwritten or deleted:\n%s", raw)
	}
}

func TestTokenScopedGatewayLockReleaseRemovesOnlyCurrentOwnerAndReportsReleaseFailures(t *testing.T) {
	dir := t.TempDir()
	current := newTokenLockTestStore(t, dir, 3003, 703, nil)
	currentLock, _, err := current.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "current-token",
	})
	if err != nil {
		t.Fatalf("current acquire: %v", err)
	}
	otherLock, _, err := newTokenLockTestStore(t, dir, 4004, 804, nil).Acquire(context.Background(), TokenLockRequest{
		Platform:   "discord",
		Credential: "other-token",
	})
	if err != nil {
		t.Fatalf("other acquire: %v", err)
	}

	evidence, err := currentLock.Release(context.Background())
	if err != nil {
		t.Fatalf("release current lock: %v", err)
	}
	if evidence.Status != TokenLockStatusReleased {
		t.Fatalf("release evidence = %+v, want released", evidence)
	}
	if _, err := os.Stat(currentLock.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current lock stat err = %v, want removed", err)
	}
	if _, err := os.Stat(otherLock.Path()); err != nil {
		t.Fatalf("other lock was removed: %v", err)
	}

	failingLock, _, err := current.Acquire(context.Background(), TokenLockRequest{
		Platform:   "telegram",
		Credential: "failing-release-token",
	})
	if err != nil {
		t.Fatalf("failing lock acquire: %v", err)
	}
	current.removeFile = func(string) error { return errors.New("unlink denied") }
	evidence, err = failingLock.Release(context.Background())
	if !errors.Is(err, ErrTokenLockReleaseFailed) {
		t.Fatalf("release failure err = %v, want ErrTokenLockReleaseFailed", err)
	}
	if evidence.Status != TokenLockStatusReleaseFailed || !strings.Contains(evidence.Message, "unlink denied") {
		t.Fatalf("release failure evidence = %+v, want release-failed with unlink evidence", evidence)
	}

	statusStore := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	if err := statusStore.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		GatewayState: GatewayStateStartupFailed,
		ExitReason:   "original gateway exit",
	}); err != nil {
		t.Fatalf("write original exit: %v", err)
	}
	if err := statusStore.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		TokenLockEvidence: &evidence,
	}); err != nil {
		t.Fatalf("write lock evidence: %v", err)
	}
	status, err := statusStore.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status.ExitReason != "original gateway exit" {
		t.Fatalf("ExitReason = %q, want original gateway exit preserved", status.ExitReason)
	}
	if len(status.TokenLocks) != 1 || status.TokenLocks[0].Status != TokenLockStatusReleaseFailed {
		t.Fatalf("TokenLocks = %+v, want release-failed evidence", status.TokenLocks)
	}
}

func newTokenLockTestStore(t *testing.T, dir string, pid int, startTime int64, processes fakeRuntimeProcessTable) *TokenLockStore {
	t.Helper()
	store := NewTokenLockStore(dir)
	store.now = func() time.Time { return time.Date(2026, 4, 25, 17, 0, 0, 0, time.UTC) }
	store.pid = func() int { return pid }
	store.startTime = func(got int) (int64, bool) {
		if got != pid {
			return 0, false
		}
		return startTime, true
	}
	store.argv = func() []string { return []string{"gormes", "gateway"} }
	store.processes = processes
	return store
}

type tokenLockRecordFixture struct {
	PID int `json:"pid"`
}

func readTokenLockRecordFixture(t *testing.T, path string) tokenLockRecordFixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock fixture: %v", err)
	}
	var record tokenLockRecordFixture
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode lock fixture: %v", err)
	}
	return record
}

func writeTokenLockJSONFixture(t *testing.T, path string, record map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create lock fixture dir: %v", err)
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("encode lock fixture: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("write lock fixture: %v", err)
	}
}
