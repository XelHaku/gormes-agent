package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbedClient_ParsesOpenAIResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "embedding": []float32{0.1, 0.2, 0.3, 0.4}, "index": 0},
			},
			"model": "test-model",
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	vec, err := c.Embed(context.Background(), "test-model", "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 4 {
		t.Fatalf("len = %d, want 4", len(vec))
	}
	for i, want := range []float32{0.1, 0.2, 0.3, 0.4} {
		if vec[i] != want {
			t.Errorf("vec[%d] = %v, want %v", i, vec[i], want)
		}
	}
}

func TestEmbedClient_ModelNotFoundError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": `model "nomic-embed-text" not found, try pulling it first`,
				"type":    "not_found_error",
			},
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	_, err := c.Embed(context.Background(), "nomic-embed-text", "hello")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !errors.Is(err, errEmbedModelNotFound) {
		t.Errorf("err = %v, want errors.Is(err, errEmbedModelNotFound)", err)
	}
}

func TestEmbedClient_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal"))
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	_, err := c.Embed(context.Background(), "any", "x")
	if err == nil {
		t.Fatal("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want mention of 500", err)
	}
}

func TestEmbedClient_CtxTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // longer than ctx budget below
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Embed(ctx, "any", "x")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected errors.Is(err, context.DeadlineExceeded); got %v", err)
	}
}

func TestEmbedClient_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{1}}},
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "my-key")
	_, _ = c.Embed(context.Background(), "m", "x")
	if gotAuth != "Bearer my-key" {
		t.Errorf("Authorization = %q, want 'Bearer my-key'", gotAuth)
	}
}
