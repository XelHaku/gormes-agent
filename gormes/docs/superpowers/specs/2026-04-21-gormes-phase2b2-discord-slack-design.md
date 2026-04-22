# Gormes Phase 2.B.2-2.B.3 — Discord + Slack Gateway Batch Design Spec

**Date:** 2026-04-21
**Author:** Xel (via Codex brainstorm)
**Status:** Approved for plan phase
**Scope:** Land the first two "wider gateway surface" adapters after Telegram by shipping `gormes discord` and `gormes slack` on top of the existing kernel, memory, session, and tool runtime. Use PicoClaw as a donor for channel-edge behavior only.

## Related Documents

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Specs Index](README.md)
- [Phase 2.B.1 — Telegram Scout](2026-04-19-gormes-phase2b-telegram.md)
- [Phase 2 — The Gateway](../../content/building-gormes/architecture_plan/phase-2-gateway.md)
- [Gateway Donor Map](../../content/building-gormes/gateway-donor-map/_index.md)
- [Shared Adapter Patterns](../../content/building-gormes/gateway-donor-map/shared-adapter-patterns.md)

---

## 1. Purpose

Phase 2 cannot honestly claim a wider gateway surface while Gormes still has only Telegram as a live messaging adapter. The next step should not be a generic gateway rewrite. It should be the smallest batch that proves the Telegram shape scales to more channels without polluting the kernel or re-importing PicoClaw's runtime model.

This spec ships two concrete adapters:

- **Discord** because PicoClaw's Go implementation is a strong copy candidate for startup, mention handling, typing, replies, and media shape.
- **Slack** because PicoClaw already solved the Socket Mode event loop, thread timestamps, and ACK/reaction behavior in Go.

The goal is not parity with every Hermes messaging feature. The goal is to turn `internal/telegram` into a repeatable Gormes adapter pattern and to prove that Phase 2.B.2+ is real, not aspirational.

---

## 2. Locked Decisions

### 2.1 Batch shape

This is a **Discord + Slack batch**, but the implementation order is fixed:

1. extract minimal shared runtime boot from the existing Telegram command
2. land Discord end to end
3. land Slack end to end
4. do a small cleanup pass only if duplication is now obvious

No other channel lands in this batch.

### 2.2 Gormes architecture stays authoritative

PicoClaw is a donor repo for channel-edge mechanics only.

- **Gormes owns** the kernel, session mapping, memory store, recall, telemetry, tool registry, and overall Operative System AI architecture.
- **PicoClaw donates** SDK choices, event-loop patterns, typing/reaction/thread behavior, outbound send/reply/media flow, and channel-specific transport quirks.

This batch must not import PicoClaw's bus, manager, or channel-runtime ownership model.

### 2.3 No generic framework first

Do **not** build a replacement `internal/gateway` framework before shipping real channels.

The only allowed shared extraction in this batch is shared startup/wiring code that removes obvious duplication across `gormes telegram`, `gormes discord`, and `gormes slack`.

### 2.4 First-pass scope stays narrow

Discord first pass:

- one bot token
- DM support and one allowlisted guild/channel path
- mention-gated guild behavior
- typing indicator while streaming
- normal send/reply flow
- no voice/TTS

Slack first pass:

- Socket Mode only
- one allowlisted channel
- thread-aware replies
- correct event ACK behavior
- normal text send flow
- no slash-command parity in this batch

### 2.5 TDD is mandatory

Every new command, config path, and adapter behavior must be introduced test-first. No adapter code lands before its failing tests exist.

---

## 3. Existing Anchors

This batch is intentionally anchored to the shipped Telegram implementation rather than designed from scratch.

Primary Gormes anchors:

- `gormes/internal/telegram/bot.go`
- `gormes/internal/telegram/client.go`
- `gormes/cmd/gormes/telegram.go`
- `gormes/internal/config/config.go`
- `gormes/internal/session/*`
- `gormes/internal/memory/*`
- `gormes/internal/kernel/*`

Primary PicoClaw donor files:

- `picoclaw/pkg/channels/discord/discord.go`
- `picoclaw/pkg/channels/discord/voice.go` as reference only; voice itself stays out of scope
- `picoclaw/pkg/channels/slack/slack.go`
- `picoclaw/pkg/channels/base.go`
- `picoclaw/pkg/channels/manager.go`
- `picoclaw/pkg/channels/split.go`

The Telegram adapter remains the Gormes-side reference for package shape and kernel integration. PicoClaw informs only the Discord/Slack transport behavior.

---

## 4. Architecture

### 4.1 High-level shape

Both adapters are siblings of `internal/telegram`, not children of a new gateway framework:

