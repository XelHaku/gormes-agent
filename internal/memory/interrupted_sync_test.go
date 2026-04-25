package memory_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

func TestInterruptedTurnSkipsMemorySyncAndExtractor(t *testing.T) {
	memStore, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "memory.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer memStore.Close(context.Background())

	client := &blockingTurnClient{started: make(chan struct{}, 1)}
	k := kernel.New(kernel.Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		ChatKey:   "telegram:interrupted",
	}, client, memStore, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "remember the interrupted pineapple"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	if err := k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel}); err != nil {
		t.Fatalf("cancel submit: %v", err)
	}

	waitForTurnMemorySyncStatus(t, memStore.DB(), "remember the interrupted pineapple", "skipped")
	assertTurnMemorySyncReason(t, memStore.DB(), "remember the interrupted pineapple", "interrupted")

	llm := &fakeLLM{}
	llm.script(`{"entities":[{"name":"Pineapple","type":"CONCEPT","description":""}],"relationships":[]}`, nil)
	extractor := memory.NewExtractor(memStore, llm, memory.ExtractorConfig{
		PollInterval: 10 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  500 * time.Millisecond,
	}, nil)
	extractorCtx, extractorCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	extractor.Run(extractorCtx)
	extractorCancel()

	if calls := llm.openCalls.Load(); calls != 0 {
		t.Fatalf("extractor OpenStream calls = %d, want 0 for skipped interrupted turn", calls)
	}
	assertCount(t, memStore.DB(), `SELECT COUNT(*) FROM entities`, 0)
	assertCount(t, memStore.DB(), `SELECT COUNT(*) FROM goncho_conclusions`, 0)

	status, err := memory.ReadExtractorStatus(context.Background(), memStore.DB(), 5)
	if err != nil {
		t.Fatalf("ReadExtractorStatus: %v", err)
	}
	if status.QueueDepth != 0 {
		t.Fatalf("QueueDepth = %d, want 0 for skipped turn", status.QueueDepth)
	}
	if status.DeadLetterCount != 0 {
		t.Fatalf("DeadLetterCount = %d, want 0; interrupted skips are not extractor failures", status.DeadLetterCount)
	}
	if status.SkippedSyncCount != 1 {
		t.Fatalf("SkippedSyncCount = %d, want 1", status.SkippedSyncCount)
	}
	if len(status.RecentSkippedSyncs) != 1 || status.RecentSkippedSyncs[0].Reason != "interrupted" {
		t.Fatalf("RecentSkippedSyncs = %+v, want one interrupted skip", status.RecentSkippedSyncs)
	}
}

func TestCompletedTurnMemorySyncsAndExtractorStillRuns(t *testing.T) {
	memStore, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "memory.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer memStore.Close(context.Background())

	client := hermes.NewMockClient()
	client.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "finished response"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-completed-sync")

	k := kernel.New(kernel.Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		ChatKey:   "telegram:completed",
	}, client, memStore, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "remember the completed mango"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitForReadyTurnCount(t, memStore.DB(), "telegram:completed", 2)

	llm := &fakeLLM{}
	llm.script(`{"entities":[{"name":"Mango","type":"CONCEPT","description":""}],"relationships":[]}`, nil)
	extractor := memory.NewExtractor(memStore, llm, memory.ExtractorConfig{
		PollInterval: 10 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  500 * time.Millisecond,
	}, nil)
	extractorCtx, extractorCancel := context.WithTimeout(context.Background(), time.Second)
	defer extractorCancel()
	go extractor.Run(extractorCtx)
	waitForCount(t, memStore.DB(), `SELECT COUNT(*) FROM entities WHERE name = 'Mango'`, 1)

	if calls := llm.openCalls.Load(); calls != 1 {
		t.Fatalf("extractor OpenStream calls = %d, want 1 for completed turn batch", calls)
	}

	status, err := memory.ReadExtractorStatus(context.Background(), memStore.DB(), 5)
	if err != nil {
		t.Fatalf("ReadExtractorStatus: %v", err)
	}
	if status.SkippedSyncCount != 0 {
		t.Fatalf("SkippedSyncCount = %d, want 0 for completed turn", status.SkippedSyncCount)
	}
}

