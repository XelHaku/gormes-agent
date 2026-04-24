---
title: "WeiXin"
weight: 110
---

# WeiXin

WeiXin is the opposite of WeCom in reuse terms: PicoClaw proves the channel is possible, but much of the code is shaped around Tencent iLink's session token rules, QR-login bootstrap, and encrypted CDN behavior rather than a clean reusable gateway edge.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now groups personal WeChat into the Phase `2.B.10` regional/device adapter tranche. Gormes has a contract-tested shared-bot seam in `internal/channels/weixin` for policy-gated ingress and reply-path behavior, but the real iLink transport/bootstrap binding remains planned.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/internal/channels/weixin/bot.go`, `gormes/internal/channels/weixin/bot_test.go`, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/weixin.md`.

Keep the boundary explicit: PicoClaw donates ideas and channel-specific edge behavior. Gormes remains authoritative for architecture, session ownership, and operator policy.

## Why This Adapter Is Reusable

The donor is useful, but mostly as a pattern library rather than as code to transplant.

- `pkg/channels/weixin/api.go` captures the Tencent iLink request shape, headers, versioning, and QR endpoints. That is worth reusing conceptually.
- `pkg/channels/weixin/auth.go`, `cmd/picoclaw/internal/auth/weixin.go`, and `web/backend/api/weixin.go` prove both native terminal QR login and web QR login flows, including redirect-host handling during polling.
- `pkg/channels/weixin/media.go` is the strongest technical donor because iLink media handling is genuinely channel-specific: AES-128-ECB encryption/decryption, CDN download/upload URLs, retry behavior, and voice/media conversion constraints.
- `pkg/channels/weixin/state.go` shows the real burden of the channel: context-token persistence, typing-ticket caching, paused sessions after token expiry, and disk-backed poll cursors.

The problem is scope coupling. Too much of the donor code exists to compensate for iLink's session model. That makes it valuable to study, but expensive to copy blindly.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/weixin/weixin.go`
- `picoclaw/pkg/channels/weixin/api.go`
- `picoclaw/pkg/channels/weixin/auth.go`
- `picoclaw/pkg/channels/weixin/media.go`
- `picoclaw/pkg/channels/weixin/state.go`
- `picoclaw/pkg/channels/weixin/types.go`
- `picoclaw/pkg/channels/weixin/weixin_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/weixin.go`
- `picoclaw/web/backend/api/weixin.go`
- `picoclaw/docs/channels/weixin/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/weixin.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- The API surface and header conventions in `api.go`, especially the iLink version markers and endpoint split between QR bootstrap and normal authenticated calls.
- Media encryption and CDN handling patterns from `media.go`, including `downloadAndDecryptCDNBuffer`, `uploadBufferToCDN`, and `sendUploadedMedia`.
- The session-expiry pause logic and typing-ticket cache ideas from `state.go`.
- Targeted tests in `weixin_test.go` that pin down AES key parsing, CDN URL escaping, and upload/download fallback behavior.

Rebuild in Gormes-native form:

- `handleInboundMessage` and `Send` should not be copied literally. The donor assumes every outbound reply depends on a previously cached `context_token` keyed by `from_user_id`, and that constraint should be re-expressed inside Gormes' own session model.
- QR login code should be rebuilt as Gormes setup/control-plane work, not imported from PicoClaw's CLI and web handlers.
- Disk state layout from `state.go` should not be copied. Paths, persistence ownership, and failure policy need to follow Gormes conventions.
- Message formatting and Markdown behavior should follow the Hermes-facing Weixin docs, not just PicoClaw's current choices.

## Gormes Mapping

- `pollLoop` is the donor for long-poll receive behavior, cursor persistence, retry, and session-expiry handling.
- `handleInboundMessage` shows how iLink messages collapse into a direct-chat model centered on `from_user_id`, `context_token`, and an item list of text/media payloads.
- `Send` and `sendTextMessage` explain the core incompatibility a porter must solve: outbound messages are only valid when Gormes can recover the current iLink context token.
- `StartTyping` and `sendTypingStatus` map to adapter-local UX hooks, but only after `getTypingTicket` caching is reimplemented.
- `auth.go` plus the CLI/web onboarding files should influence future Gormes setup UX, but they are not part of the runtime adapter boundary.

## Implementation Notes

- Treat Tencent iLink auth as volatile external dependency work. The QR bootstrap is valuable evidence, but not stable enough to copy without fresh revalidation.
- Port the data model and crypto/media helpers before the main adapter. They are the most channel-specific and least entangled with PicoClaw runtime assumptions.
- Expect a Gormes-specific state component for `context_token`, long-poll cursor, and typing-ticket persistence.
- Keep the native QR-login flow in scope documentation because it is the only reasonable operator story for personal WeChat, but keep it separate from the adapter package.
- Be conservative about roadmap priority. This channel is operationally complex even before Gormes-specific policy is layered on top.

## Risks / Mismatches

- The donor is tightly coupled to iLink's current auth and session semantics. If Tencent changes QR flow, redirect behavior, or token handling, copied code will age badly.
- Outbound delivery depends on a cached `context_token`; losing that state makes send fail hard. That is a deeper architectural constraint than most other adapters have.
- Media handling is technically solid but expensive: AES-128-ECB transforms, CDN retries, upload ticket negotiation, and per-media message shaping.
- Personal WeChat is strategically sensitive and may have weaker long-term stability than enterprise-oriented adapters.

## Port Order Recommendation

1. Decide first whether personal WeChat is worth Phase 2 engineering time at all.
2. If yes, port types/API/crypto-media helpers before any full adapter loop.
3. Rebuild long-poll receive and context-token persistence in Gormes-owned state.
4. Rebuild QR bootstrap separately for CLI/web setup.
5. Add typing indicators and richer media only after text send/receive survives token expiry and restart scenarios.

## Code References

- `picoclaw/pkg/channels/weixin/weixin.go`: `Start`, `pollLoop`, `handleInboundMessage`, `Send`.
- `picoclaw/pkg/channels/weixin/api.go`
- `picoclaw/pkg/channels/weixin/auth.go`: `PerformLoginInteractive`.
- `picoclaw/pkg/channels/weixin/media.go`: `downloadAndDecryptCDNBuffer`, `uploadBufferToCDN`, `sendTextMessage`, `sendUploadedMedia`, `StartTyping`, `SendMedia`.
- `picoclaw/pkg/channels/weixin/state.go`: `pauseSession`, `getTypingTicket`, cursor and context-token persistence helpers.
- `picoclaw/pkg/channels/weixin/types.go`
- `picoclaw/pkg/channels/weixin/weixin_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/weixin.go`
- `picoclaw/web/backend/api/weixin.go`
- `picoclaw/docs/channels/weixin/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/weixin.md`

Recommendation: `adapt pattern only`.