```text
gormes/
├── cmd/gormes/
│   ├── telegram.go              # existing
│   ├── discord.go               # new
│   ├── slack.go                 # new
│   └── gateway_runtime.go       # new, minimal shared boot extraction
├── internal/
│   ├── telegram/                # existing reference shape
│   ├── discord/                 # new
│   └── slack/                   # new
```

The split is:

- **adapter packages** own platform SDK use, inbound parsing, outbound rendering, and platform-specific state
- **shared command runtime boot** owns the construction of session map, memory store, recall, embedder, extractor, tool registry, kernel, telemetry, and Hermes HTTP client
- **kernel** remains platform-agnostic

### 4.2 Shared runtime boot extraction

The current `cmd/gormes/telegram.go` wires together:

- config load
- session map open and key lookup
- memory store open
- mirror/extractor/semantic recall setup
- Hermes HTTP client
- tool registry
- delegation registration
- kernel construction
- telemetry
- shutdown behavior

That shared stack must be extracted into one helper used by all three commands. The helper is intentionally narrow: it prepares runtime dependencies for a single platform command and does not become a new public gateway subsystem.

### 4.3 Adapter package pattern

Each adapter package should mirror Telegram's testable split:

- `client.go` — the narrow interface the adapter uses from the SDK
- `real_client.go` — production wrapper over the real SDK
- `bot.go` or `adapter.go` — top-level adapter type and run loop
- `render.go` — render-frame to platform-message formatting
- `mock_test.go` — mock client + shared test helpers
- `*_test.go` — inbound/outbound/end-to-end tests

Exact filenames can vary slightly per channel, but the roles must remain separate.

---

## 5. Channel-Specific Design

### 5.1 Discord

Recommended SDK: `github.com/bwmarrin/discordgo`, matching PicoClaw's donor choice.

First-pass Discord behavior:

- start one bot session
- accept either:
  - direct messages to the bot, or
- messages from one allowlisted guild/channel pair
- in guild channels, require a bot mention by default
- convert accepted input into `kernel.PlatformEventSubmit`
- while the kernel streams, emit a typing indicator loop
- send the final answer as a normal message in the same DM or channel

Out of scope:

- voice connections
- TTS
- rich embed composition
- broad multi-guild routing

PicoClaw donor value:

- startup/session open pattern
- mention detection and group trigger behavior
- typing loop lifecycle
- reply/media send shape

### 5.2 Slack

Recommended SDKs:

- `github.com/slack-go/slack`
- `github.com/slack-go/slack/socketmode`

Both align with PicoClaw's donor implementation.

First-pass Slack behavior:

- start one Socket Mode session
- accept messages only from one allowlisted channel
- ACK events promptly and deterministically
- translate inbound messages into `kernel.PlatformEventSubmit`
- keep thread context when replying inside a thread
- send final answers as channel messages or thread replies
- do **not** ship reaction-based pending ACK UX in this batch; transport ACK correctness is enough

Out of scope:

- slash commands
- broad workspace routing
- modal / interactive workflows
- webhook mode

PicoClaw donor value:

- Socket Mode event loop
- channel/thread timestamp handling
- pending ACK / reaction pattern
- file-upload shape for later expansion

---

## 6. Config Design

Add two new config sections beside `TelegramCfg` in `internal/config/config.go`.

### 6.1 `DiscordCfg`

Required first-pass fields:

- `bot_token string`
- `allowed_channel_id string`
- `allowed_guild_id string`
- `mention_required bool`
- `coalesce_ms int`

Defaults:

- `mention_required = true`
- `coalesce_ms = 1000`

Notes:

- `allowed_channel_id` may be empty only when the adapter is intentionally running in DM-only mode.
- `allowed_guild_id` is optional when Discord is used only for DMs.

### 6.2 `SlackCfg`

Required first-pass fields:

- `bot_token string`
- `app_token string`
- `allowed_channel_id string`
- `socket_mode bool`
- `coalesce_ms int`
- `reply_in_thread bool`

Defaults:

- `socket_mode = true`
- `coalesce_ms = 1000`
- `reply_in_thread = true`

### 6.3 Config philosophy

Do not attempt to port the full upstream Hermes platform/env surface in this batch.

This batch should expose only enough config to safely run one controlled Discord surface and one controlled Slack surface. Broader platform parity remains Phase 2.F / 5.O work.

---

## 7. Data Flow

For both adapters, the flow is intentionally the same as Telegram:

1. the platform SDK receives an event
2. the adapter rejects unauthorized or irrelevant traffic at the edge
3. accepted traffic becomes a `kernel.PlatformEvent`
4. the kernel processes the turn using the existing Go runtime
5. the adapter listens to `kernel.RenderFrame` output and converts it into platform-native UX

Platform packages own:

- message parsing
- mention detection
- channel/thread identity
- typing indicators
- ack/reaction quirks
- channel-specific formatting

They do **not** own:

