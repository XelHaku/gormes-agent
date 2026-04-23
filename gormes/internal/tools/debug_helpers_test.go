package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebugSessionDisabledByDefault(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	session := NewDebugSession("test_tool", DebugSessionOptions{
		EnvVar: "TEST_DEBUG_DISABLED",
		LogDir: logDir,
		NewID:  func() string { return "disabled-session" },
		Now:    func() time.Time { return time.Date(2026, 4, 23, 19, 13, 17, 0, time.UTC) },
	})

	if session.Active() {
		t.Fatal("Active() = true, want false")
	}

	session.LogCall("search", map[string]any{"query": "hello"})

	info := session.SessionInfo()
	if info.Enabled {
		t.Fatal("SessionInfo().Enabled = true, want false")
	}
	if info.SessionID != "" {
		t.Fatalf("SessionInfo().SessionID = %q, want empty", info.SessionID)
	}
	if info.LogPath != "" {
		t.Fatalf("SessionInfo().LogPath = %q, want empty", info.LogPath)
	}
	if info.TotalCalls != 0 {
		t.Fatalf("SessionInfo().TotalCalls = %d, want 0", info.TotalCalls)
	}

	if _, err := session.Save(); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}
	if _, err := os.Stat(logDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want not-exist", logDir, err)
	}
}

func TestDebugSessionSaveWritesOnlyOnDemandWithRedaction(t *testing.T) {
	t.Setenv("TEST_DEBUG_ENABLED", "TRUE")

	logDir := filepath.Join(t.TempDir(), "debug", "logs")
	nowCalls := 0
	session := NewDebugSession("test_tool", DebugSessionOptions{
		EnvVar: "TEST_DEBUG_ENABLED",
		LogDir: logDir,
		NewID:  func() string { return "session-123" },
		Now: func() time.Time {
			times := []time.Time{
				time.Date(2026, 4, 23, 19, 13, 17, 0, time.UTC),
				time.Date(2026, 4, 23, 19, 13, 19, 0, time.UTC),
			}
			if nowCalls >= len(times) {
				return times[len(times)-1]
			}
			ts := times[nowCalls]
			nowCalls++
			return ts
		},
	})

	if !session.Active() {
		t.Fatal("Active() = false, want true")
	}
	if _, err := os.Stat(logDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want not-exist before Save", logDir, err)
	}

	session.LogCall("search", map[string]any{
		"query":   "hello",
		"api_key": "super-secret",
		"nested": map[string]any{
			"authorization": "Bearer top-secret",
			"keep":          "value",
		},
		"headers": []any{
			map[string]any{"token": "nested-secret"},
			"ok",
		},
	})

	info := session.SessionInfo()
	if !info.Enabled {
		t.Fatal("SessionInfo().Enabled = false, want true")
	}
	if info.SessionID != "session-123" {
		t.Fatalf("SessionInfo().SessionID = %q, want session-123", info.SessionID)
	}
	if info.TotalCalls != 1 {
		t.Fatalf("SessionInfo().TotalCalls = %d, want 1", info.TotalCalls)
	}
	if !strings.HasSuffix(info.LogPath, filepath.Join("debug", "logs", "test_tool_debug_session-123.json")) {
		t.Fatalf("SessionInfo().LogPath = %q, want suffix %q", info.LogPath, filepath.Join("debug", "logs", "test_tool_debug_session-123.json"))
	}

	path, err := session.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if path != info.LogPath {
		t.Fatalf("Save() path = %q, want %q", path, info.LogPath)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q): %v", path, err)
	}

	var got struct {
		SessionID    string                   `json:"session_id"`
		StartTime    string                   `json:"start_time"`
		EndTime      string                   `json:"end_time"`
		DebugEnabled bool                     `json:"debug_enabled"`
		TotalCalls   int                      `json:"total_calls"`
		ToolCalls    []map[string]interface{} `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}

	if got.SessionID != "session-123" {
		t.Fatalf("session_id = %q, want session-123", got.SessionID)
	}
	if got.StartTime != "2026-04-23T19:13:17Z" {
		t.Fatalf("start_time = %q, want 2026-04-23T19:13:17Z", got.StartTime)
	}
	if got.EndTime != "2026-04-23T19:13:19Z" {
		t.Fatalf("end_time = %q, want 2026-04-23T19:13:19Z", got.EndTime)
	}
	if !got.DebugEnabled {
		t.Fatal("debug_enabled = false, want true")
	}
	if got.TotalCalls != 1 {
		t.Fatalf("total_calls = %d, want 1", got.TotalCalls)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0]["tool_name"] != "search" {
		t.Fatalf("tool_calls[0].tool_name = %v, want search", got.ToolCalls[0]["tool_name"])
	}
	if got.ToolCalls[0]["api_key"] != "[REDACTED]" {
		t.Fatalf("tool_calls[0].api_key = %v, want [REDACTED]", got.ToolCalls[0]["api_key"])
	}

	nested, ok := got.ToolCalls[0]["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool_calls[0].nested type = %T, want map[string]interface{}", got.ToolCalls[0]["nested"])
	}
	if nested["authorization"] != "[REDACTED]" {
		t.Fatalf("tool_calls[0].nested.authorization = %v, want [REDACTED]", nested["authorization"])
	}
	if nested["keep"] != "value" {
		t.Fatalf("tool_calls[0].nested.keep = %v, want value", nested["keep"])
	}

	headers, ok := got.ToolCalls[0]["headers"].([]interface{})
	if !ok || len(headers) != 2 {
		t.Fatalf("tool_calls[0].headers = %#v, want 2-entry slice", got.ToolCalls[0]["headers"])
	}
	header0, ok := headers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("tool_calls[0].headers[0] type = %T, want map[string]interface{}", headers[0])
	}
	if header0["token"] != "[REDACTED]" {
		t.Fatalf("tool_calls[0].headers[0].token = %v, want [REDACTED]", header0["token"])
	}
}
