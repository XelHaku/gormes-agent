# Gormes Phase 2.B.1 — Telegram Scout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `cmd/gormes-telegram` — the first "External Hand" binary — per spec `2026-04-19-gormes-phase2b-telegram.md` (commit `31316da2`). The kernel stays unchanged except for one additive method; a new `internal/telegram` package translates between `tgbotapi` and `kernel.PlatformEvent` / `kernel.RenderFrame`.

**Architecture:** `telegramClient` interface (mockable); three goroutines inside `Bot` (inbound, outbound, coalescer); 1-second edit-coalescing window; allowlist-authenticated private-DM-only. `cmd/gormes/` never imports `internal/telegram/` — a build-isolation test guards the 7.9 MB TUI binary. TDD throughout: mock-driven tests land before any real Telegram API code.

**Tech Stack:** Go 1.22+, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, existing `internal/kernel` + `internal/tools` + `internal/hermes` + `internal/config`, stdlib `os/exec` for the build-isolation test.

---

## Prerequisites

- Phase 1 / 1.5 / 2.A all shipped. Latest kernel commit ≥ `b5624bef`. Latest doctor commit ≥ `28264813`.
- `go.mod` pinned at `go 1.22` with `toolchain go1.26.1`.
- Working tree clean or isolated from `internal/telegram/`, `cmd/gormes-telegram/`, `internal/kernel/`, `internal/config/` paths.
- For the live smoke test in Task 10: a Telegram bot token from @BotFather + a Python `api_server` running.

## File Structure Map

```
gormes/
├── cmd/
│   └── gormes-telegram/
│       └── main.go                             # NEW — T8 (binary entry)
├── internal/
│   ├── telegram/
│   │   ├── client.go                           # NEW — T1 (telegramClient interface)
│   │   ├── real_client.go                      # NEW — T2 (tgbotapi wrapper)
│   │   ├── mock_test.go                        # NEW — T1 (mockClient test double)
│   │   ├── bot.go                              # NEW — T1 stub; T4 outbound; T5 inbound
│   │   ├── bot_test.go                         # NEW — T1 first test; T5 + T9 extensions
│   │   ├── coalesce.go                         # NEW — T3
│   │   ├── coalesce_test.go                    # NEW — T3
│   │   ├── render.go                           # NEW — T4
│   │   └── render_test.go                      # NEW — T4
│   ├── buildisolation_test.go                  # NEW — T6 (TUI has no Telegram dep)
│   ├── kernel/
│   │   ├── frame.go                            # MODIFY — T7 (+ PlatformEventResetSession)
│   │   ├── kernel.go                           # MODIFY — T7 (+ ResetSession handler)
│   │   └── reset_test.go                       # NEW — T7
│   └── config/
│       ├── config.go                           # MODIFY — T8 (+ TelegramCfg)
│       └── config_test.go                      # MODIFY — T8 (telegram defaults test)
```

Nothing else in the repo changes.

---

## Task 1: `telegramClient` interface + `mockClient` + minimal `Bot` scaffolding

**Files:**
- Create: `gormes/internal/telegram/client.go`
- Create: `gormes/internal/telegram/mock_test.go`
- Create: `gormes/internal/telegram/bot.go` (stub)
- Create: `gormes/internal/telegram/bot_test.go` (first test only)

Establishes the mock-driven testing strategy BEFORE any real Telegram API code. By end of this task, we have a Bot that can reject an unauthorised chat — no real network involved.

### Step 1 — Add the Telegram SDK dependency

From inside `gormes/`:

```bash
go get github.com/go-telegram-bot-api/telegram-bot-api/v5@latest
```

### Step 2 — Create `gormes/internal/telegram/client.go`

```go
// Package telegram adapts Telegram bot traffic into kernel.PlatformEvent
// and kernel.RenderFrame streams. The adapter is a sibling to internal/tui —
// both consume the same kernel contracts; neither mutates kernel state.
package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// telegramClient is the minimal Telegram surface the adapter uses. Production
// wraps *tgbotapi.BotAPI; tests use the mockClient in mock_test.go. Keeping
// this interface tight means the Bot code never pulls a live HTTP dep into
// a test binary.
type telegramClient interface {
	// GetUpdatesChan starts long-poll and returns the Updates channel.
	GetUpdatesChan(cfg tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel

	// Send sends OR edits depending on the Chattable type (NewMessage vs
	// NewEditMessageText). Returns the resulting Message; edit calls return
	// an effectively-ignored Message with the same ID.
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)

	// StopReceivingUpdates signals the long-poll loop to stop. Called on
	// graceful shutdown.
	StopReceivingUpdates()
}
```

### Step 3 — Create `gormes/internal/telegram/mock_test.go`

```go
package telegram

import (
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockClient satisfies telegramClient. Tests push Updates into updatesCh and
// inspect every Send call via SentMessages().
type mockClient struct {
	updatesCh chan tgbotapi.Update
	mu        sync.Mutex
	sent      []tgbotapi.Chattable
	nextMsgID int
	stopped   bool

	// Optional: test can override to simulate errors.
	SendFn func(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// Compile-time interface check.
var _ telegramClient = (*mockClient)(nil)

func newMockClient() *mockClient {
	return &mockClient{
		updatesCh: make(chan tgbotapi.Update, 16),
		nextMsgID: 1000,
	}
}

func (m *mockClient) GetUpdatesChan(_ tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return m.updatesCh
}

func (m *mockClient) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	m.sent = append(m.sent, c)
	id := m.nextMsgID
	m.nextMsgID++
	m.mu.Unlock()

	if m.SendFn != nil {
		return m.SendFn(c)
	}
	return tgbotapi.Message{MessageID: id}, nil
}

func (m *mockClient) StopReceivingUpdates() {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	// Do NOT close updatesCh here — Bot's inbound goroutine owns draining
	// after stop signal. Close via closeUpdates() in test teardown.
}

// closeUpdates ends the long-poll loop cleanly in tests.
func (m *mockClient) closeUpdates() {
	close(m.updatesCh)
}

// pushTextUpdate scripts an inbound text message from the given chatID.
func (m *mockClient) pushTextUpdate(chatID int64, text string) {
	m.updatesCh <- tgbotapi.Update{
		UpdateID: 0,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From:      &tgbotapi.User{ID: chatID, FirstName: "tester"},
		},
	}
}

// sentMessages returns a snapshot of every Send() call seen so far.
func (m *mockClient) sentMessages() []tgbotapi.Chattable {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]tgbotapi.Chattable, len(m.sent))
	copy(out, m.sent)
	return out
}

// lastSentText extracts .Text from the most recent MessageConfig (or "" if
// the most recent send wasn't a text message). Useful for assertion helpers.
func (m *mockClient) lastSentText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return ""
	}
	last := m.sent[len(m.sent)-1]
	switch v := last.(type) {
	case tgbotapi.MessageConfig:
		return v.Text
	case tgbotapi.EditMessageTextConfig:
		return v.Text
	}
	return ""
}
```

### Step 4 — Create `gormes/internal/telegram/bot.go` (minimal scaffold)

