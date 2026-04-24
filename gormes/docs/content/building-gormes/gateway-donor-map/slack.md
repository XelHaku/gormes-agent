---
title: "Slack"
weight: 30
---

# Slack

Slack is partially ported in Phase 2.B.3, but PicoClaw still donates useful parity material around shared-gateway wiring, Socket Mode hardening, thread routing, richer media, and acknowledgment UX.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now marks Slack as in progress for Phase 2.B.3. Gormes has a Go `internal/slack` Socket Mode bot with threaded replies, placeholder updates, and session persistence, but it is not yet registered as a shared `gateway.Channel` from `cmd/gormes gateway` and still needs shared `CommandRegistry` parsing plus config wiring before the adapter is treated as shipped.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and operator-facing behavior were verified in-tree against `gormes/internal/slack/bot.go`, `gormes/cmd/gormes/gateway.go`, `gormes/internal/gateway/commands.go`, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/slack.md`.

PicoClaw already demonstrates a viable Slack edge with:

- Socket Mode startup
- auth and bot identity discovery
- message, mention, and slash-command ingress
- thread timestamp routing
- emoji-based pending acknowledgments
- Slack file download and upload handling

Keep the boundary explicit: PicoClaw contributes Slack transport mechanics only. Gormes architecture remains authoritative for session keys, command handling, and runtime ownership.

## Why This Adapter Is Reusable

Slack's donor surface is reusable because the hardest parts are genuinely Slack-specific.

- Socket Mode wiring in `Start`, `eventLoop`, and `handleEventsAPI` is already isolated from the rest of PicoClaw.
- `pendingAcks` is a practical transport-edge UX mechanism: react with `eyes` on ingress, then swap to `white_check_mark` after successful delivery.
- Slack thread behavior is easy to get subtly wrong, and PicoClaw already codifies the `channel/thread_ts` split plus outbound target resolution helpers.
- Media upload shape is explicit: per-part local path resolution, `UploadFileV2`, filename/title handling, and thread-aware uploads.

This remains a strong donor because the current Slack edge still has closeout gaps around shared gateway registration, generic command parsing, richer ack UX, deeper thread handling, and media/attachment polish.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/slack/slack.go`
- `picoclaw/pkg/channels/slack/slack_test.go`
- `picoclaw/docs/channels/slack/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/slack.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- Socket Mode startup and event-loop shape from `picoclaw/pkg/channels/slack/slack.go`.
- `pendingAcks` as a concept. The `eyes` reaction on receipt and `white_check_mark` after successful send is a transport-local affordance with clear UX value.
- Thread timestamp helpers from `parseSlackChatID`, `resolveSlackOutboundTarget`, and `resolveSlackMediaOutboundTarget`.
- Separate handlers for plain message events, app mentions, and slash commands.
- Media upload shape from `SendMedia`, especially the use of `ThreadTimestamp`, `Filename`, and `Title`.

Rebuild in Gormes-native form:

- Session identity. Gormes should decide whether the canonical key is `(workspace, channel)`, `(workspace, channel, thread)`, or `(workspace, channel, user)` according to its own gateway policy, not PicoClaw's `chatID` string concatenation.
- Command execution. PicoClaw forwards slash commands into its bus; Gormes should tie Slack slash ingress to its own command and kernel surfaces.
- File storage integration. Rebuild around Gormes storage and audit expectations rather than PicoClaw's media-store contract.
- Mention policy details. Upstream Hermes docs include thread-follow and channel behavior rules that should remain the product source of truth.

## Gormes Mapping

- PicoClaw `Start` maps directly to the current `internal/slack` lifecycle: auth test first, remember bot identity, start the event loop, then run the Socket Mode client.
- `handleMessageEvent`, `handleAppMention`, and `handleSlashCommand` map to three distinct ingress paths Gormes will also need.
- `pendingAcks` maps well to a Gormes adapter-local transient state map keyed by delivery target, not to any shared runtime component.
- `resolveSlackOutboundTarget` and `resolveSlackMediaOutboundTarget` should inform Gormes' thread routing, especially because Slack replies depend on `thread_ts` rather than a separate topic ID type.
- `SendMedia` is the donor for outbound file uploads; the lack of a stable posted-message timestamp in `UploadFileV2` should influence Gormes' return contract as well.
- The remaining Gormes-specific closeout is not another private Slack loop; it is adapting this bot to the shared `gateway.Channel`/`gateway.Manager` path used by Telegram and Discord.

## Implementation Notes

- Socket Mode should remain Gormes' default Slack path unless Phase 2.B.3 explicitly demands inbound webhooks. It avoids public HTTP exposure and matches both PicoClaw and the current upstream Hermes operator story.
- Route generic slash commands through `gateway.ParseInboundText` rather than the current adapter-local string switch before claiming shared gateway-command parity.
- Keep `pendingAcks` adapter-local and best-effort. Failed reactions should not fail the turn.
- Preserve the distinction between channel messages and app mentions. PicoClaw treats mentions as an explicit path that can create a synthetic thread key when no thread exists yet.
- Thread timestamp handling is not optional. Slack conversations drift into threads immediately, and outbound routing must preserve them.
- For uploads, copy the shape, not the exact return behavior: `UploadFileV2` does not hand back a normal message timestamp, so Gormes should not pretend file IDs are delivery message IDs.

## Risks / Mismatches

- PicoClaw treats Socket Mode as the implementation. If Gormes later wants a webhook path for enterprise deployment, Slack routing abstractions must be widened without discarding the Socket Mode donor.
- `pendingAcks` is useful, but it is also Slack-specific polish. Do not let that mechanism leak into shared adapter contracts.
- Thread identity is easy to muddle. PicoClaw encodes it as `channel/thread_ts`; Gormes may prefer a structured key internally.
- Upstream Hermes Slack docs describe richer policy around thread replies and shared-session behavior. PicoClaw covers the transport edge well, but not the full product semantics.

## Port Order Recommendation

1. Adapt `internal/slack` onto the shared `gateway.Channel`/`gateway.Manager` path and register it from `cmd/gormes gateway`.
2. Route generic slash commands through `gateway.ParseInboundText` and keep Slack-specific commands as adapter-local affordances only where needed.
3. Keep Socket Mode as the baseline and tighten lifecycle tests before widening features.
4. Add `pendingAcks` only after the current send and reply flow is locked down.
5. Add file download and upload support after the session and reply model is correct.

## Code References

- `picoclaw/pkg/channels/slack/slack.go`: `Start`, `Stop`, `Send`, `SendMedia`, `ReactToMessage`, `eventLoop`, `handleEventsAPI`, `handleMessageEvent`, `handleAppMention`, `handleSlashCommand`, `parseSlackChatID`, `resolveSlackOutboundTarget`, `resolveSlackMediaOutboundTarget`.
- `picoclaw/pkg/channels/slack/slack_test.go`
- `picoclaw/docs/channels/slack/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/slack.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `copy candidate`.
