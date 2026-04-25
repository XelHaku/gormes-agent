package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_GonchoDefaults(t *testing.T) {
	isolateGonchoConfig(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Goncho.Enabled {
		t.Error("Goncho.Enabled default = false, want true")
	}
	if cfg.Goncho.Workspace != "gormes" {
		t.Errorf("Goncho.Workspace default = %q, want gormes", cfg.Goncho.Workspace)
	}
	if cfg.Goncho.ObserverPeer != "gormes" {
		t.Errorf("Goncho.ObserverPeer default = %q, want gormes", cfg.Goncho.ObserverPeer)
	}
	if cfg.Goncho.RecentMessages != 4 {
		t.Errorf("Goncho.RecentMessages default = %d, want 4", cfg.Goncho.RecentMessages)
	}
	if cfg.Goncho.MaxMessageSize != 25_000 {
		t.Errorf("Goncho.MaxMessageSize default = %d, want 25000", cfg.Goncho.MaxMessageSize)
	}
	if cfg.Goncho.MaxFileSize != 5_242_880 {
		t.Errorf("Goncho.MaxFileSize default = %d, want 5242880", cfg.Goncho.MaxFileSize)
	}
	if cfg.Goncho.GetContextMaxTokens != 100_000 {
		t.Errorf("Goncho.GetContextMaxTokens default = %d, want 100000", cfg.Goncho.GetContextMaxTokens)
	}
	if !cfg.Goncho.ReasoningEnabled {
		t.Error("Goncho.ReasoningEnabled default = false, want true")
	}
	if !cfg.Goncho.PeerCardEnabled {
		t.Error("Goncho.PeerCardEnabled default = false, want true")
	}
	if !cfg.Goncho.SummaryEnabled {
		t.Error("Goncho.SummaryEnabled default = false, want true")
	}
	if cfg.Goncho.DreamEnabled {
		t.Error("Goncho.DreamEnabled default = true, want false until fixtures exist")
	}
	if cfg.Goncho.DeriverWorkers != 1 {
		t.Errorf("Goncho.DeriverWorkers default = %d, want 1", cfg.Goncho.DeriverWorkers)
	}
	if cfg.Goncho.RepresentationBatchMaxTokens != 1024 {
		t.Errorf("Goncho.RepresentationBatchMaxTokens default = %d, want 1024", cfg.Goncho.RepresentationBatchMaxTokens)
	}
	if cfg.Goncho.DialecticDefaultLevel != "low" {
		t.Errorf("Goncho.DialecticDefaultLevel default = %q, want low", cfg.Goncho.DialecticDefaultLevel)
	}
}

func TestLoad_GonchoEnvOverridesFile(t *testing.T) {
	cfgHome := isolateGonchoConfig(t)
	writeGonchoConfigFile(t, cfgHome, `
[goncho]
enabled = true
workspace = "file-workspace"
observer_peer = "file-observer"
recent_messages = 6
max_message_size = 111
max_file_size = 222
get_context_max_tokens = 333
reasoning_enabled = true
peer_card_enabled = true
summary_enabled = true
dream_enabled = false
deriver_workers = 2
representation_batch_max_tokens = 444
dialectic_default_level = "minimal"
`)

	t.Setenv("GORMES_GONCHO_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_WORKSPACE", "env-workspace")
	t.Setenv("GORMES_GONCHO_OBSERVER_PEER", "env-observer")
	t.Setenv("GORMES_GONCHO_RECENT_MESSAGES", "7")
	t.Setenv("GORMES_GONCHO_MAX_MESSAGE_SIZE", "25001")
	t.Setenv("GORMES_GONCHO_MAX_FILE_SIZE", "5242881")
	t.Setenv("GORMES_GONCHO_GET_CONTEXT_MAX_TOKENS", "99999")
	t.Setenv("GORMES_GONCHO_REASONING_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_PEER_CARD_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_SUMMARY_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_DREAM_ENABLED", "true")
	t.Setenv("GORMES_GONCHO_DERIVER_WORKERS", "3")
	t.Setenv("GORMES_GONCHO_REPRESENTATION_BATCH_MAX_TOKENS", "2048")
	t.Setenv("GORMES_GONCHO_DIALECTIC_DEFAULT_LEVEL", "high")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Goncho.Enabled {
		t.Error("Goncho.Enabled = true, want env false")
	}
	if cfg.Goncho.Workspace != "env-workspace" {
		t.Errorf("Goncho.Workspace = %q, want env-workspace", cfg.Goncho.Workspace)
	}
	if cfg.Goncho.ObserverPeer != "env-observer" {
		t.Errorf("Goncho.ObserverPeer = %q, want env-observer", cfg.Goncho.ObserverPeer)
	}
	if cfg.Goncho.RecentMessages != 7 {
		t.Errorf("Goncho.RecentMessages = %d, want 7", cfg.Goncho.RecentMessages)
	}
	if cfg.Goncho.MaxMessageSize != 25_001 {
		t.Errorf("Goncho.MaxMessageSize = %d, want 25001", cfg.Goncho.MaxMessageSize)
	}
	if cfg.Goncho.MaxFileSize != 5_242_881 {
		t.Errorf("Goncho.MaxFileSize = %d, want 5242881", cfg.Goncho.MaxFileSize)
	}
	if cfg.Goncho.GetContextMaxTokens != 99_999 {
		t.Errorf("Goncho.GetContextMaxTokens = %d, want 99999", cfg.Goncho.GetContextMaxTokens)
	}
	if cfg.Goncho.ReasoningEnabled {
		t.Error("Goncho.ReasoningEnabled = true, want env false")
	}
	if cfg.Goncho.PeerCardEnabled {
		t.Error("Goncho.PeerCardEnabled = true, want env false")
	}
	if cfg.Goncho.SummaryEnabled {
		t.Error("Goncho.SummaryEnabled = true, want env false")
	}
	if !cfg.Goncho.DreamEnabled {
		t.Error("Goncho.DreamEnabled = false, want env true")
	}
	if cfg.Goncho.DeriverWorkers != 3 {
		t.Errorf("Goncho.DeriverWorkers = %d, want 3", cfg.Goncho.DeriverWorkers)
	}
	if cfg.Goncho.RepresentationBatchMaxTokens != 2048 {
		t.Errorf("Goncho.RepresentationBatchMaxTokens = %d, want 2048", cfg.Goncho.RepresentationBatchMaxTokens)
	}
	if cfg.Goncho.DialecticDefaultLevel != "high" {
		t.Errorf("Goncho.DialecticDefaultLevel = %q, want high", cfg.Goncho.DialecticDefaultLevel)
	}
}

func TestLoad_GonchoRejectsInvalidDialecticDefaultLevel(t *testing.T) {
	cfgHome := isolateGonchoConfig(t)
	writeGonchoConfigFile(t, cfgHome, `
[goncho]
dialectic_default_level = "extreme"
`)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid dialectic_default_level error")
	}
	if !strings.Contains(err.Error(), "goncho.dialectic_default_level") {
		t.Fatalf("Load() error = %v, want goncho.dialectic_default_level", err)
	}
}

