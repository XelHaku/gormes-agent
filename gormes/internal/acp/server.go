package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type Options struct {
	AgentInfo    Implementation
	NewSession   func(context.Context, string) (Session, error)
	NewSessionID func() string
}

type Server struct {
	agentInfo    Implementation
	newSession   func(context.Context, string) (Session, error)
	newSessionID func() string

	writeMu     sync.Mutex
	stateMu     sync.RWMutex
	initialized bool
	sessions    map[string]Session
}

func NewServer(opts Options) *Server {
	agentInfo := opts.AgentInfo
	if strings.TrimSpace(agentInfo.Name) == "" {
		agentInfo.Name = "gormes"
	}
	if strings.TrimSpace(agentInfo.Title) == "" {
		agentInfo.Title = "Gormes"
	}
	if strings.TrimSpace(agentInfo.Version) == "" {
		agentInfo.Version = "dev"
	}
	newID := opts.NewSessionID
	if newID == nil {
		newID = func() string { return uuid.NewString() }
	}
	return &Server{
		agentInfo:    agentInfo,
		newSession:   opts.NewSession,
		newSessionID: newID,
		sessions:     make(map[string]Session),
	}
}

func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	if s.newSession == nil {
		return errors.New("acp: NewSession is required")
	}

	var wg sync.WaitGroup
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.Method == "" {
			continue
		}
		if req.Method == "session/prompt" {
			wg.Add(1)
			go func(req rpcRequest) {
				defer wg.Done()
				s.handle(ctx, w, req)
			}(req)
			continue
		}
		s.handle(ctx, w, req)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	wg.Wait()
	s.closeSessions()
	return nil
}

func (s *Server) handle(ctx context.Context, w io.Writer, req rpcRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(w, req)
	case "session/new":
		s.handleSessionNew(ctx, w, req)
	case "session/prompt":
		s.handleSessionPrompt(ctx, w, req)
	case "session/cancel":
		s.handleSessionCancel(req)
	case "session/close":
		s.handleSessionClose(w, req)
	default:
		s.writeError(w, req.idValue(), -32601, "method not found")
	}
}

func (s *Server) handleInitialize(w io.Writer, req rpcRequest) {
	var params initializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.idValue(), -32602, "invalid initialize params")
		return
	}
	s.stateMu.Lock()
	s.initialized = true
	s.stateMu.Unlock()

	resp := map[string]any{
		"protocolVersion": ProtocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession": false,
			"promptCapabilities": map[string]any{
				"audio":           false,
				"embeddedContext": false,
				"image":           false,
			},
			"mcpCapabilities": map[string]any{
				"http": false,
				"sse":  false,
			},
			"sessionCapabilities": map[string]any{
				"close": map[string]any{},
			},
		},
		"agentInfo":   s.agentInfo,
		"authMethods": []any{},
	}
	s.writeResult(w, req.idValue(), resp)
}

func (s *Server) handleSessionNew(ctx context.Context, w io.Writer, req rpcRequest) {
	if !s.isInitialized() {
		s.writeError(w, req.idValue(), -32000, "initialize must be called before session/new")
		return
	}
	var params sessionNewParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.idValue(), -32602, "invalid session/new params")
		return
	}
	if !filepath.IsAbs(params.CWD) {
		s.writeError(w, req.idValue(), -32602, "session/new cwd must be absolute")
		return
	}
	session, err := s.newSession(ctx, params.CWD)
	if err != nil {
		s.writeError(w, req.idValue(), -32000, err.Error())
		return
	}
	sessionID := s.newSessionID()
	s.stateMu.Lock()
	s.sessions[sessionID] = session
	s.stateMu.Unlock()
	s.writeResult(w, req.idValue(), map[string]any{"sessionId": sessionID})
}

func (s *Server) handleSessionPrompt(ctx context.Context, w io.Writer, req rpcRequest) {
	if !s.isInitialized() {
		s.writeError(w, req.idValue(), -32000, "initialize must be called before session/prompt")
		return
	}
	var params sessionPromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.idValue(), -32602, "invalid session/prompt params")
		return
	}
	session, ok := s.lookupSession(params.SessionID)
	if !ok {
		s.writeError(w, req.idValue(), -32000, "unknown session")
		return
	}
	result, err := session.Prompt(ctx, params.Prompt, func(update SessionUpdate) {
		s.writeNotification(w, "session/update", map[string]any{
			"sessionId": params.SessionID,
			"update":    update,
		})
	})
	if err != nil {
		s.writeError(w, req.idValue(), -32000, err.Error())
		return
	}
	s.writeResult(w, req.idValue(), result)
}

func (s *Server) handleSessionCancel(req rpcRequest) {
	var params sessionCancelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return
	}
	session, ok := s.lookupSession(params.SessionID)
	if !ok {
		return
	}
	session.Cancel()
}

func (s *Server) handleSessionClose(w io.Writer, req rpcRequest) {
	var params sessionCloseParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.idValue(), -32602, "invalid session/close params")
		return
	}
	s.stateMu.Lock()
	session, ok := s.sessions[params.SessionID]
	if ok {
		delete(s.sessions, params.SessionID)
	}
	s.stateMu.Unlock()
	if ok {
		_ = session.Close()
	}
	s.writeResult(w, req.idValue(), map[string]any{})
}

func (s *Server) isInitialized() bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.initialized
}

func (s *Server) lookupSession(id string) (Session, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

func (s *Server) closeSessions() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for id, session := range s.sessions {
		_ = session.Close()
		delete(s.sessions, id)
	}
}

func (s *Server) writeNotification(w io.Writer, method string, params any) {
	_ = s.writeMessage(w, rpcEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (s *Server) writeResult(w io.Writer, id any, result any) {
	_ = s.writeMessage(w, rpcEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) writeError(w io.Writer, id any, code int, message string) {
	_ = s.writeMessage(w, rpcEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	})
}

func (s *Server) writeMessage(w io.Writer, msg rpcEnvelope) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := w.Write(raw); err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) idValue() any {
	if len(r.ID) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(r.ID, &v); err != nil {
		return string(r.ID)
	}
	return v
}

type rpcEnvelope struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Method  string    `json:"method,omitempty"`
	Params  any       `json:"params,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeParams struct {
	ProtocolVersion int `json:"protocolVersion"`
}

type sessionNewParams struct {
	CWD string `json:"cwd"`
}

type sessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type sessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

type sessionCloseParams struct {
	SessionID string `json:"sessionId"`
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
