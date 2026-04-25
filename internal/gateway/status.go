package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const runtimeStatusKind = "gormes-gateway"

// GatewayState is the process-level lifecycle state persisted in
// gateway_state.json for operator readouts.
type GatewayState string

const (
	GatewayStateStarting      GatewayState = "starting"
	GatewayStateRunning       GatewayState = "running"
	GatewayStateDraining      GatewayState = "draining"
	GatewayStateStopped       GatewayState = "stopped"
	GatewayStateStartupFailed GatewayState = "startup_failed"
)

// PlatformState is the per-channel lifecycle state persisted alongside the
// process-level gateway state.
type PlatformState string

const (
	PlatformStateStarting PlatformState = "starting"
	PlatformStateRunning  PlatformState = "running"
	PlatformStateStopped  PlatformState = "stopped"
	PlatformStateFailed   PlatformState = "failed"
)

// RuntimeStatus is the shared gateway status read model.
type RuntimeStatus struct {
	Kind              string                           `json:"kind"`
	PID               int                              `json:"pid"`
	StartTime         int64                            `json:"start_time,omitempty"`
	Generation        uint64                           `json:"generation"`
	Command           string                           `json:"command,omitempty"`
	Argv              []string                         `json:"argv,omitempty"`
	ProcessValidation RuntimeProcessValidation         `json:"process_validation,omitempty"`
	GatewayState      GatewayState                     `json:"gateway_state"`
	ExitReason        string                           `json:"exit_reason"`
	ActiveAgents      int                              `json:"active_agents"`
	Platforms         map[string]PlatformRuntimeStatus `json:"platforms"`
	Proxy             ProxyRuntimeStatus               `json:"proxy"`
	TokenLocks        []TokenLockEvidence              `json:"token_locks,omitempty"`
	DrainTimeouts     []RuntimeDrainTimeoutEvidence    `json:"drain_timeouts,omitempty"`
	ResumePending     []RuntimeResumePendingEvidence   `json:"resume_pending,omitempty"`
	NonResumable      []RuntimeNonResumableEvidence    `json:"non_resumable,omitempty"`
	ExpiryFinalized   []RuntimeExpiryFinalizedEvidence `json:"expiry_finalized,omitempty"`
	UpdatedAt         string                           `json:"updated_at"`
}

// RuntimeProcessValidationStatus classifies how much trust callers can place
// in the PID identity stored next to gateway_state.json.
type RuntimeProcessValidationStatus string

const (
	RuntimeProcessValidationMissingState     RuntimeProcessValidationStatus = "missing_state"
	RuntimeProcessValidationMissingPIDFile   RuntimeProcessValidationStatus = "missing_pid_file"
	RuntimeProcessValidationStalePID         RuntimeProcessValidationStatus = "stale_pid"
	RuntimeProcessValidationPIDReused        RuntimeProcessValidationStatus = "pid_reused"
	RuntimeProcessValidationStopped          RuntimeProcessValidationStatus = "stopped_process"
	RuntimeProcessValidationPermissionDenied RuntimeProcessValidationStatus = "permission_denied"
	RuntimeProcessValidationLive             RuntimeProcessValidationStatus = "live"
)

// RuntimeProcessValidation is read-only evidence produced when a runtime
// status snapshot is checked against process identity evidence.
type RuntimeProcessValidation struct {
	Status            RuntimeProcessValidationStatus `json:"status,omitempty"`
	Live              bool                           `json:"live"`
	Message           string                         `json:"message,omitempty"`
	PID               int                            `json:"pid,omitempty"`
	ExpectedStartTime int64                          `json:"expected_start_time,omitempty"`
	ActualStartTime   int64                          `json:"actual_start_time,omitempty"`
	Command           string                         `json:"command,omitempty"`
	CheckedAt         string                         `json:"checked_at,omitempty"`
}

