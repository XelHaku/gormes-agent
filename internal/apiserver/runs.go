package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultRunStreamTTL = 5 * time.Minute

type runRegistry struct {
	mu    sync.Mutex
	ttl   time.Duration
	now   func() time.Time
	runs  map[string]*runRecord
	swept int
}

type runRecord struct {
	id          string
	createdAt   time.Time
	events      []runEvent
	subscribers []chan runEvent
	done        bool
	consumed    bool
}

type runEvent struct {
	Event     string        `json:"event"`
	RunID     string        `json:"run_id"`
	Timestamp int64         `json:"timestamp"`
	Delta     string        `json:"delta,omitempty"`
	Output    string        `json:"output,omitempty"`
	Usage     ResponseUsage `json:"usage,omitempty"`
	Error     string        `json:"error,omitempty"`
}

func newRunRegistry(ttl time.Duration, now func() time.Time) *runRegistry {
	if ttl <= 0 {
		ttl = defaultRunStreamTTL
	}
	if now == nil {
		now = time.Now
	}
	return &runRegistry{
		ttl:  ttl,
		now:  now,
		runs: make(map[string]*runRecord),
	}
}

func (r *runRegistry) setClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	r.mu.Lock()
	r.now = now
	r.mu.Unlock()
}

func (r *runRegistry) create(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[id] = &runRecord{id: id, createdAt: r.now()}
}

func (r *runRegistry) publish(id string, ev runEvent) {
	r.mu.Lock()
	rec := r.runs[id]
	if rec == nil {
		r.mu.Unlock()
		return
	}
	rec.events = append(rec.events, ev)
	subs := append([]chan runEvent(nil), rec.subscribers...)
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (r *runRegistry) finish(id string) {
	r.mu.Lock()
	rec := r.runs[id]
	if rec == nil {
		r.mu.Unlock()
		return
	}
	rec.done = true
	subs := append([]chan runEvent(nil), rec.subscribers...)
	rec.subscribers = nil
	r.mu.Unlock()
	for _, ch := range subs {
		close(ch)
	}
}

func (r *runRegistry) subscribe(id string) ([]runEvent, <-chan runEvent, bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.runs[id]
	if rec == nil {
		return nil, nil, false, false
	}
	rec.consumed = true
	backlog := append([]runEvent(nil), rec.events...)
	if rec.done {
		return backlog, nil, true, true
	}
	ch := make(chan runEvent, 32)
	rec.subscribers = append(rec.subscribers, ch)
	return backlog, ch, true, false
}

func (r *runRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, id)
}

func (r *runRegistry) sweepOrphans() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	var swept int
	for id, rec := range r.runs {
		if rec.consumed || len(rec.subscribers) > 0 {
			continue
		}
		if now.Sub(rec.createdAt) > r.ttl {
			delete(r.runs, id)
			swept++
		}
	}
	r.swept += swept
	return swept
}

func (r *runRegistry) stats() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return map[string]any{
		"active":         len(r.runs),
		"orphaned_swept": r.swept,
		"ttl_seconds":    int(r.ttl.Seconds()),
	}
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	if s.loop == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Native turn loop is not configured", "server_error", "", "turn_loop_unavailable")
		return
	}
	body, err := readLimitedBody(w, r, s.maxBodyBytes)
	if err != nil {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "Request body too large.", "invalid_request_error", "", "body_too_large")
		return
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid JSON in request body", "invalid_request_error", "", "invalid_json")
		return
	}
	runID := "run_" + randomHexFromTime(s.now())
	turnReq, _, errResp := s.buildResponseTurnRequest(req)
	if errResp != nil {
		writeOpenAIError(w, errResp.status, errResp.message, "invalid_request_error", errResp.param, errResp.code)
		return
	}
	if explicit := stringField(req, "session_id"); explicit != "" {
		turnReq.SessionID = explicit
	} else if turnReq.SessionID == "" || strings.HasPrefix(turnReq.SessionID, "api-") {
		turnReq.SessionID = runID
	}
	s.runs.setClock(s.now)
	s.runs.sweepOrphans()
	s.runs.create(runID)
	go s.runAsyncTurn(runID, turnReq)
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": runID, "status": "started"})
}

func (s *Server) runAsyncTurn(runID string, turnReq TurnRequest) {
	now := s.now().Unix()
	s.runs.publish(runID, runEvent{Event: "run.started", RunID: runID, Timestamp: now})
	result, err := s.loop.StreamTurn(context.Background(), turnReq, StreamCallbacks{
		OnToken: func(token string) error {
			s.runs.publish(runID, runEvent{Event: "message.delta", RunID: runID, Timestamp: s.now().Unix(), Delta: token})
			return nil
		},
	})
	if err != nil {
		s.runs.publish(runID, runEvent{Event: "run.failed", RunID: runID, Timestamp: s.now().Unix(), Error: err.Error()})
		s.runs.finish(runID)
		return
	}
	s.runs.publish(runID, runEvent{
		Event:     "run.completed",
		RunID:     runID,
		Timestamp: s.now().Unix(),
		Output:    result.Content,
		Usage: ResponseUsage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	})
	s.runs.finish(runID)
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
	runID, ok := strings.CutSuffix(suffix, "/events")
	if !ok || runID == "" || strings.Contains(runID, "/") {
		writeOpenAIError(w, http.StatusNotFound, "Run not found", "invalid_request_error", "", "run_not_found")
		return
	}
	backlog, ch, exists, done := s.runs.subscribe(runID)
	if !exists {
		writeOpenAIError(w, http.StatusNotFound, "Run not found: "+runID, "invalid_request_error", "", "run_not_found")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	for _, ev := range backlog {
		writeSSEData(w, ev)
	}
	flush(w)
	if done {
		writeSSEComment(w, "stream closed")
		flush(w)
		s.runs.remove(runID)
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				writeSSEComment(w, "stream closed")
				flush(w)
				s.runs.remove(runID)
				return
			}
			writeSSEData(w, ev)
			flush(w)
		}
	}
}

func (s *Server) sweepOrphanedRuns() int {
	s.runs.setClock(s.now)
	return s.runs.sweepOrphans()
}

func (s *Server) runHealthStatus() map[string]any {
	return s.runs.stats()
}

func writeSSEComment(w http.ResponseWriter, text string) {
	_, _ = w.Write([]byte(": " + text + "\n\n"))
}
