package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscordToolsetConfigLoadsServerActionAllowlist(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	cfgDir := filepath.Join(cfgHome, "gormes")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(`
[discord]
token = "bot-abc"
server_actions = ["list_guilds", "fetch_messages"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Discord.ServerActions, []string{"list_guilds", "fetch_messages"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Discord.ServerActions from file = %v, want %v", got, want)
	}

	t.Setenv("GORMES_DISCORD_SERVER_ACTIONS", " pin_message, add_role ,, remove_role ")
	cfg, err = Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Discord.ServerActions, []string{"pin_message", "add_role", "remove_role"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Discord.ServerActions from env = %v, want %v", got, want)
	}
}
