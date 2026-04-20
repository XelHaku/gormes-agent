package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// errEmbedModelNotFound is returned when the Ollama endpoint reports 404
// with a "model not found" body. Callers (the Embedder worker) handle
// this by logging a one-per-minute WARN and waiting; it's not a crash.
var errEmbedModelNotFound = errors.New("memory: embed model not loaded")

// embedClient is a narrow HTTP client for the OpenAI-compatible
// /v1/embeddings endpoint. Deliberately separate from hermes.Client —
// the kernel's Client interface is focused on chat streaming; mixing
// embedding concerns in would widen that surface for a feature only
// the memory package uses.
type embedClient struct {
	baseURL    string // e.g. "http://localhost:11434"
	apiKey     string // optional — "Bearer <key>" if non-empty
	httpClient *http.Client
}

func newEmbedClient(baseURL, apiKey string) *embedClient {
	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		dt = &http.Transport{}
	}
	transport := dt.Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	return &embedClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 0, Transport: transport},
	}
}

// Embed calls POST /v1/embeddings with the given model + input. Returns
// the first (and only) embedding vector from the response's `data[0]`.
// The caller is responsible for L2-normalizing the result before storage.
func (c *embedClient) Embed(ctx context.Context, model, input string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": model,
		"input": input,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memory: embed HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		if strings.Contains(strings.ToLower(string(raw)), "not found") {
			return nil, fmt.Errorf("%w: %s", errEmbedModelNotFound, string(raw))
		}
		return nil, fmt.Errorf("memory: embed 404: %s", string(raw))
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memory: embed HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var wire struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("memory: embed decode: %w", err)
	}
	if len(wire.Data) == 0 || len(wire.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("memory: embed response has no vector")
	}
	return wire.Data[0].Embedding, nil
}
