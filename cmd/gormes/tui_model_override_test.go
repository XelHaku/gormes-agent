package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/spf13/cobra"
)

func TestTUIModelOverride_TopLevelFlagsResolveStaticAliasWithoutHealthCheck(t *testing.T) {
	setupTUIModelOverrideTestEnv(t)
	writeTUIModelOverrideConfig(t, []byte(`
[hermes]
endpoint = "http://127.0.0.1:1"
model = "stale-configured-model"
`))

	var got tuiInvocation
	var calls int
	cmd := newRootCommandWithRuntime(rootRuntime{
		runResolvedTUI: func(_ *cobra.Command, invocation tuiInvocation) error {
			calls++
			got = invocation
			return nil
		},
	})

	stdout, stderr, err := executeTUIModelOverrideCommand(cmd, "--offline", "--model", "sonnet", "--provider", "anthropic")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if calls != 1 {
		t.Fatalf("runResolvedTUI calls = %d, want 1", calls)
	}
	if got.Inference.Provider != "anthropic" || got.Inference.ProviderSource != config.InferenceValueSourceFlag {
		t.Fatalf("provider resolution = %+v, want anthropic from flag", got.Inference)
	}
	if got.Inference.Model != "claude-sonnet-4-20250514" || got.Inference.ModelSource != config.InferenceValueSourceFlag {
		t.Fatalf("model resolution = %+v, want claude-sonnet-4-20250514 from flag alias", got.Inference)
	}
	if got.Config.Hermes.Model != "stale-configured-model" {
		t.Fatalf("invocation config model = %q, want stale configured default preserved", got.Config.Hermes.Model)
	}
	if strings.Contains(stderr, "api_server") {
		t.Fatalf("stderr contains api_server health output:\n%s", stderr)
	}
}

func TestTUIModelOverride_ProviderWithoutExplicitModelExits2BeforeStartup(t *testing.T) {
	setupTUIModelOverrideTestEnv(t)
	writeTUIModelOverrideConfig(t, []byte(`
[hermes]
model = "stale-configured-model"
`))

	var tuiCalls int
	cmd := newRootCommandWithRuntime(rootRuntime{
		runResolvedTUI: func(*cobra.Command, tuiInvocation) error {
			tuiCalls++
			return nil
		},
	})

	stdout, stderr, err := executeTUIModelOverrideCommand(cmd, "--offline", "--provider", "anthropic")
	if err == nil {
		t.Fatalf("Execute() error = nil, want provider/model ambiguity error\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if code := exitCodeFromError(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if tuiCalls != 0 {
		t.Fatalf("runResolvedTUI calls = %d, want 0", tuiCalls)
	}
	for _, want := range []string{
		"gormes tui: --provider requires --model (or GORMES_INFERENCE_MODEL)",
		"Pass both explicitly, or neither to use your configured defaults.",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q\nstderr=%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "api_server") {
		t.Fatalf("stderr contains api_server health output:\n%s", stderr)
	}
}

func TestTUIModelOverride_EnvOverridesResolveAliasWithoutMutatingConfig(t *testing.T) {
	setupTUIModelOverrideTestEnv(t)
	writeTUIModelOverrideConfig(t, []byte(`
[hermes]
model = "configured-model"
`))
	t.Setenv("GORMES_INFERENCE_MODEL", "sonnet")
	t.Setenv("GORMES_INFERENCE_PROVIDER", "anthropic")

	var got tuiInvocation
	cmd := newRootCommandWithRuntime(rootRuntime{
		runResolvedTUI: func(_ *cobra.Command, invocation tuiInvocation) error {
			got = invocation
			return nil
		},
	})

	stdout, stderr, err := executeTUIModelOverrideCommand(cmd, "--offline")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if got.Inference.Provider != "anthropic" || got.Inference.ProviderSource != config.InferenceValueSourceEnv {
		t.Fatalf("provider resolution = %+v, want anthropic from env", got.Inference)
	}
	if got.Inference.Model != "claude-sonnet-4-20250514" || got.Inference.ModelSource != config.InferenceValueSourceEnv {
		t.Fatalf("model resolution = %+v, want claude-sonnet-4-20250514 from env alias", got.Inference)
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load(nil): %v", err)
	}
	if cfg.Hermes.Model != "configured-model" {
		t.Fatalf("cfg.Hermes.Model = %q, want persisted configured-model", cfg.Hermes.Model)
	}
}

func setupTUIModelOverrideTestEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("GORMES_ENDPOINT", "")
	t.Setenv("GORMES_MODEL", "")
	t.Setenv("GORMES_API_KEY", "")
	t.Setenv("GORMES_INFERENCE_MODEL", "")
	t.Setenv("GORMES_INFERENCE_PROVIDER", "")
}

func writeTUIModelOverrideConfig(t *testing.T, data []byte) {
	t.Helper()
	path := config.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func executeTUIModelOverrideCommand(cmd *cobra.Command, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