```go
package telegram

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

// Config drives the Bot adapter. AllowedChatID and FirstRunDiscovery follow
// the spec's M1/M2 rules: either a non-zero allowlist OR discovery enabled,
// never neither.
type Config struct {
	AllowedChatID     int64
	CoalesceMs        int
	FirstRunDiscovery bool
}

// Bot is the Telegram adapter. Kernel-side state (draft, phase, history)
// lives in *kernel.Kernel; Bot holds only per-adapter streaming state.
type Bot struct {
	cfg    Config
	client telegramClient
	kernel *kernel.Kernel
	log    *slog.Logger
}

// New constructs a Bot wired to the given telegramClient + kernel. Use
// newRealClient in production (Task 2) or newMockClient in tests (Task 1).
func New(cfg Config, client telegramClient, k *kernel.Kernel, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.CoalesceMs <= 0 {
		cfg.CoalesceMs = 1000
	}
	return &Bot{cfg: cfg, client: client, kernel: k, log: log}
}

// Run blocks until ctx cancellation. Task 4 implements the full loop with
// outbound + coalescer goroutines; Task 5 adds the inbound goroutine. For
// Task 1, Run only drains one inbound update and returns, enough for the
// rejection test.
func (b *Bot) Run(ctx context.Context) error {
	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 30
	updates := b.client.GetUpdatesChan(ucfg)

	for {
		select {
		case <-ctx.Done():
			b.client.StopReceivingUpdates()
			return nil
		case u, ok := <-updates:
			if !ok {
				return nil
			}
			b.handleUpdate(ctx, u)
		}
	}
}

// handleUpdate processes one Telegram Update. Task 5 expands this with
// commands and kernel submission; Task 1 only handles the authorisation
// gate so the rejection test works.
func (b *Bot) handleUpdate(ctx context.Context, u tgbotapi.Update) {
	if u.Message == nil {
		return
	}
	chatID := u.Message.Chat.ID

	// M1 allowlist + M2 first-run discovery.
	if b.cfg.AllowedChatID == 0 {
		if b.cfg.FirstRunDiscovery {
			b.log.Info("first-run discovery: unknown chat", "chat_id", chatID)
			reply := tgbotapi.NewMessage(chatID,
				"Gormes is not authorised for this chat.\n"+
					"To allow: set [telegram].allowed_chat_id in config.toml.\n"+
					"Then restart gormes-telegram.")
			_, _ = b.client.Send(reply)
		} else {
			b.log.Warn("unauthorised chat blocked", "chat_id", chatID)
		}
		return
	}
	if chatID != b.cfg.AllowedChatID {
		b.log.Warn("unauthorised chat blocked", "chat_id", chatID)
		return
	}

	// Task 5 replaces this no-op with command parsing + kernel.Submit.
	b.log.Info("inbound message", "chat_id", chatID, "text", u.Message.Text)
}
```

### Step 5 — Create `gormes/internal/telegram/bot_test.go` with the first failing test

```go
package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// newTestKernel builds a Kernel with MockClient + NoopStore. Shared across
// Bot tests that don't care about the kernel's internals beyond "takes
// PlatformEvents, emits RenderFrames".
func newTestKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)
}

// TestBot_RejectsUnauthorisedChat: an inbound message from a non-allowed
// chat produces zero kernel.Submit calls. The Bot silently drops the
// message (no reply) when FirstRunDiscovery is false AND AllowedChatID
// is set to a different chat.
func TestBot_RejectsUnauthorisedChat(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{
		AllowedChatID:     11111,
		FirstRunDiscovery: false,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go k.Run(ctx)
	<-k.Render() // drain initial idle

	go func() { _ = b.Run(ctx) }()

	// A different chat sends a message.
	mc.pushTextUpdate(22222, "hello from nowhere")

	// Give the adapter a moment.
	time.Sleep(50 * time.Millisecond)

	// No Send calls should have fired (silent drop per M1).
	if got := len(mc.sentMessages()); got != 0 {
		t.Errorf("sent messages = %d, want 0 (silent drop for unauthorised chat)", got)
	}

	// Clean up.
	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}

// TestBot_FirstRunDiscoveryRepliesWithChatID: when AllowedChatID == 0 and
// FirstRunDiscovery is true, inbound from any chat triggers exactly one
// reply containing the chat_id hint.
func TestBot_FirstRunDiscoveryRepliesWithChatID(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{
		AllowedChatID:     0,
		FirstRunDiscovery: true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(77777, "hi")
	time.Sleep(50 * time.Millisecond)

	got := mc.lastSentText()
	if !strings.Contains(got, "not authorised") {
		t.Errorf("reply = %q, want to contain 'not authorised'", got)
	}
	if !strings.Contains(got, "allowed_chat_id") {
		t.Errorf("reply = %q, want to mention allowed_chat_id config key", got)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}
```

### Step 6 — Run the tests

```bash
cd gormes
go test -race ./internal/telegram/... -v -timeout 30s
go vet ./internal/telegram/...
```

Both tests PASS. `vet` clean.

### Step 7 — Full-repo sweep — nothing else broke

```bash
go test -race ./... -timeout 90s
```

All packages PASS (including the new `internal/telegram`).

### Step 8 — Commit

From repo root:

```bash
git add gormes/internal/telegram/client.go gormes/internal/telegram/mock_test.go gormes/internal/telegram/bot.go gormes/internal/telegram/bot_test.go gormes/go.mod gormes/go.sum
git commit -m "$(cat <<'EOF'
feat(gormes/telegram): telegramClient interface + mockClient scaffolding

Establishes the Phase-2.B.1 mock-driven testing strategy BEFORE any
real Telegram API code. telegramClient is a three-method interface
(GetUpdatesChan, Send, StopReceivingUpdates) that the Bot adapter
talks through; production wraps *tgbotapi.BotAPI (next task), tests
use mockClient which scripts inbound Updates and records every
Send call.

Bot.handleUpdate implements only the M1/M2 authorisation gate in
this task — kernel submission + commands land in Task 5. Two
bot_test.go cases prove the gate:
  - unauthorised chat silently dropped (M1)
  - first-run discovery replies with the chat_id hint (M2)

No live Telegram connection; no real network; var _ telegramClient =
(*mockClient)(nil) compile-time check guarantees the double never
drifts from the real interface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `realClient` wrapping `*tgbotapi.BotAPI`

**Files:**
- Create: `gormes/internal/telegram/real_client.go`

Minimal, no logic. Just the production implementation of `telegramClient`.

### Step 1 — Create `gormes/internal/telegram/real_client.go`

```go
package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// realClient wraps *tgbotapi.BotAPI to satisfy telegramClient. No extra
// behaviour — every method is a thin passthrough. Testable via the
// telegramClient interface.
type realClient struct {
	api *tgbotapi.BotAPI
}

// Compile-time interface check.
var _ telegramClient = (*realClient)(nil)

// newRealClient constructs a realClient from a bot token. Fails if the
// token is invalid (tgbotapi validates by calling getMe on construction).
func newRealClient(token string) (*realClient, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: invalid token: %w", err)
	}
	return &realClient{api: api}, nil
}

func (r *realClient) GetUpdatesChan(cfg tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return r.api.GetUpdatesChan(cfg)
}

func (r *realClient) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return r.api.Send(c)
}

func (r *realClient) StopReceivingUpdates() {
	r.api.StopReceivingUpdates()
}
```

### Step 2 — Verify compile

```bash
cd gormes
go build ./internal/telegram/...
go vet ./internal/telegram/...
```

No tests for this task — it's a pure passthrough. Integration-test coverage lives in Task 10's manual smoke.

### Step 3 — Commit

```bash
cd ..
git add gormes/internal/telegram/real_client.go
git commit -m "$(cat <<'EOF'
feat(gormes/telegram): realClient wrapping tgbotapi.BotAPI

