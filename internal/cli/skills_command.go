package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/TrebuchetDynamics/gormes-agent/internal/skills"
	"github.com/spf13/cobra"
)

type SkillsCommandDeps struct {
	ListInstalledSkills func(skills.ListOptions, map[string]struct{}) []skills.SkillRow
	DisabledSkills      func(platform string) map[string]struct{}
}

func NewSkillsCommand(deps SkillsCommandDeps) *cobra.Command {
	root := &cobra.Command{
		Use:   "skills",
		Short: "Manage skills",
	}
	root.AddCommand(NewSkillsListCommand(deps))
	return root
}

func NewSkillsListCommand(deps SkillsCommandDeps) *cobra.Command {
	if deps.ListInstalledSkills == nil {
		deps.ListInstalledSkills = skills.ListInstalledSkills
	}
	if deps.DisabledSkills == nil {
		deps.DisabledSkills = func(string) map[string]struct{} { return nil }
	}

	var opts skills.ListOptions
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			source := normalizedSkillsListSource(opts.Source)
			if source == "" {
				return fmt.Errorf("invalid skills list source %q", opts.Source)
			}
			opts.Source = source
			disabled := deps.DisabledSkills("")
			rows := deps.ListInstalledSkills(opts, disabled)
			return writeSkillsList(cmd, rows, opts.EnabledOnly)
		},
	}
	cmd.Flags().StringVar(&opts.Source, "source", "all", "filter by installed skill source: all, hub, builtin, or local")
	cmd.Flags().BoolVar(&opts.EnabledOnly, "enabled-only", false, "hide disabled skills")
	return cmd
}

func writeSkillsList(cmd *cobra.Command, rows []skills.SkillRow, enabledOnly bool) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "Name\tCategory\tSource\tTrust\tStatus"); err != nil {
		return err
	}

	enabledCount := 0
	disabledCount := 0
	for _, row := range rows {
		if row.Status == skills.SkillStatusDisabled {
			disabledCount++
		} else {
			enabledCount++
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.Name, row.Category, row.Source, row.Trust, row.Status); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	if enabledOnly {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%d enabled shown\n", enabledCount)
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%d enabled, %d disabled\n", enabledCount, disabledCount)
	return err
}

func normalizedSkillsListSource(source string) string {
	switch source {
	case "", "all":
		return "all"
	case "hub", "builtin", "local":
		return source
	default:
		return ""
	}
}
