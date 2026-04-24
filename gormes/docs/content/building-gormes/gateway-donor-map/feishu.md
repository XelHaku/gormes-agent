---
title: "Feishu"
weight: 120
---

# Feishu

Feishu is a plausible Phase 2 adapter target because PicoClaw already split the implementation into transport helpers, reply-context logic, token-cache workaround, and architecture-specific build targets. That split makes the donor readable and easier to judge than most of the China-facing set.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now groups Feishu into the Phase `2.B.10` regional/device adapter tranche. Gormes has a contract-tested shared-bot seam in `internal/channels/feishu` for ingress policy and reply-target preservation, but no real Feishu transport/bootstrap binding yet.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/internal/channels/feishu/bot.go`, `gormes/internal/channels/feishu/bot_test.go`, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/feishu.md`.

Keep the boundary explicit: PicoClaw donates Feishu edge mechanics only. Gormes remains authoritative for lifecycle ownership, session policy, and feature scope.

## Why This Adapter Is Reusable

The donor is reusable because responsibilities are already separated in the way a porter wants.

- `common.go` contains shared parsing and content helpers: mention cleanup, card-image extraction, and small utility functions.
- `feishu_64.go` contains the real transport implementation: SDK client startup, websocket event loop, send/edit/react/media behavior, inbound message handling, and resource download.
- `feishu_32.go` is a deliberate stub for unsupported architectures, which is useful because it makes the support boundary explicit instead of leaving it implicit in build failures.
- `feishu_reply.go` isolates the reply/thread context logic, including message lookup and wrapper formatting.
- `token_cache.go` isolates a donor-specific workaround for stale tenant token caching in the upstream SDK.

That file split makes Feishu useful not only as code, but also as a decomposition template for future Gormes `internal/channels/feishu` runtime files around the current contract seam.
Only the inbound/runtime skeleton, media download path, and reply lookup logic are copy-worthy; send, edit, react, and card UX in `feishu_64.go` should be treated as pattern-only.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/feishu/common.go`
- `picoclaw/pkg/channels/feishu/feishu_32.go`
- `picoclaw/pkg/channels/feishu/feishu_64.go`
- `picoclaw/pkg/channels/feishu/common_test.go`
- `picoclaw/pkg/channels/feishu/feishu_64_test.go`
- `picoclaw/pkg/channels/feishu/feishu_reply.go`
- `picoclaw/pkg/channels/feishu/token_cache.go`
- `picoclaw/pkg/channels/feishu/feishu_reply_test.go`
- `picoclaw/docs/channels/feishu/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/feishu.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- The file-level split itself: shared helpers, runtime implementation, reply logic, and token-cache workaround should stay separate.
- `handleMessageReceive` and `downloadInboundMedia` from `feishu_64.go`; they cover the hard transport work: SDK event intake, mention gating, tenant metadata, and message-resource download.
- `feishu_reply.go` reply-target resolution and wrapper formatting. The tests show the intended shape clearly.
- `token_cache.go` if the same SDK bug still exists when Gormes implements the adapter.
- The 32-bit unsupported stub pattern from `feishu_32.go` if Gormes chooses to support Feishu only on 64-bit targets.

Rebuild in Gormes-native form:

- Placeholder sending, reactions, and editing should be re-expressed through Gormes' own UX contracts, not copied blindly from PicoClaw interfaces.
- `Send`, `EditMessage`, `SendPlaceholder`, `ReactToMessage`, and card-focused fallback behavior in `feishu_64.go` are donor patterns, not direct copy targets.
- The exact message wrapper syntax in `formatReplyContext` should be reviewed against Gormes prompt conventions before reuse.
- Config and setup semantics should follow the Hermes-facing Feishu docs, especially around WebSocket versus webhook mode, even though PicoClaw currently emphasizes websocket SDK mode.

## Gormes Mapping

- `feishu_64.go` is the donor for a future `internal/channels/feishu/runtime.go` style file on the inbound side: start SDK client, fetch bot identity, receive messages, and download message resources cleanly.
- `common.go` maps to adapter-local parsing helpers shared by inbound and outbound flows.
- `feishu_reply.go` maps to Gormes thread/reply enrichment logic. It is especially useful because it handles both parent/root resolution and message fetch fallback.
- `token_cache.go` should inform a very small adapter-local compatibility shim rather than a shared platform-agnostic cache abstraction.
- `feishu_32.go` matters for planning: if Gormes wants Feishu as a Phase 2 target, it should document the 64-bit requirement early instead of discovering it at release time.

## Implementation Notes

- Keep Feishu scoped as a websocket-first adapter unless product requirements force webhook mode. The PicoClaw donor is strongest on SDK/websocket transport.
- Preserve the early allowlist check before media download; that is good edge hygiene and avoids wasted network I/O.
- Port reply-context behavior with its tests. Reply/thread handling is subtle and the donor already captures the edge cases that matter.
- Validate whether the token-cache invalidation bug still exists in the SDK version Gormes uses. If it does, keep the workaround local and explicit.
- If Phase 2 only needs text plus attachments, AI-card fallback and richer editing features can follow later.

## Risks / Mismatches

- The donor is 64-bit-first. That is acceptable, but it reduces the adapter's reach on smaller devices compared with other channels.
- Upstream Hermes docs describe both websocket and webhook deployment stories. PicoClaw only donates one of those strongly.
- `formatReplyContext` is prompt-facing behavior, not just transport behavior. Gormes may want a different wrapper syntax or kernel-side representation.
- Feishu SDK behavior can drift, so the value here is structural donor guidance plus targeted code reuse, not permanent copy-paste certainty.

## Port Order Recommendation

1. Port shared helpers and the 64-bit runtime skeleton first.
2. Port inbound message handling, mention detection, and media download next.
3. Port reply-context logic with the existing tests as the specification.
4. Add send/edit/react/media polish after the basic adapter loop is stable.
5. Only consider webhook parity if Phase 2 deployment requirements actually need it.

Feishu still looks like a reasonable Phase 2 target, but the recommendation is narrow: copy the inbound/runtime skeleton, media download, and reply lookup pieces, then rebuild the send/edit/react/card UX around Gormes-native contracts.

## Code References

- `picoclaw/pkg/channels/feishu/common.go`
- `picoclaw/pkg/channels/feishu/feishu_32.go`
- `picoclaw/pkg/channels/feishu/feishu_64.go`: `Start`, `Send`, `handleMessageReceive`, `fetchBotOpenID`, `isBotMentioned`, `downloadInboundMedia`, `downloadResource`.
- `picoclaw/pkg/channels/feishu/common_test.go`
- `picoclaw/pkg/channels/feishu/feishu_64_test.go`
- `picoclaw/pkg/channels/feishu/feishu_reply.go`: `prependReplyContext`, `resolveReplyTargetMessageID`, `fetchMessageByID`, `formatReplyContext`.
- `picoclaw/pkg/channels/feishu/token_cache.go`
- `picoclaw/pkg/channels/feishu/feishu_reply_test.go`
- `picoclaw/docs/channels/feishu/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/feishu.md`

Recommendation: `adapt pattern only`.