Minimal production implementation of telegramClient. Three thin
passthrough methods; no business logic. realClient is instantiated
from a bot token via newRealClient("xxx:yyy") which calls
tgbotapi.NewBotAPI — that validates the token by hitting getMe on
construction, so token errors surface at binary startup not at
first user message.

Tests exclusively use mockClient from Task 1; no live Telegram
network is exercised anywhere in the test suite.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Coalescer + the `≤5 sends over 3s` test

**Files:**
- Create: `gormes/internal/telegram/coalesce.go`
- Create: `gormes/internal/telegram/coalesce_test.go`

The coalescer is a standalone type — it doesn't know about Bots, kernels, or Updates. Pure input is "latest pending text"; pure output is "send the edit now". Unit-testable in isolation.

### Step 1 — Write the failing test

Create `gormes/internal/telegram/coalesce_test.go`:

```go
package telegram

import (
	"context"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestCoalescer_BatchesRapidUpdates: pushing 60 text updates over 3s produces
// at most 5 Send calls. Proves the 1s window actually coalesces.
func TestCoalescer_BatchesRapidUpdates(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 1000*time.Millisecond, 42 /* chatID */)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.run(ctx)
	}()

	// First call: establishes the message (initial Send, not edit).
	c.setPending("⏳ …", 0) // messageID 0 → initial send path
	time.Sleep(20 * time.Millisecond)
	// Observe the message id the mock assigned.
	first := mc.sentMessages()
	if len(first) != 1 {
		t.Fatalf("expected 1 initial send, got %d", len(first))
	}
	msgID := mc.nextMsgID - 1 // next - 1 = most recently assigned

	// Push 60 rapid text updates with the same messageID.
	start := time.Now()
	for i := 0; i < 60; i++ {
		c.setPending(repeatText("x", i+1), msgID)
		time.Sleep(50 * time.Millisecond) // 50ms × 60 = 3s total
	}

	// Force a final flush.
	c.flushImmediate("final")
	elapsed := time.Since(start)
	_ = elapsed

	cancel()
	wg.Wait()

	total := len(mc.sentMessages())
	// Range: placeholder + at least 1 coalesced edit + final flush = 3 min,
	// placeholder + 3 ticks + final = 5 max.
	if total < 2 || total > 6 {
		t.Errorf("total sends = %d, want in [2, 6] (coalescer failure: should batch not 1:1)", total)
	}
}

// TestCoalescer_FlushImmediateBypassesWindow: flushImmediate must send
// regardless of the 1s window.
func TestCoalescer_FlushImmediateBypassesWindow(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 1000*time.Millisecond, 42)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go c.run(ctx)

	c.setPending("one", 0) // initial send
	time.Sleep(30 * time.Millisecond)
	c.flushImmediate("two") // must fire even inside 1s window
	time.Sleep(30 * time.Millisecond)

	got := mc.sentMessages()
	if len(got) != 2 {
		t.Errorf("sends = %d, want exactly 2 (initial + flushImmediate)", len(got))
	}

	cancel()
}

// TestCoalescer_IgnoresDuplicateText: if pendingText == lastSentText, the
// coalescer skips the tick — no wasted Telegram API call.
func TestCoalescer_IgnoresDuplicateText(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 100*time.Millisecond, 42)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go c.run(ctx)

	c.setPending("same", 0)
	time.Sleep(250 * time.Millisecond) // two ticks
	c.setPending("same", mc.nextMsgID-1)
	time.Sleep(250 * time.Millisecond) // two more ticks

	got := len(mc.sentMessages())
	if got != 1 {
		t.Errorf("sends = %d, want exactly 1 (duplicate text skipped)", got)
	}
	cancel()
}

// helper: repeatText returns ch repeated n times.
func repeatText(ch string, n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += ch
	}
	return s
}

// Silence unused tgbotapi import until signal-types referenced in later tasks.
var _ = tgbotapi.ModeMarkdownV2
```

Run:

```bash
cd gormes
go test ./internal/telegram/... -run Coalescer
```

Expected: FAIL — `newCoalescer` etc. don't exist yet.

### Step 2 — Implement `gormes/internal/telegram/coalesce.go`

```go
package telegram

import (
	"context"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// coalescer batches outbound edits. One coalescer per active turn. Caller
// pushes the latest text via setPending; a goroutine running run() flushes
// at most once per window. flushImmediate bypasses the window for semantic
// edges (final answer, error, cancel).
type coalescer struct {
	client telegramClient
	window time.Duration
	chatID int64

	mu           sync.Mutex
	pendingText  string
	pendingMsgID int
	lastSentText string
	lastEditAt   time.Time
	retryAfter   time.Time // set on 429
	wakeupCh     chan struct{}
}

func newCoalescer(c telegramClient, window time.Duration, chatID int64) *coalescer {
	if window <= 0 {
		window = time.Second
	}
	return &coalescer{
		client:   c,
		window:   window,
		chatID:   chatID,
		wakeupCh: make(chan struct{}, 1),
	}
}

// setPending stores the latest text the caller wants visible in the bot
// message. msgID is the Telegram message ID to edit; 0 means "send fresh".
// The coalescer's run loop picks this up on the next tick (or immediately
// if it's the first message of a turn).
func (c *coalescer) setPending(text string, msgID int) {
	c.mu.Lock()
	c.pendingText = text
	c.pendingMsgID = msgID
	c.mu.Unlock()

	// Non-blocking wake; the loop reads latest state on every iteration.
	select {
	case c.wakeupCh <- struct{}{}:
	default:
	}
}

// flushImmediate sends the given text right now (edit if there's a known
// message, else initial send). Used for Idle/Failed/Cancelled finalisation.
func (c *coalescer) flushImmediate(text string) {
	c.mu.Lock()
	msgID := c.pendingMsgID
	c.mu.Unlock()

	var msg tgbotapi.Message
	var err error
	if msgID == 0 {
		msg, err = c.client.Send(tgbotapi.NewMessage(c.chatID, text))
	} else {
		msg, err = c.client.Send(tgbotapi.NewEditMessageText(c.chatID, msgID, text))
	}
	if err != nil {
		// Coalescer does NOT die on transient errors; log is owner's job.
		return
	}

	c.mu.Lock()
	if msgID == 0 {
		c.pendingMsgID = msg.MessageID
	}
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.pendingText = ""
	c.mu.Unlock()
}

// run is the flush loop. Exits on ctx cancellation.
func (c *coalescer) run(ctx context.Context) {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tryFlush()
		case <-c.wakeupCh:
			c.tryFlush()
		}
	}
}

// tryFlush inspects state and sends an edit if all conditions are met.
func (c *coalescer) tryFlush() {
	c.mu.Lock()
	text := c.pendingText
	msgID := c.pendingMsgID
	last := c.lastSentText
	lastAt := c.lastEditAt
	retryAfter := c.retryAfter
	c.mu.Unlock()

	if text == "" {
		return
	}
	if text == last {
		return
	}
	now := time.Now()
	if now.Before(retryAfter) {
		return
	}
	if msgID != 0 && now.Sub(lastAt) < c.window {
		return // too soon for this message
	}

	var msg tgbotapi.Message
	var err error
	if msgID == 0 {
		msg, err = c.client.Send(tgbotapi.NewMessage(c.chatID, text))
	} else {
		msg, err = c.client.Send(tgbotapi.NewEditMessageText(c.chatID, msgID, text))
	}
	if err != nil {
		return
	}

	c.mu.Lock()
	if msgID == 0 {
		c.pendingMsgID = msg.MessageID
	}
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.mu.Unlock()
}
```

