---
title: "Webhook"
weight: 150
---

# Webhook

PicoClaw's generic webhook support is useful as ingress scaffolding, but it is not a substitute for Gormes' gateway architecture.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` plans both a gateway Webhook adapter in Phase 2.B.10 and a broader webhook subscription system in Phase 2.D. That means Gormes already expects webhook ingress to exist inside a larger runtime, not as a freestanding channel manager.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes planning and upstream Hermes operator behavior were verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `gormes/docs/content/upstream-hermes/user-guide/messaging/webhooks.md`.

Keep the boundary explicit: PicoClaw contributes generic HTTP ingress patterns only. Gormes gateway architecture remains authoritative.

## Why This Adapter Is Reusable

The reusable value is deliberately small and sharp:

- a narrow `WebhookHandler` interface for adapters that expose an HTTP path
- a narrow `HealthChecker` interface for health endpoints on the same shared server
- a dynamic HTTP mux that supports mount and unmount without rebuilding the whole server

Those patterns are worth keeping because webhook-family adapters such as LINE or future generic webhook triggers need a common mount surface.

The donor is not a direct architecture donor because it says nothing about route configuration, HMAC policy, prompt templating, delivery routing, persistence, or webhook subscriptions at the level described by upstream Hermes docs.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/webhook.go`
- `picoclaw/pkg/channels/dynamic_mux.go`
- Generic context only: `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/webhooks.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- the `WebhookHandler` contract: path plus `http.Handler`
- the `HealthChecker` contract for shared health exposure
- `dynamicServeMux` path matching behavior: exact match first, then longest subtree prefix
- dynamic registration and unregistration for adapters that appear or disappear with config reloads

Rebuild in Gormes-native form:

- webhook route config, secrets, event filters, prompt templates, and delivery destinations
- lifecycle ownership of the shared HTTP server
- subscription persistence and CLI integration
- observability, request tracing, and auth failure reporting

## Gormes Mapping

- `WebhookHandler` maps well to a small internal interface inside Gormes' gateway HTTP layer.
- `dynamicServeMux` is a reasonable donor if Gormes wants live route registration for multiple webhook-capable adapters.
- The upstream Hermes webhook doc shows that Gormes will need a higher layer above this donor: route definitions, HMAC validation, prompt rendering, and downstream delivery.
- The generic webhook page should therefore sit beside adapter-specific webhook users like LINE, not replace them.

## Implementation Notes

- Use this donor as shared ingress plumbing, not as the webhook product itself.
- If Gormes supports config reload or dynamic subscriptions, `dynamicServeMux` is a good fit because it avoids rebuilding the server just to swap handlers.
- Preserve the donor's simple and predictable routing semantics. They are enough for adapter mounts and health paths.
- Keep request validation and business logic outside the mux. The mux should only dispatch.

## Risks / Mismatches

- The donor is intentionally minimal. It does not solve signature validation, payload templating, storage, retries, or cross-platform delivery.
- Over-copying it would encourage a too-thin webhook feature that falls short of the richer Gormes plan already documented.
- A dynamic mux is useful, but it can complicate lifecycle and metrics unless Gormes wraps it cleanly.
- Webhook-family adapters and generic webhook triggers should share HTTP plumbing, but they should not collapse into one indistinct feature.

## Port Order Recommendation

1. Build the shared HTTP gateway surface first.
2. Port a small handler-registration interface and, if needed, dynamic mux behavior.
3. Add adapter-specific webhook consumers such as LINE on top of that shared surface.
4. Build the richer generic webhook subscription system separately above the shared HTTP layer.

## Code References

- `picoclaw/pkg/channels/webhook.go`: `WebhookHandler`, `HealthChecker`.
- `picoclaw/pkg/channels/dynamic_mux.go`: `dynamicServeMux`, `Handle`, `HandleFunc`, `Unhandle`, `ServeHTTP`.
- `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/webhooks.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `adapt pattern only`.
