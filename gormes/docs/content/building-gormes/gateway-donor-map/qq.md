---
title: "QQ"
weight: 90
---

# QQ

QQ is one of the stronger donors in this task set because PicoClaw uses the official bot platform rather than a purely community bridge.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now groups QQ Bot into the Phase `2.B.10` regional/device adapter tranche. Gormes has a contract-tested shared-bot seam in `internal/channels/qqbot` for DM/group policy, mention gating, and passive-reply sequencing, but no real official QQ transport/bootstrap binding yet.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and operator-facing behavior were verified in-tree against `gormes/internal/channels/qqbot/bot.go`, `gormes/internal/channels/qqbot/bot_test.go`, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`.

Keep the boundary explicit: PicoClaw contributes official QQ Bot transport mechanics only. Gormes architecture remains authoritative for platform event shape, session mapping, and runtime ownership.

## Why This Adapter Is Reusable

QQ is reusable because the donor already covers the official transport split that Gormes will also need:

- token source creation and refresh
- botgo SDK initialization
- WebSocket session startup and event registration
- separate handling for C2C and group `@` messages
- explicit group-vs-direct routing state
- passive-reply metadata using last inbound message IDs and `msg_seq`
- attachment download and media normalization
- rich media upload through the official `/files` API

This is not just shape reference. Much of the donor logic is platform-specific and can transfer with relatively small structural changes.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/qq/qq.go`
- `picoclaw/pkg/channels/qq/audio_duration.go`
- `picoclaw/pkg/channels/qq/botgo_logger.go`
- `picoclaw/pkg/channels/qq/qq_test.go`
- `picoclaw/docs/channels/qq/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- official SDK bootstrap in `Start`: token refresh, OpenAPI client, handler registration, websocket info lookup, and session-manager startup
- inbound split between `handleC2CMessage` and `handleGroupATMessage`
- `chatType`, `lastMsgID`, and `msgSeqCounters` as adapter-local routing state
- `Send` with separate group and direct message paths plus markdown toggle
- `SendMedia`, `uploadMedia`, and `sendUploadedMedia` for official QQ rich media flow
- attachment download helpers and attachment-note synthesis
- `sanitizeURLs` for QQ group-message URL filtering
- `qqAudioDuration` and `outboundMediaType` to decide when audio can be sent as voice vs file
- `botGoLogger` to demote noisy heartbeat traffic

Rebuild in Gormes-native form:

- config and credential ownership. Gormes should decide how App ID, secret, sandbox controls, and home-channel policy live in its config surface.
- platform event identity. PicoClaw records `account_id`, `group_id`, and direct sender IDs in `Raw`; Gormes should normalize these through its own event contracts.
- voice transcription policy. Upstream Hermes docs describe richer STT behavior than PicoClaw's Go donor currently implements.
- retry, backoff, and health reporting should match Gormes runtime conventions instead of PicoClaw's local adapter choices.

## Gormes Mapping

- `Start` maps directly to a future `internal/channels/qqbot` adapter bootstrap.
- `handleC2CMessage` and `handleGroupATMessage` map to two primary ingress pipelines Gormes will need from day one.
- `chatType` is a practical donor because outbound QQ sends need to know whether a chat ID is direct or group.
- `applyPassiveReplyMetadata` is important: QQ reply behavior is tied to last inbound message ID plus `msg_seq`, so Gormes should preserve that state machine.
- `SendMedia` and `audio_duration.go` are especially valuable because the official QQ media flow is nontrivial and platform-specific.

## Implementation Notes

- Start with the official bot flow, not OneBot compatibility. The subsystem inventory already points Gormes toward QQ Bot as the planned target.
- Preserve the donor's distinction between C2C and group `@` ingress. Group delivery semantics are not the same as direct chat semantics on QQ.
- Keep media upload logic adapter-local. The `/files` pre-upload step and `file_info` handoff are specific to QQ.
- Port the tests around attachment-only messages and base64 media upload early. They capture real edge cases rather than superficial wiring.
- Reuse `botGoLogger` or an equivalent so heartbeat noise does not overwhelm operator logs.

## Risks / Mismatches

- PicoClaw is already opinionated about the `botgo` SDK. If Gormes chooses a different official SDK surface later, the control flow should transfer but the code will not copy verbatim.
- URL sanitization is an API workaround, not a universal messaging behavior. Keep it local to QQ.
- Audio send behavior depends on local file inspection and QQ voice limits; that logic must be tested carefully around codecs and file sizes.
- Upstream Hermes docs include guild and richer STT expectations that are not fully represented in the PicoClaw donor.

## Port Order Recommendation

1. Port official auth, token refresh, WebSocket startup, and C2C or group text ingress first.
2. Add passive-reply metadata and correct group-vs-direct outbound routing.
3. Add attachment ingest and text send parity.
4. Add rich media upload and audio-duration-based voice/file selection.
5. Layer in markdown, STT, or richer delivery variants only after the base official bot path is stable.

## Code References

- `picoclaw/pkg/channels/qq/qq.go`: `NewQQChannel`, `Start`, `Stop`, `Send`, `StartTyping`, `SendMedia`, `uploadMedia`, `buildMediaUpload`, `outboundMediaType`, `sendUploadedMedia`, `applyPassiveReplyMetadata`, `handleC2CMessage`, `handleGroupATMessage`, `extractInboundAttachments`, `downloadAttachment`, `sanitizeURLs`, `VoiceCapabilities`.
- `picoclaw/pkg/channels/qq/audio_duration.go`: `qqAudioDuration`, `qqWAVDuration`, `qqOggDuration`.
- `picoclaw/pkg/channels/qq/botgo_logger.go`
- `picoclaw/pkg/channels/qq/qq_test.go`
- `picoclaw/docs/channels/qq/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `copy candidate`.