func TestLoad_GonchoRejectsNegativeLimits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "recent messages", field: "recent_messages"},
		{name: "max message size", field: "max_message_size"},
		{name: "max file size", field: "max_file_size"},
		{name: "context max tokens", field: "get_context_max_tokens"},
		{name: "deriver workers", field: "deriver_workers"},
		{name: "representation batch max tokens", field: "representation_batch_max_tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgHome := isolateGonchoConfig(t)
			writeGonchoConfigFile(t, cfgHome, "\n[goncho]\n"+tc.field+" = -1\n")

			_, err := Load(nil)
			if err == nil {
				t.Fatal("Load() error = nil, want negative limit error")
			}
			if !strings.Contains(err.Error(), "goncho."+tc.field) {
				t.Fatalf("Load() error = %v, want goncho.%s", err, tc.field)
			}
		})
	}
}

func TestLoad_GonchoToRuntimeConfig(t *testing.T) {
	isolateGonchoConfig(t)
	t.Setenv("GORMES_GONCHO_WORKSPACE", "runtime-workspace")
	t.Setenv("GORMES_GONCHO_OBSERVER_PEER", "runtime-observer")
	t.Setenv("GORMES_GONCHO_RECENT_MESSAGES", "8")
	t.Setenv("GORMES_GONCHO_MAX_MESSAGE_SIZE", "12345")
	t.Setenv("GORMES_GONCHO_MAX_FILE_SIZE", "67890")
	t.Setenv("GORMES_GONCHO_GET_CONTEXT_MAX_TOKENS", "555")
	t.Setenv("GORMES_GONCHO_REASONING_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_PEER_CARD_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_SUMMARY_ENABLED", "false")
	t.Setenv("GORMES_GONCHO_DREAM_ENABLED", "true")
	t.Setenv("GORMES_GONCHO_DERIVER_WORKERS", "4")
	t.Setenv("GORMES_GONCHO_REPRESENTATION_BATCH_MAX_TOKENS", "777")
	t.Setenv("GORMES_GONCHO_DIALECTIC_DEFAULT_LEVEL", "medium")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := cfg.Goncho.RuntimeConfig()

	if rt.WorkspaceID != "runtime-workspace" {
		t.Errorf("WorkspaceID = %q, want runtime-workspace", rt.WorkspaceID)
	}
	if rt.ObserverPeerID != "runtime-observer" {
		t.Errorf("ObserverPeerID = %q, want runtime-observer", rt.ObserverPeerID)
	}
	if rt.RecentMessages != 8 {
		t.Errorf("RecentMessages = %d, want 8", rt.RecentMessages)
	}
	if rt.MaxMessageSize != 12_345 {
		t.Errorf("MaxMessageSize = %d, want 12345", rt.MaxMessageSize)
	}
	if rt.MaxFileSize != 67_890 {
		t.Errorf("MaxFileSize = %d, want 67890", rt.MaxFileSize)
	}
	if rt.GetContextMaxTokens != 555 {
		t.Errorf("GetContextMaxTokens = %d, want 555", rt.GetContextMaxTokens)
	}
	if rt.ReasoningEnabled {
		t.Error("ReasoningEnabled = true, want false")
	}
	if rt.PeerCardEnabled {
		t.Error("PeerCardEnabled = true, want false")
	}
	if rt.SummaryEnabled {
		t.Error("SummaryEnabled = true, want false")
	}
	if !rt.DreamEnabled {
		t.Error("DreamEnabled = false, want true")
	}
	if rt.DeriverWorkers != 4 {
		t.Errorf("DeriverWorkers = %d, want 4", rt.DeriverWorkers)
	}
	if rt.RepresentationBatchMaxTokens != 777 {
		t.Errorf("RepresentationBatchMaxTokens = %d, want 777", rt.RepresentationBatchMaxTokens)
	}
	if rt.DialecticDefaultLevel != "medium" {
		t.Errorf("DialecticDefaultLevel = %q, want medium", rt.DialecticDefaultLevel)
	}
}

