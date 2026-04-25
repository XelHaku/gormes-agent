package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDashboardContract_CoversNativeDashboardEndpoints(t *testing.T) {
	loop := &dashboardContractLoop{
		result: TurnResult{
			Content:   "native answer",
			SessionID: "sess-dashboard",
			Usage:     Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
			Messages:  []ChatMessage{{Role: "assistant", Content: "native answer"}},
		},
		streamTokens: []string{"native ", "stream"},
		toolProgress: []ToolProgressEvent{{
			Name:    "repo_search",
			Preview: "scanning internal/apiserver",
			Status:  "running",
		}},
	}
	srv := NewServer(Config{
		ModelName:     "gormes-agent",
		ProviderName:  "native",
		Loop:          loop,
		ResponseStore: NewResponseStore(10),
		ModelProviders: []DashboardModelProvider{{
			Name:        "Native Gormes",
			Slug:        "native",
			Models:      []string{"gormes-agent"},
			TotalModels: 1,
			IsCurrent:   true,
		}},
		OAuthProviders: []DashboardOAuthProvider{{
			ID:         "anthropic",
			Name:       "Anthropic",
			Flow:       "external",
			CLICommand: "gormes auth add anthropic",
			DocsURL:    "https://docs.anthropic.com",
			Status: DashboardOAuthStatus{
				LoggedIn: false,
				Error:    "not_configured",
			},
		}},
	})
	h := srv.Handler()

	chat := postJSON(t, h, "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}, nil)
	if chat.Code != http.StatusOK {
		t.Fatalf("chat status = %d, want 200; body=%s", chat.Code, chat.Body.String())
	}
	if got := chat.Header().Get("X-Hermes-Session-Id"); got != "sess-dashboard" {
		t.Fatalf("chat session header = %q, want sess-dashboard", got)
	}

	stream := postJSON(t, h, "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"stream":   true,
		"messages": []any{map[string]any{"role": "user", "content": "stream please"}},
	}, map[string]string{"X-Hermes-Session-Id": "sess-dashboard"})
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want 200; body=%s", stream.Code, stream.Body.String())
	}
	if got := stream.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("stream Content-Type = %q, want text/event-stream", got)
	}
	for _, want := range []string{`"object":"chat.completion.chunk"`, `"content":"native "`, "data: [DONE]"} {
		if !strings.Contains(stream.Body.String(), want) {
			t.Fatalf("stream body missing %q: %s", want, stream.Body.String())
		}
	}

	response := postJSON(t, h, "/v1/responses", map[string]any{
		"model": "gormes-agent",
		"input": "persist this turn",
	}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("response status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var created ResponseObject
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("response ID is empty")
	}

	sessions := getJSON(t, h, "/api/sessions?limit=10&offset=0", nil)
	if sessions.Code != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200; body=%s", sessions.Code, sessions.Body.String())
	}
	var sessionList struct {
		Sessions []struct {
			ID           string  `json:"id"`
			Model        *string `json:"model"`
			MessageCount int     `json:"message_count"`
			Preview      *string `json:"preview"`
		} `json:"sessions"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(sessions.Body.Bytes(), &sessionList); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if sessionList.Total != 1 || len(sessionList.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want one native session", sessionList)
	}
	if got := sessionList.Sessions[0].ID; got != "sess-dashboard" {
		t.Fatalf("session id = %q, want sess-dashboard", got)
	}
	if sessionList.Sessions[0].MessageCount == 0 || sessionList.Sessions[0].Preview == nil || *sessionList.Sessions[0].Preview == "" {
		t.Fatalf("session summary missing message count or preview: %+v", sessionList.Sessions[0])
	}

	modelOptions := getJSON(t, h, "/api/model/options", nil)
	if modelOptions.Code != http.StatusOK {
		t.Fatalf("model options status = %d, want 200; body=%s", modelOptions.Code, modelOptions.Body.String())
	}
	var options struct {
		Model     string `json:"model"`
		Provider  string `json:"provider"`
		Providers []struct {
			Name      string   `json:"name"`
			Slug      string   `json:"slug"`
			Models    []string `json:"models"`
			IsCurrent bool     `json:"is_current"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(modelOptions.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode model options: %v", err)
	}
	if options.Model != "gormes-agent" || options.Provider != "native" || len(options.Providers) != 1 || !options.Providers[0].IsCurrent {
		t.Fatalf("model options = %+v, want current native provider", options)
	}

	oauth := getJSON(t, h, "/api/providers/oauth", nil)
	if oauth.Code != http.StatusOK {
		t.Fatalf("oauth status = %d, want 200; body=%s", oauth.Code, oauth.Body.String())
	}
	var oauthStatus struct {
		Providers []struct {
			ID     string `json:"id"`
			Flow   string `json:"flow"`
			Status struct {
				LoggedIn bool   `json:"logged_in"`
				Error    string `json:"error"`
			} `json:"status"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(oauth.Body.Bytes(), &oauthStatus); err != nil {
		t.Fatalf("decode oauth: %v", err)
	}
	if len(oauthStatus.Providers) != 1 || oauthStatus.Providers[0].ID != "anthropic" || oauthStatus.Providers[0].Status.LoggedIn {
		t.Fatalf("oauth providers = %+v, want configured disconnected provider", oauthStatus.Providers)
	}
	if oauthStatus.Providers[0].Status.Error != "not_configured" {
		t.Fatalf("oauth error = %q, want not_configured", oauthStatus.Providers[0].Status.Error)
	}

	run := postJSON(t, h, "/v1/runs", map[string]any{"input": "show tool progress"}, nil)
	if run.Code != http.StatusAccepted {
		t.Fatalf("run status = %d, want 202; body=%s", run.Code, run.Body.String())
	}
	var started struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(run.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	events := getJSON(t, h, "/v1/runs/"+started.RunID+"/events", nil)
	if events.Code != http.StatusOK {
		t.Fatalf("events status = %d, want 200; body=%s", events.Code, events.Body.String())
	}
	for _, want := range []string{`"event":"tool.progress"`, `"name":"repo_search"`, `"preview":"scanning internal/apiserver"`} {
		if !strings.Contains(events.Body.String(), want) {
			t.Fatalf("tool progress stream missing %q: %s", want, events.Body.String())
		}
	}

	deleted := deleteJSON(t, h, "/api/sessions/sess-dashboard", nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete session status = %d, want 200; body=%s", deleted.Code, deleted.Body.String())
	}
	var deleteBody struct {
		OK        bool   `json:"ok"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(deleted.Body.Bytes(), &deleteBody); err != nil {
		t.Fatalf("decode delete session: %v", err)
	}
	if !deleteBody.OK || deleteBody.SessionID != "sess-dashboard" {
		t.Fatalf("delete body = %+v, want ok for sess-dashboard", deleteBody)
	}
	missing := getJSON(t, h, "/v1/responses/"+created.ID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("response after session delete status = %d, want 404; body=%s", missing.Code, missing.Body.String())
	}
}

func TestDashboardStatus_DegradesMissingNativeAndOptionalPanels(t *testing.T) {
	srv := NewServer(Config{ModelName: "gormes-agent"})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}
	var got struct {
		Panels map[string]struct {
			State     string   `json:"state"`
			Reason    string   `json:"reason"`
			Endpoints []string `json:"endpoints"`
		} `json:"panels"`
		UpstreamReactRuntime struct {
			State    string `json:"state"`
			Required bool   `json:"required"`
		} `json:"upstream_react_runtime"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if got.Panels["chat"].State != "disabled" || !strings.Contains(got.Panels["chat"].Reason, "turn loop") {
		t.Fatalf("chat panel = %+v, want disabled turn-loop degradation", got.Panels["chat"])
	}
	if got.Panels["oauth"].State != "disabled" || got.Panels["plugins"].State != "disabled" {
		t.Fatalf("optional panel states = oauth:%+v plugins:%+v, want disabled", got.Panels["oauth"], got.Panels["plugins"])
	}
	if got.UpstreamReactRuntime.Required || got.UpstreamReactRuntime.State != "absent" {
		t.Fatalf("upstream runtime = %+v, want absent and not required", got.UpstreamReactRuntime)
	}
}

func TestDashboardContract_DoesNotAddNodeOrReactRuntimeAssets(t *testing.T) {
	for _, path := range []string{"package.json", "vite.config.ts", "node_modules"} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("internal/apiserver unexpectedly contains upstream runtime asset %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
	tsFiles, err := filepath.Glob("*.ts")
	if err != nil {
		t.Fatalf("glob TypeScript files: %v", err)
	}
	if len(tsFiles) != 0 {
		t.Fatalf("internal/apiserver TypeScript files = %v, want none", tsFiles)
	}
}

type dashboardContractLoop struct {
	mu           sync.Mutex
	calls        []TurnRequest
	result       TurnResult
	streamTokens []string
	toolProgress []ToolProgressEvent
}

func (d *dashboardContractLoop) RunTurn(_ context.Context, req TurnRequest) (TurnResult, error) {
	d.mu.Lock()
	d.calls = append(d.calls, req)
	d.mu.Unlock()
	return d.result, nil
}

func (d *dashboardContractLoop) StreamTurn(_ context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error) {
	d.mu.Lock()
	d.calls = append(d.calls, req)
	d.mu.Unlock()
	for _, ev := range d.toolProgress {
		if cb.OnToolProgress != nil {
			if err := cb.OnToolProgress(ev); err != nil {
				return TurnResult{}, err
			}
		}
	}
	for _, token := range d.streamTokens {
		if cb.OnToken != nil {
			if err := cb.OnToken(token); err != nil {
				return TurnResult{}, err
			}
		}
	}
	return d.result, nil
}