// PlatformRuntimeStatus is one platform/channel's status entry inside the
// shared runtime status model.
type PlatformRuntimeStatus struct {
	State        PlatformState `json:"state"`
	ErrorMessage string        `json:"error_message"`
	UpdatedAt    string        `json:"updated_at"`
}

// ProxyRuntimeStatus reports gateway proxy mode health for operator readouts.
type ProxyRuntimeStatus struct {
	State        string `json:"state"`
	URL          string `json:"url,omitempty"`
	ErrorMessage string `json:"error_message"`
	UpdatedAt    string `json:"updated_at"`
}

type RuntimeResumePendingEvidence struct {
	SessionKey string `json:"session_key,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Source     string `json:"source,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	MarkedAt   string `json:"marked_at,omitempty"`
}

type RuntimeDrainTimeoutEvidence struct {
	SessionKey   string `json:"session_key,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	Source       string `json:"source,omitempty"`
	ChatID       string `json:"chat_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
	TimeoutAt    string `json:"timeout_at,omitempty"`
	ActiveAgents int    `json:"active_agents,omitempty"`
}

type RuntimeNonResumableEvidence struct {
	SessionKey string `json:"session_key,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Source     string `json:"source,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	At         string `json:"at,omitempty"`
}

type RuntimeExpiryFinalizedEvidence struct {
	SessionID             string `json:"session_id,omitempty"`
	Source                string `json:"source,omitempty"`
	ChatID                string `json:"chat_id,omitempty"`
	UserID                string `json:"user_id,omitempty"`
	ExpiryFinalized       bool   `json:"expiry_finalized"`
	MigratedMemoryFlushed bool   `json:"migrated_memory_flushed,omitempty"`
}

// RuntimeStatusUpdate carries a partial update to the shared runtime status.
type RuntimeStatusUpdate struct {
	GatewayState GatewayState
	ExitReason   string
	ActiveAgents *int

	Platform      string
	PlatformState PlatformState
	ErrorMessage  string

	ProxyState        string
	ProxyURL          string
	ProxyErrorMessage string

	DrainTimeoutEvidence    *RuntimeDrainTimeoutEvidence
	ResumePendingEvidence   *RuntimeResumePendingEvidence
	NonResumableEvidence    *RuntimeNonResumableEvidence
	ExpiryFinalizedEvidence *RuntimeExpiryFinalizedEvidence
	TokenLockEvidence       *TokenLockEvidence
}

// RuntimeStatusSnapshot is a read-only view of the runtime status file that
// preserves whether the file was present. RuntimeStatusStore.ReadRuntimeStatus
// synthesizes startup defaults for manager writers; status commands need to
// distinguish that from "no runtime evidence has been written yet".
type RuntimeStatusSnapshot struct {
	Status     RuntimeStatus
	Missing    bool
	Validation RuntimeProcessValidation
}

// RuntimeStatusWriter is the manager-facing seam for lifecycle status writes.
type RuntimeStatusWriter interface {
	UpdateRuntimeStatus(context.Context, RuntimeStatusUpdate) error
}

// RuntimeStatusStore persists the gateway runtime status as atomic JSON.
type RuntimeStatusStore struct {
	path      string
	pidPath   string
	now       func() time.Time
	pid       func() int
	startTime func(int) (int64, bool)
	argv      func() []string
	processes runtimeProcessTable
	mu        sync.Mutex
}

// NewRuntimeStatusStore returns a JSON-backed runtime status store.
func NewRuntimeStatusStore(path string) *RuntimeStatusStore {
	return &RuntimeStatusStore{
		path:      path,
		pidPath:   filepath.Join(filepath.Dir(path), "gateway.pid"),
		now:       func() time.Time { return time.Now().UTC() },
		pid:       os.Getpid,
		startTime: procProcessStartTime,
		argv:      func() []string { return append([]string(nil), os.Args...) },
		processes: procRuntimeProcessTable{},
	}
}

