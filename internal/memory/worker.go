package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
)

const (
	memorySyncPending     = "pending"
	memorySyncReady       = "ready"
	memorySyncSkipped     = "skipped"
	memorySyncInterrupted = "interrupted"
)

// turnPayload is the shared JSON schema for AppendUserTurn and
// FinalizeAssistantTurn. See spec §7.3.
type turnPayload struct {
	SessionID        string `json:"session_id"`
	Content          string `json:"content"`
	TsUnix           int64  `json:"ts_unix"`
	ChatID           string `json:"chat_id"`     // new in 3.C; empty string for non-scoped turns
	Cron             int    `json:"cron"`        // 0 when absent = non-cron turn
	CronJobID        string `json:"cron_job_id"` // "" when absent -> NULL via nullIfEmpty
	MetaJSON         string `json:"meta_json"`
	TurnKey          string `json:"turn_key"`
	MemorySyncStatus string `json:"memory_sync_status"`
	MemorySyncReason string `json:"memory_sync_reason"`
}

// SkipMemorySync records that a kernel turn reached finalization through an
// interrupt/cancel path. It intentionally reuses the existing store queue so
// the update is ordered after the pre-stream AppendUserTurn insert.
func (s *SqliteStore) SkipMemorySync(ctx context.Context, turnKey, reason string) error {
	turnKey = strings.TrimSpace(turnKey)
	if turnKey == "" {
		return nil
	}
	reason = normalizeMemorySyncReason(reason)
	if reason == "" {
		reason = memorySyncInterrupted
	}
	payload, _ := json.Marshal(map[string]any{
		"turn_key":           turnKey,
		"memory_sync_status": memorySyncSkipped,
		"memory_sync_reason": reason,
	})
	_, err := s.Exec(ctx, store.Command{Kind: store.FinalizeAssistantTurn, Payload: payload})
	return err
}

// run is the worker loop. Exactly one goroutine owns s.db.
func (s *SqliteStore) run() {
	defer close(s.done)
	for cmd := range s.queue {
		s.handleCommand(cmd)
	}
}

func (s *SqliteStore) handleCommand(cmd store.Command) {
	var p turnPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		s.log.Warn("memory: malformed payload, dropping",
			"kind", cmd.Kind.String(), "err", err)
		return
	}
	if cmd.Kind == store.FinalizeAssistantTurn && normalizeMemorySyncStatus(p.MemorySyncStatus) == memorySyncSkipped {
		if p.TurnKey == "" {
			s.log.Warn("memory: skipped sync marker missing turn_key")
			return
		}
		reason := normalizeMemorySyncReason(p.MemorySyncReason)
		if reason == "" {
			reason = memorySyncInterrupted
		}
		if err := s.markMemorySyncSkipped(context.Background(), p.TurnKey, reason); err != nil {
			s.log.Warn("memory: mark sync skipped failed", "err", err)
		}
		return
	}
	if p.Content == "" {
		s.log.Warn("memory: empty content, dropping",
			"kind", cmd.Kind.String())
		return
	}
	var role string
	switch cmd.Kind {
	case store.AppendUserTurn:
		role = "user"
	case store.FinalizeAssistantTurn:
		role = "assistant"
	default:
		s.log.Warn("memory: unknown command kind, dropping", "kind", cmd.Kind.String())
		return
	}
	syncStatus := normalizeMemorySyncStatus(p.MemorySyncStatus)
	if syncStatus == "" {
		if cmd.Kind == store.AppendUserTurn && strings.TrimSpace(p.TurnKey) != "" {
			syncStatus = memorySyncPending
		} else {
			syncStatus = memorySyncReady
		}
	}
	syncReason := normalizeMemorySyncReason(p.MemorySyncReason)
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO turns(
			session_id, role, content, ts_unix, chat_id, cron, cron_job_id,
			meta_json, turn_key, memory_sync_status, memory_sync_reason
		)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SessionID, role, p.Content, p.TsUnix, p.ChatID, p.Cron,
		nullIfEmpty(p.CronJobID), nullIfEmpty(p.MetaJSON), nullIfEmpty(p.TurnKey),
		syncStatus, nullIfEmpty(syncReason))
	if err != nil {
		s.log.Warn("memory: INSERT failed", "kind", cmd.Kind.String(), "err", err)
		return
	}
	if cmd.Kind == store.FinalizeAssistantTurn && strings.TrimSpace(p.TurnKey) != "" && syncStatus == memorySyncReady {
		if err := s.markMemorySyncReady(context.Background(), p.TurnKey); err != nil {
			s.log.Warn("memory: mark sync ready failed", "err", err)
		}
	}
}

// nullIfEmpty returns nil for empty strings so the database sees a
// SQL NULL. Used by AppendUserTurn's cron_job_id column write — non-
// cron turns omit the field; writing "" instead of NULL would violate
// the idiomatic "NULL means unset" expectation for downstream readers.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *SqliteStore) markMemorySyncReady(ctx context.Context, turnKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE turns
		 SET memory_sync_status = 'ready', memory_sync_reason = NULL
		 WHERE turn_key = ? AND memory_sync_status = 'pending'`,
		turnKey)
	return err
}

func (s *SqliteStore) markMemorySyncSkipped(ctx context.Context, turnKey, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE turns
		 SET memory_sync_status = 'skipped', memory_sync_reason = ?
		 WHERE turn_key = ? AND memory_sync_status = 'pending'`,
		reason, turnKey)
	return err
}

func normalizeMemorySyncStatus(status string) string {
	switch strings.TrimSpace(status) {
	case memorySyncPending:
		return memorySyncPending
	case memorySyncReady:
		return memorySyncReady
	case memorySyncSkipped:
		return memorySyncSkipped
	default:
		return ""
	}
}

func normalizeMemorySyncReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case memorySyncInterrupted:
		return memorySyncInterrupted
	case "cancelled":
		return "cancelled"
	case "client_disconnect":
		return "client_disconnect"
	default:
		return ""
	}
}
