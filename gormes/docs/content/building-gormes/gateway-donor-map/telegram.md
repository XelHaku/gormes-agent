---
title: "Telegram"
weight: 10
---

# Telegram

Telegram is not a greenfield port for Gormes. `gormes/internal/telegram/` already ships the core adapter, so the question here is which PicoClaw edge mechanics are worth pulling in as follow-up deltas.

## Status

Gormes already shipped Telegram for Phase 2.B.1, with long-poll ingress, a single-chat allow/discovery model, streamed edit coalescing, session persistence, and MarkdownV2-safe rendering in `gormes/internal/telegram/`.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current shipped Gormes behavior was verified in-tree against `gormes/internal/telegram/bot.go` and `gormes/internal/telegram/render.go`.

PicoClaw's Telegram donor set is still useful, but only for gaps around richer transport behavior:

- platform command menu registration
- group-trigger and mention parsing beyond a single allowed chat
- thread or topic routing with `MessageThreadID`
- richer Markdown conversion and media handling

Keep the boundary explicit: PicoClaw is donor input for Telegram edge behavior only. Gormes kernel, session mapping, and runtime ownership remain authoritative.

## Why This Adapter Is Reusable

Telegram is one of PicoClaw's cleanest chat adapters because most of the interesting logic sits at the transport edge rather than in global runtime code.

- `picoclaw/pkg/channels/telegram/command_registration.go` is a good donor for "derive Telegram command menu state from a shared registry, then retry in the background without blocking startup."
- `picoclaw/pkg/channels/telegram/parse_markdown_to_md_v2.go` and `picoclaw/pkg/channels/telegram/parser_markdown_to_html.go` are useful because Telegram formatting is a platform-specific mess. The conversion logic is separable from PicoClaw's bus.
- `picoclaw/pkg/channels/telegram/telegram_group_command_filter_test.go` captures hard-earned group-trigger rules around mentions and `/command@botname` handling.
- `picoclaw/pkg/channels/telegram/telegram.go` shows how Telegram topics, placeholder editing, typing, and media dispatch fit together in one adapter.

The donor is less useful for inbound runtime structure because `gormes/internal/telegram/bot.go` already owns Gormes' simpler single-adapter lifecycle.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/telegram/telegram.go`
- `picoclaw/pkg/channels/telegram/command_registration.go`
- `picoclaw/pkg/channels/telegram/parse_markdown_to_md_v2.go`
- `picoclaw/pkg/channels/telegram/parser_markdown_to_html.go`
- `picoclaw/pkg/channels/telegram/telegram_dispatch_test.go`
- `picoclaw/pkg/channels/telegram/telegram_group_command_filter_test.go`
- `picoclaw/docs/channels/telegram/README.md`
- `gormes/internal/telegram/bot.go`
- `gormes/internal/telegram/render.go`

## What To Copy vs What To Rebuild

Copy candidates:

- Command registration from `picoclaw/pkg/channels/telegram/command_registration.go`. The async retry loop, idempotent `GetMyCommands` comparison, and non-blocking startup behavior are directly reusable if Gormes later exposes more than `/start`, `/stop`, and `/new`.
- Markdown conversion ideas from `picoclaw/pkg/channels/telegram/parse_markdown_to_md_v2.go` and `picoclaw/pkg/channels/telegram/parser_markdown_to_html.go`. Gormes already escapes outbound text in `gormes/internal/telegram/render.go`, but PicoClaw covers richer markdown input, nested entities, HTML fallback, and parse-error downgrade behavior.
- Group command filtering behavior from `picoclaw/pkg/channels/telegram/telegram_group_command_filter_test.go`. The `/new@testbot` and `/new@otherbot` cases are the right donor tests if Gormes moves from a single-operator chat toward group support.

Rebuild in Gormes-native form:

- Inbound lifecycle. `gormes/internal/telegram/bot.go` is built around `kernel.PlatformEvent`, render frames, and a coalescer; do not import PicoClaw's bus-oriented `HandleInboundContext` call graph.
- Session ownership and authorization. PicoClaw keys behavior off adapter chat IDs and `BaseChannel`; current Gormes behavior in `gormes/internal/telegram/bot.go` only accepts `Config.AllowedChatID`, or, when `AllowedChatID == 0` and `FirstRunDiscovery` is enabled, replies with a discovery message instead of processing the turn. Keep that model authoritative unless Gormes explicitly broadens Telegram scope.
- Rendering pipeline. Gormes currently treats Telegram as plain escaped MarkdownV2 text with a tool-status tail. If richer formatting is needed, extend `gormes/internal/telegram/render.go` rather than porting PicoClaw's outbound path whole.

