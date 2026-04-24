---
title: "OneBot"
weight: 80
---

# OneBot

OneBot is useful as a bridge-pattern donor, not as a platform target that should define Gormes' native QQ strategy.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` currently groups QQ Bot into the Phase `2.B.10` regional/device adapter tranche, but it still does not list OneBot as a first-class Gormes gateway platform. That is the right default: OneBot is a community bridge protocol sitting in front of other runtimes.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes planning and the upstream Hermes QQ operator story were verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`.

Keep the boundary explicit: PicoClaw's OneBot adapter is donor input for community bridge mechanics only. Gormes architecture remains authoritative, and the official QQ Bot path should stay the primary plan.

## Why This Adapter Is Reusable

The donor is reusable where OneBot is genuinely protocol-specific:

- persistent WebSocket connection with token auth
- reconnect and ping handling
- request/response correlation through `echo`
- inbound event classification by `post_type`
- CQ-code or segment parsing into normalized text, reply, mention, and media
- group-vs-private routing via prefixed chat IDs
- reply insertion and group reaction support

What is not broadly reusable is the product assumption behind it: Gormes would be depending on an external OneBot-compatible bridge such as napcat or go-cqhttp, plus all of that ecosystem's operational quirks.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/onebot/onebot.go`
- `picoclaw/docs/channels/onebot/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- the WebSocket session pattern: connect, ping, listen, reconnect
- `sendAPIRequest` with `echo`-based correlation for request/response APIs riding the same socket
- `parseMessageSegments` for translating OneBot segment arrays into text, media refs, and reply metadata
- `buildSendRequest` and `buildMessageSegments` for outbound group/private routing and reply insertion
- adapter-local deduplication and last-message tracking

Rebuild in Gormes-native form:

- chat identity. PicoClaw prefixes chat IDs as `group:` and `private:` strings; Gormes should keep structured platform events internally.
- media storage and cleanup. The donor downloads media directly and stores it via the PicoClaw media store.
- allow-list and runtime lifecycle ownership. Keep those attached to Gormes gateway contracts, not PicoClaw's bus.
- platform choice. Do not let a community bridge displace the official QQ Bot path unless the product intentionally chooses that tradeoff.

## Gormes Mapping

- `connect`, `listen`, `reconnectLoop`, and `pinger` are a good donor if Gormes ever adds a bridge-backed WebSocket adapter class.
- `handleRawEvent` and `handleMessage` show how to separate socket framing from platform event normalization.
- `parseMessageSegments` is the best stealable logic here. It already handles text, `@` mentions, replies, images, video, files, and voice records.
- `ReactToMessage` demonstrates how bridge-specific affordances like emoji likes should stay optional and adapter-local.
- If Gormes adds OneBot at all, it should be framed as a compatibility adapter, not as the canonical QQ implementation.

## Implementation Notes

- Keep OneBot behind an explicit "bridge protocol" label in docs and code comments.
- If a future porter needs it, steal the segment parser and socket request/response pattern first; those are the highest-value pieces.
- Preserve the donor's skepticism about transient state: dedup ring, pending request map, and reconnect loop are all practical for long-lived bridge sockets.
- Do not generalize `group:` and `private:` string prefixes into shared gateway architecture. They are local encoding tricks for this adapter.

## Risks / Mismatches

- OneBot behavior depends on the upstream bridge implementation. Protocol dialects and feature support vary across deployments.
- This path adds another runtime hop between Gormes and the actual messaging platform.
- Media download URLs, CQ-code parsing, and reaction support are all bridge-dependent.
- If Gormes already plans the official QQ Bot adapter, maintaining OneBot in parallel risks duplicated effort for overlapping user value.

## Port Order Recommendation

1. Prefer the official QQ Bot path first.
2. Only consider OneBot if there is a concrete need for napcat or go-cqhttp compatibility.
3. If that need exists, port socket lifecycle and segment parsing before adding media or reaction polish.
4. Keep it clearly labeled as a secondary compatibility adapter.

## Code References

- `picoclaw/pkg/channels/onebot/onebot.go`: `NewOneBotChannel`, `ReactToMessage`, `Start`, `connect`, `pinger`, `fetchSelfID`, `sendAPIRequest`, `reconnectLoop`, `Send`, `SendMedia`, `buildMessageSegments`, `buildSendRequest`, `listen`, `parseMessageSegments`, `handleRawEvent`, `handleMessage`.
- `picoclaw/docs/channels/onebot/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `adapt pattern only`.
