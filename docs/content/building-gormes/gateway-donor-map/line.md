---
title: "LINE"
weight: 70
---

# LINE

LINE matters mainly as a webhook-first channel. The donor value is not a full adapter transplant; it is a clean example of how to hang a LINE-specific HTTP ingress edge off the shared gateway HTTP surface.

## Status

`docs/content/building-gormes/architecture_plan/subsystem-inventory.md` does not currently list LINE as a planned Gormes gateway platform. The shared subsystem inventory does, however, establish that Gormes expects gateway connectors and HTTP-facing subsystems rather than each platform owning the whole runtime.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Gormes architectural expectations were verified in-tree against `docs/content/building-gormes/architecture_plan/subsystem-inventory.md`.

Keep the boundary explicit: PicoClaw contributes LINE webhook and Messaging API edge mechanics only. Gormes gateway architecture remains authoritative.

## Why This Adapter Is Reusable

The reusable part is the HTTP edge:

- signature verification against `X-Line-Signature`
- request body size limiting before deeper processing
- immediate `200 OK` acknowledgment with async event processing
- mapping LINE source types into a normalized chat ID
- reply-token caching for short-lived reply semantics
- push fallback when reply tokens expire

That is useful because LINE is a webhook-first platform, and the donor already shows how to express that as an adapter implementing `WebhookPath` plus `http.Handler`.

The implementation is not a direct copy candidate because outbound media support is intentionally incomplete and the donor assumes PicoClaw's shared webhook server and media-store conventions.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/line/line.go`
- `picoclaw/pkg/channels/line/line_test.go`
- `picoclaw/docs/channels/line/README.md`
- `docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- `WebhookPath`, `ServeHTTP`, and `webhookHandler` as the LINE ingress skeleton
- HMAC verification and max-body-size protection
- reply-token caching with age checks
- quote-token handling for reply context
- bot-profile fetch for mention detection fallbacks
- direct-vs-group source resolution and group-trigger gating

Rebuild in Gormes-native form:

- HTTP server ownership. Gormes should mount LINE on its own gateway HTTP subsystem, not adopt PicoClaw's shared manager wiring.
- async processing and observability. The `200 OK` then `go processEvent(...)` pattern is correct, but request tracing, metrics, and shutdown semantics should be Gormes-native.
- media output. PicoClaw falls back to text because LINE media sends need publicly reachable URLs; Gormes must decide whether to support hosted media, signed URLs, or text-only fallback.
- session mapping. Chat identity for user, room, and group contexts should follow Gormes event contracts, not PicoClaw's direct string mapping by itself.

## Gormes Mapping

- `WebhookPath` and `ServeHTTP` map well to a future Gormes webhook-capable adapter mounted on the shared gateway HTTP server.
- `processEvent` is the donor for inbound normalization: resolve source, cache reply token, detect mentions, download inbound media, and emit a platform event.
- `Send` is a useful LINE-specific outbound pattern because it prefers the short-lived Reply API and falls back to Push API.
- `StartTyping` is adapter-local polish built on LINE's loading animation endpoint and should remain optional because it only works in 1:1 chats.
- `SendMedia` is explicitly not a reusable outbound implementation; it documents the current limitation instead.

## Implementation Notes

- LINE should be designed as a webhook family adapter inside Gormes, alongside the patterns in [shared-adapter-patterns](../shared-adapter-patterns/) and the generic [webhook](../webhook/) dossier.
- Preserve the order of HTTP defenses from the donor: method check, body size limit, signature validation, JSON decode, quick acknowledgment.
- Reply-token age matters. Gormes should preserve the "try reply first, then push" behavior because it directly affects LINE API cost and user experience.
- Treat bot info lookup as optional enhancement for mention detection, not as a startup blocker.

## Risks / Mismatches

- LINE requires public HTTPS webhook delivery. That deployment reality is external to the adapter and must be accounted for in Gormes operator docs.
- Outbound media remains the largest gap. PicoClaw does not solve hosted media delivery.
- Mention detection is partly heuristic because Official Account mention metadata is inconsistent.
- The donor assumes a fairly simple concurrency model; Gormes may want stronger lifecycle control than bare goroutines per event.

## Port Order Recommendation

1. Build the HTTP mount and signature-verified webhook ingress first.
2. Add text message ingress plus reply-token caching and Reply API sends.
3. Add Push API fallback and group-trigger handling.
4. Add inbound media download and storage after the basic event model is stable.
5. Only then decide whether outbound media deserves a hosted-URL design.

## Code References

- `picoclaw/pkg/channels/line/line.go`: `Start`, `fetchBotInfo`, `WebhookPath`, `ServeHTTP`, `webhookHandler`, `verifySignature`, `processEvent`, `isBotMentioned`, `stripBotMention`, `resolveChatID`, `Send`, `SendMedia`, `sendReply`, `sendPush`, `StartTyping`, `sendLoading`, `callAPI`, `downloadContent`.
- `picoclaw/pkg/channels/line/line_test.go`
- `picoclaw/docs/channels/line/README.md`
- `docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `adapt pattern only`.
