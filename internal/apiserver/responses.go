package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
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

	turnReq, responseContext, errResp := s.buildResponseTurnRequest(req)
	if errResp != nil {
		writeOpenAIError(w, errResp.status, errResp.message, "invalid_request_error", errResp.param, errResp.code)
		return
	}

	responseID := "resp_" + randomHexFromTime(s.now())
	created := s.now().Unix()
	if boolField(req, "stream", false) {
		s.writeStreamingResponse(w, r, responseID, created, turnReq, responseContext)
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

	response := responseObjectFromTurn(responseID, created, turnReq.Model, result)
	if responseContext.store {
		fullHistory := append([]ChatMessage(nil), responseContext.historyForStorage...)
		fullHistory = append(fullHistory, responseMessagesForStorage(result)...)
		stored := StoredResponse{
			Response:            response,
			ConversationHistory: fullHistory,
			Instructions:        responseContext.instructions,
			SessionID:           sessionID,
		}
		if err := s.responseStore.Put(responseID, stored); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "response_store_failed")
			return
		}
		if responseContext.conversation != "" {
			if err := s.responseStore.SetConversation(responseContext.conversation, responseID); err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "response_store_failed")
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) writeStreamingResponse(w http.ResponseWriter, r *http.Request, responseID string, created int64, turnReq TurnRequest, responseContext responseTurnContext) {
	sessionID := turnReq.SessionID
	if sessionID != "" {
		w.Header().Set("X-Hermes-Session-Id", sessionID)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	streamCtx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()

	var (
		partialText strings.Builder
		writeErr    error
	)
	persist := func(response ResponseObject, history []ChatMessage, snapshotSessionID string) error {
		if snapshotSessionID == "" {
			snapshotSessionID = sessionID
		}
		return s.persistResponseSnapshot(responseID, response, responseContext, history, snapshotSessionID)
	}
	persistIncomplete := func(result TurnResult) {
		text := partialText.String()
		if text == "" {
			text = result.Content
		}
		incomplete := responseObjectFromText(responseID, created, turnReq.Model, "incomplete", text, result.Usage)
		_ = persist(incomplete, responseHistoryWithAssistant(responseContext.historyForStorage, text), sessionID)
	}

	createdResponse := responseObjectFromText(responseID, created, turnReq.Model, "in_progress", "", Usage{})
	if err := writeSSEEvent(w, "response.created", map[string]any{
		"type":     "response.created",
		"response": createdResponse,
	}); err != nil {
		writeErr = err
		cancelStream()
		persistIncomplete(TurnResult{})
		return
	}
	if err := persist(createdResponse, append([]ChatMessage(nil), responseContext.historyForStorage...), sessionID); err != nil {
		_ = writeSSEEvent(w, "response.failed", map[string]any{
			"type":  "response.failed",
			"error": err.Error(),
		})
		flush(w)
		return
	}
	flush(w)

	result, err := s.loop.StreamTurn(streamCtx, turnReq, StreamCallbacks{
		OnToken: func(token string) error {
			if token == "" {
				return nil
			}
			partialText.WriteString(token)
			if err := writeSSEEvent(w, "response.output_text.delta", map[string]any{
				"type":        "response.output_text.delta",
				"response_id": responseID,
				"delta":       token,
			}); err != nil {
				writeErr = err
				cancelStream()
				return err
			}
			flush(w)
			return nil
		},
	})
	if result.SessionID != "" {
		sessionID = result.SessionID
	}
	if err != nil {
		if writeErr != nil || errors.Is(err, context.Canceled) || streamCtx.Err() != nil || r.Context().Err() != nil {
			persistIncomplete(result)
			return
		}
		text := partialText.String()
		if text == "" {
			text = err.Error()
		}
		failed := responseObjectFromText(responseID, created, turnReq.Model, "failed", text, result.Usage)
		_ = persist(failed, responseHistoryWithAssistant(responseContext.historyForStorage, text), sessionID)
		_ = writeSSEEvent(w, "response.failed", map[string]any{
			"type":     "response.failed",
			"response": failed,
		})
		flush(w)
		return
	}

	if result.SessionID == "" {
		result.SessionID = sessionID
	}
	if result.Content == "" && len(result.Messages) == 0 && partialText.Len() > 0 {
		result.Content = partialText.String()
	}
	completed := responseObjectFromTurn(responseID, created, turnReq.Model, result)
	fullHistory := append([]ChatMessage(nil), responseContext.historyForStorage...)
	fullHistory = append(fullHistory, responseMessagesForStorage(result)...)
	if err := persist(completed, fullHistory, sessionID); err != nil {
		_ = writeSSEEvent(w, "response.failed", map[string]any{
			"type":  "response.failed",
			"error": err.Error(),
		})
		flush(w)
		return
	}
	_ = writeSSEEvent(w, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": completed,
	})
	flush(w)
}

