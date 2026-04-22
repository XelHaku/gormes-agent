package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
)

func TestSqliteStore_FinalizeAssistantTurnPersistsMetaJSON(t *testing.T) {
	path := t.TempDir() + "/memory.db"
	s, err := OpenSqlite(path, 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	payload, err := json.Marshal(map[string]any{
		"session_id": "sess-meta",
		"content":    "done",
		"ts_unix":    1745000001,
		"meta_json":  `{"tool_calls":[{"name":"echo","arguments":{"text":"hi"}}]}`,
	})
	if err != nil {
		t.Fatalf("Marshal(payload): %v", err)
	}

	if _, err := s.Exec(context.Background(), store.Command{
		Kind:    store.FinalizeAssistantTurn,
		Payload: payload,
	}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var meta string
		err := s.db.QueryRow(`SELECT COALESCE(meta_json, '') FROM turns WHERE session_id = 'sess-meta'`).Scan(&meta)
		if err == nil && meta != "" {
			if meta != `{"tool_calls":[{"name":"echo","arguments":{"text":"hi"}}]}` {
				t.Fatalf("meta_json = %q, want tool-call payload", meta)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("meta_json was not persisted within 2s")
}