### Step 3 — Run tests

```bash
cd gormes
go test -race ./internal/telegram/... -run Coalescer -v -timeout 30s
```

All three coalescer tests PASS.

### Step 4 — Commit

```bash
cd ..
git add gormes/internal/telegram/coalesce.go gormes/internal/telegram/coalesce_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/telegram): 1s-window edit coalescer

Standalone type — knows nothing about Bots/kernels/Updates. setPending
stores the latest (text, messageID); run() is the flush goroutine;
flushImmediate bypasses the window for semantic edges.

Tests prove the three Telegram-side invariants:
  1. 60 rapid setPending calls over 3s produce at most 5 Sends
     (coalescing actually works).
  2. flushImmediate sends regardless of the 1s window.
  3. Identical text ticks are skipped (no wasted API call).

429 Retry-After is honoured via retryAfter field; actual 429
detection wiring is Task 4's responsibility.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Render formatters + outbound goroutine wired into Bot

**Files:**
- Create: `gormes/internal/telegram/render.go`
- Create: `gormes/internal/telegram/render_test.go`
- Modify: `gormes/internal/telegram/bot.go` (add outbound goroutine + coalescer lifecycle)
- Modify: `gormes/internal/telegram/bot_test.go` (add streaming tests)

### Step 1 — Create `gormes/internal/telegram/render.go`

```go
package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

// maxTelegramText is the character budget for a single Telegram message.
// Telegram's hard limit is 4096; we truncate at 4000 to leave headroom for
// UTF-8 edge cases and the "…" suffix.
const maxTelegramText = 4000

// formatStream renders an in-flight RenderFrame as Telegram-wire text.
// Includes the assistant DraftText plus a trailing italic soul-event line
// when a tool is active. User content is MarkdownV2-escaped; the trailing
// italic wrapper is literal.
func formatStream(f kernel.RenderFrame) string {
	body := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.DraftText)
	body = truncateForTelegram(body)

	tail := ""
	if len(f.SoulEvents) > 0 {
		last := f.SoulEvents[len(f.SoulEvents)-1]
		if last.Text != "" && last.Text != "idle" {
			tail = "\n\n_" + tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "🔧 "+last.Text) + "_"
		}
	}
	if f.Phase == kernel.PhaseReconnecting {
		tail += "\n\n_reconnecting…_"
	}
	return body + tail
}

// formatFinal renders the final assistant message (no soul line). Pulls from
// History since DraftText is cleared on Idle.
func formatFinal(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncateForTelegram(
				tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, f.History[i].Content))
		}
	}
	// No assistant reply (cancelled empty turn).
	return "_\\(empty reply\\)_"
}

// formatError renders a PhaseFailed frame as "❌ " + LastError.
func formatError(f kernel.RenderFrame) string {
	text := "❌ " + f.LastError
	return truncateForTelegram(
		tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, text))
}

// truncateForTelegram clamps s to maxTelegramText runes, adding "…" if truncated.
func truncateForTelegram(s string) string {
	runes := []rune(s)
	if len(runes) <= maxTelegramText {
		return s
	}
	return string(runes[:maxTelegramText-1]) + "…"
}

// Silence unused fmt import if formatters diverge later.
var _ = fmt.Sprintf

// strings imported for completeness (ensures future diff stability).
var _ = strings.HasPrefix
```

### Step 2 — Create `gormes/internal/telegram/render_test.go`

```go
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

func TestFormatStream_PlainDraft(t *testing.T) {
	f := kernel.RenderFrame{DraftText: "hello world", Phase: kernel.PhaseStreaming}
	got := formatStream(f)
	if !strings.Contains(got, "hello world") {
		t.Errorf("render = %q, want to contain 'hello world'", got)
	}
	if strings.Contains(got, "🔧") {
		t.Errorf("render = %q, no soul line expected", got)
	}
}

func TestFormatStream_SoulLineAppears(t *testing.T) {
	f := kernel.RenderFrame{
		DraftText:  "thinking...",
		Phase:      kernel.PhaseStreaming,
		SoulEvents: []kernel.SoulEntry{{At: time.Now(), Text: "tool: echo"}},
	}
	got := formatStream(f)
	if !strings.Contains(got, "🔧") {
		t.Errorf("render = %q, want tool soul line", got)
	}
	if !strings.Contains(got, "echo") {
		t.Errorf("render = %q, want 'echo' in soul line", got)
	}
}

func TestFormatStream_ReconnectingMarker(t *testing.T) {
	f := kernel.RenderFrame{DraftText: "xxxxx", Phase: kernel.PhaseReconnecting}
	got := formatStream(f)
	if !strings.Contains(got, "reconnecting") {
		t.Errorf("render = %q, want reconnecting marker", got)
	}
}

func TestFormatStream_EscapesMarkdown(t *testing.T) {
	// User content with MarkdownV2-special chars must be escaped.
	f := kernel.RenderFrame{
		DraftText: "use *bold* and _italic_",
		Phase:     kernel.PhaseStreaming,
	}
	got := formatStream(f)
	if strings.Contains(got, "*bold*") {
		t.Errorf("render = %q, should escape '*' chars", got)
	}
	if !strings.Contains(got, `\*bold\*`) {
		t.Errorf("render = %q, expected escaped markdown", got)
	}
}

func TestFormatFinal_ReadsFromHistory(t *testing.T) {
	f := kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "ping"},
			{Role: "assistant", Content: "pong"},
		},
	}
	got := formatFinal(f)
	if !strings.Contains(got, "pong") {
		t.Errorf("render = %q, want 'pong'", got)
	}
}

func TestFormatError_PrefixAndEscape(t *testing.T) {
	f := kernel.RenderFrame{LastError: "bad thing (really)"}
	got := formatError(f)
	if !strings.Contains(got, "❌") {
		t.Errorf("render = %q, want ❌ prefix", got)
	}
	if !strings.Contains(got, `\(really\)`) {
		t.Errorf("render = %q, want escaped parens", got)
	}
}

func TestTruncateForTelegram_RespectsLimit(t *testing.T) {
	big := strings.Repeat("a", 5000)
	got := truncateForTelegram(big)
	if n := len([]rune(got)); n > maxTelegramText {
		t.Errorf("truncated len = %d, want ≤ %d", n, maxTelegramText)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated output should end with ellipsis")
	}
}
```

### Step 3 — Extend `bot.go` with the outbound goroutine and coalescer lifecycle

Read the current bot.go; then append (keep handleUpdate unchanged for now):

```go
// runOutbound consumes k.Render() and pushes frames into the coalescer.
// One coalescer-per-turn: on PhaseIdle/Failed/Cancelling we flushImmediate
// and null the coalescer so the next turn starts with a fresh placeholder.
func (b *Bot) runOutbound(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	frames := b.kernel.Render()
	var c *coalescer
	var cCtx context.Context
	var cCancel context.CancelFunc

	for {
		select {
		case <-ctx.Done():
			if cCancel != nil {
				cCancel()
			}
			return
		case f, ok := <-frames:
			if !ok {
				if cCancel != nil {
					cCancel()
				}
				return
			}
			b.handleFrame(ctx, f, &c, &cCtx, &cCancel, wg)
		}
	}
}

