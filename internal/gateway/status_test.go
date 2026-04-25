package gateway

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRuntimeStatusStore_MergesChannelLifecycleIntoReadModel(t *testing.T) {
	store := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))

	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		GatewayState: GatewayStateStarting,
	}); err != nil {
		t.Fatalf("write gateway starting: %v", err)
	}
	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		Platform:      "telegram",
		PlatformState: PlatformStateStarting,
	}); err != nil {
		t.Fatalf("write telegram starting: %v", err)
	}
	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		Platform:      "discord",
		PlatformState: PlatformStateFailed,
		ErrorMessage:  "discord: open session: denied",
	}); err != nil {
		t.Fatalf("write discord failed: %v", err)
	}
	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		Platform:      "telegram",
		PlatformState: PlatformStateRunning,
		ErrorMessage:  "",
	}); err != nil {
		t.Fatalf("write telegram running: %v", err)
	}

	status, err := store.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("read status: %v", err)
	}

	if status.Kind != "gormes-gateway" {
		t.Fatalf("Kind = %q, want gormes-gateway", status.Kind)
	}
	if status.GatewayState != GatewayStateStarting {
		t.Fatalf("GatewayState = %q, want %q", status.GatewayState, GatewayStateStarting)
	}
	if got := status.Platforms["telegram"].State; got != PlatformStateRunning {
		t.Fatalf("telegram state = %q, want %q", got, PlatformStateRunning)
	}
	if got := status.Platforms["telegram"].ErrorMessage; got != "" {
		t.Fatalf("telegram error = %q, want cleared empty error", got)
	}
	if got := status.Platforms["discord"].State; got != PlatformStateFailed {
		t.Fatalf("discord state = %q, want %q", got, PlatformStateFailed)
	}
	if got := status.Platforms["discord"].ErrorMessage; got != "discord: open session: denied" {
		t.Fatalf("discord error = %q, want startup failure", got)
	}
}

func TestRuntimeStatusStore_ClearsStaleExitReasonOnFreshStart(t *testing.T) {
	store := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))

	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		GatewayState: GatewayStateStartupFailed,
		ExitReason:   "telegram polling conflict",
	}); err != nil {
		t.Fatalf("write startup failure: %v", err)
	}
	if err := store.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		GatewayState: GatewayStateStarting,
	}); err != nil {
		t.Fatalf("write fresh start: %v", err)
	}

	status, err := store.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status.GatewayState != GatewayStateStarting {
		t.Fatalf("GatewayState = %q, want %q", status.GatewayState, GatewayStateStarting)
	}
	if status.ExitReason != "" {
		t.Fatalf("ExitReason = %q, want cleared stale failure", status.ExitReason)
	}
}