type blockingTurnClient struct {
	started chan struct{}
}

func (c *blockingTurnClient) Health(ctx context.Context) error { return nil }

func (c *blockingTurnClient) OpenStream(ctx context.Context, _ hermes.ChatRequest) (hermes.Stream, error) {
	return &blockingTurnStream{started: c.started, sessionID: "sess-interrupted-sync"}, nil
}

func (c *blockingTurnClient) OpenRunEvents(ctx context.Context, _ string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

type blockingTurnStream struct {
	started   chan<- struct{}
	sessionID string
	emitted   bool
}

func (s *blockingTurnStream) SessionID() string { return s.sessionID }
func (s *blockingTurnStream) Close() error      { return nil }

func (s *blockingTurnStream) Recv(ctx context.Context) (hermes.Event, error) {
	if !s.emitted {
		s.emitted = true
		select {
		case s.started <- struct{}{}:
		default:
		}
		return hermes.Event{Kind: hermes.EventToken, Token: "partial response"}, nil
	}
	<-ctx.Done()
	return hermes.Event{}, ctx.Err()
}

type fakeLLM struct {
	mu        sync.Mutex
	scripts   []fakeResp
	openCalls atomic.Int64
}

type fakeResp struct {
	body string
	err  error
}

func (f *fakeLLM) script(body string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, fakeResp{body: body, err: err})
}

func (f *fakeLLM) Health(ctx context.Context) error { return nil }

func (f *fakeLLM) OpenStream(ctx context.Context, _ hermes.ChatRequest) (hermes.Stream, error) {
	f.openCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.scripts) == 0 {
		return &fakeStream{body: `{"entities":[],"relationships":[]}`}, nil
	}
	r := f.scripts[0]
	f.scripts = f.scripts[1:]
	if r.err != nil {
		return nil, r.err
	}
	return &fakeStream{body: r.body}, nil
}

func (f *fakeLLM) OpenRunEvents(ctx context.Context, _ string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

type fakeStream struct {
	body string
	emit bool
}

func (s *fakeStream) SessionID() string { return "" }
func (s *fakeStream) Close() error      { return nil }

func (s *fakeStream) Recv(ctx context.Context) (hermes.Event, error) {
	select {
	case <-ctx.Done():
		return hermes.Event{}, ctx.Err()
	default:
	}
	if !s.emit {
		s.emit = true
		return hermes.Event{Kind: hermes.EventToken, Token: s.body}, nil
	}
	return hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"}, errors.New("eof")
}

func waitForTurnMemorySyncStatus(t *testing.T, db *sql.DB, content, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last string
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = db.QueryRow(
			`SELECT memory_sync_status FROM turns WHERE content = ? ORDER BY id DESC LIMIT 1`,
			content,
		).Scan(&last)
		if lastErr == nil && last == want {
			return
		}
		if errors.Is(lastErr, sql.ErrNoRows) {
			lastErr = nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("turn %q memory_sync_status = %q err=%v, want %q", content, last, lastErr, want)
}

func assertTurnMemorySyncReason(t *testing.T, db *sql.DB, content, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(
		`SELECT COALESCE(memory_sync_reason, '') FROM turns WHERE content = ? ORDER BY id DESC LIMIT 1`,
		content,
	).Scan(&got); err != nil {
		t.Fatalf("query memory_sync_reason: %v", err)
	}
	if got != want {
		t.Fatalf("memory_sync_reason = %q, want %q", got, want)
	}
}

func waitForReadyTurnCount(t *testing.T, db *sql.DB, chatID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got int
	for time.Now().Before(deadline) {
		_ = db.QueryRow(
			`SELECT COUNT(*) FROM turns WHERE chat_id = ? AND memory_sync_status = 'ready'`,
			chatID,
		).Scan(&got)
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ready turn count for %q = %d, want %d", chatID, got, want)
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}

func waitForCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got int
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = db.QueryRow(query).Scan(&got)
		if lastErr == nil && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s = %d err=%v, want %d", query, got, lastErr, want)
}