func (s *Server) persistResponseSnapshot(responseID string, response ResponseObject, responseContext responseTurnContext, history []ChatMessage, sessionID string) error {
	if !responseContext.store {
		return nil
	}
	stored := StoredResponse{
		Response:            response,
		ConversationHistory: append([]ChatMessage(nil), history...),
		Instructions:        responseContext.instructions,
		SessionID:           sessionID,
	}
	if err := s.responseStore.Put(responseID, stored); err != nil {
		return err
	}
	if responseContext.conversation != "" {
		return s.responseStore.SetConversation(responseContext.conversation, responseID)
	}
	return nil
}

func (s *Server) handleResponseByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	responseID := strings.TrimPrefix(r.URL.Path, "/v1/responses/")
	if responseID == "" || strings.Contains(responseID, "/") {
		writeOpenAIError(w, http.StatusNotFound, "Response not found", "invalid_request_error", "", "response_not_found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		stored, ok, err := s.responseStore.Get(responseID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "response_store_failed")
			return
		}
		if !ok {
			writeOpenAIError(w, http.StatusNotFound, "Response not found: "+responseID, "invalid_request_error", "", "response_not_found")
			return
		}
		writeJSON(w, http.StatusOK, stored.Response)
	case http.MethodDelete:
		deleted, err := s.responseStore.Delete(responseID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "response_store_failed")
			return
		}
		if !deleted {
			writeOpenAIError(w, http.StatusNotFound, "Response not found: "+responseID, "invalid_request_error", "", "response_not_found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      responseID,
			"object":  "response",
			"deleted": true,
		})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
	}
}

type responseTurnContext struct {
	instructions      string
	conversation      string
	store             bool
	historyForStorage []ChatMessage
}

func (s *Server) buildResponseTurnRequest(body map[string]any) (TurnRequest, responseTurnContext, *requestError) {
	rawInput, ok := body["input"]
	if !ok || rawInput == nil {
		return TurnRequest{}, responseTurnContext{}, &requestError{
			status:  http.StatusBadRequest,
			message: "Missing 'input' field",
			param:   "input",
			code:    "missing_input",
		}
	}
	inputMessages, errResp := normalizeResponseInput(rawInput)
	if errResp != nil {
		return TurnRequest{}, responseTurnContext{}, errResp
	}
	if len(inputMessages) == 0 {
		return TurnRequest{}, responseTurnContext{}, &requestError{status: http.StatusBadRequest, message: "No user message found in input", code: "missing_user_message"}
	}

	instructions := stringField(body, "instructions")
	previousResponseID := stringField(body, "previous_response_id")
	conversation := stringField(body, "conversation")
	if conversation != "" && previousResponseID != "" {
		return TurnRequest{}, responseTurnContext{}, &requestError{
			status:  http.StatusBadRequest,
			message: "Cannot use both 'conversation' and 'previous_response_id'",
			code:    "invalid_response_chain",
		}
	}
	if conversation != "" {
		if resolved, ok, err := s.responseStore.GetConversation(conversation); err != nil {
			return TurnRequest{}, responseTurnContext{}, &requestError{status: http.StatusInternalServerError, message: err.Error(), code: "response_store_failed"}
		} else if ok {
			previousResponseID = resolved
		}
	}

	conversationHistory, errResp := normalizeExplicitConversationHistory(body["conversation_history"])
	if errResp != nil {
		return TurnRequest{}, responseTurnContext{}, errResp
	}
	sessionID := ""
	if len(conversationHistory) == 0 && previousResponseID != "" {
		stored, ok, err := s.responseStore.Get(previousResponseID)
		if err != nil {
			return TurnRequest{}, responseTurnContext{}, &requestError{status: http.StatusInternalServerError, message: err.Error(), code: "response_store_failed"}
		}
		if !ok {
			s.recordPreviousResponseMiss()
			return TurnRequest{}, responseTurnContext{}, &requestError{
				status:  http.StatusNotFound,
				message: "Previous response not found: " + previousResponseID,
				param:   "previous_response_id",
				code:    "previous_response_not_found",
			}
		}
		conversationHistory = append(conversationHistory, stored.ConversationHistory...)
		sessionID = stored.SessionID
		if instructions == "" {
			instructions = stored.Instructions
		}
	}

	last := inputMessages[len(inputMessages)-1]
	if !hasVisibleText(last.Content) {
		return TurnRequest{}, responseTurnContext{}, &requestError{status: http.StatusBadRequest, message: "No user message found in input", code: "missing_user_message"}
	}
	turnHistory := append([]ChatMessage(nil), conversationHistory...)
	turnHistory = append(turnHistory, inputMessages[:len(inputMessages)-1]...)
	if stringField(body, "truncation") == "auto" && len(turnHistory) > 100 {
		turnHistory = turnHistory[len(turnHistory)-100:]
	}
	if sessionID == "" {
		sessionID = deriveChatSessionID(instructions, firstUserContent(append(turnHistory, last)))
	}
	model := stringField(body, "model")
	if model == "" {
		model = s.modelName
	}
	store := true
	if rawStore, ok := body["store"].(bool); ok {
		store = rawStore
	}
	historyForStorage := append([]ChatMessage(nil), turnHistory...)
	historyForStorage = append(historyForStorage, last)
	return TurnRequest{
			Model:        model,
			UserMessage:  last.Content,
			History:      turnHistory,
			SystemPrompt: instructions,
			SessionID:    sessionID,
		}, responseTurnContext{
			instructions:      instructions,
			conversation:      conversation,
			store:             store,
			historyForStorage: historyForStorage,
		}, nil
}

