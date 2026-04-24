package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newFakeProbe returns a LockProcessProbe whose state is controlled by the
// caller through the returned maps. Any PID not present in tokens is
// treated as a stopped process.
func newFakeProbe(tokens map[int]string) LockProcessProbe {
	return func(pid int) (string, bool, error) {
		tok, ok := tokens[pid]
		if !ok {
			return "", false, nil
		}
		return tok, true, nil
	}
}

func newTestLockStore(t *testing.T, pid int, token string, probe LockProcessProbe) (*LockStore, string) {
	t.Helper()
	dir := t.TempDir()
	return NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          pid,
		StartToken:   token,
		Now:          func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC) },
		ProcessProbe: probe,
	}), dir
}

func TestHashCredential_Deterministic_And_HidesRawToken(t *testing.T) {
	h1 := HashCredential("telegram", "supersecret-bot-token")
	h2 := HashCredential("telegram", "supersecret-bot-token")
	h3 := HashCredential("telegram", "different-token")
	h4 := HashCredential("discord", "supersecret-bot-token")

	if h1 == "" {
		t.Fatalf("HashCredential returned empty string")
	}
	if h1 != h2 {
		t.Errorf("HashCredential not deterministic: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("HashCredential did not change when credential changed: %q", h1)
	}
	if h1 == h4 {
		t.Errorf("HashCredential did not change when platform changed: %q", h1)
	}
	if strings.Contains(h1, "supersecret") {
		t.Errorf("HashCredential leaked raw token: %q", h1)
	}
}

func TestLockStore_TryAcquire_NewLock_CreatesFile(t *testing.T) {
	probe := newFakeProbe(map[int]string{4242: "start-token-a"})
	store, dir := newTestLockStore(t, 4242, "start-token-a", probe)

	lock, ok, err := store.TryAcquire("telegram", "secret-token")
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatalf("TryAcquire: acquired=false on fresh directory")
	}
	if lock == nil {
		t.Fatalf("TryAcquire: nil lock on success")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("lock directory has %d entries, want 1", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "telegram-") {
		t.Errorf("lock filename %q missing platform prefix", name)
	}
	if strings.Contains(name, "secret-token") {
		t.Errorf("lock filename leaks raw credential: %q", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("lock file JSON: %v", err)
	}
	if decoded["platform"] != "telegram" {
		t.Errorf("lock file platform=%v, want telegram", decoded["platform"])
	}
	if pid, _ := decoded["pid"].(float64); int(pid) != 4242 {
		t.Errorf("lock file pid=%v, want 4242", decoded["pid"])
	}
	if decoded["start_token"] != "start-token-a" {
		t.Errorf("lock file start_token=%v, want start-token-a", decoded["start_token"])
	}

	if err := lock.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}
}

func TestLockStore_TryAcquire_ExistingLiveOwner_Refuses(t *testing.T) {
	probe := newFakeProbe(map[int]string{1111: "tok-1", 2222: "tok-2"})
	dir := t.TempDir()

	// First acquirer: PID 1111.
	storeA := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          1111,
		StartToken:   "tok-1",
		ProcessProbe: probe,
	})
	if _, ok, err := storeA.TryAcquire("telegram", "shared-token"); err != nil || !ok {
		t.Fatalf("first TryAcquire ok=%v err=%v", ok, err)
	}

	// Second acquirer: different PID 2222, same credential.
	storeB := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          2222,
		StartToken:   "tok-2",
		ProcessProbe: probe,
	})
	lock, ok, err := storeB.TryAcquire("telegram", "shared-token")
	if err != nil {
		t.Fatalf("second TryAcquire err=%v", err)
	}
	if ok {
		t.Fatalf("second TryAcquire should have been refused; got ok=true, lock=%+v", lock)
	}

	// First owner's file must be preserved untouched.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("lock directory has %d entries after refused acquire, want 1", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if pid, _ := decoded["pid"].(float64); int(pid) != 1111 {
		t.Errorf("lock file pid clobbered by refused acquire: got %v, want 1111", decoded["pid"])
	}
}

