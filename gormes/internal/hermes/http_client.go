package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultChatCompletionsPath = "/v1/chat/completions"
const defaultHealthPath = "/health"

type httpClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewHTTPClient returns a Client that talks HTTP+SSE to a Hermes-compatible
// api_server. baseURL example: "http://127.0.0.1:8642".
// The returned client streams without a global timeout so long turns
// (minutes, with tool use) are not truncated; see per-phase timeouts inside.
func NewHTTPClient(baseURL, apiKey string) Client {
	return &httpClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 0},
	}
}

func (c *httpClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+defaultHealthPath, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(body)}
	}
	return nil
}

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orChatRequest struct {
	Model    string      `json:"model"`
	Messages []orMessage `json:"messages"`
	Stream   bool        `json:"stream"`
}

func (c *httpClient) OpenStream(ctx context.Context, req ChatRequest) (Stream, error) {
	msgs := make([]orMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = orMessage{Role: m.Role, Content: m.Content}
	}
	body, err := json.Marshal(orChatRequest{Model: req.Model, Messages: msgs, Stream: true})
	if err != nil {
		return nil, err
	}

	// The response-header phase has a 5s budget; the streaming body does not.
	headCtx, headCancel := context.WithTimeout(ctx, 5*time.Second)
	httpReq, err := http.NewRequestWithContext(headCtx, http.MethodPost, c.baseURL+defaultChatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		headCancel()
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if req.SessionID != "" {
		httpReq.Header.Set("X-Hermes-Session-Id", req.SessionID)
	}

	resp, err := c.http.Do(httpReq)
	headCancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	// The body stays open for streaming; chatStream owns the Close.
	return newChatStream(resp.Body, resp.Header.Get("X-Hermes-Session-Id")), nil
}

// OpenRunEvents subscribes to SSE stream for a run's events.
// 404 returns ErrRunEventsNotSupported for non-Hermes servers.
func (c *httpClient) OpenRunEvents(ctx context.Context, runID string) (RunEventStream, error) {
	// Use a short header-phase budget; the body stays open indefinitely for streaming.
	headCtx, headCancel := context.WithTimeout(ctx, 5*time.Second)
	req, err := http.NewRequestWithContext(headCtx, http.MethodGet, fmt.Sprintf("%s/v1/runs/%s/events", c.baseURL, runID), nil)
	if err != nil {
		headCancel()
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	headCancel()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		_ = resp.Body.Close()
		return nil, ErrRunEventsNotSupported
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	return newRunEventStream(resp.Body), nil
}
