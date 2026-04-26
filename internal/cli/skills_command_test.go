package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/skills"
	"github.com/spf13/cobra"
)

func TestSkillsListCommand_RendersStatusColumnAndSummary(t *testing.T) {
	rows := []skills.SkillRow{
		{Name: "hub-skill", Category: "x", Source: "hub", Trust: "community", Status: "disabled"},
		{Name: "builtin-skill", Category: "x", Source: "builtin", Trust: "builtin", Status: "enabled"},
		{Name: "local-skill", Category: "x", Source: "local", Trust: "local", Status: "enabled"},
	}
	cmd := NewSkillsCommand(SkillsCommandDeps{
		ListInstalledSkills: func(opts skills.ListOptions, disabled map[string]struct{}) []skills.SkillRow {
			if opts.Source != "all" || opts.EnabledOnly {
				t.Fatalf("opts = %+v, want source=all enabledOnly=false", opts)
			}
			if _, ok := disabled["hub-skill"]; !ok {
				t.Fatalf("disabled set = %#v, want hub-skill", disabled)
			}
			return rows
		},
		DisabledSkills: func(platform string) map[string]struct{} {
			if platform != "" {
				t.Fatalf("platform = %q, want empty", platform)
			}
			return map[string]struct{}{"hub-skill": {}}
		},
	})

	stdout, err := executeSkillsCommand(cmd, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"Name", "Category", "Source", "Trust", "Status", "hub-skill", "disabled", "2 enabled, 1 disabled"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestSkillsListCommand_RendersEnabledOnlySummary(t *testing.T) {
	var gotOpts skills.ListOptions
	cmd := NewSkillsCommand(SkillsCommandDeps{
		ListInstalledSkills: func(opts skills.ListOptions, disabled map[string]struct{}) []skills.SkillRow {
			gotOpts = opts
			return []skills.SkillRow{
				{Name: "builtin-skill", Category: "x", Source: "builtin", Trust: "builtin", Status: "enabled"},
				{Name: "local-skill", Category: "x", Source: "local", Trust: "local", Status: "enabled"},
			}
		},
		DisabledSkills: func(platform string) map[string]struct{} {
			return map[string]struct{}{"hub-skill": {}}
		},
	})

	stdout, err := executeSkillsCommand(cmd, "list", "--source", "builtin", "--enabled-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantOpts := skills.ListOptions{Source: "builtin", EnabledOnly: true}
	if !reflect.DeepEqual(gotOpts, wantOpts) {
		t.Fatalf("opts = %+v, want %+v", gotOpts, wantOpts)
	}
	if strings.Contains(stdout, "disabled") {
		t.Fatalf("enabled-only stdout mentioned disabled rows:\n%s", stdout)
	}
	if !strings.Contains(stdout, "2 enabled shown") {
		t.Fatalf("stdout missing enabled-only summary:\n%s", stdout)
	}
}

func TestSkillsListCommand_PlatformArgNotPropagated(t *testing.T) {
	t.Setenv("HERMES_PLATFORM", "telegram")
	var seenPlatform string
	cmd := NewSkillsCommand(SkillsCommandDeps{
		ListInstalledSkills: func(opts skills.ListOptions, disabled map[string]struct{}) []skills.SkillRow {
			return nil
		},
		DisabledSkills: func(platform string) map[string]struct{} {
			seenPlatform = platform
			return nil
		},
	})

	if _, err := executeSkillsCommand(cmd, "list"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if seenPlatform != "" {
		t.Fatalf("disabled skills resolver saw platform %q, want empty", seenPlatform)
	}
}

func executeSkillsCommand(cmd *cobra.Command, args ...string) (string, error) {
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}