// UpdateRuntimeStatus merges update into the persisted read model and writes it
// atomically.
func (s *RuntimeStatusStore) UpdateRuntimeStatus(ctx context.Context, update RuntimeStatusUpdate) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.readLocked()
	if err != nil {
		return err
	}
	s.merge(&status, update)
	return s.writeLocked(ctx, status)
}

// ReadRuntimeStatus reads the current runtime status model from disk.
func (s *RuntimeStatusStore) ReadRuntimeStatus(ctx context.Context) (RuntimeStatus, error) {
	if s == nil || s.path == "" {
		return RuntimeStatus{}, nil
	}
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

// ReadRuntimeStatusSnapshot reads the current runtime status model from disk
// without synthesizing a startup status when the file is missing or empty.
func (s *RuntimeStatusStore) ReadRuntimeStatusSnapshot(ctx context.Context) (RuntimeStatusSnapshot, error) {
	if s == nil || s.path == "" {
		return RuntimeStatusSnapshot{Missing: true}, nil
	}
	if err := ctx.Err(); err != nil {
		return RuntimeStatusSnapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return RuntimeStatusSnapshot{Missing: true}, nil
	}
	if err != nil {
		return RuntimeStatusSnapshot{}, fmt.Errorf("read runtime status: %w", err)
	}
	if len(raw) == 0 {
		return RuntimeStatusSnapshot{Missing: true}, nil
	}

	var status RuntimeStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return RuntimeStatusSnapshot{}, fmt.Errorf("decode runtime status: %w", err)
	}
	if status.Platforms == nil {
		status.Platforms = map[string]PlatformRuntimeStatus{}
	}
	return RuntimeStatusSnapshot{Status: status}, nil
}

// ReadValidatedRuntimeStatusSnapshot reads runtime status and annotates it with
// PID/start-time validation evidence. When validation proves the persisted
// state is stale, the returned status is cleaned in memory so callers do not
// treat old running channels as live.
func (s *RuntimeStatusStore) ReadValidatedRuntimeStatusSnapshot(ctx context.Context) (RuntimeStatusSnapshot, error) {
	snapshot, err := s.ReadRuntimeStatusSnapshot(ctx)
	if err != nil {
		return RuntimeStatusSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return RuntimeStatusSnapshot{}, err
	}

	validation := s.validateRuntimeProcess(snapshot)
	snapshot.Validation = validation
	snapshot.Status = applyRuntimeProcessValidation(snapshot.Status, validation, snapshot.Missing)
	return snapshot, nil
}

