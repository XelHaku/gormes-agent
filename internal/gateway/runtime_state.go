package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// RuntimeState is the on-disk read model mirroring upstream
// gateway_state.json. It captures the owning process identity (PID and a
// platform-specific StartTime used to detect PID reuse) alongside a
// snapshot of per-channel lifecycle state. Channel wiring lands in a
// later slice; this file only freezes the persistence + identity seam.
type RuntimeState struct {
	PID       int             `json:"pid"`
	StartTime int64           `json:"start_time,omitempty"`
	Channels  []ChannelStatus `json:"channels,omitempty"`
}

// ProcessProbe reports whether a PID is currently running and (when the
// host exposes it) a stable StartTime fingerprint for PID-reuse
// detection. Implementations must return alive=false, startTime=0, err=nil
// for missing processes; transport-level errors go through err.
type ProcessProbe interface {
	IsRunning(pid int) (alive bool, startTime int64, err error)
}

// DefaultProcessProbe returns a platform-best-effort probe. On Linux it
// uses signal-0 to detect live processes and /proc/<pid>/stat field 22
// (starttime in clock ticks since boot) to derive an identity
// fingerprint. On other OSes it falls back to signal-0 only and returns
// startTime=0, letting callers relax identity checking.
func DefaultProcessProbe() ProcessProbe { return defaultProbe{} }

type defaultProbe struct{}

func (defaultProbe) IsRunning(pid int) (bool, int64, error) {
	if pid <= 0 {
		return false, 0, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		switch {
		case errors.Is(err, os.ErrProcessDone), errors.Is(err, syscall.ESRCH):
			return false, 0, nil
		case errors.Is(err, syscall.EPERM):
			// Process exists but we lack permission to signal it.
		default:
			return false, 0, nil
		}
	}
	return true, readProcStartTime(pid), nil
}

// readProcStartTime returns the /proc/<pid>/stat starttime fingerprint on
// Linux and zero everywhere else. A zero result tells callers to skip
// strict identity matching for this platform.
func readProcStartTime(pid int) int64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	// /proc/<pid>/stat prefixes `pid (comm) state …`. The comm field can
	// contain whitespace, so split only the suffix after the final ')'.
	s := string(data)
	commEnd := strings.LastIndexByte(s, ')')
	if commEnd < 0 || commEnd+2 >= len(s) {
		return 0
	}
	tokens := strings.Fields(s[commEnd+2:])
	// After the close-paren, tokens[0] is field 3 (state). starttime is
	// stat field 22, i.e. tokens[19].
	if len(tokens) < 20 {
		return 0
	}
	v, err := strconv.ParseInt(tokens[19], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// SaveRuntimeState atomically writes state to path via temp-file + rename.
// The parent directory is created with 0o755 if missing.
func SaveRuntimeState(path string, state RuntimeState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime state: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir runtime state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadRuntimeState reads the state file at path. It returns
// (zero, false, nil) when the file is absent, (state, true, nil) on
// success, and a non-nil error when the file exists but is unreadable or
// corrupt.
func LoadRuntimeState(path string) (RuntimeState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeState{}, false, nil
		}
		return RuntimeState{}, false, err
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeState{}, false, fmt.Errorf("parse runtime state: %w", err)
	}
	return state, true, nil
}

// IsStale reports whether the owning process for this state no longer
// exists, or whether its identity has drifted (PID reuse). Both sides
// must hold a non-zero StartTime for the identity check to fire — hosts
// that cannot report a StartTime fall back to "alive is fresh".
func (s RuntimeState) IsStale(probe ProcessProbe) (bool, error) {
	if probe == nil {
		probe = DefaultProcessProbe()
	}
	if s.PID <= 0 {
		return true, nil
	}
	alive, startTime, err := probe.IsRunning(s.PID)
	if err != nil {
		return false, err
	}
	if !alive {
		return true, nil
	}
	if s.StartTime != 0 && startTime != 0 && s.StartTime != startTime {
		return true, nil
	}
	return false, nil
}

// CleanStaleRuntimeState removes path iff the owner reported by the file
// is stale. Returns (removed, err); a missing file is a no-op.
func CleanStaleRuntimeState(path string, probe ProcessProbe) (bool, error) {
	state, ok, err := LoadRuntimeState(path)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	stale, err := state.IsStale(probe)
	if err != nil {
		return false, err
	}
	if !stale {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}
