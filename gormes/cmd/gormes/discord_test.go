package main

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestValidateDiscordConfig_RejectsMissingToken(t *testing.T) {
	err := validateDiscordConfig(config.Config{})
	if err == nil || !strings.Contains(err.Error(), "no Discord bot token") {
		t.Fatalf("err = %v, want missing token error", err)
	}
}

func TestValidateDiscordConfig_RejectsGuildWithoutChannel(t *testing.T) {
	cfg := config.Config{}
	cfg.Discord.BotToken = "discord-token"
	cfg.Discord.AllowedGuildID = "guild-1"
	err := validateDiscordConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "allowed_channel_id") {
		t.Fatalf("err = %v, want allowed_channel_id validation error", err)
	}
}

func TestNewRootCmd_RegistersDiscord(t *testing.T) {
	root := newRootCmd()
	if root.Commands()[0] == nil {
		t.Fatal("root has no subcommands")
	}
	names := make(map[string]bool)
	for _, cmd := range root.Commands() {
		names[cmd.Name()] = true
	}
	if !names["discord"] {
		t.Fatal("root command missing discord subcommand")
	}
}
