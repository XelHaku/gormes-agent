package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/skills"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Sync, list, and install skills from the local hub cache",
}

var skillsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local hub catalog directories into the install cache",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(nil)
		if err != nil {
			return err
		}
		hub := skills.NewHub(cfg.SkillsRoot(), cfg.Skills.MaxDocumentBytes)
		lock, err := hub.SyncLocalCatalogs(cmd.Context(), time.Now().UTC())
		if err != nil {
			return err
		}
		if len(lock.Skills) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "synced 0 skills")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "synced %d skills\n", len(lock.Skills))
		for _, skill := range lock.Skills {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", skill.Ref, skill.Description)
		}
		return nil
	},
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills or hub catalog entries",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(nil)
		if err != nil {
			return err
		}
		source, _ := cmd.Flags().GetString("source")
		hub := skills.NewHub(cfg.SkillsRoot(), cfg.Skills.MaxDocumentBytes)

		switch strings.ToLower(strings.TrimSpace(source)) {
		case "", "installed":
			items, err := hub.ListInstalled()
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Ref != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.Name, item.Description, item.Ref)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", item.Name, item.Description)
			}
			return nil
		case "hub":
			lock, err := hub.LoadLock()
			if err != nil {
				return err
			}
			if len(lock.Skills) == 0 {
				lock, err = hub.SyncLocalCatalogs(cmd.Context(), time.Now().UTC())
				if err != nil {
					return err
				}
			}
			for _, skill := range lock.Skills {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", skill.Ref, skill.Description)
			}
			return nil
		default:
			return fmt.Errorf("unsupported skills list source %q", source)
		}
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install <ref>",
	Short: "Install a synced hub skill into the active skills store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(nil)
		if err != nil {
			return err
		}
		hub := skills.NewHub(cfg.SkillsRoot(), cfg.Skills.MaxDocumentBytes)
		lock, err := hub.LoadLock()
		if err != nil {
			return err
		}
		if len(lock.Skills) == 0 {
			if _, err := hub.SyncLocalCatalogs(cmd.Context(), time.Now().UTC()); err != nil {
				return err
			}
		}

		meta, err := hub.Install(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n", meta.Ref)
		return nil
	},
}

func init() {
	skillsListCmd.Flags().String("source", "installed", "list installed skills or the synced hub catalog")
	skillsCmd.AddCommand(skillsSyncCmd, skillsListCmd, skillsInstallCmd)
}