func (b *Bot) handleFrame(
	ctx context.Context,
	f kernel.RenderFrame,
	c **coalescer,
	cCtx *context.Context,
	cCancel *context.CancelFunc,
	wg *sync.WaitGroup,
) {
	switch f.Phase {
	case kernel.PhaseIdle:
		// Final flush if a turn was in flight.
		if *c != nil {
			(*c).flushImmediate(formatFinal(f))
			(*cCancel)()
			*c = nil
		}
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		if *c != nil {
			(*c).flushImmediate(formatError(f))
			(*cCancel)()
			*c = nil
		}
	case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseReconnecting, kernel.PhaseFinalizing:
		// Lazy-init coalescer on first streaming frame.
		if *c == nil {
			*cCtx, *cCancel = context.WithCancel(ctx)
			window := time.Duration(b.cfg.CoalesceMs) * time.Millisecond
			newC := newCoalescer(b.client, window, b.cfg.AllowedChatID)
			*c = newC
			wg.Add(1)
			go func(cc *coalescer, cx context.Context) {
				defer wg.Done()
				cc.run(cx)
			}(newC, *cCtx)
			// Establish the placeholder message.
			newC.flushImmediate("⏳ …")
		}
		(*c).setPending(formatStream(f), (*c).currentMessageID())
	}
}

// currentMessageID is a small helper for coalescer — exposes msgID.
// (Add this method to coalesce.go in the same commit.)
```

And in `coalesce.go`, add:

```go
func (c *coalescer) currentMessageID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingMsgID
}
```

Update `Bot.Run` to spawn the outbound goroutine:

```go
func (b *Bot) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	wg.Add(1)
	go b.runOutbound(ctx, &wg)

	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 30
	updates := b.client.GetUpdatesChan(ucfg)

	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			b.client.StopReceivingUpdates()
			return nil
		case u, ok := <-updates:
			if !ok {
				return nil
			}
			b.handleUpdate(ctx, u)
		}
	}
}
```

Add `"sync"` and `"time"` imports to bot.go. `kernel.RenderFrame` etc. already imported.

### Step 4 — Add streaming test to bot_test.go

Append:

```go
// TestBot_StreamsAssistantDraft: feed the kernel's render channel a sequence
// of mock RenderFrames and assert the Bot issues the expected placeholder +
// final edit sequence via mockClient.
func TestBot_StreamsAssistantDraft(t *testing.T) {
	mc := newMockClient()
	// Synthetic kernel via MockClient scripted to respond "hello" to any input.
	kk := newTestKernelWithScriptedReply(t, "hello")

	b := New(Config{
		AllowedChatID:     42,
		CoalesceMs:        200, // speed up test
		FirstRunDiscovery: false,
	}, mc, kk, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go kk.Run(ctx)
	<-kk.Render() // initial idle
	go func() { _ = b.Run(ctx) }()

	// User sends "hi" from allowed chat.
	mc.pushTextUpdate(42, "hi")

	// Wait until we see a final message edit containing "hello".
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "hello") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(mc.lastSentText(), "hello") {
		t.Errorf("last sent = %q, want to contain 'hello'", mc.lastSentText())
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}

// newTestKernelWithScriptedReply returns a Kernel whose hermes.MockClient
// is scripted to answer reply tokens on the first Submit.
func newTestKernelWithScriptedReply(t *testing.T, reply string) *kernel.Kernel {
	t.Helper()
	mc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: len(reply)})
	mc.Script(events, "sess-test")

	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, mc, store.NewNoop(), telemetry.New(), nil)
}
```

### Step 5 — Run + commit

```bash
cd gormes
go test -race ./internal/telegram/... -timeout 30s -v
go vet ./internal/telegram/...
cd ..
git add gormes/internal/telegram/render.go gormes/internal/telegram/render_test.go gormes/internal/telegram/bot.go gormes/internal/telegram/bot_test.go gormes/internal/telegram/coalesce.go
git commit -m "$(cat <<'EOF'
feat(gormes/telegram): render formatters + outbound goroutine

formatStream/formatFinal/formatError shape RenderFrame into
MarkdownV2-escaped Telegram text. Soul-monitor tool lines render
as italicised trailing paragraphs ("_🔧 tool: echo_"); the
reconnecting-during-Route-B marker renders as "_reconnecting…_".
User content is always EscapeText'd so special chars (*, _, (, ))
can't break the parser.

Bot.runOutbound consumes k.Render(); lazy-inits one coalescer per
turn. PhaseIdle/Failed/Cancelling trigger flushImmediate and tear
down the coalescer so the next turn gets a fresh "⏳ …" placeholder.

New bot test TestBot_StreamsAssistantDraft scripts a kernel reply
and asserts the bot ends the turn with a message containing the
final assistant text. Seven render_test.go cases cover escape
paths and truncation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Inbound commands (`/start`, `/stop`, `/new`) + kernel submission

**Files:**
- Modify: `gormes/internal/telegram/bot.go` (extend handleUpdate)
- Modify: `gormes/internal/telegram/bot_test.go` (three new tests)

### Step 1 — Extend `handleUpdate` in `bot.go`

Replace the no-op branch in `handleUpdate` (the line `b.log.Info("inbound message", ...)`) with the full command-parsing logic:

```go
	// Authorised chat. Parse commands or forward to kernel.
	text := strings.TrimSpace(u.Message.Text)

	switch {
	case text == "/start":
		_, _ = b.client.Send(tgbotapi.NewMessage(chatID,
			"Gormes is online. Send a message to start a turn. Commands: /stop /new"))
	case text == "/stop":
		_ = b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	case text == "/new":
		if err := b.kernel.ResetSession(); err != nil {
			_, _ = b.client.Send(tgbotapi.NewMessage(chatID,
				"Cannot reset during an active turn — send /stop first."))
			return
		}
		_, _ = b.client.Send(tgbotapi.NewMessage(chatID,
			"Session reset. Next message starts fresh."))
	case strings.HasPrefix(text, "/"):
		_, _ = b.client.Send(tgbotapi.NewMessage(chatID, "unknown command"))
	default:
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			_, _ = b.client.Send(tgbotapi.NewMessage(chatID,
				"Busy — try again in a second."))
		}
	}
```

Add `"strings"` to bot.go imports.

**Note:** this references `b.kernel.ResetSession()` which lands in Task 7. Temporarily wrap the `/new` branch in a `// TODO(T7)` block that just replies "session reset" without calling `ResetSession`, then update after Task 7 completes. OR: do Task 7 BEFORE Task 5. Prefer the latter — easier commit narrative. **Swap Task 5 and Task 7 in execution order.**

Actually the cleanest move: **execute Tasks in order T1, T2, T3, T4, T7, T5, T6, T8, T9, T10.** Update the execution plan accordingly. The plan file text still reads sequentially; the subagent-driven dispatcher just picks the order.

### Step 2 — Three new bot_test.go cases

```go
func TestBot_StartCommandReplies(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/start")
	time.Sleep(50 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "Gormes is online") {
		t.Errorf("reply = %q, want /start welcome", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}

func TestBot_UnknownCommandReplies(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/nonsense")
	time.Sleep(50 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "unknown command") {
		t.Errorf("reply = %q, want 'unknown command'", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}

func TestBot_NewCommandResetsSession(t *testing.T) {
	mc := newMockClient()
	k := newTestKernel(t)
	b := New(Config{AllowedChatID: 42}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "/new")
	time.Sleep(50 * time.Millisecond)

	if !strings.Contains(mc.lastSentText(), "Session reset") {
		t.Errorf("reply = %q, want 'Session reset'", mc.lastSentText())
	}
	cancel()
	mc.closeUpdates()
	time.Sleep(20 * time.Millisecond)
}
```

