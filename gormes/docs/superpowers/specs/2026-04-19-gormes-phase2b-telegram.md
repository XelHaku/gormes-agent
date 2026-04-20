# Gormes Phase 2.B.1 — Telegram Scout Design Spec

**Date:** 2026-04-19
**Author:** Xel (via Claude Code brainstorm)
**Status:** Approved for plan phase
**Scope:** Phase 2.B.1 of the 5-phase roadmap — the first "External Hand" binary. Ships a single-purpose `cmd/gormes-telegram` binary that drives the existing Phase-1/1.5/2.A kernel over a Telegram bot private-DM interface.

**Vocabulary decision:** there is no `internal/gateway` package. The Telegram adapter lives at `internal/telegram/` as a sibling to `internal/tui/` — both consume the existing `PlatformEvent` / `RenderFrame` contracts. The kernel does not change a single line.

## Related Documents

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Specs Index](README.md)
- [Phase 2.A — Tool Registry](2026-04-19-gormes-phase2-tools-design.md)

---

## 1. Purpose

Put Gormes on your phone. A Telegram bot that receives a user's DM, passes the text through the same `kernel.Submit(PlatformEventSubmit)` the TUI uses today, and streams the response back by editing the original bot message at 1-second intervals. The kernel stays platform-agnostic; the Telegram adapter is a thin translator between `tgbotapi` Updates and `kernel.PlatformEvent`, and between `kernel.RenderFrame` and `editMessageText`.

This ships the "Industrial Portability" promise: one `scp gormes-telegram user@vps:/usr/local/bin/` + a systemd unit and a `$5` VPS is running your personal bot.

---

## 2. Relationship to prior phases

| Phase | What it owns |
|---|---|
| Phase 1 | TUI + kernel state machine + hermes HTTP/SSE client + zero-store |
| Phase 1.5 | Route-B reconnect + compat-probe + discipline tests + `.ai` rename |
| Phase 2.A | `internal/tools` + `tool_calls` flow in `runTurn` + doctor `CheckTools` |
| **Phase 2.B.1 (this spec)** | **`internal/telegram` + `cmd/gormes-telegram` binary** |
| Phase 2.B.2 | Discord adapter (same shape, different SDK) |
| Phase 3 | `internal/brain` — prompt assembly, native LLM provider |

Phase 2.B.1 changes pre-existing packages in exactly **two** places, both additive:
- `internal/config` — adds a new `[telegram]` TOML block + `TelegramCfg` struct field (no behavior change for existing callers).
- `internal/kernel` — adds one new method `ResetSession()` + one new `PlatformEventResetSession` enum value to implement the `/new` Telegram command. See §8 for the full contract. This is the FIRST change to `internal/kernel` outside its own package since Phase 2.A landed, and it is necessary-not-optional.

`internal/tools`, `internal/hermes`, `internal/doctor`, `internal/tui`, `internal/store`, `internal/telemetry`, `pkg/gormes`, and `cmd/gormes/` stay byte-identical.

---

## 3. Locked Architectural Decisions

From brainstorm. All five principal + five micro-decisions locked before spec-writing.

### Principal decisions

| Decision | Value | Rationale |
|---|---|---|
| Architecture | **Approach A** — per-platform binaries | TUI stays 7.9 MB; Telegram SDK never enters that binary. Crash isolation per platform. `scp`-and-run deployment. |
| Telegram SDK | `github.com/go-telegram-bot-api/telegram-bot-api/v5` | Mature, de-facto, pure Go (CGO-free preserved), edge-cases already handled. |
| Transport | **Long-polling** | No HTTPS / cert / ingress / webhook plumbing. Works behind a home router. |
| Multi-chat | **Single chat per process** | One kernel, one mission. Multi-chat is Phase 3+ when memory layer can partition sessions. |
| Streaming | **Edit-as-tokens with 1 s coalesce** | Respects Telegram's ~1 edit/sec limit while keeping the "live" feel. |

### Micro-decisions (M1–M5)

| # | Decision | Value |
|---|---|---|
| M1 | Chat allowlist | `[telegram].allowed_chat_id` required; unauthenticated chats get one polite "not authorised" message and are then muted for the rest of the session. |
| M2 | First-run discovery | If `allowed_chat_id == 0`, the adapter logs incoming chat_ids to stderr AND replies with `send this to the operator: chat_id=NNN`. Configurable via `[telegram].first_run_discovery`. |
| M3 | Long messages | Truncate at 4000 chars with `…` suffix. Telegram's hard limit is 4096; the 96-char gap covers UTF-8 edge cases + ellipsis rendering. |
| M4 | Commands | `/start` (welcome), `/stop` (cancel in-flight turn), `/new` (clear `X-Hermes-Session-Id` → next message starts a fresh Python session). |
| M5 | Bot token source | Env `GORMES_TELEGRAM_TOKEN` preferred. `[telegram].bot_token` accepted but a startup log line warns against committing it. |