func (s *RuntimeStatusStore) validateRuntimeProcess(snapshot RuntimeStatusSnapshot) RuntimeProcessValidation {
	checkedAt := ""
	if s != nil && s.now != nil {
		checkedAt = s.now().Format(time.RFC3339Nano)
	}
	if snapshot.Missing {
		return RuntimeProcessValidation{
			Status:    RuntimeProcessValidationMissingState,
			Live:      false,
			Message:   "runtime status is missing",
			CheckedAt: checkedAt,
		}
	}

	pidRecord, err := readRuntimeStatusRecord(s.pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeProcessValidation{
				Status:    RuntimeProcessValidationMissingPIDFile,
				Live:      false,
				PID:       snapshot.Status.PID,
				Message:   "runtime PID file is missing",
				CheckedAt: checkedAt,
			}
		}
		return RuntimeProcessValidation{
			Status:    RuntimeProcessValidationMissingPIDFile,
			Live:      false,
			PID:       snapshot.Status.PID,
			Message:   err.Error(),
			CheckedAt: checkedAt,
		}
	}
	if mismatch := runtimePIDRecordMismatch(snapshot.Status, pidRecord); mismatch != "" {
		return RuntimeProcessValidation{
			Status:            RuntimeProcessValidationStalePID,
			Live:              false,
			PID:               snapshot.Status.PID,
			ExpectedStartTime: snapshot.Status.StartTime,
			Command:           snapshot.Status.Command,
			Message:           mismatch,
			CheckedAt:         checkedAt,
		}
	}

	pid := pidRecord.PID
	if pid <= 0 {
		pid = snapshot.Status.PID
	}
	expectedStartTime := pidRecord.StartTime
	if expectedStartTime == 0 {
		expectedStartTime = snapshot.Status.StartTime
	}
	command := pidRecord.Command
	if command == "" {
		command = snapshot.Status.Command
	}
	validation := RuntimeProcessValidation{
		PID:               pid,
		ExpectedStartTime: expectedStartTime,
		Command:           command,
		CheckedAt:         checkedAt,
	}
	if pid <= 0 {
		validation.Status = RuntimeProcessValidationStalePID
		validation.Message = "runtime PID is missing or invalid"
		return validation
	}

	processes := s.processes
	if processes == nil {
		processes = procRuntimeProcessTable{}
	}
	process, err := processes.LookupRuntimeProcess(pid)
	if err != nil {
		validation.Live = false
		switch {
		case errors.Is(err, errRuntimeProcessPermissionDenied):
			validation.Status = RuntimeProcessValidationPermissionDenied
			validation.Message = "process lookup was denied"
		case errors.Is(err, errRuntimeProcessNotFound):
			validation.Status = RuntimeProcessValidationStalePID
			validation.Message = "process is not running"
		default:
			validation.Status = RuntimeProcessValidationStalePID
			validation.Message = err.Error()
		}
		return validation
	}

	validation.ActualStartTime = process.StartTime
	if expectedStartTime != 0 && process.StartTime != 0 && process.StartTime != expectedStartTime {
		validation.Status = RuntimeProcessValidationPIDReused
		validation.Message = "process start time does not match runtime status"
		return validation
	}
	if process.Stopped {
		validation.Status = RuntimeProcessValidationStopped
		validation.Message = "process is stopped"
		return validation
	}

	validation.Status = RuntimeProcessValidationLive
	validation.Live = true
	if validation.Command == "" {
		validation.Command = process.Command
	}
	return validation
}

func runtimePIDRecordMismatch(status RuntimeStatus, pidRecord RuntimeStatus) string {
	if status.Kind != "" && pidRecord.Kind != "" && status.Kind != pidRecord.Kind {
		return "runtime PID record kind does not match status"
	}
	if status.PID > 0 && pidRecord.PID > 0 && status.PID != pidRecord.PID {
		return "runtime PID record pid does not match status"
	}
	if status.StartTime > 0 && pidRecord.StartTime > 0 && status.StartTime != pidRecord.StartTime {
		return "runtime PID record start time does not match status"
	}
	if status.Generation > 0 && pidRecord.Generation > 0 && status.Generation != pidRecord.Generation {
		return "runtime PID record generation does not match status"
	}
	if status.Command != "" && pidRecord.Command != "" && status.Command != pidRecord.Command {
		return "runtime PID record command does not match status"
	}
	return ""
}

