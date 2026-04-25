package apiserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultModelName              = "gormes-agent"
	defaultMaxRequestBytes  int64 = 1_000_000
	maxNormalizedTextLength       = 65_536
	maxContentListSize            = 1_000
)

// Config wires the native API server HTTP surface.
type Config struct {
	APIKey        string
	ModelName     string
	MaxBodyBytes  int64
	Loop          TurnLoop
	ResponseStore *ResponseStore
	RunTTL        time.Duration
}

// Server exposes the OpenAI-compatible HTTP routes that can be mounted by the
// gateway binary.
type Server struct {
	apiKey                 string
	modelName              string
	maxBodyBytes           int64
	loop                   TurnLoop
	responseStore          *ResponseStore
	runs                   *runRegistry
	statusMu               sync.Mutex
	previousResponseMisses int
	now                    func() time.Time
	mux                    *http.ServeMux
}

// ChatMessage is the normalized text shape passed from HTTP into gateway turns.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is the OpenAI function-call metadata preserved in response chains.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TurnRequest is the chat-completions request after OpenAI message/content
// normalization and session-handle resolution.
type TurnRequest struct {
	Model        string
	UserMessage  string
	History      []ChatMessage
	SystemPrompt string
	SessionID    string
}

// Usage is the OpenAI-compatible token accounting shape used by both normal
// and streaming chat-completion responses.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// TurnResult is the native turn-loop result consumed by HTTP response writers.
type TurnResult struct {
	Content      string
	SessionID    string
	Usage        Usage
	FinishReason string
	Messages     []ChatMessage
}

// StreamCallbacks receives token deltas from a streaming native turn.
type StreamCallbacks struct {
	OnToken func(string) error
}

// TurnLoop is the minimal adapter seam between HTTP and the native Gormes turn
// loop. NewKernelTurnLoop provides the production implementation.
type TurnLoop interface {
	RunTurn(ctx context.Context, req TurnRequest) (TurnResult, error)
	StreamTurn(ctx context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error)
}

// NewServer constructs the route set without binding a socket.
func NewServer(cfg Config) *Server {
	model := strings.TrimSpace(cfg.ModelName)
	if model == "" {
		model = defaultModelName
	}
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxRequestBytes
	}
	responseStore := cfg.ResponseStore
	if responseStore == nil {
		responseStore = NewResponseStore(defaultMaxStoredResponses)
	}
	runTTL := cfg.RunTTL
	if runTTL <= 0 {
		runTTL = defaultRunStreamTTL
	}
	s := &Server{
		apiKey:        cfg.APIKey,
		modelName:     model,
		maxBodyBytes:  maxBody,
		loop:          cfg.Loop,
		responseStore: responseStore,
		runs:          newRunRegistry(runTTL, time.Now),
		now:           time.Now,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns an http.Handler suitable for httptest or http.Server.
func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/health", s.handleHealth)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/responses", s.handleResponses)
	s.mux.HandleFunc("/v1/responses/", s.handleResponseByID)
	s.mux.HandleFunc("/v1/runs", s.handleRuns)
	s.mux.HandleFunc("/v1/runs/", s.handleRunEvents)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"platform":  "gormes-agent",
		"responses": s.responseHealthStatus(),
		"runs":      s.runHealthStatus(),
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":         s.modelName,
				"object":     "model",
				"created":    s.now().Unix(),
				"owned_by":   "gormes",
				"permission": []any{},
				"root":       s.modelName,
				"parent":     nil,
			},
		},
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
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
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || errors.Is(err, errBodyTooLarge) {
			writeOpenAIError(w, http.StatusRequestEntityTooLarge, "Request body too large.", "invalid_request_error", "", "body_too_large")
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "", "invalid_request_body")
		return
	}

	var bodyReq chatCompletionRequest
	if err := json.Unmarshal(body, &bodyReq); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid JSON in request body", "invalid_request_error", "", "invalid_json")
		return
	}
	if len(bodyReq.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Missing or invalid 'messages' field", "invalid_request_error", "messages", "invalid_messages")
		return
	}

	turnReq, errResp := s.buildTurnRequest(r, bodyReq)
	if errResp != nil {
		writeOpenAIError(w, errResp.status, errResp.message, "invalid_request_error", errResp.param, errResp.code)
		return
	}
	model := strings.TrimSpace(bodyReq.Model)
	if model == "" {
		model = s.modelName
	}
	turnReq.Model = model

	completionID := "chatcmpl-" + randomHexFromTime(s.now())
	created := s.now().Unix()
	if bodyReq.Stream {
		s.writeStreamingChatCompletion(w, r, completionID, created, model, turnReq)
		return
	}

	result, err := s.loop.RunTurn(r.Context(), turnReq)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "turn_failed")
		return
	}
	sessionID := result.SessionID
	if sessionID == "" {
		sessionID = turnReq.SessionID
	}
	if sessionID != "" {
		w.Header().Set("X-Hermes-Session-Id", sessionID)
	}
	writeJSON(w, http.StatusOK, chatCompletionResponse(completionID, created, model, result))
}

