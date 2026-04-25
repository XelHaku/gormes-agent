package skills_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/skills"
)

func TestPreprocessSkillContentSubstitutesTemplatesAndKeepsShellLiteralByDefault(t *testing.T) {
	dir := t.TempDir()
	content := "Run ${HERMES_SKILL_DIR}/scripts/do.sh in ${HERMES_SESSION_ID}; shell !`printf SHOULD_NOT_RUN`"

	got, err := skills.PreprocessSkillContent(context.Background(), content, skills.PreprocessOptions{
		SkillDir:  dir,
		SessionID: "session-123",
	})
	if err != nil {
		t.Fatalf("PreprocessSkillContent() error = %v", err)
	}

	want := "Run " + dir + "/scripts/do.sh in session-123; shell !`printf SHOULD_NOT_RUN`"
	if got != want {
		t.Fatalf("PreprocessSkillContent() = %q, want %q", got, want)
	}
	if strings.Contains(got, "shell SHOULD_NOT_RUN") {
		t.Fatalf("inline shell ran with default options: %q", got)
	}
}

func TestPreprocessSkillContentRunsBoundedInlineShellWhenEnabled(t *testing.T) {
	dir := t.TempDir()

	got, err := skills.PreprocessSkillContent(context.Background(), "Value: !`printf 123456789`", skills.PreprocessOptions{
		SkillDir:             dir,
		InlineShell:          true,
		InlineShellTimeout:   time.Second,
		InlineShellMaxOutput: 4,
	})
	if err != nil {
		t.Fatalf("PreprocessSkillContent() error = %v", err)
	}
	if got != "Value: 1234...[truncated]" {
		t.Fatalf("PreprocessSkillContent() = %q, want bounded output", got)
	}
}

func TestRuntimeBuildSkillBlockReportsUnavailableSkillsWithoutPromptInjection(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "active/enabled", `---
name: enabled-skill
description: Use deterministic vars
---

Use ${HERMES_SESSION_ID} from ${HERMES_SKILL_DIR}.`)
	writeSkill(t, root, "active/disabled", `---
name: disabled-skill
description: Must not load
---

disabled body`)
	writeSkill(t, root, "active/mac-only", `---
name: mac-only
description: Darwin only
platforms: [macos]
---

mac body`)
	writeSkill(t, root, "active/needs-key", `---
name: needs-key
description: Requires setup
required_environment_variables: [TENOR_API_KEY]
---

secret body`)
	writeSkill(t, root, "active/bad-shell", `---
name: bad-shell
description: Fails preprocessing
---

Bad !`+"`exit 7`"+``)

	runtime := skills.NewRuntime(root, 8*1024, 5, "")
	block, names, statuses, err := runtime.BuildSkillBlockWithOptions(context.Background(), "enabled disabled mac needs key bad shell", skills.RuntimeOptions{
		DisabledSkillNames: map[string]bool{"disabled-skill": true},
		Platform:           "linux",
		Env:                map[string]string{},
		Preprocess: skills.PreprocessOptions{
			SessionID:          "session-xyz",
			InlineShell:        true,
			InlineShellTimeout: time.Second,
		},
	})
	if err != nil {
		t.Fatalf("BuildSkillBlockWithOptions() error = %v", err)
	}

	if !reflect.DeepEqual(names, []string{"enabled-skill"}) {
		t.Fatalf("names = %#v, want enabled skill only", names)
	}
	for _, forbidden := range []string{"disabled body", "mac body", "secret body", "Bad !`exit 7`"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("block injected unavailable skill content %q:\n%s", forbidden, block)
		}
	}
	if !strings.Contains(block, "Use session-xyz from "+filepath.Join(root, "active", "enabled")+".") {
		t.Fatalf("block did not contain preprocessed enabled skill:\n%s", block)
	}

	gotStatuses := statusByName(statuses)
	wantStatuses := map[string]skills.SkillStatusCode{
		"enabled-skill":  skills.SkillStatusAvailable,
		"disabled-skill": skills.SkillStatusDisabled,
		"mac-only":       skills.SkillStatusUnsupported,
		"needs-key":      skills.SkillStatusMissingPrerequisite,
		"bad-shell":      skills.SkillStatusPreprocessingFailed,
	}
	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Fatalf("statuses = %#v, want %#v", gotStatuses, wantStatuses)
	}
}