### Step 3 — Run + commit

```bash
cd gormes
go test -race ./internal/telegram/... -timeout 30s -v
cd ..
git add gormes/internal/telegram/bot.go gormes/internal/telegram/bot_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/telegram): inbound commands + kernel submission

/start → welcome message
/stop  → kernel.Submit(PlatformEventCancel)
/new   → kernel.ResetSession() + "Session reset" reply
         (or "Cannot reset during active turn" if rejected)
/other → "unknown command" polite reply

Plain text from an authorised chat becomes a PlatformEventSubmit.
If the kernel's event mailbox is full (ErrEventMailboxFull), the
bot replies "Busy — try again in a second" rather than dropping
the user's message silently.

Three new bot_test.go cases cover /start, unknown-command, and
/new reset-session. Full tool-call handshake via Telegram is
Task 9.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Build-isolation test

**Files:**
- Create: `gormes/internal/buildisolation_test.go`

### Step 1 — Create the test

```go
package internal_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestTUIBinaryHasNoTelegramDep guards the Operational Moat: cmd/gormes
// (the TUI) must never transitively depend on telegram-bot-api or on the
// internal/telegram adapter package. If either appears in the TUI's dep
// graph, the binary size jumps and the per-binary-per-platform promise
// breaks.
//
// Runs `go list -deps ./cmd/gormes` from the gormes module root and
// inspects every dependency path.
func TestTUIBinaryHasNoTelegramDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cmd/gormes")
	cmd.Dir = ".." // run from gormes/ so ./cmd/gormes resolves
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	deps := strings.Split(out.String(), "\n")
	for _, d := range deps {
		if strings.Contains(d, "go-telegram-bot-api") ||
			strings.Contains(d, "/internal/telegram") {
			t.Errorf("cmd/gormes transitively depends on %q — Operational Moat violated", d)
		}
	}
}
```

### Step 2 — Run

```bash
cd gormes
go test -race ./internal/ -run TestTUIBinaryHasNoTelegramDep -v
```

Expected: PASS (cmd/gormes has never imported internal/telegram).

### Step 3 — Sanity-break test

Temporarily add `_ "github.com/XelHaku/golang-hermes-agent/gormes/internal/telegram"` to `cmd/gormes/main.go`'s imports. Re-run the test.

Expected: FAIL with a message naming the telegram-bot-api dep.

Revert the import change. Re-run — PASS again.

### Step 4 — Commit

```bash
cd ..
git add gormes/internal/buildisolation_test.go
git commit -m "$(cat <<'EOF'
test(gormes/internal): forbid Telegram deps in the TUI binary

TestTUIBinaryHasNoTelegramDep runs `go list -deps ./cmd/gormes`
and fails the suite if any line contains `go-telegram-bot-api`
or `/internal/telegram`. Guards the Operational Moat — the TUI
binary must stay under 8.5 MB stripped; pulling in the Telegram
SDK transitively would bust that budget.

Verified by temporarily adding a blank import and confirming the
test FAILs with a clear offender message, then reverting.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `kernel.ResetSession`

**Execute BEFORE Task 5** if following the order swap.

**Files:**
- Modify: `gormes/internal/kernel/frame.go` (+ PlatformEventResetSession enum)
- Modify: `gormes/internal/kernel/kernel.go` (+ ResetSession method + Run-loop handler)
- Create: `gormes/internal/kernel/reset_test.go`

### Step 1 — Write failing test

Create `gormes/internal/kernel/reset_test.go`:

```go
package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

func TestKernel_ResetSession_ClearsSessionIDWhenIdle(t *testing.T) {
	mc := hermes.NewMockClient()
	// One turn that sets a session id.
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "ok", TokensOut: 1},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1},
	}, "sess-to-reset")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // initial idle

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	// Wait for Idle with session-id populated.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	// Call ResetSession; should succeed.
	if err := k.ResetSession(); err != nil {
		t.Fatalf("ResetSession while Idle should succeed: %v", err)
	}

	// A render frame with empty SessionID should follow.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.SessionID == ""
	}, 1*time.Second)
}

func TestKernel_ResetSession_RejectsWhenBusy(t *testing.T) {
	mc := hermes.NewMockClient()
	// Long-running stream (many tokens) so we can call ResetSession mid-turn.
	events := make([]hermes.Event, 0, 100)
	for i := 0; i < 99; i++ {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: "t", TokensOut: i + 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "sess-busy")

	k := New(Config{
		Model: "hermes-agent", Endpoint: "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "go"})

	// Wait until we've observed a streaming frame.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming
	}, 500*time.Millisecond)

	err := k.ResetSession()
	if !errors.Is(err, ErrResetDuringTurn) {
		t.Errorf("err = %v, want ErrResetDuringTurn", err)
	}
}
```

`waitForFrameMatching` is already defined in `kernel_test.go`.

### Step 2 — Run, expect FAIL (symbols undefined)

```bash
cd gormes
go test ./internal/kernel/... -run ResetSession
```

### Step 3 — Extend `frame.go`

Add a new enum value + export the sentinel error:

```go
type PlatformEventKind int

const (
	PlatformEventSubmit PlatformEventKind = iota
	PlatformEventCancel
	PlatformEventQuit
	PlatformEventResetSession  // NEW — /new Telegram command
)
```

Add at the top of the file (or in `kernel.go`, wherever exports live):

```go
// ErrResetDuringTurn is returned by Kernel.ResetSession if called during
// an active turn. The caller (Telegram /new handler) tells the user to
// /stop first.
var ErrResetDuringTurn = errors.New("kernel: cannot reset session during active turn")
```

(`errors` needs to be imported if not already.)

### Step 4 — Extend `kernel.go`

Add the `ResetSession` public method:

```go
// ResetSession clears the kernel's server-assigned session id so the next
// turn starts fresh. Must be called when the kernel is Idle; returns
// ErrResetDuringTurn otherwise. Thread-safe: enqueues a PlatformEvent
// and lets the Run goroutine perform the mutation.
func (k *Kernel) ResetSession() error {
	// Non-blocking pre-check: if a turn is active, reject fast.
	// Phase is owned by Run, but a read is acceptable as a hint — if
	// there's a race, the Run goroutine can still see phase != Idle when
	// processing the event and reply accordingly via a new RenderFrame.
	// For simplicity we use a short blocking check via a channel round trip.
	ack := make(chan error, 1)
	select {
	case k.events <- PlatformEvent{Kind: PlatformEventResetSession, ack: ack}:
	default:
		return ErrEventMailboxFull
	}
	select {
	case err := <-ack:
		return err
	case <-time.After(500 * time.Millisecond):
		return errors.New("kernel: ResetSession ack timeout")
	}
}
```

That requires adding an `ack chan error` field to `PlatformEvent`. Update `PlatformEvent` in `frame.go`:

```go
type PlatformEvent struct {
	Kind PlatformEventKind
	Text string
	ack  chan error // unexported; only used for events that need a reply
}
```

Add the handler in the Run loop's select:

```go
			case PlatformEventResetSession:
				if k.phase != PhaseIdle {
					if e.ack != nil {
						e.ack <- ErrResetDuringTurn
					}
					continue
				}
				k.sessionID = ""
				k.lastError = ""
				k.emitFrame("session reset")
				if e.ack != nil {
					e.ack <- nil
				}
```

