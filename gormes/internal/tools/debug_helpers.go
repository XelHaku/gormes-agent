package tools

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const redactedValue = "[REDACTED]"

// DebugSessionOptions configures a DebugSession.
type DebugSessionOptions struct {
	EnvVar string
	LogDir string
	NewID  func() string
	Now    func() time.Time
}

// DebugSessionInfo is a lightweight summary for callers that need to expose
// the current debug-session state without forcing a file write.
type DebugSessionInfo struct {
	Enabled    bool   `json:"enabled"`
	SessionID  string `json:"session_id"`
	LogPath    string `json:"log_path"`
	TotalCalls int    `json:"total_calls"`
}

// DebugSession records per-tool diagnostic entries when a tool-specific env
// toggle is enabled. It stays inert until then and does not write to disk
// until Save is called.
type DebugSession struct {
	toolName  string
	enabled   bool
	sessionID string
	logDir    string
	startTime time.Time
	now       func() time.Time
	calls     []map[string]any
}

// NewDebugSession constructs a new per-tool debug session.
func NewDebugSession(toolName string, opts DebugSessionOptions) *DebugSession {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	logDir := strings.TrimSpace(opts.LogDir)
	if logDir == "" {
		logDir = filepath.Join(xdgDataHome(), "gormes", "logs")
	}

	s := &DebugSession{
		toolName: strings.TrimSpace(toolName),
		enabled:  strings.EqualFold(strings.TrimSpace(os.Getenv(strings.TrimSpace(opts.EnvVar))), "true"),
		logDir:   logDir,
		now:      nowFn,
	}
	if !s.enabled {
		return s
	}

	newID := opts.NewID
	if newID == nil {
		newID = newDebugSessionID
	}
	s.sessionID = strings.TrimSpace(newID())
	if s.sessionID == "" {
		s.sessionID = newDebugSessionID()
	}
	s.startTime = nowFn()
	return s
}

// Active reports whether the debug session is currently enabled.
func (s *DebugSession) Active() bool {
	return s != nil && s.enabled
}

// LogCall appends a redacted tool-call entry to the in-memory log.
func (s *DebugSession) LogCall(callName string, callData map[string]any) {
	if !s.Active() {
		return
	}

	entry := sanitizeDebugFields(callData)
	entry["timestamp"] = s.now().Format(time.RFC3339)
	entry["tool_name"] = strings.TrimSpace(callName)
	s.calls = append(s.calls, entry)
}

// Save persists the current log to disk. Disabled sessions are a no-op.
func (s *DebugSession) Save() (string, error) {
	if !s.Active() {
		return "", nil
	}

	path := s.logPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	payload := struct {
		SessionID    string           `json:"session_id"`
		StartTime    string           `json:"start_time"`
		EndTime      string           `json:"end_time"`
		DebugEnabled bool             `json:"debug_enabled"`
		TotalCalls   int              `json:"total_calls"`
		ToolCalls    []map[string]any `json:"tool_calls"`
	}{
		SessionID:    s.sessionID,
		StartTime:    s.startTime.UTC().Format(time.RFC3339),
		EndTime:      s.now().Format(time.RFC3339),
		DebugEnabled: true,
		TotalCalls:   len(s.calls),
		ToolCalls:    cloneDebugCalls(s.calls),
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// SessionInfo returns the current session summary without writing the log.
func (s *DebugSession) SessionInfo() DebugSessionInfo {
	if !s.Active() {
		return DebugSessionInfo{}
	}
	return DebugSessionInfo{
		Enabled:    true,
		SessionID:  s.sessionID,
		LogPath:    s.logPath(),
		TotalCalls: len(s.calls),
	}
}

func (s *DebugSession) logPath() string {
	return filepath.Join(s.logDir, s.toolName+"_debug_"+s.sessionID+".json")
}

func redactDebugValue(key string, value any) any {
	if isSensitiveDebugKey(key) {
		return redactedValue
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for childKey, childValue := range v {
			out[childKey] = redactDebugValue(childKey, childValue)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = redactDebugValue("", child)
		}
		return out
	default:
		return value
	}
}

func sanitizeDebugFields(callData map[string]any) map[string]any {
	if len(callData) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(callData))
	for key, value := range callData {
		out[key] = redactDebugValue(key, value)
	}
	return out
}

func cloneDebugCalls(calls []map[string]any) []map[string]any {
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, sanitizeDebugFields(call))
	}
	return out
}

func isSensitiveDebugKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "api_key", "apikey", "token", "access_token", "refresh_token", "authorization", "password", "secret", "cookie":
		return true
	default:
		return false
	}
}

func newDebugSessionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(buf[:])
}
