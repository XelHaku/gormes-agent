package config

import (
	"strings"
	"testing"
	"time"
)

func TestGonchoDreamConfigIdleTimeoutDefaultsEnvAndValidation(t *testing.T) {
	isolateGonchoConfig(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Goncho.DreamIdleTimeoutMinutes != 60 {
		t.Fatalf("DreamIdleTimeoutMinutes default = %d, want 60", cfg.Goncho.DreamIdleTimeoutMinutes)
	}
	if got := cfg.Goncho.RuntimeConfig().DreamIdleTimeout; got != time.Hour {
		t.Fatalf("RuntimeConfig DreamIdleTimeout = %s, want 1h", got)
	}

	isolateGonchoConfig(t)
	t.Setenv("GORMES_GONCHO_DREAM_IDLE_TIMEOUT_MINUTES", "45")
	cfg, err = Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Goncho.DreamIdleTimeoutMinutes != 45 {
		t.Fatalf("DreamIdleTimeoutMinutes env = %d, want 45", cfg.Goncho.DreamIdleTimeoutMinutes)
	}
	if got := cfg.Goncho.RuntimeConfig().DreamIdleTimeout; got != 45*time.Minute {
		t.Fatalf("RuntimeConfig DreamIdleTimeout = %s, want 45m", got)
	}

	cfgHome := isolateGonchoConfig(t)
	writeGonchoConfigFile(t, cfgHome, `
[goncho]
dream_idle_timeout_minutes = -1
`)
	_, err = Load(nil)
	if err == nil {
		t.Fatal("Load() error = nil, want negative dream_idle_timeout_minutes error")
	}
	if !strings.Contains(err.Error(), "goncho.dream_idle_timeout_minutes") {
		t.Fatalf("Load() error = %v, want goncho.dream_idle_timeout_minutes", err)
	}
}

func TestGonchoDreamConfigFileMapsIdleTimeoutToRuntimeConfig(t *testing.T) {
	cfgHome := isolateGonchoConfig(t)
	writeGonchoConfigFile(t, cfgHome, `
[goncho]
dream_enabled = true
dream_idle_timeout_minutes = 90
`)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := cfg.Goncho.RuntimeConfig()
	if !rt.DreamEnabled {
		t.Fatal("RuntimeConfig DreamEnabled = false, want true")
	}
	if rt.DreamIdleTimeout != 90*time.Minute {
		t.Fatalf("RuntimeConfig DreamIdleTimeout = %s, want 90m", rt.DreamIdleTimeout)
	}
}

func TestGonchoDreamConfigIsolationClearsIdleTimeoutEnv(t *testing.T) {
	t.Setenv("GORMES_GONCHO_DREAM_IDLE_TIMEOUT_MINUTES", "45")
	isolateGonchoConfig(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Goncho.DreamIdleTimeoutMinutes != 60 {
		t.Fatalf("DreamIdleTimeoutMinutes after isolation = %d, want default 60", cfg.Goncho.DreamIdleTimeoutMinutes)
	}
}
