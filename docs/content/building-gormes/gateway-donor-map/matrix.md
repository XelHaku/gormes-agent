---
title: "Matrix"
weight: 50
---

# Matrix

Matrix is a plausible future Gormes adapter, but PicoClaw should be treated as a transport donor, not as the authority for Matrix session policy.

## Status

`docs/content/building-gormes/architecture_plan/subsystem-inventory.md` marks Matrix as planned for Phase 2.B.8. Gormes currently mirrors upstream Hermes operator docs for Matrix, but there is no Go Matrix adapter yet.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and operator-facing behavior were verified in-tree against `docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `docs/content/upstream-hermes/user-guide/messaging/matrix.md`.

Keep the boundary explicit: PicoClaw contributes Matrix transport mechanics only. Gormes architecture remains authoritative for session identity, room or thread policy, and runtime ownership.

## Why This Adapter Is Reusable

Matrix is reusable because PicoClaw already isolates most of the protocol-heavy work inside one adapter:

- auth and startup validation in `NewMatrixChannel` and `Start`
- sync loop registration and invite auto-join handling
- plaintext plus formatted outbound send
- media upload and inbound media download
- mention stripping, room-kind detection, and group-trigger gating
- optional E2EE bootstrap through `cryptohelper`
- typing, placeholder send, and edit-in-place support

That said, the donor is stronger as an adapter shape reference than as a full product donor. Upstream Hermes docs describe DM behavior, room mention rules, thread isolation, per-user room sessions, and auto-threading semantics that are not all encoded in PicoClaw's simpler room-based `ChatID` model.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/matrix/matrix.go`
- `picoclaw/pkg/channels/matrix/matrix_test.go`
- `picoclaw/docs/channels/matrix/README.md`
- `docs/content/upstream-hermes/user-guide/messaging/matrix.md`
- `docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- startup and credential validation around homeserver, user ID, access token, and optional device ID
- `handleMemberEvent` auto-join flow for invite acceptance
- `messageContent` plus markdown-to-HTML formatting split
- `SendMedia`, `downloadMedia`, and the inbound media classification helpers
- `StartTyping`, `SendPlaceholder`, and `EditMessage` as Matrix-specific implementations of shared adapter capabilities
- mention parsing and stripping from `isBotMentioned`, `stripUserMention`, and related tests
- `roomKindCache` as a practical performance guard around room membership lookups

Rebuild in Gormes-native form:

- session model and routing. PicoClaw uses room IDs as `ChatID`; upstream Hermes docs describe room, thread, and per-user isolation rules that Gormes must choose explicitly.
- thread ownership. PicoClaw records reply metadata, but it does not implement the full Matrix thread policy described by upstream Hermes.
- encryption persistence and key storage. The `cryptohelper` wiring is useful, but database pathing, passphrase handling, and lifecycle policy should be Gormes-native.
- outbound rendering and delivery state. Placeholder and edit support should fit Gormes' gateway contracts, not PicoClaw's bus flow.

## Gormes Mapping

- `NewMatrixChannel` and `Start` map cleanly to a future `internal/matrix` adapter bootstrap: validate creds, create client, register handlers, optionally initialize crypto, then sync.
- `handleMessageEvent` is the donor for inbound edge processing, but Gormes should decide how its session key composes room identity with optional thread and optional sender dimensions, in line with the upstream Hermes Matrix model for rooms, threads, and shared-room per-user isolation.
- `extractInboundContent` and `extractInboundMedia` map directly to a Gormes transport edge that normalizes Matrix text, files, images, audio, and video into platform events plus stored media references.
- `Send`, `SendMedia`, `StartTyping`, `SendPlaceholder`, and `EditMessage` map well to shared outbound adapter capabilities from [shared-adapter-patterns](../shared-adapter-patterns/).
- `handleMemberEvent` should remain adapter-local. Invite acceptance is Matrix transport behavior, not shared gateway policy.

## Implementation Notes

- Treat the upstream Hermes Matrix doc as the authority for future Gormes product semantics. PicoClaw does not settle the thread/session questions by itself.
- If Gormes wants Matrix E2EE, copy the initialization pattern from `initCrypto`, but re-home the state path and operational controls under Gormes conventions.
- Keep room-kind detection adapter-local. `isGroupRoom` is useful because Matrix DM vs room inference is protocol-specific.
- Preserve the split between plain text and formatted HTML content. Matrix clients benefit from rich formatting, but the plain `Body` must remain sane.
- Reuse the donor tests around mention detection and media temp-file handling as the first regression suite for a future port.

## Risks / Mismatches

- PicoClaw's `ChatID` is just the room ID. That is too small if Gormes later wants thread-level or per-user isolation inside shared rooms.
- E2EE support increases operational surface area immediately: device IDs, crypto DB, passphrase stability, and decryption failure modes.
- Matrix room semantics are more varied than Telegram or Slack. Auto-join, mention-only behavior, replies, and thread support need explicit Gormes policy.
- The donor code is strong on transport mechanics, but it is not a full answer to the richer upstream Hermes room and thread behavior described in the in-tree docs.

## Port Order Recommendation

1. Start with auth, sync, invite-join, plain text send, and inbound text handling.
2. Add room-kind detection, mention parsing, and group-trigger gating.
3. Add media ingress and outbound upload once the basic room model is stable.
4. Add typing, placeholder, and edit support after the outbound contract is settled.
5. Add E2EE only after the non-encrypted path is solid and Gormes has a clear key-storage story.

## Code References

- `picoclaw/pkg/channels/matrix/matrix.go`: `NewMatrixChannel`, `Start`, `initCrypto`, `Send`, `messageContent`, `SendMedia`, `StartTyping`, `SendPlaceholder`, `EditMessage`, `handleMemberEvent`, `handleMessageEvent`, `decryptEvent`, `extractInboundContent`, `downloadMedia`, `isGroupRoom`, `isBotMentioned`, `stripUserMention`.
- `picoclaw/pkg/channels/matrix/matrix_test.go`: mention parsing, room-kind cache behavior, media temp-dir, media extension, and download tests.
- `picoclaw/docs/channels/matrix/README.md`
- `docs/content/upstream-hermes/user-guide/messaging/matrix.md`
- `docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `adapt pattern only`.