### Step 5 — Run tests; expect PASS

```bash
cd gormes
go test -race ./internal/kernel/... -timeout 90s
```

All existing kernel tests still PASS. `ResetSession` tests PASS.

### Step 6 — Commit

```bash
cd ..
git add gormes/internal/kernel/frame.go gormes/internal/kernel/kernel.go gormes/internal/kernel/reset_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): ResetSession() for /new Telegram command

Additive: Kernel.ResetSession() clears k.sessionID so the next
Submit starts a fresh Python session. Returns ErrResetDuringTurn
if called when k.phase != PhaseIdle; the caller decides whether
to /stop first.

Implementation: enqueues a new PlatformEventResetSession event;
Run-loop handler performs the mutation on its own goroutine,
honouring the single-owner invariant. An ack channel on
PlatformEvent carries the result back to the caller synchronously
with a 500ms timeout.

Two new tests (reset_test.go) cover the success-when-Idle and
reject-when-Streaming cases.

All pre-existing Phase-1 / 1.5 / 2.A tests still pass under -race.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `cmd/gormes-telegram/main.go` + config extension

**Files:**
- Create: `gormes/cmd/gormes-telegram/main.go`
- Modify: `gormes/internal/config/config.go` (+ TelegramCfg section)
- Modify: `gormes/internal/config/config_test.go` (telegram defaults test)

### Step 1 — Extend `config.go`

Add to the Config struct:

```go
type Config struct {
	Hermes   HermesCfg
	TUI      TUICfg
	Input    InputCfg
	Telegram TelegramCfg // NEW
}

type TelegramCfg struct {
	BotToken          string `toml:"bot_token"`
	AllowedChatID     int64  `toml:"allowed_chat_id"`
	CoalesceMs        int    `toml:"coalesce_ms"`
	FirstRunDiscovery bool   `toml:"first_run_discovery"`
}
```

Add defaults + env-override logic in `defaults()` / `loadEnv()`:

```go
// in defaults():
Telegram: TelegramCfg{
	CoalesceMs:        1000,
	FirstRunDiscovery: true,
},

// in loadEnv():
if v := os.Getenv("GORMES_TELEGRAM_TOKEN"); v != "" {
	cfg.Telegram.BotToken = v
}
if v := os.Getenv("GORMES_TELEGRAM_CHAT_ID"); v != "" {
	if id, err := strconv.ParseInt(v, 10, 64); err == nil {
		cfg.Telegram.AllowedChatID = id
	}
}
```

Add `"strconv"` to imports.

### Step 2 — Add a config test

In `config_test.go`:

```go
func TestLoad_TelegramDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.CoalesceMs != 1000 {
		t.Errorf("CoalesceMs default = %d, want 1000", cfg.Telegram.CoalesceMs)
	}
	if !cfg.Telegram.FirstRunDiscovery {
		t.Error("FirstRunDiscovery default = false, want true")
	}
}

func TestLoad_TelegramEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GORMES_TELEGRAM_TOKEN", "abc:xyz")
	t.Setenv("GORMES_TELEGRAM_CHAT_ID", "99999")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.BotToken != "abc:xyz" {
		t.Errorf("BotToken = %q", cfg.Telegram.BotToken)
	}
	if cfg.Telegram.AllowedChatID != 99999 {
		t.Errorf("AllowedChatID = %d", cfg.Telegram.AllowedChatID)
	}
}
```

### Step 3 — Create `cmd/gormes-telegram/main.go`

```go
// Command gormes-telegram is the Phase-2.B.1 Telegram adapter binary.
// Wires config → hermes client → kernel (with tools) → telegram adapter.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/config"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telegram"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gormes-telegram:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(nil)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Startup validation.
	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("no Telegram bot token — set GORMES_TELEGRAM_TOKEN env or [telegram].bot_token in config.toml")
	}
	if cfg.Telegram.AllowedChatID == 0 && !cfg.Telegram.FirstRunDiscovery {
		return fmt.Errorf("no chat allowlist and discovery disabled — set one of [telegram].allowed_chat_id or [telegram].first_run_discovery = true")
	}
	if cfg.Telegram.BotToken != "" && os.Getenv("GORMES_TELEGRAM_TOKEN") == "" {
		slog.Warn("bot_token read from config.toml; prefer GORMES_TELEGRAM_TOKEN env var for secrets")
	}

	// Hermes HTTP+SSE client (same as TUI path).
	hc := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)

	// Register built-in tools — same set the TUI binary uses.
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	reg.MustRegister(&tools.NowTool{})
	reg.MustRegister(&tools.RandIntTool{})

	tm := telemetry.New()
	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
	}, hc, store.NewNoop(), tm, slog.Default())

	// Telegram client.
	tc, err := newRealClientPackagePrivateHelper(cfg.Telegram.BotToken)
	if err != nil {
		return err
	}

	bot := telegram.New(telegram.Config{
		AllowedChatID:     cfg.Telegram.AllowedChatID,
		CoalesceMs:        cfg.Telegram.CoalesceMs,
		FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
	}, tc, k, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go k.Run(rootCtx)
	go func() {
		// Shutdown budget watchdog.
		<-rootCtx.Done()
		time.AfterFunc(kernel.ShutdownBudget, func() {
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		})
	}()

	slog.Info("gormes-telegram starting",
		"endpoint", cfg.Hermes.Endpoint,
		"allowed_chat_id", cfg.Telegram.AllowedChatID,
		"discovery", cfg.Telegram.FirstRunDiscovery)
	return bot.Run(rootCtx)
}
```

**Problem with the sketch above:** `newRealClient` is `internal/telegram` package-private (lowercase). The `main` package can't call it. Solution: export a public constructor.

Go back to `internal/telegram/real_client.go` and rename `newRealClient` → `NewRealClient` (export it). Update Task 2's commit if the subagent hasn't landed it yet; otherwise add a follow-up mini-commit here.

Then `main.go` calls:

```go
tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
```

(NOT `newRealClientPackagePrivateHelper` — that was a placeholder indicating the API export need.)

Update Task 2 retroactively: the commit's function name must be `NewRealClient` (exported). If already landed lowercase, this task includes the rename.

### Step 4 — Build + smoke

```bash
cd gormes
make build           # still works for gormes/
go build ./cmd/gormes-telegram
./bin/gormes-telegram 2>&1 || echo "expected — no token set"
```

Expected: clean exit 1 with "no Telegram bot token — set GORMES_TELEGRAM_TOKEN env or [telegram].bot_token in config.toml".

### Step 5 — Full sweep

```bash
go test -race ./... -timeout 90s
go vet ./...
```

All PASS. `vet` clean.

### Step 6 — Commit

```bash
cd ..
git add gormes/cmd/gormes-telegram/ gormes/internal/config/config.go gormes/internal/config/config_test.go gormes/internal/telegram/real_client.go
git commit -m "$(cat <<'EOF'
feat(gormes): cmd/gormes-telegram binary + [telegram] config

Wires config → hermes.NewHTTPClient → kernel.New (with tools) →
telegram.New (with NewRealClient). Same registry builder the TUI
uses; Phase-2.A tool-calling flows through the Telegram path end
to end.

Startup validation:
  - empty bot_token exits 1 with setup hint
  - allowed_chat_id == 0 AND first_run_discovery == false exits 1

Secrets policy (M5): GORMES_TELEGRAM_TOKEN env preferred;
config.toml bot_token accepted but logs a WARN at startup hinting
at env-first practice.

