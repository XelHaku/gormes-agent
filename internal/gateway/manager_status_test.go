package gateway

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManager_RunWritesChannelLifecycleToRuntimeStatus(t *testing.T) {
	tg := newFakeChannel("telegram")
	store := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	m := NewManagerWithSubmitter(ManagerConfig{
		RuntimeStatus: store,
	}, &fakeKernel{}, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	waitFor(t, 200*time.Millisecond, func() bool {
		status, err := store.ReadRuntimeStatus(context.Background())
		if err != nil {
			return false
		}
		return status.GatewayState == GatewayStateRunning &&
			status.Platforms["telegram"].State == PlatformStateRunning
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}

	status, err := store.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status.GatewayState != GatewayStateStopped {
		t.Fatalf("GatewayState = %q, want %q", status.GatewayState, GatewayStateStopped)
	}
	if got := status.Platforms["telegram"].State; got != PlatformStateStopped {
		t.Fatalf("telegram state = %q, want %q", got, PlatformStateStopped)
	}
}

func TestManager_RunWritesStartupFailureToRuntimeStatus(t *testing.T) {
	startupErr := errors.New("discord: open session: denied")
	ch := &startupFailedChannel{name: "discord", runErr: startupErr}
	store := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))

	m := NewManagerWithSubmitter(ManagerConfig{
		RuntimeStatus: store,
	}, &fakeKernel{}, slog.Default())
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return startup error")
	}
	if !errors.Is(err, startupErr) {
		t.Fatalf("Run error = %v, want startup error %v", err, startupErr)
	}

	status, readErr := store.ReadRuntimeStatus(context.Background())
	if readErr != nil {
		t.Fatalf("read status: %v", readErr)
	}
	if status.GatewayState != GatewayStateStartupFailed {
		t.Fatalf("GatewayState = %q, want %q", status.GatewayState, GatewayStateStartupFailed)
	}
	if !strings.Contains(status.ExitReason, startupErr.Error()) {
		t.Fatalf("ExitReason = %q, want startup error", status.ExitReason)
	}
	platform := status.Platforms["discord"]
	if platform.State != PlatformStateFailed {
		t.Fatalf("discord state = %q, want %q", platform.State, PlatformStateFailed)
	}
	if platform.ErrorMessage != startupErr.Error() {
		t.Fatalf("discord error = %q, want %q", platform.ErrorMessage, startupErr.Error())
	}
}
