package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func TestGatewayStatusCommand_NoChannelsSucceedsWithoutOpeningRuntimeClients(t *testing.T) {
	setupGatewayStatusTestEnv(t)

	stdout, stderr, err := executeGatewayStatusCommand(t)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	for _, want := range []string{
		"runtime: missing",
		"channels: none configured",
		"- pairing missing: pairing state is missing",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q\n%s", want, stdout)
		}
	}
	assertGatewayStatusDidNotOpenRuntimeStores(t)
}

func TestGatewayStatusCommand_RendersConfiguredChannelsFromReadModels(t *testing.T) {
	setupGatewayStatusTestEnv(t)
	writeGatewayStatusConfig(t, []byte(`
[telegram]
bot_token = "12345:bogus"
allowed_chat_id = 42

[discord]
token = "bogus-discord-token"
allowed_channel_id = "D123"
`))

	now := time.Date(2026, 4, 25, 20, 0, 0, 0, time.UTC)
	pairing := gateway.NewXDGPairingStore()
	if err := pairing.RecordPendingPairing(context.Background(), gateway.PairingPendingRecord{
		Platform:  "telegram",
		UserID:    "telegram-user",
		UserName:  "Ada",
		Code:      "TGREADY",
		CreatedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("record pending pairing: %v", err)
	}
	if err := pairing.RecordApprovedPairing(context.Background(), gateway.PairingApprovedRecord{
		Platform:   "discord",
		UserID:     "discord-owner",
		UserName:   "Grace",
		ApprovedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record approved pairing: %v", err)
	}

	runtimeStatus := gateway.NewRuntimeStatusStore(config.GatewayRuntimeStatusPath())
	if err := runtimeStatus.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
		GatewayState: gateway.GatewayStateRunning,
	}); err != nil {
		t.Fatalf("write gateway runtime: %v", err)
	}
	if err := runtimeStatus.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
		Platform:      "telegram",
		PlatformState: gateway.PlatformStateRunning,
	}); err != nil {
		t.Fatalf("write telegram runtime: %v", err)
	}
	if err := runtimeStatus.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
		Platform:      "discord",
		PlatformState: gateway.PlatformStateFailed,
		ErrorMessage:  "discord: open session: denied",
	}); err != nil {
		t.Fatalf("write discord runtime: %v", err)
	}

	stdout, stderr, err := executeGatewayStatusCommand(t)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	discordRow := "- discord: lifecycle=failed error=\"discord: open session: denied\"; pairing=paired pending=0 approved=1; target=allowed_channel_id=D123"
	telegramRow := "- telegram: lifecycle=running; pairing=unpaired pending=1 approved=0; target=allowed_chat_id=42"
	if !strings.Contains(stdout, discordRow) {
		t.Fatalf("stdout missing discord row\n%s", stdout)
	}
	if !strings.Contains(stdout, telegramRow) {
		t.Fatalf("stdout missing telegram row\n%s", stdout)
	}
	if strings.Index(stdout, discordRow) > strings.Index(stdout, telegramRow) {
		t.Fatalf("channel rows are not sorted\n%s", stdout)
	}
	for _, want := range []string{
		"- pending telegram user=telegram-user code=TGREADY",
		"- approved discord user=discord-owner name=Grace",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q\n%s", want, stdout)
		}
	}
	assertGatewayStatusDidNotOpenRuntimeStores(t)
}

func setupGatewayStatusTestEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
}

func writeGatewayStatusConfig(t *testing.T, data []byte) {
	t.Helper()
	path := config.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func executeGatewayStatusCommand(t *testing.T) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"gateway", "status"})
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func assertGatewayStatusDidNotOpenRuntimeStores(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		config.SessionDBPath(),
		config.MemoryDBPath(),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("gateway status opened runtime store %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat runtime store %s: %v", path, err)
		}
	}
}
