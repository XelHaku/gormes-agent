package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

type gatewaySlackTestKernel struct {
	renders chan kernel.RenderFrame
}

func newGatewaySlackTestKernel() *gatewaySlackTestKernel {
	return &gatewaySlackTestKernel{renders: make(chan kernel.RenderFrame)}
}

func (k *gatewaySlackTestKernel) Submit(kernel.PlatformEvent) error { return nil }
func (k *gatewaySlackTestKernel) ResetSession() error               { return nil }
func (k *gatewaySlackTestKernel) Render() <-chan kernel.RenderFrame { return k.renders }

type gatewaySlackTestChannel struct {
	name       string
	runErr     error
	runStarted chan struct{}
	runOnce    sync.Once
}

func newGatewaySlackTestChannel(name string) *gatewaySlackTestChannel {
	return &gatewaySlackTestChannel{name: name, runStarted: make(chan struct{})}
}

func (c *gatewaySlackTestChannel) Name() string { return c.name }

func (c *gatewaySlackTestChannel) Run(ctx context.Context, _ chan<- gateway.InboundEvent) error {
	c.runOnce.Do(func() { close(c.runStarted) })
	if c.runErr != nil {
		return c.runErr
	}
	<-ctx.Done()
	return nil
}

func (c *gatewaySlackTestChannel) Send(context.Context, string, string) (string, error) {
	return "msg-1", nil
}

type recordingGatewayRuntimeStatus struct {
	mu      sync.Mutex
	updates []gateway.RuntimeStatusUpdate
}

func (s *recordingGatewayRuntimeStatus) UpdateRuntimeStatus(_ context.Context, update gateway.RuntimeStatusUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, update)
	return nil
}

func (s *recordingGatewayRuntimeStatus) hasPlatformState(platform string, state gateway.PlatformState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, update := range s.updates {
		if update.Platform == platform && update.PlatformState == state {
			return true
		}
	}
	return false
}

func (s *recordingGatewayRuntimeStatus) platformError(platform string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.updates) - 1; i >= 0; i-- {
		update := s.updates[i]
		if update.Platform == platform && update.ErrorMessage != "" {
			return update.ErrorMessage
		}
	}
	return ""
}

func TestGatewaySlackRegistration_RegistersSlackThroughManagerWhenEnabled(t *testing.T) {
	cfg := config.Config{
		Slack: config.SlackCfg{
			Enabled:           true,
			BotToken:          "xoxb-test",
			AppToken:          "xapp-test",
			AllowedChannelID:  "C123",
			CoalesceMs:        250,
			FirstRunDiscovery: true,
		},
	}
	allowedChats := map[string]string{}
	allowDiscovery := map[string]bool{}
	status := &recordingGatewayRuntimeStatus{}
	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats:   allowedChats,
		AllowDiscovery: allowDiscovery,
		CoalesceMs:     1000,
		RuntimeStatus:  status,
	}, newGatewaySlackTestKernel(), slog.Default())

	fakeSlack := newGatewaySlackTestChannel("slack")
	factories := gatewayChannelFactories{
		Slack: func(got config.Config, _ *slog.Logger) (gateway.Channel, error) {
			if got.Slack.BotToken != "xoxb-test" || got.Slack.AppToken != "xapp-test" {
				t.Fatalf("factory saw Slack tokens %#v", got.Slack)
			}
			return fakeSlack, nil
		},
	}

	registered, err := registerConfiguredGatewayChannels(mgr, cfg, allowedChats, allowDiscovery, factories, status, slog.Default())
	if err != nil {
		t.Fatalf("registerConfiguredGatewayChannels: %v", err)
	}
	if registered != 1 || mgr.ChannelCount() != 1 {
		t.Fatalf("registered/channel count = %d/%d, want 1/1", registered, mgr.ChannelCount())
	}
	if allowedChats["slack"] != "C123" {
		t.Fatalf("allowedChats[slack] = %q, want C123", allowedChats["slack"])
	}
	if !allowDiscovery["slack"] {
		t.Fatal("allowDiscovery[slack] = false, want true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Run(ctx) }()

	select {
	case <-fakeSlack.runStarted:
	case <-time.After(time.Second):
		t.Fatal("Slack channel did not enter the shared Manager Run lifecycle")
	}
	waitForGatewaySlackCondition(t, time.Second, func() bool {
		return status.hasPlatformState("slack", gateway.PlatformStateRunning)
	})
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Manager Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Manager Run did not return after cancellation")
	}
}