func isolateGonchoConfig(t *testing.T) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HERMES_HOME", "")
	for _, key := range []string{
		"GORMES_GONCHO_ENABLED",
		"GORMES_GONCHO_WORKSPACE",
		"GORMES_GONCHO_OBSERVER_PEER",
		"GORMES_GONCHO_RECENT_MESSAGES",
		"GORMES_GONCHO_MAX_MESSAGE_SIZE",
		"GORMES_GONCHO_MAX_FILE_SIZE",
		"GORMES_GONCHO_GET_CONTEXT_MAX_TOKENS",
		"GORMES_GONCHO_REASONING_ENABLED",
		"GORMES_GONCHO_PEER_CARD_ENABLED",
		"GORMES_GONCHO_SUMMARY_ENABLED",
		"GORMES_GONCHO_DREAM_ENABLED",
		"GORMES_GONCHO_DREAM_IDLE_TIMEOUT_MINUTES",
		"GORMES_GONCHO_DERIVER_WORKERS",
		"GORMES_GONCHO_REPRESENTATION_BATCH_MAX_TOKENS",
		"GORMES_GONCHO_DIALECTIC_DEFAULT_LEVEL",
	} {
		t.Setenv(key, "")
	}
	return cfgHome
}

func writeGonchoConfigFile(t *testing.T, cfgHome, body string) {
	t.Helper()
	dir := filepath.Join(cfgHome, "gormes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
