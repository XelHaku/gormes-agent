package gateway

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeProbe is an injectable ProcessProbe used to drive the stale-PID and
// process-identity tests without depending on host-specific /proc state.
type fakeProbe struct {
	alive     bool
	startTime int64
	err       error
	lastPID   int
}

func (p *fakeProbe) IsRunning(pid int) (bool, int64, error) {
	p.lastPID = pid
	return p.alive, p.startTime, p.err
}

func TestRuntimeState_JSONRoundtripPreservesIdentityAndChannels(t *testing.T) {
	want := RuntimeState{
		PID:       4242,
		StartTime: 1_700_000_000,
		Channels: []ChannelStatus{
			{
				Platform:    "telegram",
				Phase:       LifecyclePhaseRunning,
				LastUpdated: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"pid":4242`) {
		t.Errorf("JSON missing pid field: %s", raw)
	}
	if !strings.Contains(string(raw), `"start_time":1700000000`) {
		t.Errorf("JSON missing start_time field: %s", raw)
	}

	var got RuntimeState
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.PID != want.PID || got.StartTime != want.StartTime {
		t.Errorf("identity = (%d,%d), want (%d,%d)", got.PID, got.StartTime, want.PID, want.StartTime)
	}
	if len(got.Channels) != 1 || got.Channels[0].Platform != "telegram" {
		t.Errorf("channels = %+v, want telegram snapshot", got.Channels)
	}
	if got.Channels[0].Phase != LifecyclePhaseRunning {
		t.Errorf("channel phase = %q, want %q", got.Channels[0].Phase, LifecyclePhaseRunning)
	}
}

func TestSaveRuntimeState_AtomicWriteAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway_state.json")

	state := RuntimeState{PID: 42, StartTime: 99}
	if err := SaveRuntimeState(path, state); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "gateway_state.json" {
			continue
		}
		t.Errorf("residual file after atomic save: %q", name)
	}

	got, ok, err := LoadRuntimeState(path)
	if err != nil {
		t.Fatalf("LoadRuntimeState: %v", err)
	}
	if !ok {
		t.Fatalf("LoadRuntimeState ok=false, want true")
	}
	if got.PID != 42 || got.StartTime != 99 {
		t.Errorf("state = %+v, want PID=42 StartTime=99", got)
	}
}

func TestSaveRuntimeState_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "gateway_state.json")

	if err := SaveRuntimeState(path, RuntimeState{PID: 1}); err != nil {
		t.Fatalf("SaveRuntimeState with missing parent: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file missing after SaveRuntimeState: %v", err)
	}
}

func TestLoadRuntimeState_MissingFileReportsNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")

	got, ok, err := LoadRuntimeState(path)
	if err != nil {
		t.Fatalf("LoadRuntimeState: %v", err)
	}
	if ok {
		t.Errorf("ok = true on missing file, want false (state=%+v)", got)
	}
}

func TestLoadRuntimeState_CorruptJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway_state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, _, err := LoadRuntimeState(path); err == nil {
		t.Errorf("LoadRuntimeState corrupt file err = nil, want non-nil")
	}
}

func TestIsStale_ProcessNotRunning(t *testing.T) {
	state := RuntimeState{PID: 1234, StartTime: 100}
	probe := &fakeProbe{alive: false}

	stale, err := state.IsStale(probe)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Errorf("IsStale=false when probe reports dead process, want true")
	}
	if probe.lastPID != 1234 {
		t.Errorf("probe called with pid=%d, want 1234", probe.lastPID)
	}
}

func TestIsStale_PIDReusedDifferentStartTime(t *testing.T) {
	state := RuntimeState{PID: 1234, StartTime: 100}
	probe := &fakeProbe{alive: true, startTime: 200}

	stale, err := state.IsStale(probe)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Errorf("IsStale=false when start_time differs (PID reuse), want true")
	}
}

func TestIsStale_AliveMatchingProcessIsFresh(t *testing.T) {
	state := RuntimeState{PID: 1234, StartTime: 100}
	probe := &fakeProbe{alive: true, startTime: 100}

	stale, err := state.IsStale(probe)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Errorf("IsStale=true when process alive+matching, want false")
	}
}

func TestIsStale_MissingStartTimeSkipsIdentityCheck(t *testing.T) {
	// When either side is zero we must not treat the PID as reused —
	// platforms without /proc can't reliably report start-time.
	state := RuntimeState{PID: 1234, StartTime: 0}
	probe := &fakeProbe{alive: true, startTime: 999}

	stale, err := state.IsStale(probe)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Errorf("IsStale=true when state.StartTime is zero, want false")
	}
}

func TestIsStale_InvalidPIDIsAlwaysStale(t *testing.T) {
	state := RuntimeState{PID: 0}
	probe := &fakeProbe{alive: true, startTime: 100}

	stale, err := state.IsStale(probe)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Errorf("IsStale=false for invalid pid=0, want true")
	}
}

func TestIsStale_ProbeErrorSurfaces(t *testing.T) {
	state := RuntimeState{PID: 1234, StartTime: 100}
	boom := errors.New("probe exploded")
	probe := &fakeProbe{alive: false, err: boom}

	if _, err := state.IsStale(probe); !errors.Is(err, boom) {
		t.Errorf("IsStale err = %v, want wrapping %v", err, boom)
	}
}

func TestCleanStaleRuntimeState_RemovesStaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway_state.json")
	if err := SaveRuntimeState(path, RuntimeState{PID: 1, StartTime: 10}); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}

	removed, err := CleanStaleRuntimeState(path, &fakeProbe{alive: false})
	if err != nil {
		t.Fatalf("CleanStaleRuntimeState: %v", err)
	}
	if !removed {
		t.Fatalf("removed=false when file was stale, want true")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file still present after clean: %v", err)
	}
}

func TestCleanStaleRuntimeState_KeepsFileWhenOwnerAlive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway_state.json")
	if err := SaveRuntimeState(path, RuntimeState{PID: 1, StartTime: 10}); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}

	removed, err := CleanStaleRuntimeState(path, &fakeProbe{alive: true, startTime: 10})
	if err != nil {
		t.Fatalf("CleanStaleRuntimeState: %v", err)
	}
	if removed {
		t.Errorf("removed=true when owner process still alive, want false")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file unexpectedly removed: %v", err)
	}
}

func TestCleanStaleRuntimeState_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	removed, err := CleanStaleRuntimeState(path, &fakeProbe{alive: false})
	if err != nil {
		t.Fatalf("CleanStaleRuntimeState: %v", err)
	}
	if removed {
		t.Errorf("removed=true when file missing, want false")
	}
}

func TestDefaultProcessProbe_ReportsCurrentProcessAlive(t *testing.T) {
	probe := DefaultProcessProbe()
	alive, _, err := probe.IsRunning(os.Getpid())
	if err != nil {
		t.Fatalf("IsRunning(self): %v", err)
	}
	if !alive {
		t.Fatal("default probe reported current process as dead")
	}
}

func TestDefaultProcessProbe_ReportsNonexistentPIDDead(t *testing.T) {
	// PIDs in the reserved range should never map to a live process in
	// test environments. Using math.MaxInt32 stays inside the int domain
	// on both 32- and 64-bit hosts.
	probe := DefaultProcessProbe()
	alive, _, err := probe.IsRunning(2_147_483_000)
	if err != nil {
		t.Fatalf("IsRunning(huge): %v", err)
	}
	if alive {
		t.Fatal("default probe reported fictional PID as alive")
	}
}