- turn state
- tool loop
- memory or recall behavior
- session lifecycle policy beyond platform keying and persistence hooks

---

## 8. Failure Behavior

First-pass adapters must fail boringly and predictably.

### 8.1 Startup failures

- invalid or missing token: command returns a precise startup error
- auth failure during open: command exits early
- invalid allowlist config: command rejects startup rather than running open-by-default accidentally

### 8.2 Inbound failures

- unauthorized channel/guild/user traffic: ignored, with debug/warn logging
- malformed event payloads: ignored, logged if useful
- kernel admission failure or busy state: adapter sends a short "busy, try again" style message where appropriate

### 8.3 Outbound failures

- send/edit failure: log, do not retry forever, do not corrupt kernel state
- typing loop must stop on final frame, failure, cancellation, and shutdown
- Slack must ACK events promptly regardless of whether downstream processing succeeds

### 8.4 Persistence failures

- session-map write failure is warning-only
- memory store remains part of the shared runtime boot; adapter must not own its recovery policy

### 8.5 Explicitly deferred behavior

This batch must not smuggle in:

- reconnect supervisors beyond SDK defaults
- generic lifecycle manager work from Phase 2.F
- cron delivery sinks for Discord/Slack unless the sink seam drops in with near-zero extra complexity

---

## 9. Testing Strategy

TDD slices are fixed and must be implemented in order.

### 9.1 Slice 1 — Config + command wiring

Tests must prove:

- new config sections load correctly
- missing required tokens fail with precise errors
- unsafe configs are rejected before adapters boot
- the shared runtime boot extraction preserves Telegram behavior

### 9.2 Slice 2 — Inbound adapter behavior

Discord tests:

- DM accepted
- allowlisted guild/channel accepted
- guild message without mention rejected when `mention_required=true`
- malformed or irrelevant events ignored

Slack tests:

- allowed channel accepted
- disallowed channel ignored
- thread metadata preserved for replies
- ACK behavior is correct

### 9.3 Slice 3 — Outbound render flow

Tests must prove:

- render frames map to sane platform messages
- typing/start/final lifecycle behaves correctly
- coalescing does not spam sends
- shutdown cancels goroutines cleanly

### 9.4 Slice 4 — Command-level integration

Each command needs an integration-style test with mocked platform client and shared runtime dependencies:

- one inbound message traverses adapter -> kernel -> outbound response
- session keying and persistence shape remain stable
- command shutdown is clean and bounded

### 9.5 Non-goals for first test batch

The first implementation plan should not require live Discord or Slack integration tests. Mocked adapter tests and command-level integration tests are enough for the initial landing.

---

## 10. File-Level Deliverable Shape

Expected new or modified files:

```text
gormes/
├── cmd/gormes/
│   ├── discord.go
│   ├── discord_test.go
│   ├── slack.go
│   ├── slack_test.go
│   ├── gateway_runtime.go
│   └── gateway_runtime_test.go
├── internal/config/
│   ├── config.go
│   └── config_test.go
├── internal/discord/
│   ├── client.go
│   ├── real_client.go
│   ├── bot.go
│   ├── render.go
│   ├── mock_test.go
│   ├── bot_test.go
│   └── render_test.go
└── internal/slack/
    ├── client.go
    ├── real_client.go
    ├── bot.go
    ├── render.go
    ├── mock_test.go
    ├── bot_test.go
    └── render_test.go
```

The exact test file split can move if the final plan finds a better arrangement, but the responsibilities above are locked.

---

## 11. Out Of Scope

This batch does **not** include:

- WhatsApp
- generic multi-platform gateway framework
- webhook-family channels
- hooks, pairing, restart, or lifecycle manager work
- Discord voice or TTS
- Slack slash-command parity
- full Hermes CLI parity
- dotenv/config migration work beyond what already exists
- large refactors not forced by the adapter work

---

## 12. Success Criteria

This batch succeeds when all of the following are true.

### 12.1 Discord success

- `gormes discord` boots with valid config
- DM or one configured guild/channel can submit a turn
- guild traffic can require mention
- streaming/final render UX is sane
- shutdown is clean
- tests prove the above

### 12.2 Slack success

- `gormes slack` boots in Socket Mode with valid config
- one allowlisted channel can submit a turn
- replies can stay in-thread
- event ACK behavior is correct
- outbound render flow does not spam
- shutdown is clean
- tests prove the above

### 12.3 Architectural success

- the kernel remains platform-agnostic
- PicoClaw donor behavior is used only at the transport edge
- Telegram behavior is not regressed by the shared runtime extraction
- the result lowers the cost of later adapters rather than increasing it

If Discord and Slack land but the implementation leaves Phase 2.B.4+ harder to reason about than before, the batch failed architecturally even if the commands compile.
