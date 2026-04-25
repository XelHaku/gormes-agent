package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Inspect persisted memory and extractor state",
}

func init() {
	memoryCmd.AddCommand(memoryStatusCmd)
}

var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show extractor queue depth and dead letters",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path := config.MemoryDBPath()
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("memory database not found at %s", path)
			}
			return err
		}

		db, err := sql.Open("sqlite3", path)
		if err != nil {
			return fmt.Errorf("open memory db: %w", err)
		}
		defer db.Close()

		status, err := memory.ReadExtractorStatus(context.Background(), db, 0)
		if err != nil {
			return err
		}
		cfg, err := config.Load(nil)
		if err != nil {
			return err
		}
		gonchoCfg := cfg.Goncho.RuntimeConfig()
		queueStatus, err := goncho.ReadQueueStatus(context.Background(), db, goncho.QueueStatusConfig{
			DreamEnabled:     gonchoCfg.DreamEnabled,
			WorkspaceID:      gonchoCfg.WorkspaceID,
			ObserverPeerID:   gonchoCfg.ObserverPeerID,
			DreamIdleTimeout: gonchoCfg.DreamIdleTimeout,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), formatMemoryStatus(status, queueStatus))
		return err
	},
}

func formatMemoryStatus(status memory.ExtractorStatus, queueStatus goncho.QueueStatus) string {
	return formatExtractorStatus(status) + formatGonchoQueueStatus(queueStatus)
}

func formatExtractorStatus(status memory.ExtractorStatus) string {
	var b strings.Builder
	b.WriteString("Extractor status\n")
	b.WriteString(fmt.Sprintf("worker_health: %s\n", status.WorkerHealth))
	b.WriteString(fmt.Sprintf("queue_depth: %d\n", status.QueueDepth))
	b.WriteString(fmt.Sprintf("dead_letters: %d\n", status.DeadLetterCount))
	b.WriteString(formatDeadLetterSummary(status.ErrorSummary))
	if len(status.RecentDeadLetters) == 0 {
		b.WriteString("recent_dead_letters: none\n")
		return b.String()
	}
	b.WriteString("recent_dead_letters:\n")
	for _, dl := range status.RecentDeadLetters {
		b.WriteString(fmt.Sprintf("- turn_id=%d session_id=%s chat_id=%s attempts=%d error=%q\n",
			dl.ID, dl.SessionID, dl.ChatID, dl.Attempts, dl.Error))
	}
	return b.String()
}

func formatGonchoQueueStatus(status goncho.QueueStatus) string {
	var b strings.Builder
	b.WriteString("Goncho queue status (observability/debugging only; not synchronization; do not wait for empty queue)\n")
	for _, taskType := range goncho.QueueTaskTypes {
		counts := status.WorkUnits[taskType]
		b.WriteString(fmt.Sprintf("%s: total=%d pending=%d in_progress=%d completed=%d\n",
			taskType,
			counts.TotalWorkUnits,
			counts.PendingWorkUnits,
			counts.InProgressWorkUnits,
			counts.CompletedWorkUnits,
		))
	}
	b.WriteString(formatDreamQueueEvidence(status.Dream))
	if status.Degraded {
		if status.WorkUnits["dream"].TotalWorkUnits > 0 {
			b.WriteString("goncho_queue: degraded (dream work intent tracked locally; representation/summary workers unavailable)\n")
		} else {
			b.WriteString("goncho_queue: unavailable (zero tracked work units)\n")
		}
	}
	return b.String()
}

func formatDreamQueueEvidence(status goncho.DreamQueueStatus) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("dream_status: %s\n", status.Status))
	b.WriteString(fmt.Sprintf("dream_scheduler_table: %s\n", availableWord(status.TablePresent)))
	if len(status.Evidence) == 0 {
		b.WriteString("dream_evidence: none\n")
		return b.String()
	}
	b.WriteString("dream_evidence:\n")
	for _, item := range status.Evidence {
		b.WriteString(fmt.Sprintf("- %s: %s", item.Code, item.Reason))
		if item.ObservedPeerID != "" {
			b.WriteString(fmt.Sprintf(" observed=%s", item.ObservedPeerID))
		}
		if item.CooldownUntil > 0 {
			b.WriteString(fmt.Sprintf(" cooldown_until=%d", item.CooldownUntil))
		}
		if item.IdleUntil > 0 {
			b.WriteString(fmt.Sprintf(" idle_until=%d", item.IdleUntil))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatDeadLetterSummary(items []memory.DeadLetterErrorSummary) string {
	if len(items) == 0 {
		return "dead_letter_summary: none\n"
	}

	var b strings.Builder
	b.WriteString("dead_letter_summary:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("- error=%q count=%d\n", item.Error, item.Count))
	}
	return b.String()
}
