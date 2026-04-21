# Gormes Phase 2.B.2 — Gateway Chassis + Discord Design Spec

**Date:** 2026-04-21
**Author:** Xel (via Claude Code brainstorm)
**Status:** Approved design direction; ready for writing-plans
**Scope:** Phase 2.B.2 — extract a reusable **gateway chassis** from the shipped Telegram adapter, refactor Telegram onto it, and port **Discord** from picoclaw as the second channel that validates the chassis. Slack, WhatsApp, Signal, Email, and SMS are explicit follow-ups that each consume this chassis in their own spec.

**Vocabulary decision:** `internal/telegram/` is deleted at the end of this spec. The chassis lives at `internal/gateway/` and owns cross-channel mechanics (frame routing, coalescing, session-map persistence, auth gate, command normalization). Individual channels live at `internal/channels/<name>/`. This matches picoclaw's mental model (`pkg/channels/*`) and makes the channel surface an enumerable set rather than a proliferation of top-level packages.

## Related Documents

- [Executive Roadmap — Phase 2](../../content/building-gormes/architecture_plan/_index.md)
- [Specs Index](README.md)
- [Phase 2.B.1 — Telegram Scout](2026-04-19-gormes-phase2b-telegram.md) — the adapter being refactored
- [Gateway Donor Map](2026-04-20-gormes-gateway-donor-map-design.md) — companion docs-only effort that indexes picoclaw donors
- [Phase 2.A — Tool Registry](2026-04-19-gormes-phase2-tools-design.md)

---

## 1. Purpose

Phase 2.B.1 shipped one working adapter (Telegram). Phase 2.B.2 ships the pattern: a shared chassis that makes every subsequent adapter a thin SDK translator rather than a from-scratch integration.

Two channels prove the chassis better than one. Telegram alone is already generalized into its own package — refactoring it is a move, not a design lesson. A second real channel built on the same interface surfaces the capabilities that were implicit in Telegram and forces the abstraction to be honest. Discord is the closest donor fit in picoclaw and carries the fewest platform quirks, so it is the right second channel for this spec.

By the end of this spec:

- A gormes contributor can add Slack/WhatsApp/Signal/Email/SMS by writing one file under `internal/channels/<name>/` that satisfies `gateway.Channel` plus whichever capability interfaces the platform supports.
- The kernel is untouched. Manager consumes `kernel.Render()` and submits `PlatformEvent`s the same way Telegram does today.
- All shipped Telegram behavior is byte-identical after refactor — this spec has zero product-visible behavior change for existing users, but doubles the channel surface and sets the template for the remaining five.

---

## 2. Relationship to prior phases

| Phase | What it owns |
|---|---|
| Phase 1 | TUI + kernel state machine + hermes HTTP/SSE + zero-store |
| Phase 1.5 | Route-B reconnect + compat-probe + discipline tests |
| Phase 2.A | `internal/tools` + `tool_calls` flow + doctor `CheckTools` |
| Phase 2.B.1 | `internal/telegram/` + `gormes telegram` subcommand (shipped) |
| Phase 2.C | `internal/session` session-map persistence (shipped, consumed by chassis) |
| **Phase 2.B.2 (this spec)** | **`internal/gateway/` chassis + `internal/channels/telegram/` refactor + `internal/channels/discord/` new** |
| Phase 2.B.2 follow-ups | `internal/channels/slack/`, `/whatsapp/`, `/signal/`, `/email/`, `/sms/` — each its own spec, each consumes this chassis |
| Phase 3 | memory, unchanged |

Pre-existing packages that change in this spec:

- `internal/telegram/` — moved to `internal/channels/telegram/` and refactored to implement `gateway.Channel`. Coalescer and session-map persistence are extracted to `internal/gateway/`.
- `internal/config` — adds `[discord]` TOML block + `DiscordCfg` struct field. Existing `[telegram]` block stays byte-identical.
- `cmd/gormes/` — adds a new `gormes gateway` cobra subcommand alongside the existing `gormes telegram`. `gormes gateway` wires a `gateway.Manager` around whatever channels are enabled (Telegram when `[telegram]` is configured, Discord when `[discord]` is configured, at least one required). The existing `gormes telegram` subcommand stays as a thin alias that constructs a Manager with Telegram-only enabled — zero user-visible change for existing systemd units.

`internal/kernel`, `internal/tools`, `internal/hermes`, `internal/doctor`, `internal/tui`, `internal/store`, `internal/telemetry`, `internal/session`, `internal/memory`, and `pkg/gormes` stay byte-identical.

---

## 3. Locked Architectural Decisions

### Principal decisions