func (s *Server) writeStreamingChatCompletion(w http.ResponseWriter, r *http.Request, completionID string, created int64, model string, turnReq TurnRequest) {
	sessionID := turnReq.SessionID
	if sessionID != "" {
		w.Header().Set("X-Hermes-Session-Id", sessionID)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeSSEData(w, chatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChunkChoice{{
			Index: 0,
			Delta: map[string]string{"role": "assistant"},
		}},
	})
	flush(w)

	result, err := s.loop.StreamTurn(r.Context(), turnReq, StreamCallbacks{
		OnToken: func(token string) error {
			writeSSEData(w, chatCompletionChunk{
				ID:      completionID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []chatCompletionChunkChoice{{
					Index: 0,
					Delta: map[string]string{"content": token},
				}},
			})
			flush(w)
			return nil
		},
	})
	if err != nil {
		writeSSEEvent(w, "error", openAIErrorEnvelope("Internal server error: "+err.Error(), "server_error", "", "stream_failed"))
		writeSSEDone(w)
		flush(w)
		return
	}
	if result.SessionID != "" && sessionID == "" {
		// Header-phase streaming cannot publish a late provider session handle,
		// but keeping this branch documents the intended continuity fallback.
		sessionID = result.SessionID
	}
	writeSSEData(w, chatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChunkChoice{{
			Index:        0,
			Delta:        map[string]string{},
			FinishReason: stringPtr("stop"),
		}},
		Usage: usagePayload(result.Usage),
	})
	writeSSEDone(w)
	flush(w)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.apiKey == "" {
		return true
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if hmac.Equal([]byte(token), []byte(s.apiKey)) {
			return true
		}
	}
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return hmac.Equal([]byte(key), []byte(s.apiKey))
	}
	return false
}

type chatCompletionRequest struct {
	Model    string            `json:"model"`
	Messages []incomingMessage `json:"messages"`
	Stream   bool              `json:"stream"`
}

type incomingMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type requestError struct {
	status  int
	message string
	param   string
	code    string
}

func (s *Server) buildTurnRequest(r *http.Request, req chatCompletionRequest) (TurnRequest, *requestError) {
	var (
		systemParts  []string
		conversation []ChatMessage
		firstUser    string
	)
	for idx, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content, err := normalizeChatContent(msg.Content)
		if err != nil {
			return TurnRequest{}, &requestError{
				status:  http.StatusBadRequest,
				message: err.message,
				param:   fmt.Sprintf("messages[%d].content", idx),
				code:    err.code,
			}
		}
		switch role {
		case "system", "developer":
			if strings.TrimSpace(content) != "" {
				systemParts = append(systemParts, content)
			}
		case "user", "assistant":
			conversation = append(conversation, ChatMessage{Role: role, Content: content})
			if role == "user" && firstUser == "" {
				firstUser = content
			}
		}
	}

	lastUser := -1
	for i := len(conversation) - 1; i >= 0; i-- {
		if conversation[i].Role == "user" {
			lastUser = i
			break
		}
	}
	if lastUser < 0 || !hasVisibleText(conversation[lastUser].Content) {
		return TurnRequest{}, &requestError{
			status:  http.StatusBadRequest,
			message: "No user message found in messages",
			code:    "missing_user_message",
		}
	}

	systemPrompt := strings.Join(systemParts, "\n")
	sessionID := strings.TrimSpace(r.Header.Get("X-Hermes-Session-Id"))
	if strings.ContainsAny(sessionID, "\r\n\x00") {
		return TurnRequest{}, &requestError{
			status:  http.StatusBadRequest,
			message: "Invalid session ID",
			param:   "X-Hermes-Session-Id",
			code:    "invalid_session_id",
		}
	}
	if sessionID == "" {
		sessionID = deriveChatSessionID(systemPrompt, firstUser)
	}

	return TurnRequest{
		UserMessage:  conversation[lastUser].Content,
		History:      append([]ChatMessage(nil), conversation[:lastUser]...),
		SystemPrompt: systemPrompt,
		SessionID:    sessionID,
	}, nil
}

type contentNormalizeError struct {
	code    string
	message string
}