---

## 4. Architecture

### 4.1 Package layout

```
gormes/
├── cmd/
│   ├── gormes/                     # UNCHANGED — 7.9 MB TUI binary
│   └── gormes-telegram/            # NEW — Telegram bot binary
│       └── main.go
├── internal/
│   ├── telegram/                   # NEW — adapter package
│   │   ├── bot.go                  # Bot struct, Run() loop, inbound+outbound goroutines
│   │   ├── client.go               # telegramClient interface, realClient wrapper
│   │   ├── coalesce.go             # 1-second edit coalescer
│   │   ├── render.go               # RenderFrame → Telegram-text formatter (soul line, truncation)
│   │   ├── mock_test.go            # MockTelegramAPI test double + shared test helpers
│   │   ├── bot_test.go             # end-to-end kernel+mock tests
│   │   └── coalesce_test.go
│   └── config/
│       └── config.go               # MODIFY — add TelegramCfg section
```

**Zero changes to:** `internal/kernel`, `internal/tools`, `internal/hermes`, `internal/doctor`, `internal/store`, `internal/telemetry`, `internal/tui`, `pkg/gormes`, `cmd/gormes/`.

### 4.2 Process model

```
                        gormes-telegram (pid N)
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│   Telegram API                   internal/telegram.Bot                  │
│   ───────────                    ─────────────────────                  │
│                                                                         │
│    UpdatesChan ──────► inbound goroutine                                │
│    (long-poll,          │ filter allowed_chat_id                        │
│     25s timeout)        │ /start, /stop, /new command parsing           │
│                         │ plain text → PlatformEventSubmit              │
│                         ▼                                               │
│                    k.Submit(...)                                        │
│                         │                                               │
│                         ▼                                               │
│                   internal/kernel (UNCHANGED)                           │
│                         │                                               │
│                         ▼                                               │
│                    k.Render() ◄── RenderFrame ── outbound goroutine     │
│                                                   │                     │
│                                                   ▼                     │
│                                             coalescer (1s ticker)       │
│                                                   │                     │
│                                                   ▼                     │
│    editMessageText ◄──────── bot.Send ◄── latest (draft + soul-line)    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
                                 │
                                 │ HTTP+SSE (Phase-1 hermes.Client)
                                 ▼
                         Python api_server :8642
```

Three goroutines inside `Bot`:

- **inbound** — consumes `tgbotapi.UpdatesChannel`, filters + translates to `kernel.PlatformEvent`.
- **outbound** — consumes `k.Render()`, pushes the latest frame into the coalescer.
- **coalescer** — 1-second ticker. On tick, if there's a pending frame, emits one `editMessageText`. Semantic edges (Idle, Failed) flush immediately regardless of timer.

### 4.3 Build isolation

`cmd/gormes/` never imports `internal/telegram/`. Verified at test time by:

```bash
go list -deps ./cmd/gormes | grep -q 'telegram-bot-api' && exit 1 || exit 0
```