func TestGatewaySlackRegistration_MissingTokensDegradeWithoutBlockingDiscord(t *testing.T) {
	cfg := config.Config{
		Discord: config.DiscordCfg{
			Token:            "discord-token",
			AllowedChannelID: "D123",
		},
		Slack: config.SlackCfg{
			Enabled:          true,
			AllowedChannelID: "C123",
		},
	}
	status := &recordingGatewayRuntimeStatus{}
	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats:  map[string]string{},
		RuntimeStatus: status,
	}, newGatewaySlackTestKernel(), slog.Default())
	slackFactoryCalled := false
	discordFactoryCalled := false
	factories := gatewayChannelFactories{
		Discord: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			discordFactoryCalled = true
			return newGatewaySlackTestChannel("discord"), nil
		},
		Slack: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			slackFactoryCalled = true
			return newGatewaySlackTestChannel("slack"), nil
		},
	}

	registered, err := registerConfiguredGatewayChannels(mgr, cfg, map[string]string{}, map[string]bool{}, factories, status, slog.Default())
	if err != nil {
		t.Fatalf("registerConfiguredGatewayChannels: %v", err)
	}
	if registered != 1 || !discordFactoryCalled {
		t.Fatalf("registered=%d discordFactoryCalled=%t, want Discord to register", registered, discordFactoryCalled)
	}
	if slackFactoryCalled {
		t.Fatal("Slack factory was called despite missing credentials")
	}
	errText := status.platformError("slack")
	for _, want := range []string{"missing", "bot_token", "app_token"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("Slack status error %q missing %q", errText, want)
		}
	}
	if !status.hasPlatformState("slack", gateway.PlatformStateFailed) {
		t.Fatal("Slack runtime status did not record failed state for missing credentials")
	}
}

func TestGatewaySlackRegistration_SlackStartupFailureDegradesWithoutBlockingDiscord(t *testing.T) {
	cfg := config.Config{
		Discord: config.DiscordCfg{
			Token:            "discord-token",
			AllowedChannelID: "D123",
		},
		Slack: config.SlackCfg{
			Enabled:          true,
			BotToken:         "xoxb-test",
			AppToken:         "xapp-test",
			AllowedChannelID: "C123",
		},
	}
	status := &recordingGatewayRuntimeStatus{}
	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats:  map[string]string{},
		RuntimeStatus: status,
	}, newGatewaySlackTestKernel(), slog.Default())
	factories := gatewayChannelFactories{
		Discord: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			return newGatewaySlackTestChannel("discord"), nil
		},
		Slack: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			return nil, errors.New("socket mode startup denied")
		},
	}

	registered, err := registerConfiguredGatewayChannels(mgr, cfg, map[string]string{}, map[string]bool{}, factories, status, slog.Default())
	if err != nil {
		t.Fatalf("registerConfiguredGatewayChannels: %v", err)
	}
	if registered != 1 || mgr.ChannelCount() != 1 {
		t.Fatalf("registered/channel count = %d/%d, want Discord only", registered, mgr.ChannelCount())
	}
	errText := status.platformError("slack")
	if !strings.Contains(errText, "socket mode startup denied") {
		t.Fatalf("Slack status error = %q, want startup failure", errText)
	}
	if !status.hasPlatformState("slack", gateway.PlatformStateFailed) {
		t.Fatal("Slack runtime status did not record failed state for startup failure")
	}
}

func TestGatewaySlackRegistration_TelegramOnlyDoesNotRequireSlack(t *testing.T) {
	cfg := config.Config{
		Telegram: config.TelegramCfg{BotToken: "telegram-token", AllowedChatID: 42},
	}
	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats: map[string]string{},
	}, newGatewaySlackTestKernel(), slog.Default())
	slackFactoryCalled := false
	factories := gatewayChannelFactories{
		Telegram: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			return newGatewaySlackTestChannel("telegram"), nil
		},
		Slack: func(config.Config, *slog.Logger) (gateway.Channel, error) {
			slackFactoryCalled = true
			return newGatewaySlackTestChannel("slack"), nil
		},
	}

	registered, err := registerConfiguredGatewayChannels(mgr, cfg, map[string]string{}, map[string]bool{}, factories, nil, slog.Default())
	if err != nil {
		t.Fatalf("registerConfiguredGatewayChannels: %v", err)
	}
	if registered != 1 || mgr.ChannelCount() != 1 {
		t.Fatalf("registered/channel count = %d/%d, want Telegram only", registered, mgr.ChannelCount())
	}
	if slackFactoryCalled {
		t.Fatal("Slack factory was called for Telegram-only config")
	}
}

