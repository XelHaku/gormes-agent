package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/transcript"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Inspect and export persisted sessions",
}

func init() {
	sessionExportCmd.Flags().String("format", "markdown", "export format")
	sessionCmd.AddCommand(sessionExportCmd)
}

var sessionExportCmd = &cobra.Command{
	Use:   "export <session-id>",
	Short: "Export a persisted session transcript",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		if format != "markdown" {
			return fmt.Errorf("unsupported export format %q", format)
		}

		path := config.MemoryDBPath()
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("memory database not found at %s", path)
			}
			return err
		}

		db, err := sql.Open("sqlite3", path)
		if err != nil {
			return fmt.Errorf("open transcript db: %w", err)
		}
		defer db.Close()

		out, err := transcript.ExportMarkdown(context.Background(), db, args[0])
		if err != nil {
			if errors.Is(err, transcript.ErrSessionNotFound) {
				return fmt.Errorf("session %q not found", args[0])
			}
			return err
		}

		_, err = fmt.Fprint(cmd.OutOrStdout(), out)
		return err
	},
}