func TestLockStore_TryAcquire_StalePID_StoppedProcess_TakesOver(t *testing.T) {
	dir := t.TempDir()

	// First: PID 3333 acquires, pretending it's alive.
	aliveProbe := newFakeProbe(map[int]string{3333: "tok-3"})
	storeA := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          3333,
		StartToken:   "tok-3",
		ProcessProbe: aliveProbe,
	})
	if _, ok, err := storeA.TryAcquire("slack", "slack-token-z"); err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}

	// Now PID 3333 is gone (not in probe map), and PID 4444 tries.
	goneProbe := newFakeProbe(map[int]string{4444: "tok-4"})
	storeB := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          4444,
		StartToken:   "tok-4",
		ProcessProbe: goneProbe,
	})
	lock, ok, err := storeB.TryAcquire("slack", "slack-token-z")
	if err != nil {
		t.Fatalf("takeover TryAcquire err=%v", err)
	}
	if !ok || lock == nil {
		t.Fatalf("takeover TryAcquire ok=%v lock=%v, want acquired", ok, lock)
	}

	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if pid, _ := decoded["pid"].(float64); int(pid) != 4444 {
		t.Errorf("lock file pid after takeover = %v, want 4444", decoded["pid"])
	}
	if decoded["start_token"] != "tok-4" {
		t.Errorf("lock file start_token after takeover = %v, want tok-4", decoded["start_token"])
	}
}

func TestLockStore_TryAcquire_StartTokenMismatch_TakesOver(t *testing.T) {
	dir := t.TempDir()

	// First acquirer records start token "tok-old".
	storeA := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          5555,
		StartToken:   "tok-old",
		ProcessProbe: newFakeProbe(map[int]string{5555: "tok-old"}),
	})
	if _, ok, err := storeA.TryAcquire("discord", "dt-xyz"); err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}

	// Now PID 5555 is reused but with a different start token; the stored
	// start_token no longer matches the live process, so the lock is stale.
	reusedProbe := newFakeProbe(map[int]string{5555: "tok-new"})
	storeB := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          6666,
		StartToken:   "tok-6",
		ProcessProbe: reusedProbe,
	})
	lock, ok, err := storeB.TryAcquire("discord", "dt-xyz")
	if err != nil {
		t.Fatalf("takeover TryAcquire err=%v", err)
	}
	if !ok || lock == nil {
		t.Fatalf("takeover TryAcquire ok=%v lock=%v, want acquired", ok, lock)
	}

	entries, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if pid, _ := decoded["pid"].(float64); int(pid) != 6666 {
		t.Errorf("lock file pid after token-mismatch takeover = %v, want 6666", decoded["pid"])
	}
}

func TestLockStore_TryAcquire_DifferentCredentials_DoNotCollide(t *testing.T) {
	probe := newFakeProbe(map[int]string{7777: "tok-7"})
	store, dir := newTestLockStore(t, 7777, "tok-7", probe)

	l1, ok1, err1 := store.TryAcquire("telegram", "bot-token-A")
	l2, ok2, err2 := store.TryAcquire("telegram", "bot-token-B")
	if err1 != nil || !ok1 || l1 == nil {
		t.Fatalf("first acquire ok=%v err=%v", ok1, err1)
	}
	if err2 != nil || !ok2 || l2 == nil {
		t.Fatalf("second acquire ok=%v err=%v", ok2, err2)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("want two lock files for distinct credentials, got %d", len(entries))
	}
}

