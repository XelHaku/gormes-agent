package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_BuiltinDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GORMES_ENDPOINT", "")
	t.Setenv("GORMES_API_KEY", "")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hermes.Endpoint != "http://127.0.0.1:8642" {
		t.Errorf("default endpoint = %q", cfg.Hermes.Endpoint)
	}
	if cfg.Hermes.Model != "hermes-agent" {
		t.Errorf("default model = %q", cfg.Hermes.Model)
	}
	if cfg.Input.MaxBytes != 200_000 || cfg.Input.MaxLines != 10_000 {
		t.Errorf("default input limits = %d/%d", cfg.Input.MaxBytes, cfg.Input.MaxLines)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	dir := filepath.Join(cfgHome, "gormes")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
[hermes]
endpoint = "http://file:8642"
`), 0o644)
	t.Setenv("GORMES_ENDPOINT", "http://env:8642")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hermes.Endpoint != "http://env:8642" {
		t.Errorf("endpoint = %q, want env", cfg.Hermes.Endpoint)
	}
}

func TestLoad_FlagOverridesEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GORMES_ENDPOINT", "http://env:8642")
	cfg, err := Load([]string{"--endpoint", "http://flag:8642"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hermes.Endpoint != "http://flag:8642" {
		t.Errorf("endpoint = %q, want flag", cfg.Hermes.Endpoint)
	}
}

func TestLoad_SecretsNeverOnFlags(t *testing.T) {
	// Sanity: --api-key must NOT be a valid flag (secrets live in env/TOML only).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := Load([]string{"--api-key", "nope"})
	if err == nil {
		t.Error("expected --api-key to be rejected as an unknown flag")
	}
}
