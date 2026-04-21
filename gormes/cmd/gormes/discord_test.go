package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

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

func TestNewRootCmd_MakesResumePersistentForDiscord(t *testing.T) {
	root := newRootCmd()
	if root.PersistentFlags().Lookup("resume") == nil {
		t.Fatal("root command missing persistent resume flag")
	}

	discord, _, err := root.Find([]string{"discord"})
	if err != nil {
		t.Fatalf("find discord command: %v", err)
	}
	if discord == nil {
		t.Fatal("discord command not found")
	}
	if discord.InheritedFlags().Lookup("resume") == nil {
		t.Fatal("discord command does not inherit resume flag")
	}
}

func TestDiscordCommand_AcceptsResumeFlagPath(t *testing.T) {
	oldRunE := discordCmd.RunE
	t.Cleanup(func() {
		discordCmd.RunE = oldRunE
	})

	var gotResume string
	discordCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		gotResume, _ = cmd.Flags().GetString("resume")
		return nil
	}

	for _, args := range [][]string{
		{"discord", "--resume", "test-sid"},
		{"--resume", "test-sid", "discord"},
	} {
		root := newRootCmd()
		root.SetArgs(args)
		gotResume = ""
		if err := root.Execute(); err != nil {
			t.Fatalf("args %v: execute failed: %v", args, err)
		}
		if gotResume != "test-sid" {
			t.Fatalf("args %v: got resume %q, want %q", args, gotResume, "test-sid")
		}
	}
}