func TestLockStore_Release_RemovesLockFile(t *testing.T) {
	probe := newFakeProbe(map[int]string{8888: "tok-8"})
	store, dir := newTestLockStore(t, 8888, "tok-8", probe)

	lock, ok, err := store.TryAcquire("slack", "slk-token")
	if err != nil || !ok {
		t.Fatalf("acquire ok=%v err=%v", ok, err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("directory should be empty after Release, got %d entries", len(entries))
	}

	// Idempotent: calling Release again is a no-op.
	if err := lock.Release(); err != nil {
		t.Errorf("second Release returned error: %v", err)
	}
}

func TestLockStore_Release_DoesNotClobberForeignOwner(t *testing.T) {
	dir := t.TempDir()

	// PID 9001 acquires.
	storeA := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          9001,
		StartToken:   "tok-a",
		ProcessProbe: newFakeProbe(map[int]string{9001: "tok-a"}),
	})
	lockA, ok, err := storeA.TryAcquire("telegram", "shared-cred")
	if err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}

	// PID 9001 "dies", PID 9002 takes over the lock.
	storeB := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          9002,
		StartToken:   "tok-b",
		ProcessProbe: newFakeProbe(map[int]string{9002: "tok-b"}),
	})
	if _, ok, err := storeB.TryAcquire("telegram", "shared-cred"); err != nil || !ok {
		t.Fatalf("takeover acquire ok=%v err=%v", ok, err)
	}

	// Now the original (zombie) lock handle from storeA tries to Release.
	// It must NOT delete the new owner's file.
	if err := lockA.Release(); err != nil {
		t.Fatalf("zombie Release error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("lock directory wiped by zombie Release: got %d entries, want 1", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if pid, _ := decoded["pid"].(float64); int(pid) != 9002 {
		t.Errorf("foreign lock pid clobbered: got %v, want 9002", decoded["pid"])
	}
}

func TestLockStore_TryAcquire_CorruptLockFile_OverwritesIt(t *testing.T) {
	dir := t.TempDir()

	// Compute filename and drop garbage bytes.
	path := filepath.Join(dir, "telegram-"+HashCredential("telegram", "cred-1")+".lock")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt lock: %v", err)
	}

	store := NewLockStore(LockStoreConfig{
		BaseDir:      dir,
		PID:          1234,
		StartToken:   "tok",
		ProcessProbe: newFakeProbe(map[int]string{1234: "tok"}),
	})
	lock, ok, err := store.TryAcquire("telegram", "cred-1")
	if err != nil {
		t.Fatalf("TryAcquire on corrupt file: %v", err)
	}
	if !ok || lock == nil {
		t.Fatalf("TryAcquire on corrupt file: want acquired, got ok=%v", ok)
	}
	data, _ := os.ReadFile(path)
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("lock file not rewritten as valid JSON: %v", err)
	}
	if pid, _ := decoded["pid"].(float64); int(pid) != 1234 {
		t.Errorf("pid after overwrite = %v, want 1234", decoded["pid"])
	}
}

func TestLockStore_TryAcquire_EmptyInputs_ReturnError(t *testing.T) {
	store, _ := newTestLockStore(t, 1, "tok", newFakeProbe(map[int]string{1: "tok"}))
	if _, _, err := store.TryAcquire("", "cred"); err == nil {
		t.Errorf("TryAcquire with empty platform: want error")
	}
	if _, _, err := store.TryAcquire("telegram", ""); err == nil {
		t.Errorf("TryAcquire with empty credential: want error")
	}
}

func TestDefaultLockStore_UsesLiveProcessProbe(t *testing.T) {
	// Default probe should see the currently running test process as alive.
	probe := defaultLockProcessProbe
	tok, running, err := probe(os.Getpid())
	if err != nil {
		t.Fatalf("defaultLockProcessProbe(self) err=%v", err)
	}
	if !running {
		t.Fatalf("defaultLockProcessProbe(self) running=false; want true")
	}
	if tok == "" {
		t.Errorf("defaultLockProcessProbe(self) returned empty start token")
	}

	// A very high PID that almost certainly does not exist.
	_, running, err = probe(0x7fffffff)
	if err != nil {
		t.Fatalf("defaultLockProcessProbe(fake pid) err=%v", err)
	}
	if running {
		t.Errorf("defaultLockProcessProbe(fake pid) running=true; want false")
	}
}

func TestScopedLockDir_UsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/gormes-scoped-lock-xdg-test")
	got := ScopedLockDir()
	want := "/tmp/gormes-scoped-lock-xdg-test/gormes/gateway/locks"
	if got != want {
		t.Errorf("ScopedLockDir() = %q, want %q", got, want)
	}
}
