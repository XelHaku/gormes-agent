---
title: "DingTalk"
weight: 130
---

# DingTalk

DingTalk is a good example of why transport shape matters more than headline platform popularity. PicoClaw's donor is small, but it already uses Stream Mode, which means inbound delivery comes from a long-lived outbound connection instead of from the shared webhook server pattern used elsewhere.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` marks DingTalk as Phase `2.B.15` and planned. Gormes ships upstream Hermes operator docs for DingTalk setup, but no Go adapter yet.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`.

Keep the boundary explicit: PicoClaw contributes the DingTalk edge. Gormes architecture, session model, and UX policy remain authoritative.

## Why This Adapter Is Reusable

This donor is reusable because it is small and transport-centered, but that does not make it a broad copy target.

- `dingtalk.go` starts the official stream client, registers one callback, stores the `session_webhook` per chat, and replies through that webhook.
- That means the donor already proves the key architectural fact: DingTalk inbound delivery does not require Gormes to stand up the shared webhook server for the first adapter version.
- Group mention handling is localized in `onChatBotMessageReceived`, and the tests pin down the mention-strip and direct-chat fallback behavior.

The donor does not yet show advanced DingTalk features from the upstream Hermes docs, and the outbound side is intentionally narrow because it depends on callback-scoped `session_webhook` state.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
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

- `Start` maps directly to a future `internal/dingtalk` runtime entrypoint and is the main reason this donor matters.
- `onChatBotMessageReceived` is the donor for DM/group distinction, mention gating, sender identity extraction, and session-webhook capture.
- `Send` and `SendDirectReply` explain the outbound architecture boundary: delivery is tied to the `session_webhook` from the inbound callback, not to a general API client with globally addressable chat IDs.
- The Hermes-facing doc should remain the product source of truth for user-visible behavior and optional DingTalk-specific enhancements.

## Implementation Notes

- Lean into Stream Mode. It reduces first-port complexity because Gormes does not need a separate inbound webhook service for this adapter.
- Keep `session_webhook` as adapter-local ephemeral state keyed by the conversation identifier.
- Port the mention-strip behavior early; group usability depends on it and the donor tests already specify the rule.
- Expect the first Gormes version to be narrower than the Hermes docs. That is acceptable if text delivery and group/DM policy are correct first.
- Treat this donor as a source of inbound and reply-path patterns, not as an end-to-end DingTalk adapter ready to transplant.

## Risks / Mismatches

- `session_webhook` is transient callback state. If Gormes needs proactive sends without recent inbound traffic, the first donor shape will be insufficient.
- The donor is intentionally minimal compared with the upstream Hermes DingTalk story. Features like cards and richer gateway UX are not proven here.
- The Stream SDK is the right first transport, but it adds dependency lock-in to DingTalk's client library and callback model.

## Port Order Recommendation

1. Port Stream Mode startup and shutdown first.
2. Port inbound callback parsing, group mention filtering, and webhook capture next.
3. Port text replies through session webhooks.
4. Only then layer on richer DingTalk-specific UX from the Hermes docs.

For the initial adapter, the donor is best used to shape inbound handling and callback-bound replies, then rebuilt around Gormes' own broader outbound expectations.

## Code References

- `picoclaw/pkg/channels/dingtalk/dingtalk.go`: `Start`, `Stop`, `Send`, `onChatBotMessageReceived`, `SendDirectReply`.
- `picoclaw/pkg/channels/dingtalk/dingtalk_test.go`
- `picoclaw/docs/channels/dingtalk/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`

Recommendation: `adapt pattern only`.
