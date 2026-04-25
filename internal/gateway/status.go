package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Kind         string                           `json:"kind"`
	PID          int                              `json:"pid"`
	GatewayState GatewayState                     `json:"gateway_state"`
	ExitReason   string                           `json:"exit_reason"`
	ActiveAgents int                              `json:"active_agents"`
	Platforms    map[string]PlatformRuntimeStatus `json:"platforms"`
	Proxy        ProxyRuntimeStatus               `json:"proxy"`
	UpdatedAt    string                           `json:"updated_at"`
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
}

// RuntimeStatusWriter is the manager-facing seam for lifecycle status writes.
type RuntimeStatusWriter interface {
	UpdateRuntimeStatus(context.Context, RuntimeStatusUpdate) error
}

// RuntimeStatusStore persists the gateway runtime status as atomic JSON.
type RuntimeStatusStore struct {
	path string
	now  func() time.Time
	pid  func() int
	mu   sync.Mutex
}

// NewRuntimeStatusStore returns a JSON-backed runtime status store.
func NewRuntimeStatusStore(path string) *RuntimeStatusStore {
	return &RuntimeStatusStore{
		path: path,
		now:  func() time.Time { return time.Now().UTC() },
		pid:  os.Getpid,
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

func (s *RuntimeStatusStore) merge(status *RuntimeStatus, update RuntimeStatusUpdate) {
	status.Kind = runtimeStatusKind
	status.PID = s.pid()
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

func (s *RuntimeStatusStore) readLocked() (RuntimeStatus, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return RuntimeStatus{
			Kind:         runtimeStatusKind,
			PID:          s.pid(),
			GatewayState: GatewayStateStarting,
			Platforms:    map[string]PlatformRuntimeStatus{},
			UpdatedAt:    s.now().Format(time.RFC3339Nano),
		}, nil
	}
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("read runtime status: %w", err)
	}
	if len(raw) == 0 {
		return RuntimeStatus{
			Kind:      runtimeStatusKind,
			PID:       s.pid(),
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
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create runtime status dir: %w", err)
	}

	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime status: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".gateway_state-*.tmp")
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
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace runtime status: %w", err)
	}
	return nil
}