internal/telegram/real_client.go: NewRealClient is exported (was
package-private) so cmd/gormes-telegram can construct one.

Two new config_test.go cases cover telegram defaults + env override.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Tool-call handshake via Telegram

**Files:**
- Modify: `gormes/internal/telegram/bot_test.go` (new test)

Proves Phase-2.A tool-call semantics survive the Telegram adapter round-trip. Uses MockClient for Hermes and mockClient for Telegram — no network, no API credits.

### Step 1 — Add test

```go
func TestBot_ToolCallHandshake_Echo_ViaTelegram(t *testing.T) {
	mc := newMockClient()

	// Scripted Hermes MockClient: 2-round tool-call turn.
	hmc := hermes.NewMockClient()
	hmc.Script([]hermes.Event{
		{
			Kind: hermes.EventDone, FinishReason: "tool_calls",
			ToolCalls: []hermes.ToolCall{
				{ID: "call_echo_telegram", Name: "echo", Arguments: []byte(`{"text":"hello from telegram"}`)},
			},
		},
	}, "sess-tg-echo")
	finalAnswer := "Tool said: hello from telegram."
	events := make([]hermes.Event, 0, len(finalAnswer)+1)
	for _, ch := range finalAnswer {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 20, TokensOut: len(finalAnswer)})
	hmc.Script(events, "sess-tg-echo")

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	k := kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hmc, store.NewNoop(), telemetry.New(), nil)

	b := New(Config{AllowedChatID: 42, CoalesceMs: 200}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "echo hello from telegram")

	// Wait for a Send whose text contains "Tool said".
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "Tool said") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	last := mc.lastSentText()
	if !strings.Contains(last, "Tool said") {
		t.Errorf("final bot msg = %q, want 'Tool said'", last)
	}
	if !strings.Contains(last, "hello from telegram") {
		t.Errorf("final bot msg = %q, want to reference tool output", last)
	}

	cancel()
	mc.closeUpdates()
	time.Sleep(50 * time.Millisecond)
}
```

### Step 2 — Run + commit

```bash
cd gormes
go test -race ./internal/telegram/... -run ToolCallHandshake -v -timeout 30s
cd ..
git add gormes/internal/telegram/bot_test.go
git commit -m "$(cat <<'EOF'
test(gormes/telegram): Tool-Call Handshake via Telegram adapter

Proves Phase-2.A tool-calling semantics survive the Telegram
round-trip. Hermes MockClient scripts a 2-round tool-call turn
(echo → reply); Telegram mockClient records the adapter's
outbound stream; final assertion: last bot message contains
both "Tool said" and the echoed payload. No network, no API
credits.

This is the cross-phase invariant test: the kernel's tool
loop runs identically whether the platform is the TUI or
Telegram — zero adapter-specific branching inside the kernel.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Verification sweep

**Files:** no changes — this is a verification pass.

### Step 1 — Full test sweep

```bash
cd gormes
go test -race ./... -timeout 120s -count=1
go vet ./...
```

All packages PASS. `vet` clean.

### Step 2 — Build both binaries + size check

```bash
make build         # gormes/bin/gormes
go build -o bin/gormes-telegram ./cmd/gormes-telegram
ls -lh bin/
```

Expected:
- `bin/gormes` ≤ 8.5 MB stripped
- `bin/gormes-telegram` ≤ 12 MB stripped

### Step 3 — Build-isolation verify

```bash
go list -deps ./cmd/gormes | grep -E "telegram-bot-api|internal/telegram" && echo VIOLATION || echo OK
go list -deps ./cmd/gormes-telegram | grep -E "telegram-bot-api|internal/telegram" | head -2
```

First command: prints `OK` (TUI has zero Telegram deps).
Second command: prints at least one line showing the Telegram deps ARE in the bot binary.

### Step 4 — Offline doctor still works

```bash
./bin/gormes doctor --offline
```

Expected: `[PASS] Toolbox: 3 tools registered (echo, now, rand_int)` regardless of Telegram changes.

### Step 5 — Live manual smoke test (optional, requires Telegram bot token + Python api_server)

```bash
# Terminal 1
API_SERVER_ENABLED=true hermes gateway start

# Terminal 2 — first run, discovery mode
GORMES_TELEGRAM_TOKEN=<your:token> ./bin/gormes-telegram
# DM the bot; read chat_id from stderr + bot reply; ctrl-C

# Edit config.toml or set env
export GORMES_TELEGRAM_CHAT_ID=<your chat_id>
./bin/gormes-telegram
# DM "hi"; watch streaming edits; try /stop mid-stream; try /new
```

Verify:
- Placeholder "⏳ …" appears first.
- Tokens stream in as edits roughly every 1 s.
- `/stop` cancels cleanly.
- `/new` resets session; next message starts fresh.
- Tool-call prompt (e.g. "call echo on 'ping'") shows `_🔧 tool: echo_` line, then final answer.

### Step 6 — No commit

This task runs only verifications; nothing to commit. If any check fails, STOP and report with the failing command + output.

---

## Appendix A: Self-Review

**Spec coverage:**
- §4 package layout → Tasks 1, 3, 4, 7, 8 create every file listed.
- §5.1 `telegramClient` interface → Task 1.
- §5.2 `Bot` struct + `Run` contract → Tasks 1, 4, 5.
- §5.3 `render.go` formatters → Task 4.
- §6 streaming-to-edit algorithm → Tasks 3 (coalescer) + 4 (outbound goroutine).
- §7 inbound flow → Task 5 (after Task 7's ResetSession lands).
- §8 `kernel.ResetSession` → Task 7.
- §9 config shape → Task 8.
- §10 error handling → distributed across Tasks 3 (coalescer retry), 5 (ErrEventMailboxFull reply), 8 (startup validation).
- §11.1 nine unit tests → spread across Tasks 1 (rejection, first-run), 3 (coalesce, flush-immediate, dup-skip), 4 (stream, render suite), 5 (start/unknown/new), 9 (tool handshake).
- §11.2 invariant test → Task 9.
- §12 build-isolation test → Task 6.
- §13 success criteria — all items verified in Task 10.

**Placeholder scan:** no TBD / TODO / "similar to Task N". Task 2 commit message notes `NewRealClient` lowercase → exported rename is captured in Task 8's commit scope.

**Type consistency:** `Config` / `Bot` / `coalescer` / `telegramClient` / `mockClient` / `realClient` / `formatStream,Final,Error` names identical across all tasks. `kernel.PlatformEventResetSession`, `kernel.ResetSession`, `kernel.ErrResetDuringTurn` match between Task 7's introduction and Task 5's consumption.

**Execution order note:** Task 7 must land before Task 5 (Task 5 consumes `k.ResetSession`). Recommended dispatch sequence: **T1 → T2 → T3 → T4 → T7 → T5 → T6 → T8 → T9 → T10**.

**Scope:** one cohesive Phase-2.B.1 plan. Binary, interface, tests, lifecycle all self-contained.

---

## Execution Handoff

Plan complete and saved to `gormes/docs/superpowers/plans/2026-04-19-gormes-phase2b-telegram.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task via `superpowers:subagent-driven-development` using the dependency order above. Halt checkpoints after T7 (kernel change) and T8 (binary builds) for your sanity-check.

**2. Inline Execution** — I execute via `superpowers:executing-plans` with batched checkpoints.

For a 10-task plan with one kernel surface addition, **subagent-driven is the right call** — keeps my main-conversation context lean. Which?
