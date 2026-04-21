---
title: "WeCom"
weight: 100
---

# WeCom

WeCom is one of the clearer China-facing donor candidates because PicoClaw already implements the official AI Bot WebSocket protocol directly. The porter question is not whether the transport edge exists; it is how much of that edge is reusable once PicoClaw's QR bootstrap and gateway glue are stripped away.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` groups WeChat work under Phase `2.B.14` as planned. Gormes also carries upstream Hermes operator docs for WeCom behavior, but no Go adapter exists yet.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `gormes/docs/content/upstream-hermes/user-guide/messaging/wecom.md`.

Keep the boundary explicit: PicoClaw is donor input for WeCom channel-edge mechanics only. Gormes architecture, session ownership, and gateway/kernel boundaries remain authoritative.

## Why This Adapter Is Reusable

The reusable part is mostly the transport protocol handling, not the onboarding surface.

- `pkg/channels/wecom/wecom.go` owns the actual AI Bot WebSocket lifecycle: subscribe, heartbeat, read loop, request/response correlation, turn tracking, and fallback from reply-mode streams to active push.
- `pkg/channels/wecom/protocol.go` is valuable because it captures the protocol frame shapes and message-body variants in one place.
- `pkg/channels/wecom/media.go` contains platform-specific media constraints, upload chunking, AES-CBC inbound decryption, and message-type mapping that a porter would otherwise need to rediscover from scattered docs.
- `pkg/channels/wecom/reqid_store.go` shows how PicoClaw preserves reply routing across time, but its persistence path and state ownership are PicoClaw-specific.

The bootstrap pieces are donor ideas, not donor code:

- `cmd/picoclaw/internal/auth/wecom.go` is CLI QR onboarding glue.
- `web/backend/api/wecom.go` is web-admin QR onboarding glue.

That split matters. WeCom is mostly donor code at the transport layer, but mostly donor ideas at the auth/bootstrap layer.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/wecom/wecom.go`
- `picoclaw/pkg/channels/wecom/protocol.go`
- `picoclaw/pkg/channels/wecom/media.go`
- `picoclaw/pkg/channels/wecom/reqid_store.go`
- `picoclaw/pkg/channels/wecom/wecom_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/wecom.go`
- `picoclaw/web/backend/api/wecom.go`
- `picoclaw/docs/channels/wecom/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/wecom.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- The WebSocket connection lifecycle from `Start`, `connectLoop`, `runConnection`, `heartbeatLoop`, and `readLoop`.
- Envelope and command structs from `protocol.go`; they document the real wire contract.
- Reply-mode turn handling from `dispatchIncoming`, `queueTurn`, `BeginStream`, `sendStreamChunk`, and `sendActivePush`.
- Media upload/download behavior from `media.go`, especially size caps, 512 KB chunk upload flow, and the distinction between native image/file/voice/video sends.
- Behavioral tests in `wecom_test.go`, especially req-id route storage, stream update/finalize flow, and fallback from stream reply to active push.

Rebuild in Gormes-native form:

- Request-route persistence. `reqid_store.go` writes under `~/.picoclaw`; Gormes needs its own state location and likely an adapter-owned cache abstraction rather than a donor file-path convention.
- QR login and credential save flow. Both `auth/wecom.go` and `web/backend/api/wecom.go` are PicoClaw control-plane glue, not transport logic.
- Inbound publication into the runtime. Replace PicoClaw's bus calls and metadata layout with Gormes kernel submission and Gormes session identity rules.
- Operator-facing config semantics. Follow the upstream Hermes WeCom behavior doc for policy and configuration shape, not PicoClaw's exact settings object.

## Gormes Mapping

- `wecom.go` should inform a future `gormes/internal/wecom` adapter split between transport session management and Gormes-facing event submission.
- `dispatchIncoming` is the donor for parsing inbound WeCom message types, extracting media, and deciding when a turn is stream-capable.
- `BeginStream` plus `wecomStreamer` maps well to Gormes' adapter-local streaming reply contract. The important idea is that WeCom streaming is tied to an active inbound turn, not to arbitrary outbound messages.
- `sendActivePush` and `sendActiveMedia` are the donor path for expired-turn fallback and proactive outbound delivery.
- The Hermes doc should remain authoritative for DM/group policy and allowlist semantics; PicoClaw only proves the transport implementation details.

## Implementation Notes

- Preserve the hard split between reply-mode streaming and proactive push. That is the center of the WeCom transport.
- Port `protocol.go` structurally first. It is easier to reason about the rest of the adapter once the command and payload types are fixed.
- Carry over duplicate suppression and turn expiry behavior; the donor already treats replayed messages and stale streams as real operational cases.
- Keep media upload logic close to the adapter. WeCom's media limits and upload protocol are too channel-specific to push into a generic shared helper.
- Treat QR flows as separate setup work. They should not leak into the transport package.

## Risks / Mismatches

- PicoClaw persists req-id routes in a local JSON file under a donor-specific home directory. Copying that literally would violate Gormes ownership boundaries.
- The donor assumes PicoClaw's message bus and media-store interfaces. The protocol logic is portable, the surrounding call graph is not.
- Upstream Hermes docs describe richer access-policy behavior than the PicoClaw donor package exposes directly. Product behavior should still come from Gormes docs.
- QR bootstrap code is fragile to vendor changes and should be revalidated when implementation begins; it is not the stable part of the donor.

## Port Order Recommendation

1. Port the protocol structs and WebSocket lifecycle first.
2. Port inbound dispatch, req-id/turn tracking, and stream reply handling next.
3. Port media upload/download and native media sends after text reply flow is stable.
4. Rebuild QR onboarding separately as setup/control-plane work.
5. Only then widen policy/config surfaces to match the final Gormes operator UX.

## Code References

- `picoclaw/pkg/channels/wecom/wecom.go`: `Start`, `BeginStream`, `Send`, `SendMedia`, `connectLoop`, `runConnection`, `dispatchIncoming`, `sendStreamChunk`, `sendActivePush`, `sendActiveMedia`.
- `picoclaw/pkg/channels/wecom/protocol.go`
- `picoclaw/pkg/channels/wecom/media.go`: `storeRemoteMedia`, `resolveOutboundPart`, `uploadOutboundMedia`, `resolveMediaRoute`.
- `picoclaw/pkg/channels/wecom/reqid_store.go`
- `picoclaw/pkg/channels/wecom/wecom_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/wecom.go`
- `picoclaw/web/backend/api/wecom.go`
- `picoclaw/docs/channels/wecom/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/wecom.md`

Recommendation: `copy candidate`.
