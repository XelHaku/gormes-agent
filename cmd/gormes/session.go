package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/transcript"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tui"
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

func newTUISaveExportFunc() tui.SessionExportFunc {
	return func(ctx context.Context, sessionID string) (string, error) {
		dbPath := config.MemoryDBPath()
		if _, err := os.Stat(dbPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("memory database not found at %s", dbPath)
			}
			return "", err
		}

		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return "", fmt.Errorf("open transcript db: %w", err)
		}
		defer db.Close()

		out, err := transcript.ExportMarkdown(ctx, db, sessionID)
		if err != nil {
			return "", err
		}

		exportDir := filepath.Join(filepath.Dir(dbPath), "sessions", "exports")
		return writeTUISaveExport(exportDir, tuiSaveExportStem(sessionID), out)
	}
}

func writeTUISaveExport(dir, stem, markdown string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("prepare session export dir: %w", err)
	}

	for i := 0; i < 1000; i++ {
		name := stem + ".md"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.md", stem, i)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return path, fmt.Errorf("create session export: %w", err)
		}

		_, writeErr := file.WriteString(markdown)
		closeErr := file.Close()
		if writeErr != nil {
			return path, fmt.Errorf("write session export: %w", writeErr)
		}
		if closeErr != nil {
			return path, fmt.Errorf("close session export: %w", closeErr)
		}
		return path, nil
	}

	return "", fmt.Errorf("session export path collision after 1000 attempts")
}

func tuiSaveExportStem(sessionID string) string {
	stem := strings.TrimSpace(sessionID)
	stem = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', 0:
			return '_'
		default:
			return r
		}
	}, stem)
	if stem == "" {
		return "session"
	}
	return stem
}