## Gormes Mapping

- PicoClaw `Start` long-poll loop maps conceptually to `(*Bot).Run` in `gormes/internal/telegram/bot.go`.
- PicoClaw `handleMessage` maps to `(*Bot).handleUpdate`, but the current Gormes authority is narrower: if `AllowedChatID` is unset it only emits the first-run discovery reply when `FirstRunDiscovery` is true, otherwise it blocks the chat; if `AllowedChatID` is set it only processes that exact chat.
- PicoClaw `parseContent` maps to `formatStream`, `formatFinal`, and `formatError` in `gormes/internal/telegram/render.go`, but Gormes currently uses escape-and-truncate rather than markdown transformation.
- PicoClaw command registration has no direct Gormes equivalent yet. If Gormes later ports the shared slash-command registry from `hermes_cli/commands.py`, Telegram menu registration should hook in there.
- PicoClaw topic handling via `MessageThreadID` has no current Gormes surface. That is a real delta if Gormes later wants forum topics or threaded operator chats.

## Implementation Notes

- Treat Markdown work as a delta on top of `gormes/internal/telegram/render.go`, not a reason to replace the existing coalescer. Gormes already has the streamed-edit behavior that PicoClaw implements through placeholder and message-edit capabilities.
- If Gormes adds command registration, copy the retry model from `picoclaw/pkg/channels/telegram/command_registration.go` and source command definitions from Gormes' eventual shared command registry, not from Telegram-only literals.
- If Gormes expands beyond one operator chat, port PicoClaw's mention and `/command@botname` tests first. The edge rules are easy to regress and expensive to rediscover.
- PicoClaw's `sendChunk` parse-mode fallback is worth borrowing if Gormes starts rendering more than plain escaped text. Telegram parse failures should degrade to plain text instead of losing the response.
- PicoClaw's media send path in `telegram.go` is donor material only if Gormes adds Telegram attachment output. It should stay behind a Gormes-native delivery interface, not a PicoClaw `MediaStore`.

## Risks / Mismatches

- Gormes Telegram today is intentionally single-chat oriented. PicoClaw assumes a broader gateway model with allow-lists, group triggers, replies, and topics. Porting those mechanics blindly would overcomplicate the shipped adapter.
- Gormes uses `go-telegram-bot-api/v5`; PicoClaw uses `telego`. The behavior is portable, but the API surface is not.
- PicoClaw's HTML and MarkdownV2 converters optimize for preserving richer model markdown. Gormes currently optimizes for safe streaming edits. Merging both without careful tests risks broken formatting or duplicate escaping.
- Command registration only pays off if Gormes has a stable shared command registry to publish. A Telegram-only command table would drift from the rest of the product.

## Port Order Recommendation

1. Keep the current shipped adapter as the base.
2. If richer formatting becomes necessary, port the markdown conversion logic and parse-fallback tests first.
3. If multi-chat Telegram becomes a goal, port group-trigger behavior and `/command@botname` tests next.
4. Only after Gormes has a shared command registry should it adopt PicoClaw-style Telegram command menu registration.
5. Treat topic or forum support and media parity as later, separate deltas.

## Code References

- `gormes/internal/telegram/bot.go`: `Run`, `handleUpdate`, `runOutbound`, `handleFrame`, `SendToChat`.
- `gormes/internal/telegram/render.go`: `formatStream`, `formatFinal`, `formatError`, `truncateForTelegram`.
- `gormes/docs/content/using-gormes/telegram-adapter.md`: stale high-level user doc; useful for operator-facing setup context, not as the authority for current shipped adapter behavior.
- `picoclaw/pkg/channels/telegram/telegram.go`: `Start`, `Send`, `sendChunk`, `EditMessage`, `SendPlaceholder`, `SendMedia`, `handleMessage`, `parseContent`, `resolveTelegramOutboundTarget`, `isBotMentioned`.
- `picoclaw/pkg/channels/telegram/command_registration.go`: `RegisterCommands`, `startCommandRegistration`, `commandRegistrationDelay`.
- `picoclaw/pkg/channels/telegram/parse_markdown_to_md_v2.go`: `markdownToTelegramMarkdownV2`, `processText`, `escapeMarkdownV2`.
- `picoclaw/pkg/channels/telegram/parser_markdown_to_html.go`: `markdownToTelegramHTML`, `extractLinks`, `extractCodeBlocks`, `extractInlineCodes`.
- `picoclaw/pkg/channels/telegram/telegram_dispatch_test.go`
- `picoclaw/pkg/channels/telegram/telegram_group_command_filter_test.go`
- `picoclaw/docs/channels/telegram/README.md`

Recommendation: `adapt pattern only`.
