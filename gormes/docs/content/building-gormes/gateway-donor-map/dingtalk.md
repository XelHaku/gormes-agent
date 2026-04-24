---
title: "DingTalk"
weight: 130
---

# DingTalk

DingTalk is a good example of why transport shape matters more than headline platform popularity. PicoClaw's donor is small, but it already uses Stream Mode, which means inbound delivery comes from a long-lived outbound connection instead of from the shared webhook server pattern used elsewhere.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now groups DingTalk into the Phase `2.B.10` regional/device adapter tranche. Gormes now has a contract-tested `internal/channels/dingtalk` bot seam plus Stream Mode/bootstrap planning, session-webhook refresh, and retry/error handling. It still does not have a real DingTalk SDK binding, receive loop, or operator-configured runtime entrypoint.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/internal/channels/dingtalk/bot.go`, `gormes/internal/channels/dingtalk/runtime.go`, their tests, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`.

Keep the boundary explicit: PicoClaw contributes the DingTalk edge. Gormes architecture, session model, and UX policy remain authoritative.

## Why This Adapter Is Reusable

This donor is reusable because it is small and transport-centered, but that does not make it a broad copy target.

- `dingtalk.go` starts the official stream client, registers one callback, stores the `session_webhook` per chat, and replies through that webhook.
- That means the donor already proves the key architectural fact: DingTalk inbound delivery does not require Gormes to stand up the shared webhook server for the first adapter version.
- Group mention handling is localized in `onChatBotMessageReceived`, and the tests pin down the mention-strip and direct-chat fallback behavior.

The donor does not yet show advanced DingTalk features from the upstream Hermes docs, and the outbound side is intentionally narrow because it depends on callback-scoped `session_webhook` state.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/dingtalk/dingtalk.go`
- `picoclaw/pkg/channels/dingtalk/dingtalk_test.go`
- `picoclaw/docs/channels/dingtalk/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy-worthy donor patterns:

- Stream Mode startup from `Start`: credential setup, stream client creation, auto-reconnect, callback registration, and run loop.
- `onChatBotMessageReceived` as the donor for inbound extraction, mention-only group gating, and per-chat `session_webhook` storage.
- `Send` plus `SendDirectReply` as the narrow outbound contract for reply-only delivery: no session webhook, no reply.
- Tests in `dingtalk_test.go`, especially the group mention stripping and direct-chat conversation-id fallback.

Rebuild in Gormes-native form:

- Replace PicoClaw bus publication with Gormes kernel event submission and session derivation.
- Revisit metadata naming and reply-handle storage so they fit Gormes conventions rather than PicoClaw's raw map style.
- Decide separately whether Gormes wants DingTalk-specific features from the upstream Hermes docs, such as richer cards, reactions, or display controls. The donor does not cover those deeply.

## Gormes Mapping

- `Start` now maps to the in-tree `internal/channels/dingtalk.DecideRuntime` contract, which freezes Stream Mode as the first ingress bootstrap before a real SDK client lands.
- `onChatBotMessageReceived` maps to `Bot.toInboundEvent`: DM/group distinction, mention gating, sender identity extraction, generic command parsing through `gateway.ParseInboundText`, and session-webhook capture are all covered in Go tests.
- `Send` and `SendDirectReply` map to `SessionWebhooks` plus `ReplySender`: delivery is tied to the `session_webhook` from the inbound callback, not to a general API client with globally addressable chat IDs.
- The Hermes-facing doc should remain the product source of truth for user-visible behavior and optional DingTalk-specific enhancements.

## Implementation Notes

- Lean into Stream Mode. It reduces first-port complexity because Gormes does not need a separate inbound webhook service for this adapter, and `DecideRuntime` now freezes that as the first transport shape.
- Keep `session_webhook` as adapter-local ephemeral state keyed by the conversation identifier; Gormes now codifies that with `SessionWebhooks` plus retrying `ReplySender` delivery.
- Port the mention-strip behavior early; group usability depends on it and the donor tests already specify the rule.
- Expect the first Gormes version to be narrower than the Hermes docs. That is acceptable if text delivery and group/DM policy are correct first.
- Treat this donor as a source of inbound and reply-path patterns, not as an end-to-end DingTalk adapter ready to transplant.

## Risks / Mismatches

- `session_webhook` is transient callback state. If Gormes needs proactive sends without recent inbound traffic, the first donor shape will be insufficient.
- The donor is intentionally minimal compared with the upstream Hermes DingTalk story. Features like cards and richer gateway UX are not proven here.
- The Stream SDK is the right first transport, but it adds dependency lock-in to DingTalk's client library and callback model.

## Port Order Recommendation

1. Keep the current bot/runtime contracts intact: `Bot.toInboundEvent`, `DecideRuntime`, `SessionWebhooks`, and `ReplySender` are the evidence-backed seam.
2. Bind that seam to the real DingTalk Stream SDK: credential validation, stream client startup, callback registration, receive-loop shutdown, and reconnect behavior.
3. Wire the real client through configuration and `cmd/gormes gateway` only after SDK lifecycle tests pass.
4. Only then layer on richer DingTalk-specific UX from the Hermes docs.

For the initial adapter, the donor is best used to shape inbound handling and callback-bound replies, then rebuilt around Gormes' own broader outbound expectations.

## Code References

- `picoclaw/pkg/channels/dingtalk/dingtalk.go`: `Start`, `Stop`, `Send`, `onChatBotMessageReceived`, `SendDirectReply`.
- `picoclaw/pkg/channels/dingtalk/dingtalk_test.go`
- `picoclaw/docs/channels/dingtalk/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`

Recommendation: `adapt pattern only`.