Enforced via `gormes/internal/buildisolation_test.go` (a Go test that runs `go list` through `os/exec` and fails the suite if `telegram-bot-api` appears anywhere in the TUI's transitive dep set).

---

## 5. Interfaces

### 5.1 `telegramClient` — the mockability seam

`tgbotapi.BotAPI` is a struct, not an interface. We wrap it in our own minimal interface containing only the methods the adapter uses:

```go
// internal/telegram/client.go

type telegramClient interface {
    // GetUpdatesChan returns a channel of inbound Updates. The real
    // implementation long-polls with the configured timeout; the mock
    // implementation returns a scripted channel that tests push into.
    GetUpdatesChan(tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel

    // Send sends OR edits depending on the Chattable type passed
    // (tgbotapi.NewMessage vs tgbotapi.NewEditMessageText).
    Send(tgbotapi.Chattable) (tgbotapi.Message, error)

    // StopReceivingUpdates signals the long-poll loop to stop. Called
    // on graceful shutdown.
    StopReceivingUpdates()
}

// realClient wraps *tgbotapi.BotAPI to satisfy telegramClient.
type realClient struct{ api *tgbotapi.BotAPI }

func newRealClient(token string) (telegramClient, error) { ... }
```

Every Telegram interaction in `bot.go` / `coalesce.go` goes through `telegramClient`. Production code and tests are identical except for the client provided at construction.

### 5.2 `Bot` — the top-level adapter type

```go
type Config struct {
    AllowedChatID      int64
    CoalesceMs         int            // default 1000
    FirstRunDiscovery  bool           // default true
}

type Bot struct {
    cfg    Config
    client telegramClient
    kernel *kernel.Kernel
    log    *slog.Logger
}

func New(cfg Config, client telegramClient, k *kernel.Kernel, log *slog.Logger) *Bot

// Run starts the three goroutines and blocks until ctx is cancelled.
// On return, the kernel has been told to quit and the long-poll has stopped.
func (b *Bot) Run(ctx context.Context) error
```

`Run` owns the three goroutines and a `sync.WaitGroup` ensuring all exit before `Run` returns. Ctx cancellation propagates to all three goroutines; the coalescer's final flush is attempted inside a 2-second shutdown budget matching the kernel's own.

### 5.3 `render.go` — RenderFrame → Telegram text

```go
// formatStream renders an in-flight RenderFrame as the text of an
// editMessageText payload. Includes the assistant DraftText plus a
// trailing soul-event line when a tool is active.
func formatStream(f kernel.RenderFrame) string

// formatFinal renders the final (PhaseIdle) RenderFrame — full assistant
// content from History, no soul line. Truncated to 4000 chars with "…".
func formatFinal(f kernel.RenderFrame) string

// formatError renders a PhaseFailed frame as "❌ " + LastError.
func formatError(f kernel.RenderFrame) string
```

Trailing soul-event shape (when present and non-"idle"):

```
<draft so far...>

_🔧 tool: echo_
```

**ParseMode:** all outbound messages use `tgbotapi.ModeMarkdownV2`. `formatStream`/`formatFinal` call `tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, draft)` on the draft body so user content can't break the markdown parser. The trailing italic wrapper is a literal `_…_` added AFTER escaping so only it renders as italic.

**Reconnecting marker** (when `Phase == PhaseReconnecting`): `formatStream` appends `\n\n_reconnecting…_` as a literal MarkdownV2-italicised line. Same escape rule as the soul marker.

---

## 6. Streaming-to-Edit Algorithm

### 6.1 State

```go
type streamState struct {
    messageID   int           // 0 means no placeholder yet
    pendingText string        // latest text not yet sent
    lastSentText string       // most recent text actually sent to Telegram
    lastEditAt  time.Time     // coalescer uses this for the 1s window
    retryAfter  time.Time     // set on 429; next edit waits until this
}
```

### 6.2 Outbound goroutine (consumes `k.Render()`)

```
for frame := range k.Render():
    switch frame.Phase:
        Connecting:
            if state.messageID == 0:
                msg = client.Send(NewMessage(chatID, "⏳ …"))
                state.messageID = msg.MessageID
                state.lastSentText = "⏳ …"
                state.lastEditAt = now()
        Streaming:
            state.pendingText = formatStream(frame)
            (coalescer will flush on its next tick)
        Reconnecting:
            state.pendingText = formatStream(frame) + "\n\n_reconnecting…_"
        Idle:
            (turn complete)
            flushImmediately(formatFinal(frame))
            state.messageID = 0  // next turn gets a fresh placeholder
        Failed, Cancelling:
            flushImmediately(formatError(frame))
            state.messageID = 0
```

### 6.3 Coalescer goroutine (1 s ticker)

```
for tick := range ticker.C:
    if state.pendingText == "" or state.pendingText == state.lastSentText:
        continue
    if now() < state.retryAfter:
        continue
    if now().Sub(state.lastEditAt) < coalesceWindow:
        continue  // too soon

    err := client.Send(NewEditMessageText(chatID, state.messageID, state.pendingText))
    if err is 429 with Retry-After:
        state.retryAfter = now() + Retry-After
        continue
    if err:
        log warning and continue (coalescer does not die on transient errors)
    state.lastSentText = state.pendingText
    state.lastEditAt = now()
```

**flushImmediately** is the semantic-edge bypass:

```
flushImmediately(text):
    if state.messageID == 0:
        msg = client.Send(NewMessage(chatID, text))
        state.messageID = msg.MessageID
    else:
        client.Send(NewEditMessageText(chatID, state.messageID, text))
    state.lastSentText = text
    state.lastEditAt = now()
    state.pendingText = ""
```

### 6.4 Why 1 second

Telegram's documented limit is 1 message edit per second per message. We set the coalesce window at exactly that bound. Over a 10-second streaming turn, a user sees ~10 edit updates — enough to feel live, nowhere near the rate ceiling.

### 6.5 Concurrency safety

`streamState` lives in a goroutine-local scope (owned by the outbound+coalescer pair, which communicate via a capacity-1 "latest pending" channel — same replace-latest pattern as the kernel's render mailbox). No mutex needed.

---

## 7. Inbound Flow

```
for update := range client.GetUpdatesChan(cfg):
    if update.Message == nil:
        continue  // ignore callback queries, edited messages, etc. in MVP

    chatID := update.Message.Chat.ID
    userID := update.Message.From.ID

    if cfg.AllowedChatID == 0 and cfg.FirstRunDiscovery:
        log.Info("first-run discovery: unknown chat", "chat_id", chatID)
        client.Send(NewMessage(chatID, fmt.Sprintf(
            "Gormes is not authorised for this chat.\n"+
            "To allow: set [telegram].allowed_chat_id = %d in config.toml.\n"+
            "Then restart gormes-telegram.", chatID)))
        continue

    if chatID != cfg.AllowedChatID:
        // Silently mute. Log the attempt at WARN.
        log.Warn("unauthorised chat blocked", "chat_id", chatID, "user_id", userID)
        continue

    text := strings.TrimSpace(update.Message.Text)

    switch {
    case text == "/start":
        client.Send(NewMessage(chatID, "Gormes is online. Send a message to start."))
    case text == "/stop":
        _ = kernel.Submit(PlatformEvent{Kind: PlatformEventCancel})
    case text == "/new":
        kernel.ResetSession()  // new method — clears k.sessionID; see §8
        client.Send(NewMessage(chatID, "Session reset. Next message starts fresh."))
    case strings.HasPrefix(text, "/"):
        client.Send(NewMessage(chatID, "unknown command"))
    default:
        if err := kernel.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: text}); err == ErrEventMailboxFull:
            client.Send(NewMessage(chatID, "Busy — try again in a second."))
        }
    }
```

---

## 8. Small kernel-API addition: `ResetSession`

Implementing `/new` cleanly requires a way to clear `k.sessionID` without starting a turn. The kernel currently has no such method — `sessionID` is set internally when the hermes stream returns one.

**Addition:** `Kernel.ResetSession() error`. Non-blocking; returns an error only if a turn is currently active (caller can decide whether to cancel first). Implementation-wise this is a new `PlatformEventKind = PlatformEventResetSession` handled in the main `Run` select; the Run loop sets `k.sessionID = ""`, emits a frame with StatusText "session reset", and continues.

This is a **small Phase-2.B.1 kernel change** — the first change to `internal/kernel` outside this package since Phase 2.A landed. It is necessary, not optional. The TUI does not use it today (TUI has no `/new` command) but gains the capability for a future `Ctrl+N` binding.

---

## 9. Configuration

`[telegram]` section added to `internal/config/config.go`:

```go
type TelegramCfg struct {
    BotToken         string `toml:"bot_token"`
    AllowedChatID    int64  `toml:"allowed_chat_id"`
    CoalesceMs       int    `toml:"coalesce_ms"`
    FirstRunDiscovery bool  `toml:"first_run_discovery"`
}
```

Example `config.toml`:

```toml
[telegram]
bot_token = ""                    # env GORMES_TELEGRAM_TOKEN preferred
allowed_chat_id = 123456789       # set after first-run discovery
coalesce_ms = 1000                # default; don't drop below 1000
first_run_discovery = true
```

Defaults when fields are zero/empty:
- `BotToken == ""` → check env `GORMES_TELEGRAM_TOKEN`; if still empty, startup fails with "no bot token".
- `CoalesceMs == 0` → 1000.
- `FirstRunDiscovery == false` and `AllowedChatID == 0` → startup fails with "no chat allowlist and discovery disabled — set one of `[telegram].allowed_chat_id` or `[telegram].first_run_discovery = true`".

Env override for `AllowedChatID`: `GORMES_TELEGRAM_CHAT_ID` (int string).

---

## 10. Error Handling

| Failure | Where | What happens |
|---|---|---|
| Invalid bot token | startup | exit 1 with "Telegram: invalid token" |
| Telegram API 429 | coalescer / inbound | honour `Retry-After`; coalescer skips ticks until the deadline passes |
| Telegram API 5xx | coalescer / inbound | log WARN, retry on next tick; no exit |
| Long-poll timeout (normal) | inbound | not an error; next `GetUpdatesChan` iteration |
| Draft exceeds 4000 chars | `formatStream` / `formatFinal` | truncate with `…` suffix |
| MessageID == 0 at final | outbound | `Send` new message instead of `Edit` — one-shot response |
| Kernel ErrEventMailboxFull on Submit | inbound | reply "Busy — try again in a second" |
| Kernel still processing when second user message arrives | inbound | kernel itself rejects; adapter observes via RenderFrame.LastError and emits it |
| Ctx cancel (SIGINT) | Run | waitgroup joins three goroutines within 2s shutdown budget; final edit attempted but not blocking |

No Telegram failure crashes the process. The adapter is designed to reconnect on its own via long-poll's retry semantics.

---

## 11. MockTelegramAPI

```go
// internal/telegram/mock_test.go

type mockClient struct {
    updates chan tgbotapi.Update
    sent    []tgbotapi.Chattable // every Send() call recorded
    mu      sync.Mutex
    idSeq   int
    stopFn  func()
}

var _ telegramClient = (*mockClient)(nil)

func newMockClient() *mockClient {
    return &mockClient{updates: make(chan tgbotapi.Update, 16)}
}

func (m *mockClient) ScriptUserMessage(chatID int64, text string)
func (m *mockClient) SentMessages() []tgbotapi.Chattable
func (m *mockClient) LastSentText() string
// ... etc
```

### 11.1 Tests using the mock

- **`TestBot_RejectsUnauthorisedChat`** — inbound message from `chat_id != allowed` produces zero `kernel.Submit` calls and one "not authorised" reply.
- **`TestBot_FirstRunDiscovery`** — inbound message to a zero-`allowed_chat_id` bot replies with the chat_id discovery hint.
- **`TestBot_StreamToEdit_Coalesces`** — scripted RenderFrames at 50 ms intervals over 3 s. Assert: `len(mockClient.SentMessages()) ≥ 2` (at least placeholder + one edit) and `≤ 5` (at most placeholder + 3 coalesced edits + final flush). The exact count varies with Go scheduler jitter, but 60 individual sends is the failure mode we're guarding against.
- **`TestBot_FinalFlushesImmediately`** — PhaseIdle frame flushes outside the coalesce window.
- **`TestBot_StopCommandCancelsKernel`** — `/stop` inbound triggers one `PlatformEventCancel`.
- **`TestBot_NewCommandResetsSession`** — `/new` triggers `ResetSession`; subsequent LLM request has empty `SessionID`.
- **`TestBot_LongMessageTruncated`** — 5000-char DraftText produces an edit whose text is ≤4000 + `…`.
- **`TestBot_Telegram429RespectsRetryAfter`** — mock returns a 429 with 2s Retry-After; next edit is deferred by at least 2s.
- **`TestBot_ShutdownWithinBudget`** — ctx cancel during streaming → all three goroutines exit within 2s.

### 11.2 Invariant-preservation test (cross-phase)

- **`TestBot_ToolCallHandshake_Echo_ViaTelegram`** — scripted hermes MockClient runs a 2-round tool-call turn (Echo); adapter observes the full roundtrip via RenderFrames and ends with an edit containing the round-2 assistant text. Proves the Telegram path preserves Phase-2.A tool-calling semantics end-to-end.

---

## 12. Build Isolation Test

`gormes/internal/buildisolation_test.go`:

```go
package internal_test

import (
    "bytes"
    "os/exec"
    "strings"
    "testing"
)

// TestTUIBinaryHasNoTelegramDep guards the Operational Moat: cmd/gormes
// must never transitively depend on telegram-bot-api. If it does, the
// TUI binary size jumps and we break the per-binary-per-platform promise.
func TestTUIBinaryHasNoTelegramDep(t *testing.T) {
    cmd := exec.Command("go", "list", "-deps", "./cmd/gormes")
    var out bytes.Buffer
    cmd.Stdout = &out
    cmd.Dir = ".." // project root relative to internal/
    if err := cmd.Run(); err != nil {
        t.Fatalf("go list: %v", err)
    }
    for _, line := range strings.Split(out.String(), "\n") {
        if strings.Contains(line, "telegram-bot-api") ||
           strings.Contains(line, "internal/telegram") {
            t.Errorf("cmd/gormes transitively depends on %q — Operational Moat violated", line)
        }
    }
}
```

Runs in the standard `go test ./...` sweep. Fails the build if someone accidentally `import`s `internal/telegram` from the TUI.

---

## 13. Success Criteria

Phase-2.B.1 is "scout-operational" when **all** hold:

1. `go build ./cmd/gormes` and `go build ./cmd/gormes-telegram` both succeed.
2. `cmd/gormes` binary size ≤ 8.5 MB stripped (budget preserves the 7.9 MB baseline).
3. `cmd/gormes-telegram` binary size ≤ 12 MB stripped.
4. `TestTUIBinaryHasNoTelegramDep` passes — `go list -deps ./cmd/gormes` contains no `telegram-bot-api` or `internal/telegram` path.
5. `go test -race ./internal/telegram/... -timeout 60s` passes all tests including the nine listed in §11.1 and the §11.2 invariant-preservation case.
6. Phase-1 / 1.5 / 2.A / doctor tests still pass under `-race`.
7. Manual smoke test (requires real bot token + api_server):
   - `./bin/gormes-telegram` starts; logs chat_id on first DM.
   - After config update, subsequent DM streams a reply with mid-turn edits visible.
   - `/stop` mid-reply truncates the response with cancel status.
   - `/new` clears the session; next DM starts fresh.
8. `go vet ./...` clean.

---

## 14. Explicit Out-of-Scope

| Feature | Where it belongs |
|---|---|
| Group chats / channels | Phase 2.B.3 (needs per-thread session routing) |
| Media in/out (images, voice, files) | Phase 2.B.4 |
| Webhook transport | Phase 2.B.2 (once long-poll is proven) |
| Multi-chat routing (one bot, many users, many kernels) | Phase 3 (memory layer must partition first) |
| `setMyCommands` (Telegram autocomplete menu) | Polish commit after MVP |
| Rich formatting (code blocks, tables, inline keyboards) | Phase 2.B.5 |
| Discord adapter | Phase 2.B.2 — sibling spec, same shape |
| Slack adapter | Phase 2.B.3 |
| Persistent session ID across process restarts | Phase 3 (memory) |
| Multiple simultaneous Telegram bot tokens (one process, multiple bots) | Not planned; run multiple `gormes-telegram` processes |

---

## 15. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `tgbotapi.UpdatesChannel` buffers silently — inbound backpressure invisible | Adapter never blocks sending to `kernel.Submit`; mailbox-full returns `ErrEventMailboxFull` and we reply "Busy". User sees a polite message, no lost data. |
| Long-poll keeps a TCP connection open; ISP/NAT may close it every ~25min | tgbotapi v5 handles reconnects automatically. We verify during smoke test. |
| 429 rate-limit storm if multiple users DM a misconfigured "allow all" | M1 default is allowlist-required; the chat filter drops unauthorised chats without an API round-trip. |
| Telegram's `editMessageText` fails silently if new text == old text | Coalescer checks `pendingText == lastSentText` before calling `Send`. |
| Long soul-event lines push message past 4000 chars | Soul-line formatter caps at 80 chars; `formatStream` truncates combined output. |
| Kernel's `ErrEventMailboxFull` gets too chatty under rapid user messaging | Adapter rate-limits its "Busy" replies (at most one per second per chat). |
| `ResetSession` race — user sends `/new` mid-turn | `ResetSession` returns an error if phase ≠ Idle; adapter forwards "cannot reset during turn — /stop first". |
| Bot token accidentally committed to config.toml | M5 startup log WARN when token read from config instead of env; README explicitly documents env-first pattern. |
| A malicious user spams a 4000-char message stream | Admission already caps at 200 KB / 10k lines in kernel; oversize inbound triggers kernel admission rejection, adapter surfaces `LastError`. |

---

## 16. Next Step

After this spec is approved, `superpowers:writing-plans` produces the Phase-2.B.1 implementation plan. Expected size: 8-10 tasks, ~3-4 hours of subagent work.

Task ordering priority (per user direction): **Task 1 is the `telegramClient` interface + `mockClient` + minimal bot scaffolding with inbound-direction tests only.** This establishes the mock-driven testing strategy before any real Telegram API code lands.

The spec is the source of truth for *what* Phase 2.B.1 is. The plan is the source of truth for *how* it gets built.