func TestSkillSlashCommandsSkipUnavailableSkillsAndBuildStableMessage(t *testing.T) {
	root := t.TempDir()
	mediaDir := writeSkill(t, root, "active/media", `---
name: Jellyfin + Jellystat 24h Summary
description: Summarize media usage
---

Run ${HERMES_SKILL_DIR}/scripts/report.sh.`)
	writeSkill(t, root, "active/disabled", `---
name: disabled-skill
description: Must not be invokable
---

disabled body`)
	writeSkill(t, root, "active/mac-only", `---
name: mac-only
description: Darwin only
platforms: [macos]
---

mac body`)

	runtime := skills.NewRuntime(root, 8*1024, 5, "")
	commands, statuses, err := runtime.SkillSlashCommands(context.Background(), skills.RuntimeOptions{
		DisabledSkillNames: map[string]bool{"disabled-skill": true},
		Platform:           "linux",
	})
	if err != nil {
		t.Fatalf("SkillSlashCommands() error = %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("len(commands) = %d, want 1: %#v", len(commands), commands)
	}
	cmd := commands[0]
	if cmd.Command != "/jellyfin-jellystat-24h-summary" {
		t.Fatalf("command key = %q, want sanitized skill command", cmd.Command)
	}
	if statusByName(statuses)["disabled-skill"] != skills.SkillStatusDisabled {
		t.Fatalf("disabled skill status not reported: %#v", statuses)
	}
	if _, ok := skills.ResolveSkillSlashCommand(commands, "jellyfin_jellystat_24h_summary"); !ok {
		t.Fatalf("underscore command form did not resolve")
	}
	if _, ok := skills.ResolveSkillSlashCommand(commands, "disabled-skill"); ok {
		t.Fatalf("disabled skill resolved as a slash command")
	}

	message := skills.BuildSkillSlashCommandMessage(cmd, "compose now", skills.SlashMessageOptions{
		RuntimeNote: "telegram",
	})
	for _, want := range []string{
		`[SYSTEM: The user has invoked the "Jellyfin + Jellystat 24h Summary" skill`,
		"Run " + mediaDir + "/scripts/report.sh.",
		"[Skill directory: " + mediaDir + "]",
		"The user has provided the following instruction alongside the skill invocation: compose now",
		"[Runtime note: telegram]",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "disabled body") || strings.Contains(message, "mac body") {
		t.Fatalf("message injected unavailable skill content:\n%s", message)
	}

	extras := []gateway.PlatformCommand{{Name: strings.TrimPrefix(cmd.Command, "/"), Description: cmd.Description}}
	tg1 := gateway.TelegramBotCommandsWith(extras)
	tg2 := gateway.TelegramBotCommandsWith(extras)
	if !reflect.DeepEqual(tg1, tg2) {
		t.Fatalf("TelegramBotCommandsWith unstable:\n%#v\n%#v", tg1, tg2)
	}
	if !platformCommandsContain(tg1, "jellyfin_jellystat_24h_summary") {
		t.Fatalf("TelegramBotCommandsWith missing sanitized skill command: %#v", tg1)
	}
	slack := gateway.SlackSubcommandMapWith(extras)
	if slack["jellyfin-jellystat-24h-summary"] != "/jellyfin-jellystat-24h-summary" {
		t.Fatalf("SlackSubcommandMapWith missing skill command: %#v", slack)
	}
	if _, ok := slack["disabled-skill"]; ok {
		t.Fatalf("SlackSubcommandMapWith exposed disabled skill: %#v", slack)
	}
}

func writeSkill(t *testing.T, root, rel, raw string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", dir, err)
	}
	return dir
}

func statusByName(statuses []skills.SkillStatus) map[string]skills.SkillStatusCode {
	out := make(map[string]skills.SkillStatusCode, len(statuses))
	for _, status := range statuses {
		out[status.Name] = status.Status
	}
	return out
}

func platformCommandsContain(commands []gateway.PlatformCommand, name string) bool {
	for _, cmd := range commands {
		if cmd.Name == name {
			return true
		}
	}
	return false
}