func (s *RuntimeStatusStore) merge(status *RuntimeStatus, update RuntimeStatusUpdate) {
	status.Kind = runtimeStatusKind
	status.PID = s.pid()
	if startTime, ok := s.startTime(status.PID); ok {
		status.StartTime = startTime
	} else {
		status.StartTime = 0
	}
	status.Generation++
	status.Argv = append([]string(nil), s.argv()...)
	status.Command = strings.Join(status.Argv, " ")
	if status.Platforms == nil {
		status.Platforms = map[string]PlatformRuntimeStatus{}
	}
	status.UpdatedAt = s.now().Format(time.RFC3339Nano)

	if update.GatewayState != "" {
		status.GatewayState = update.GatewayState
	}
	if update.ExitReason != "" ||
		update.GatewayState == GatewayStateStarting ||
		update.GatewayState == GatewayStateRunning ||
		update.GatewayState == GatewayStateStopped {
		status.ExitReason = update.ExitReason
	}
	if update.ActiveAgents != nil {
		if *update.ActiveAgents < 0 {
			status.ActiveAgents = 0
		} else {
			status.ActiveAgents = *update.ActiveAgents
		}
	}
	if update.ProxyState != "" || update.ProxyURL != "" || update.ProxyErrorMessage != "" {
		if update.ProxyState != "" {
			status.Proxy.State = update.ProxyState
		}
		if update.ProxyURL != "" {
			status.Proxy.URL = update.ProxyURL
		}
		status.Proxy.ErrorMessage = update.ProxyErrorMessage
		status.Proxy.UpdatedAt = status.UpdatedAt
	}
	if update.DrainTimeoutEvidence != nil {
		evidence := *update.DrainTimeoutEvidence
		status.DrainTimeouts = append(status.DrainTimeouts, evidence)
	}
	if update.ResumePendingEvidence != nil {
		evidence := *update.ResumePendingEvidence
		status.ResumePending = append(status.ResumePending, evidence)
	}
	if update.NonResumableEvidence != nil {
		evidence := *update.NonResumableEvidence
		status.NonResumable = append(status.NonResumable, evidence)
	}
	if update.ExpiryFinalizedEvidence != nil {
		evidence := *update.ExpiryFinalizedEvidence
		status.ExpiryFinalized = append(status.ExpiryFinalized, evidence)
	}
	if update.TokenLockEvidence != nil {
		evidence := *update.TokenLockEvidence
		status.TokenLocks = append(status.TokenLocks, evidence)
	}
	if update.Platform == "" {
		return
	}

	platform := status.Platforms[update.Platform]
	if update.PlatformState != "" {
		platform.State = update.PlatformState
	}
	platform.ErrorMessage = update.ErrorMessage
	platform.UpdatedAt = status.UpdatedAt
	status.Platforms[update.Platform] = platform
}

func applyRuntimeProcessValidation(status RuntimeStatus, validation RuntimeProcessValidation, missing bool) RuntimeStatus {
	status.ProcessValidation = validation
	if missing || validation.Live {
		return status
	}
	status.GatewayState = GatewayStateStopped
	status.ActiveAgents = 0
	for name, platform := range status.Platforms {
		switch platform.State {
		case PlatformStateStarting, PlatformStateRunning:
			platform.State = PlatformStateStopped
			status.Platforms[name] = platform
		}
	}
	switch strings.TrimSpace(strings.ToLower(status.Proxy.State)) {
	case "starting", "running", "draining":
		status.Proxy.State = "stopped"
	}
	return status
}

func (s *RuntimeStatusStore) readLocked() (RuntimeStatus, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		pid := s.pid()
		startTime, _ := s.startTime(pid)
		argv := append([]string(nil), s.argv()...)
		return RuntimeStatus{
			Kind:         runtimeStatusKind,
			PID:          pid,
			StartTime:    startTime,
			Command:      strings.Join(argv, " "),
			Argv:         argv,
			GatewayState: GatewayStateStarting,
			Platforms:    map[string]PlatformRuntimeStatus{},
			UpdatedAt:    s.now().Format(time.RFC3339Nano),
		}, nil
	}
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("read runtime status: %w", err)
	}
	if len(raw) == 0 {
		pid := s.pid()
		startTime, _ := s.startTime(pid)
		argv := append([]string(nil), s.argv()...)
		return RuntimeStatus{
			Kind:      runtimeStatusKind,
			PID:       pid,
			StartTime: startTime,
			Command:   strings.Join(argv, " "),
			Argv:      argv,
			Platforms: map[string]PlatformRuntimeStatus{},
			UpdatedAt: s.now().Format(time.RFC3339Nano),
		}, nil
	}

	var status RuntimeStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return RuntimeStatus{}, fmt.Errorf("decode runtime status: %w", err)
	}
	if status.Platforms == nil {
		status.Platforms = map[string]PlatformRuntimeStatus{}
	}
	return status, nil
}