1. **Chassis first, port second, channels-in-this-spec: Telegram + Discord.** Slack/WhatsApp/Signal/Email/SMS are deferred. Two channels is the minimum that validates an abstraction; six is a sprawling plan.
2. **Capability interfaces, not God interface.** `Channel` is the minimum every adapter must implement (name, inbound loop, basic send). Editing, typing, placeholders, reactions, and streaming are optional sub-interfaces the Manager type-asserts. Copied directly from picoclaw's pattern; proven shape.
3. **Manager owns cross-channel mechanics.** Coalescing, session-map persistence, chat-allowlist auth gate, command normalization (`/start` `/stop` `/new`), and frame routing live in `gateway.Manager`. Channels own only their SDK-specific glue.
4. **Kernel stays platform-agnostic.** No new `PlatformEvent` kinds. The `ResetSession` method added in 2.B.1 covers `/new` for every channel.
5. **Single-session, single-operator semantics carry forward.** One kernel, one active turn at a time, frames pinned to the chat that submitted the turn. Multi-chat concurrent turns remain out of scope until a later phase reshapes the kernel.

### Micro-decisions

- **Package path:** `internal/gateway/` for the chassis, `internal/channels/<name>/` for adapters. `internal/telegram/` is deleted.
- **Chat identity:** `ChatKey string` encoded as `"<platform>:<chat_id>"` — the same format `internal/session` already uses. No new types.
- **Discord SDK:** `github.com/bwmarrin/discordgo` — matches picoclaw's choice and is the de facto Go Discord client. Locked to the version picoclaw uses.
- **Subcommand:** new `gormes gateway` cobra subcommand. Existing `gormes telegram` stays functional (aliases to a single-channel Manager) so existing systemd units keep running untouched.
- **Discord scope:** text messages in one allowlisted `(guild_id, channel_id)`, commands `/start` `/stop` `/new`, streaming via edits, 👀 reaction ack on inbound. Slash commands, embeds, threads, voice, multi-guild — all deferred.
- **Coalesce window:** shared default `1000ms`, per-channel override via `CoalesceMs` on the channel's config. Current Telegram behavior preserved.
- **Error model:** SDK errors are logged and degrade gracefully (match Telegram's current pattern). Placeholder send failures fall back to direct send. Edit failures log and stop coalescing for that turn.
- **Testing seam:** every channel depends on an SDK-client interface (as Telegram already does with `telegramClient`). Discord gets an equivalent `discordSession` interface. Manager tests use a `fakeChannel` that implements every capability.
- **Doctor:** a new `CheckGateway` that verifies each enabled channel's SDK can authenticate. Extends the current `CheckTelegram` pattern.

---

## 4. Architecture

### 4.1 Package tree after this spec lands

```
gormes/internal/
├── gateway/                    # NEW — the chassis
│   ├── channel.go              # Channel interface + capability sub-interfaces
│   ├── event.go                # InboundEvent, EventKind, ChatKey helpers
│   ├── manager.go              # frame routing, auth gate, session-map, command normalization
│   ├── coalesce.go             # moved from internal/telegram/coalesce.go
│   ├── render.go               # moved from internal/telegram/render.go (formatStream/formatFinal/formatError)
│   └── *_test.go               # chassis contract tests with fake channel
├── channels/
│   ├── telegram/               # MOVED from internal/telegram/
│   │   ├── bot.go              # now implements gateway.Channel
│   │   ├── client.go
│   │   ├── real_client.go
│   │   └── *_test.go
│   └── discord/                # NEW — ported from picoclaw/pkg/channels/discord/
│       ├── bot.go              # implements gateway.Channel + capability interfaces
│       ├── client.go           # discordSession interface (testing seam)
│       ├── real_client.go      # discordgo.Session wrapper
│       └── *_test.go
└── telegram/                   # DELETED
```

### 4.2 Core interfaces (`internal/gateway/channel.go`)

```go
// Channel is the minimum every adapter implements.
type Channel interface {
    // Name returns a stable identifier ("telegram", "discord") used as the
    // platform component of ChatKey and in logs.
    Name() string

    // Run starts the inbound loop and blocks until ctx cancellation.
    // The channel pushes normalized InboundEvents into inbox.
    Run(ctx context.Context, inbox chan<- InboundEvent) error

    // Send delivers a plain-text message to the given chat_id. Returns the
    // platform's message ID so Manager can later edit it via MessageEditor.
    Send(ctx context.Context, chatID, text string) (msgID string, err error)
}

// Capability interfaces. Manager type-asserts at runtime.

type MessageEditor interface {
    EditMessage(ctx context.Context, chatID, msgID, text string) error
}

type PlaceholderCapable interface {
    // SendPlaceholder sends a "thinking" message and returns its ID so
    // Manager can edit it with streamed content via MessageEditor.
    SendPlaceholder(ctx context.Context, chatID string) (msgID string, err error)
}

type TypingCapable interface {
    // StartTyping shows a typing indicator and returns a stop function.
    // The stop function MUST be idempotent.
    StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

type ReactionCapable interface {
    // ReactToMessage adds an ack reaction and returns an undo function.
    // The undo function MUST be idempotent.
    ReactToMessage(ctx context.Context, chatID, msgID string) (undo func(), err error)
}
```

### 4.3 Normalized events (`internal/gateway/event.go`)

```go
type EventKind int

const (
    EventUnknown EventKind = iota
    EventSubmit             // user text
    EventCancel             // /stop
    EventReset              // /new
    EventStart              // /start — reply with help, no kernel submit
)

type InboundEvent struct {
    Platform string    // "telegram", "discord"
    ChatID   string    // platform-native chat/channel ID as string
    UserID   string    // platform user ID (unused in 2.B.2, logged only)
    MsgID    string    // platform message ID of the inbound message
    Kind     EventKind
    Text     string    // body for EventSubmit; empty otherwise
}

// ChatKey returns "<platform>:<chat_id>" — the format internal/session uses.
func (e InboundEvent) ChatKey() string { return e.Platform + ":" + e.ChatID }
```

### 4.4 Manager (`internal/gateway/manager.go`)

Single struct that replaces the orchestration currently embedded in `telegram.Bot.runOutbound` + `handleUpdate`. Responsibilities:

- **Inbound path:** consumes `InboundEvent`s from all channels on one inbox. For each event: runs the allowlist check, normalizes the command, submits the appropriate `kernel.PlatformEvent`, and pins `(platform, chat_id, msgID)` as the "current turn origin".
- **Outbound path:** consumes `kernel.Render()` once. For each frame, looks up the channel that owns the current turn's platform and routes the frame through the coalescer.
- **Coalescing:** one coalescer per turn, torn down on `PhaseIdle`/`PhaseFailed`/`PhaseCancelling` — identical semantics to the current Telegram behavior, just moved.
- **Session-map persistence:** port of `persistIfChanged` from `telegram.Bot`. Writes `session.Map[ChatKey] = SessionID` when the frame's SessionID changes.
- **Allowlist:** Manager config includes `map[string]string` of `platform -> allowed_chat_id`. Inbound events whose chat_id doesn't match are dropped with a WARN log. First-run discovery (currently Telegram-only) is generalized to any channel that declares it via `AllowDiscovery` in its config.

```go
type ManagerConfig struct {
    AllowedChats    map[string]string // platform -> chat_id
    AllowDiscovery  map[string]bool   // platform -> enable first-run discovery log
    CoalesceMs      int
    SessionMap      session.Map       // optional
}

type Manager struct {
    cfg      ManagerConfig
    kernel   *kernel.Kernel
    channels map[string]Channel      // keyed by Name()
    log      *slog.Logger
}

func NewManager(cfg ManagerConfig, k *kernel.Kernel, log *slog.Logger) *Manager
func (m *Manager) Register(ch Channel) error
func (m *Manager) Run(ctx context.Context) error
```

### 4.5 Discord adapter (`internal/channels/discord/`)

- Uses `github.com/bwmarrin/discordgo`. Version pinned to whatever picoclaw ships.
- `Run` calls `session.Open()`, subscribes to `MessageCreate`, translates each event into `InboundEvent{Platform:"discord", ChatID:<channel_id>, ...}`, and pushes to the Manager inbox.
- Command parsing is identical to Telegram's: leading `/start`, `/stop`, `/new`; everything else is `EventSubmit`.
- `Send` uses `ChannelMessageSend` and returns `msg.ID`.
- Implements `MessageEditor` via `ChannelMessageEdit`.
- Implements `PlaceholderCapable` by sending a `⏳` message and returning its ID — Manager edits it.
- Implements `ReactionCapable` via `MessageReactionAdd` / `MessageReactionRemove`.
- Does **not** implement `TypingCapable` in this spec (Discord supports it via `ChannelTyping`, but leaving it out keeps the port minimal; follow-up trivial).

### 4.6 Telegram refactor

Pure migration. No behavior change. Specifically:

- Package move `internal/telegram/` → `internal/channels/telegram/`. All imports updated in the same commit.
- `bot.Bot` grows `Name() string { return "telegram" }` and a translation layer from `tgbotapi.Update` to `gateway.InboundEvent`. The coalescer-driving code (`runOutbound`, `handleFrame`) is **deleted**; Manager owns it.
- Coalescer (`coalesce.go`) moves to `internal/gateway/coalesce.go`. Generic over the Channel interface — takes a `MessageEditor` + `msgID` instead of `telegramClient` + `chat_id`.
- `render.go` moves to `internal/gateway/render.go`. The `formatStream`/`formatFinal`/`formatError` helpers become channel-agnostic.
- `persistIfChanged` is deleted from telegram; Manager owns session-map writes.
- `SendToChat` (the cron delivery hook) becomes the standard `Send` on the Channel interface — cron executor holds a `gateway.Manager` reference instead of `*telegram.Bot`.

### 4.7 `gormes gateway` subcommand

New cobra subcommand in `cmd/gormes/gateway.go`. On startup:

1. Load config.
2. Build kernel, session map (same wiring as `gormes telegram` today).
3. Build `gateway.Manager`.
4. For each enabled channel (`[telegram]` if configured, `[discord]` if configured; error if none), construct the adapter and `Register` it with Manager.
5. Call `manager.Run(ctx)`, block until signal.

The existing `gormes telegram` subcommand (`cmd/gormes/telegram.go`) is refactored to internally construct a single-channel Manager with Telegram enabled — same observable behavior, zero user-visible change, systemd units untouched. This gives operators a clean deprecation path: migrate to `gormes gateway` when they want Discord, keep running `gormes telegram` otherwise.

---

## 5. Data flow

### 5.1 Inbound (Discord example)

```
discordgo.MessageCreate
  → discord.Bot.toInboundEvent(...)
  → InboundEvent{Platform:"discord", ChatID:"chan_42", Text:"hi"}
  → Manager.inbox
  → Manager.handleEvent()
      → allowlist check against cfg.AllowedChats["discord"]
      → command normalization → EventSubmit
      → kernel.Submit(PlatformEvent{Kind:PlatformEventSubmit, Text:"hi"})
      → pin currentTurn = {Platform:"discord", ChatID:"chan_42"}
```

### 5.2 Outbound (any channel)

```
kernel.Render() frame
  → Manager.dispatchFrame(f)
      → ch := channels[currentTurn.Platform]
      → on first streaming frame: coalescer spawned around ch (MessageEditor) + placeholder msgID
      → coalescer.setPending(formatStream(f))
      → on PhaseIdle/Failed/Cancelling: coalescer.flushImmediate; teardown
      → session.Map.Put(currentTurn.ChatKey, f.SessionID) if changed
```

### 5.3 Turn pinning

A second inbound event during an active turn is dropped with "busy" reply (matches Telegram's current behavior). `/stop` is the only exception — it always cancels. This keeps single-session semantics explicit in Manager rather than implicit in each channel.

---

## 6. Config

New `[discord]` block in `config.toml`:

```toml
[discord]
token = "<bot token>"
allowed_channel_id = "123456789012345678"
first_run_discovery = false    # optional, default false
coalesce_ms = 1000              # optional, default 1000
```

`[telegram]` block is byte-identical to 2.B.1. Manager's `AllowedChats` map is built from whichever blocks are present:

```go
cfg.AllowedChats["telegram"] = strconv.FormatInt(tg.AllowedChatID, 10)
cfg.AllowedChats["discord"]  = disc.AllowedChannelID
```

A channel is "enabled" iff its token is non-empty. Doctor reports which channels are enabled and whether auth succeeded.

---

## 7. TDD plan (high-level)

This spec hands over to writing-plans. The plan will expand this into red/green/refactor tasks. High-level ordering:

1. **Chassis interfaces + fake channel.** `gateway.Channel`, capability interfaces, `InboundEvent`, compile-time contract tests (`var _ Channel = (*fakeChannel)(nil)`). One test per capability interface asserts Manager type-assertion works.
2. **Manager inbound path.** Table-driven tests: fake channel pushes events → assert `kernel.Submit` called with the right `PlatformEvent`. Cover allowlist drop, each `EventKind`, busy-reply during active turn.
3. **Manager outbound path.** Fake kernel frames → assert `Send`/`EditMessage` on fake channel in the correct order. Cover: placeholder → stream edits → final → teardown; error phase; cancellation phase.
4. **Coalescer move.** Existing `internal/telegram/coalesce_test.go` → `internal/gateway/coalesce_test.go` with the coalescer generalized to `MessageEditor`. Assertion bodies unchanged.
5. **Session-map persistence move.** Port `persistIfChanged` tests from telegram to Manager.
6. **Telegram refactor.** Package move + `Bot` implements `gateway.Channel`. All existing telegram tests stay green with zero assertion changes beyond import paths.
7. **Discord adapter.** `discordSession` interface + fake; inbound `MessageCreate` → `InboundEvent` translation; `Send`/`EditMessage`/`SendPlaceholder`/`ReactToMessage` happy-path + error-path tests.
8. **Doctor `CheckGateway`.** Verifies each enabled channel's SDK authenticates. Extends current `CheckTelegram`.
9. **Subcommand wiring.** New `gormes gateway` subcommand builds Manager + enabled channels; existing `gormes telegram` subcommand refactored to construct Manager with Telegram-only. End-to-end smoke for both invocation paths (existing telegram smoke kept green, new gateway+discord smoke added).
10. **Cleanup.** Delete `internal/telegram/`; remove any remaining telegram-specific imports outside `internal/channels/telegram/`.

Each task has red-first tests. No task mixes refactor with behavior change; the Telegram refactor task is explicitly a move with green-stays-green as the pass criterion.

---

## 8. Out of scope

Explicit list so the plan stays bounded:

- Slack, WhatsApp, Signal, Email, SMS adapters. Each is a follow-up spec (`2026-04-??-gormes-phase2b2-slack-design.md`, etc.) that consumes this chassis.
- Platform-rich features: Discord embeds/threads/slash-commands, Telegram inline keyboards, WhatsApp media, Signal groups, Email threading, SMS MMS.
- Multi-chat concurrent turns. Kernel is single-session; Manager pins one active turn at a time.
- Gateway donor map documentation (`gormes/docs/content/building-gormes/gateway-donor-map/`) — that is a separate approved effort.
- Changes to `internal/kernel`. Nothing in this spec justifies touching the kernel.
- Dynamic channel registration / runtime add-remove. Channels are built once at startup from config.
- Any change to `cmd/gormes` (TUI entrypoint) other than using the renamed gateway package if it imports from there. It does not today.

---

## 9. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Package move breaks import paths across the repo. | Single commit does rename + all call-site fixes; CI runs `go build ./...` + full test suite before merge. |
| picoclaw's Discord adapter is entangled with its `bus` package. | Port the *shape* — event subscription, send/edit calls, capability split — not the implementation. Accept net-new Go where coupling is dense. Reference-only; no vendoring. |
| Designing a capability set against two channels under-generalizes the remaining five. | Keep capability set minimal (editor, placeholder, typing, reaction). New capabilities get added in follow-up specs, not forced in now. |
| Telegram behavior regression during refactor. | Refactor task is a move + interface-satisfaction only. Assertion bodies in existing tests do not change. Any test that needs a new assertion is a separate task. |
| Coalescer generalization leaks platform assumptions. | Coalescer takes a `MessageEditor` + placeholder `msgID` and nothing else. Tests use a fake `MessageEditor` with no platform semantics. |
| Single-inbox contention across channels. | Unbuffered inbox is fine — Manager processes events fast enough that channels don't block on Send. If this becomes a bottleneck, buffered channel is a one-line change. Document the assumption, don't optimize preemptively. |
| Discord rate limits bite during streaming edits. | Coalesce window already throttles edits to once per `CoalesceMs`. discordgo's built-in rate-limit handling covers the rest. If 429s appear in practice, bump window per-channel. |

---

## 10. Success criteria

This spec succeeds when:

1. `internal/gateway/` exists with a Channel interface, capability sub-interfaces, a Manager, coalescer, and event types — all tested with a fake channel.
2. `internal/channels/telegram/` exists and implements `gateway.Channel`; all prior Telegram tests are green with unchanged assertions.
3. `internal/channels/discord/` exists and implements `gateway.Channel` + `MessageEditor` + `PlaceholderCapable` + `ReactionCapable`, with SDK-backed happy-path tests and fake-session unit tests.
4. `internal/telegram/` is deleted.
5. `gormes gateway` subcommand starts, authenticates each enabled channel, and streams a turn through each end-to-end (smoke tests green). `gormes telegram` still runs Telegram standalone with identical observable behavior.
6. A contributor can write a minimal `internal/channels/slack/` by implementing the Channel interface without touching `internal/gateway/` or any other channel.
7. Doctor reports per-channel enablement + auth status.
8. Progress.json's 2.B.2 subphase gets its first shipped checkbox (`Discord`), and a note that Slack/WhatsApp/Signal/Email/SMS now follow a template rather than a clean slate.

---

## 11. Handover

After this spec is approved by review:

- Invoke the writing-plans skill against this spec.
- Plan document target: `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2b2-chassis.md`.
- Plan must preserve the task ordering in §7 and the out-of-scope boundary in §8.
