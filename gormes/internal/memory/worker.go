package memory

import (
	"context"
	"encoding/json"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// turnPayload is the shared JSON schema for AppendUserTurn and
// FinalizeAssistantTurn. See spec §7.3.
type turnPayload struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	TsUnix    int64  `json:"ts_unix"`
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
	if p.SessionID == "" || p.Content == "" {
		s.log.Warn("memory: empty session_id or content, dropping",
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
	_, err := s.db.ExecContext(context.Background(),
		"INSERT INTO turns(session_id, role, content, ts_unix) VALUES(?, ?, ?, ?)",
		p.SessionID, role, p.Content, p.TsUnix)
	if err != nil {
		s.log.Warn("memory: INSERT failed", "kind", cmd.Kind.String(), "err", err)
	}
}
