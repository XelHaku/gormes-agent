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

func TestOneshotFlags_ModelFlagParsesWithoutTUIOrHealthCheck(t *testing.T) {
	setupOneshotFlagTestEnv(t)

	var got oneshotInvocation
	var oneshotCalls int
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		runOneshot: func(_ *cobra.Command, invocation oneshotInvocation) error {
			oneshotCalls++
			got = invocation
			return nil
		},
	})
	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi", "--model", "fixture-model")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if oneshotCalls != 1 {
		t.Fatalf("runOneshot calls = %d, want 1", oneshotCalls)
	}
	if got.Prompt != "hi" {
		t.Fatalf("Prompt = %q, want hi", got.Prompt)
	}
	if got.Inference.Model != "fixture-model" || got.Inference.ModelSource != config.InferenceValueSourceFlag {
		t.Fatalf("model resolution = %+v, want fixture-model from flag", got.Inference)
	}
	if got.Inference.Provider != "" || got.Inference.ProviderSource != config.InferenceValueSourceUnset {
		t.Fatalf("provider resolution = %+v, want unset provider", got.Inference)
	}
	if !got.Inference.ProviderAutoDetectRequired {
		t.Fatalf("ProviderAutoDetectRequired = false, want true for explicit model without provider")
	}
	if strings.Contains(stderr, "api_server") {
		t.Fatalf("stderr contains api_server health output:\n%s", stderr)
	}
}

func TestOneshotFlags_ProviderWithoutExplicitModelExits2BeforeRunners(t *testing.T) {
	setupOneshotFlagTestEnv(t)
	writeOneshotFlagConfig(t, []byte(`
[hermes]
model = "stale-configured-model"
`))

	var tuiCalls, oneshotCalls int
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			tuiCalls++
			return nil
		},
		runOneshot: func(*cobra.Command, oneshotInvocation) error {
			oneshotCalls++
			return nil
		},
	})
	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi", "--provider", "openrouter")
	if err == nil {
		t.Fatalf("Execute() error = nil, want provider/model ambiguity error\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if code := exitCodeFromError(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if tuiCalls != 0 || oneshotCalls != 0 {
		t.Fatalf("runner calls = tui:%d oneshot:%d, want none", tuiCalls, oneshotCalls)
	}
	for _, want := range []string{
		"gormes -z: --provider requires --model (or GORMES_INFERENCE_MODEL)",
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

func TestOneshotFlags_ProviderFlagAllowsEnvModelWithoutMutatingConfig(t *testing.T) {
	setupOneshotFlagTestEnv(t)
	writeOneshotFlagConfig(t, []byte(`
[hermes]
model = "configured-model"
`))
	t.Setenv("GORMES_INFERENCE_MODEL", "env-model")

	var got oneshotInvocation
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		runOneshot: func(_ *cobra.Command, invocation oneshotInvocation) error {
			got = invocation
			return nil
		},
	})
	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi", "--provider", "openrouter")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if got.Inference.Model != "env-model" || got.Inference.ModelSource != config.InferenceValueSourceEnv {
		t.Fatalf("model resolution = %+v, want env-model from env", got.Inference)
	}
	if got.Inference.Provider != "openrouter" || got.Inference.ProviderSource != config.InferenceValueSourceFlag {
		t.Fatalf("provider resolution = %+v, want openrouter from flag", got.Inference)
	}
	if got.Inference.ProviderAutoDetectRequired {
		t.Fatalf("ProviderAutoDetectRequired = true, want false when provider is explicit")
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load(nil): %v", err)
	}
	if cfg.Hermes.Model != "configured-model" {
		t.Fatalf("cfg.Hermes.Model = %q, want persisted configured-model", cfg.Hermes.Model)
	}
}

func TestOneshotFlags_EnvModelAndProviderFallbacksAreRecorded(t *testing.T) {
	setupOneshotFlagTestEnv(t)
	t.Setenv("GORMES_INFERENCE_MODEL", "env-model")
	t.Setenv("GORMES_INFERENCE_PROVIDER", "openrouter")

	var got oneshotInvocation
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		runOneshot: func(_ *cobra.Command, invocation oneshotInvocation) error {
			got = invocation
			return nil
		},
	})
	stdout, stderr, err := executeOneshotFlagCommand(cmd, "--oneshot", "hi")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if got.Inference.Model != "env-model" || got.Inference.ModelSource != config.InferenceValueSourceEnv {
		t.Fatalf("model resolution = %+v, want env-model from env", got.Inference)
	}
	if got.Inference.Provider != "openrouter" || got.Inference.ProviderSource != config.InferenceValueSourceEnv {
		t.Fatalf("provider resolution = %+v, want openrouter from env", got.Inference)
	}
	if got.Inference.ProviderAutoDetectRequired {
		t.Fatalf("ProviderAutoDetectRequired = true, want false when provider comes from env")
	}
}

func TestOneshotFlags_ConfigModelFallbackIsRecorded(t *testing.T) {
	setupOneshotFlagTestEnv(t)
	writeOneshotFlagConfig(t, []byte(`
[hermes]
model = "configured-model"
`))

	var got oneshotInvocation
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		runOneshot: func(_ *cobra.Command, invocation oneshotInvocation) error {
			got = invocation
			return nil
		},
	})
	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if got.Inference.Model != "configured-model" || got.Inference.ModelSource != config.InferenceValueSourceConfig {
		t.Fatalf("model resolution = %+v, want configured-model from config", got.Inference)
	}
	if got.Inference.ProviderAutoDetectRequired {
		t.Fatalf("ProviderAutoDetectRequired = true, want false for config defaults")
	}
}

func setupOneshotFlagTestEnv(t *testing.T) {
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

func writeOneshotFlagConfig(t *testing.T, data []byte) {
	t.Helper()
	path := config.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func executeOneshotFlagCommand(cmd *cobra.Command, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