func (s *RuntimeStatusStore) writeLocked(ctx context.Context, status RuntimeStatus) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeRuntimeStatusJSONAtomic(s.path, status); err != nil {
		return err
	}
	if s.pidPath == "" {
		return nil
	}
	pidRecord := RuntimeStatus{
		Kind:       status.Kind,
		PID:        status.PID,
		StartTime:  status.StartTime,
		Generation: status.Generation,
		Command:    status.Command,
		Argv:       append([]string(nil), status.Argv...),
		UpdatedAt:  status.UpdatedAt,
	}
	if err := writeRuntimeStatusJSONAtomic(s.pidPath, pidRecord); err != nil {
		return fmt.Errorf("write runtime pid record: %w", err)
	}
	return nil
}

func writeRuntimeStatusJSONAtomic(path string, status RuntimeStatus) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create runtime status dir: %w", err)
	}
	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime status: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".gateway_state-*.tmp")
	if err != nil {
		return fmt.Errorf("create runtime status temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write runtime status temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close runtime status temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace runtime status: %w", err)
	}
	return nil
}

func readRuntimeStatusRecord(path string) (RuntimeStatus, error) {
	if path == "" {
		return RuntimeStatus{}, os.ErrNotExist
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if len(raw) == 0 {
		return RuntimeStatus{}, os.ErrNotExist
	}
	var status RuntimeStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return RuntimeStatus{}, fmt.Errorf("decode runtime PID record: %w", err)
	}
	return status, nil
}

var (
	errRuntimeProcessNotFound         = errors.New("runtime process not found")
	errRuntimeProcessPermissionDenied = errors.New("runtime process permission denied")
)

type runtimeProcessTable interface {
	LookupRuntimeProcess(pid int) (runtimeProcessInfo, error)
}

type runtimeProcessInfo struct {
	PID       int
	StartTime int64
	Command   string
	Stopped   bool
}

type procRuntimeProcessTable struct{}

func (procRuntimeProcessTable) LookupRuntimeProcess(pid int) (runtimeProcessInfo, error) {
	if pid <= 0 {
		return runtimeProcessInfo{}, errRuntimeProcessNotFound
	}
	statPath := filepath.Join("/proc", fmt.Sprint(pid), "stat")
	raw, err := os.ReadFile(statPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return runtimeProcessInfo{}, errRuntimeProcessPermissionDenied
		}
		if errors.Is(err, os.ErrNotExist) {
			return runtimeProcessInfo{}, errRuntimeProcessNotFound
		}
		return runtimeProcessInfo{}, err
	}
	startTime, state, ok := parseProcStatIdentity(string(raw))
	if !ok {
		return runtimeProcessInfo{}, errRuntimeProcessNotFound
	}
	info := runtimeProcessInfo{
		PID:       pid,
		StartTime: startTime,
		Stopped:   state == "T" || state == "t",
	}
	if cmdline, ok := readProcCmdline(pid); ok {
		info.Command = cmdline
	}
	return info, nil
}

func readProcCmdline(pid int) (string, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pid), "cmdline"))
	if err != nil || len(raw) == 0 {
		return "", false
	}
	parts := strings.Split(string(raw), "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return "", false
	}
	return strings.Join(out, " "), true
}

func procProcessStartTime(pid int) (int64, bool) {
	if pid <= 0 {
		return 0, false
	}
	raw, err := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pid), "stat"))
	if err != nil {
		return 0, false
	}
	return parseProcStatStartTime(string(raw))
}

func parseProcStatStartTime(stat string) (int64, bool) {
	startTime, _, ok := parseProcStatIdentity(stat)
	return startTime, ok
}

func parseProcStatIdentity(stat string) (int64, string, bool) {
	commEnd := strings.LastIndex(stat, ")")
	if commEnd < 0 || commEnd+2 >= len(stat) {
		return 0, "", false
	}
	fields := strings.Fields(stat[commEnd+2:])
	if len(fields) <= 19 {
		return 0, "", false
	}
	var start int64
	if _, err := fmt.Sscan(fields[19], &start); err != nil {
		return 0, "", false
	}
	return start, fields[0], true
}