func normalizeChatContent(content any) (string, *contentNormalizeError) {
	return normalizeChatContentDepth(content, 0)
}

func normalizeChatContentDepth(content any, depth int) (string, *contentNormalizeError) {
	if depth > 10 || content == nil {
		return "", nil
	}
	switch v := content.(type) {
	case string:
		return truncateText(v), nil
	case []any:
		limit := len(v)
		if limit > maxContentListSize {
			limit = maxContentListSize
		}
		parts := make([]string, 0, limit)
		total := 0
		for _, item := range v[:limit] {
			var text string
			switch p := item.(type) {
			case string:
				text = p
			case []any:
				nested, err := normalizeChatContentDepth(p, depth+1)
				if err != nil {
					return "", err
				}
				text = nested
			case map[string]any:
				partText, err := normalizeContentPart(p)
				if err != nil {
					return "", err
				}
				text = partText
			default:
				continue
			}
			if text == "" {
				continue
			}
			trimmed := truncateText(text)
			parts = append(parts, trimmed)
			total += len(trimmed)
			if total >= maxNormalizedTextLength {
				break
			}
		}
		return truncateText(strings.Join(parts, "\n")), nil
	default:
		return truncateText(fmt.Sprint(v)), nil
	}
}

func normalizeContentPart(part map[string]any) (string, *contentNormalizeError) {
	rawType, ok := part["type"]
	partType := ""
	if ok && rawType != nil {
		partType = strings.ToLower(strings.TrimSpace(fmt.Sprint(rawType)))
	}
	switch partType {
	case "text", "input_text", "output_text":
		text, ok := part["text"]
		if !ok || text == nil {
			return "", nil
		}
		return fmt.Sprint(text), nil
	case "image_url", "input_image":
		return "", nil
	case "file", "input_file":
		return "", &contentNormalizeError{
			code:    "unsupported_content_type",
			message: "Uploaded files and document inputs are not supported on this endpoint.",
		}
	case "":
		return "", &contentNormalizeError{
			code:    "invalid_content_part",
			message: "Content parts must include a type.",
		}
	default:
		return "", &contentNormalizeError{
			code:    "unsupported_content_type",
			message: fmt.Sprintf("Unsupported content part type %q. Only text and image_url/input_image parts are supported.", part["type"]),
		}
	}
}

func truncateText(s string) string {
	if len(s) <= maxNormalizedTextLength {
		return s
	}
	return s[:maxNormalizedTextLength]
}

func hasVisibleText(s string) bool {
	return strings.TrimSpace(s) != ""
}

func deriveChatSessionID(systemPrompt, firstUserMessage string) string {
	sum := sha256.Sum256([]byte(systemPrompt + "\n" + firstUserMessage))
	return "api-" + hex.EncodeToString(sum[:])[:16]
}

func randomHexFromTime(t time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d", t.UnixNano())))
	return hex.EncodeToString(sum[:])[:29]
}

type chatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []chatCompletionChunkChoice `json:"choices"`
	Usage   map[string]int              `json:"usage,omitempty"`
}

type chatCompletionChunkChoice struct {
	Index        int               `json:"index"`
	Delta        map[string]string `json:"delta"`
	Logprobs     any               `json:"logprobs"`
	FinishReason *string           `json:"finish_reason"`
}

func chatCompletionResponse(id string, created int64, model string, result TurnResult) map[string]any {
	finish := strings.TrimSpace(result.FinishReason)
	if finish == "" {
		finish = "stop"
	}
	return map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": result.Content,
				},
				"logprobs":      nil,
				"finish_reason": finish,
			},
		},
		"usage": usagePayload(result.Usage),
	}
}

func usagePayload(u Usage) map[string]int {
	return map[string]int{
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"total_tokens":      u.TotalTokens,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeOpenAIError(w http.ResponseWriter, status int, message, errType, param, code string) {
	writeJSON(w, status, openAIErrorEnvelope(message, errType, param, code))
}

func openAIErrorEnvelope(message, errType, param, code string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errType,
			"param":   nullableString(param),
			"code":    nullableString(code),
		},
	}
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func stringPtr(s string) *string { return &s }

func writeSSEData(w http.ResponseWriter, body any) {
	raw, _ := json.Marshal(body)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
}

func writeSSEEvent(w http.ResponseWriter, event string, body any) {
	raw, _ := json.Marshal(body)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

var errBodyTooLarge = errors.New("api server: request body too large")

func readLimitedBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	if r.ContentLength > maxBytes {
		return nil, errBodyTooLarge
	}
	reader := http.MaxBytesReader(w, r.Body, maxBytes)
	defer reader.Close()
	return io.ReadAll(reader)
}