func normalizeResponseInput(raw any) ([]ChatMessage, *requestError) {
	switch v := raw.(type) {
	case string:
		return []ChatMessage{{Role: "user", Content: truncateText(v)}}, nil
	case []any:
		out := make([]ChatMessage, 0, len(v))
		for idx, item := range v {
			msg, errResp := normalizeResponseInputMessage(item, fmt.Sprintf("input[%d]", idx))
			if errResp != nil {
				return nil, errResp
			}
			out = append(out, msg)
		}
		return out, nil
	default:
		return nil, &requestError{status: http.StatusBadRequest, message: "'input' must be a string or array", param: "input", code: "invalid_input"}
	}
}

func normalizeExplicitConversationHistory(raw any) ([]ChatMessage, *requestError) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, &requestError{status: http.StatusBadRequest, message: "'conversation_history' must be an array of message objects", param: "conversation_history", code: "invalid_conversation_history"}
	}
	out := make([]ChatMessage, 0, len(items))
	for idx, item := range items {
		msg, errResp := normalizeResponseInputMessage(item, fmt.Sprintf("conversation_history[%d]", idx))
		if errResp != nil {
			return nil, errResp
		}
		out = append(out, msg)
	}
	return out, nil
}

func normalizeResponseInputMessage(raw any, param string) (ChatMessage, *requestError) {
	switch v := raw.(type) {
	case string:
		return ChatMessage{Role: "user", Content: truncateText(v)}, nil
	case map[string]any:
		role := strings.ToLower(strings.TrimSpace(fmt.Sprint(v["role"])))
		if role == "" || role == "<nil>" {
			role = "user"
		}
		content, err := normalizeChatContent(v["content"])
		if err != nil {
			return ChatMessage{}, &requestError{status: http.StatusBadRequest, message: err.message, param: param + ".content", code: err.code}
		}
		msg := ChatMessage{
			Role:       role,
			Content:    content,
			ToolCalls:  parseToolCalls(v["tool_calls"]),
			ToolCallID: strings.TrimSpace(fmt.Sprint(v["tool_call_id"])),
			Name:       strings.TrimSpace(fmt.Sprint(v["name"])),
		}
		if msg.ToolCallID == "<nil>" {
			msg.ToolCallID = ""
		}
		if msg.Name == "<nil>" {
			msg.Name = ""
		}
		return msg, nil
	default:
		return ChatMessage{}, &requestError{status: http.StatusBadRequest, message: param + " must be a string or message object", param: param, code: "invalid_input_message"}
	}
}

