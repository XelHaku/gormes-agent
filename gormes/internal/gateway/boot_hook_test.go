package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

func TestStartBootHookSkipsMissingBootFile(t *testing.T) {
	client := hermes.NewMockClient()

	started := StartBootHook(context.Background(), BootHookConfig{
		Path:   filepath.Join(t.TempDir(), "BOOT.md"),
		Model:  "hermes-agent",
		Client: client,
		Log:    discardBootLogger(),
	})
	if started {
		t.Fatal("StartBootHook() = true, want false for missing BOOT.md")
	}
	if got := len(client.Requests()); got != 0 {
		t.Fatalf("client requests = %d, want 0", got)
	}
}

func TestStartBootHookRunsWrappedBootPromptInBackground(t *testing.T) {
	path := writeBootFile(t, "# Startup Checklist\n\n1. Check overnight failures.")

	client := hermes.NewMockClient()
	client.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "[SILENT]"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "")

	started := StartBootHook(context.Background(), BootHookConfig{
		Path:   path,
		Model:  "boot-model",
		Client: client,
		Log:    discardBootLogger(),
	})
	if !started {
		t.Fatal("StartBootHook() = false, want true")
	}

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(client.Requests()) == 1
	})

	reqs := client.Requests()
	if len(reqs) != 1 {
		t.Fatalf("client requests = %d, want 1", len(reqs))
	}
	req := reqs[0]
	if req.Model != "boot-model" {
		t.Fatalf("request model = %q, want %q", req.Model, "boot-model")
	}
	if len(req.Messages) != 1 {
		t.Fatalf("request messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("request message role = %q, want %q", req.Messages[0].Role, "user")
	}
	if !strings.Contains(req.Messages[0].Content, "startup boot checklist") {
		t.Fatalf("request content = %q, want boot-checklist wrapper", req.Messages[0].Content)
	}
	if !strings.Contains(req.Messages[0].Content, "Check overnight failures.") {
		t.Fatalf("request content = %q, want BOOT.md body", req.Messages[0].Content)
	}
	if !strings.Contains(req.Messages[0].Content, "[SILENT]") {
		t.Fatalf("request content = %q, want SILENT instruction", req.Messages[0].Content)
	}
}

func TestStartBootHookDoesNotBlockStartupOnBootFailure(t *testing.T) {
	path := writeBootFile(t, "# Startup Checklist\n\n1. Try boot.")
	client := newBlockingBootClient(errors.New("boot failed"))

	start := time.Now()
	started := StartBootHook(context.Background(), BootHookConfig{
		Path:   path,
		Model:  "hermes-agent",
		Client: client,
		Log:    discardBootLogger(),
	})
	if !started {
		t.Fatal("StartBootHook() = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("StartBootHook() blocked for %s, want background startup", elapsed)
	}

	select {
	case <-client.entered:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("boot client OpenStream was not invoked in background")
	}

	close(client.release)

	select {
	case <-client.done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("boot client did not exit after release")
	}
}

type blockingBootClient struct {
	mu      sync.Mutex
	requests []hermes.ChatRequest
	entered chan struct{}
	release chan struct{}
	done    chan struct{}
	openErr error
	once    sync.Once
}

func newBlockingBootClient(openErr error) *blockingBootClient {
	return &blockingBootClient{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
		openErr: openErr,
	}
}

func (c *blockingBootClient) OpenStream(ctx context.Context, req hermes.ChatRequest) (hermes.Stream, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.mu.Unlock()

	c.once.Do(func() { close(c.entered) })

	select {
	case <-c.release:
	case <-ctx.Done():
	}

	close(c.done)
	return &hermes.MockStream{}, c.openErr
}

func (*blockingBootClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

func (*blockingBootClient) Health(context.Context) error { return nil }

func writeBootFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "BOOT.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(BOOT.md): %v", err)
	}
	return path
}

func discardBootLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