func TestGatewayStatusCommand_RendersSlackConfigAndRuntimeStates(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		setupGatewayStatusTestEnv(t)

		stdout, stderr, err := executeGatewayStatusCommand(t)
		if err != nil {
			t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
		}
		if !strings.Contains(stdout, "gateway/slack: disabled") {
			t.Fatalf("stdout missing Slack disabled state\n%s", stdout)
		}
	})

	t.Run("missing_token", func(t *testing.T) {
		setupGatewayStatusTestEnv(t)
		writeGatewayStatusConfig(t, []byte(`
[slack]
enabled = true
allowed_channel_id = "C123"
`))

		stdout, stderr, err := executeGatewayStatusCommand(t)
		if err != nil {
			t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
		}
		if !strings.Contains(stdout, "- slack: lifecycle=unknown") ||
			!strings.Contains(stdout, "target=missing_tokens=bot_token,app_token") {
			t.Fatalf("stdout missing Slack missing-token row\n%s", stdout)
		}
	})

	t.Run("startup_failed_and_running", func(t *testing.T) {
		setupGatewayStatusTestEnv(t)
		writeGatewayStatusConfig(t, []byte(`
[slack]
enabled = true
bot_token = "xoxb-test"
app_token = "xapp-test"
allowed_channel_id = "C123"
`))
		runtimeStatus := gateway.NewRuntimeStatusStore(config.GatewayRuntimeStatusPath())
		if err := runtimeStatus.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
			Platform:      "slack",
			PlatformState: gateway.PlatformStateFailed,
			ErrorMessage:  "slack: socket mode startup denied",
		}); err != nil {
			t.Fatalf("write Slack failed runtime: %v", err)
		}

		stdout, stderr, err := executeGatewayStatusCommand(t)
		if err != nil {
			t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
		}
		if !strings.Contains(stdout, "- slack: lifecycle=failed error=\"slack: socket mode startup denied\"") {
			t.Fatalf("stdout missing Slack failed row\n%s", stdout)
		}

		if err := runtimeStatus.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
			Platform:      "slack",
			PlatformState: gateway.PlatformStateRunning,
		}); err != nil {
			t.Fatalf("write Slack running runtime: %v", err)
		}
		stdout, stderr, err = executeGatewayStatusCommand(t)
		if err != nil {
			t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
		}
		if !strings.Contains(stdout, "- slack: lifecycle=running") ||
			!strings.Contains(stdout, "target=allowed_channel_id=C123") {
			t.Fatalf("stdout missing Slack running row\n%s", stdout)
		}
	})
}

func TestDoctorSlackGatewayConfigReportsDisabledMissingFailedAndRunning(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		runtime gateway.RuntimeStatus
		want    []string
	}{
		{
			name: "disabled",
			want: []string{"[WARN] Gateway Slack: disabled"},
		},
		{
			name: "missing_token",
			cfg: config.Config{Slack: config.SlackCfg{
				Enabled:          true,
				AllowedChannelID: "C123",
			}},
			want: []string{"[WARN] Gateway Slack: missing_tokens=bot_token,app_token", "allowed_channel_id=C123"},
		},
		{
			name: "startup_failed",
			cfg: config.Config{Slack: config.SlackCfg{
				Enabled:          true,
				BotToken:         "xoxb-test",
				AppToken:         "xapp-test",
				AllowedChannelID: "C123",
			}},
			runtime: gateway.RuntimeStatus{Platforms: map[string]gateway.PlatformRuntimeStatus{
				"slack": {State: gateway.PlatformStateFailed, ErrorMessage: "slack: socket mode startup denied"},
			}},
			want: []string{"[WARN] Gateway Slack: startup_failed", "slack: socket mode startup denied"},
		},
		{
			name: "running",
			cfg: config.Config{Slack: config.SlackCfg{
				Enabled:          true,
				BotToken:         "xoxb-test",
				AppToken:         "xapp-test",
				AllowedChannelID: "C123",
			}},
			runtime: gateway.RuntimeStatus{Platforms: map[string]gateway.PlatformRuntimeStatus{
				"slack": {State: gateway.PlatformStateRunning},
			}},
			want: []string{"[PASS] Gateway Slack: running", "allowed_channel_id=C123"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := doctorSlackGatewayConfig(tc.cfg, tc.runtime).Format()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("doctor Slack output missing %q\n%s", want, got)
				}
			}
		})
	}
}

func waitForGatewaySlackCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