func parseToolCalls(raw any) []ToolCall {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]ToolCall, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		call := ToolCall{
			ID:   strings.TrimSpace(fmt.Sprint(m["id"])),
			Name: strings.TrimSpace(fmt.Sprint(m["name"])),
		}
		if fn, ok := m["function"].(map[string]any); ok {
			if call.Name == "" || call.Name == "<nil>" {
				call.Name = strings.TrimSpace(fmt.Sprint(fn["name"]))
			}
			call.Arguments = strings.TrimSpace(fmt.Sprint(fn["arguments"]))
		} else {
			call.Arguments = strings.TrimSpace(fmt.Sprint(m["arguments"]))
		}
		if call.ID == "<nil>" {
			call.ID = ""
		}
		if call.Name == "<nil>" {
			call.Name = ""
		}
		if call.Arguments == "<nil>" {
			call.Arguments = ""
		}
		if call.ID != "" || call.Name != "" || call.Arguments != "" {
			out = append(out, call)
		}
	}
	return out
}

func responseObjectFromText(id string, created int64, model, status, text string, usage Usage) ResponseObject {
	output := []ResponseOutputItem{}
	if strings.TrimSpace(text) != "" {
		output = append(output, responseMessageItem(text))
	}
	return ResponseObject{
		ID:        id,
		Object:    "response",
		Status:    status,
		CreatedAt: created,
		Model:     model,
		Output:    output,
		Usage: ResponseUsage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		},
	}
}

func responseObjectFromTurn(id string, created int64, model string, result TurnResult) ResponseObject {
	return ResponseObject{
		ID:        id,
		Object:    "response",
		Status:    "completed",
		CreatedAt: created,
		Model:     model,
		Output:    responseOutputItems(result),
		Usage: ResponseUsage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	}
}

func responseOutputItems(result TurnResult) []ResponseOutputItem {
	messages := result.Messages
	if len(messages) == 0 {
		messages = []ChatMessage{{Role: "assistant", Content: result.Content}}
	}
	var out []ResponseOutputItem
	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			for _, call := range msg.ToolCalls {
				out = append(out, ResponseOutputItem{
					Type:      "function_call",
					CallID:    call.ID,
					Name:      call.Name,
					Arguments: call.Arguments,
				})
			}
			if strings.TrimSpace(msg.Content) != "" {
				out = append(out, responseMessageItem(msg.Content))
			}
		case "tool":
			out = append(out, ResponseOutputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Name:   msg.Name,
				Output: msg.Content,
			})
		}
	}
	if len(out) == 0 {
		out = append(out, responseMessageItem(result.Content))
	}
	return out
}

func responseMessageItem(text string) ResponseOutputItem {
	return ResponseOutputItem{
		Type: "message",
		Role: "assistant",
		Content: []ResponseContentPart{{
			Type: "output_text",
			Text: text,
		}},
	}
}

func responseMessagesForStorage(result TurnResult) []ChatMessage {
	if len(result.Messages) > 0 {
		return append([]ChatMessage(nil), result.Messages...)
	}
	return []ChatMessage{{Role: "assistant", Content: result.Content}}
}

func responseHistoryWithAssistant(base []ChatMessage, assistantText string) []ChatMessage {
	history := append([]ChatMessage(nil), base...)
	if strings.TrimSpace(assistantText) != "" {
		history = append(history, ChatMessage{Role: "assistant", Content: assistantText})
	}
	return history
}

func firstUserContent(messages []ChatMessage) string {
	for _, msg := range messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			return msg.Content
		}
	}
	return ""
}

func stringField(body map[string]any, key string) string {
	value, ok := body[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func boolField(body map[string]any, key string, fallback bool) bool {
	value, ok := body[key]
	if !ok {
		return fallback
	}
	b, ok := value.(bool)
	if !ok {
		return fallback
	}
	return b
}

func (s *Server) recordPreviousResponseMiss() {
	s.statusMu.Lock()
	s.previousResponseMisses++
	s.statusMu.Unlock()
}

func (s *Server) responseHealthStatus() map[string]any {
	stats := s.responseStore.Stats()
	s.statusMu.Lock()
	misses := s.previousResponseMisses
	s.statusMu.Unlock()
	return map[string]any{
		"store_enabled":            stats.Enabled,
		"stored":                   stats.Size,
		"max_stored":               stats.MaxSize,
		"lru_evictions":            stats.LRUEvictions,
		"previous_response_misses": misses,
	}
}
