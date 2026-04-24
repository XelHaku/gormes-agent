---
title: "Discord"
weight: 20
---

# Discord

Discord is now shipped in Gormes, so this donor page is about parity gaps and reusable transport mechanics rather than whether PicoClaw can seed a first adapter at all.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` now marks Discord as shipped for Phase 2.B.2. Gormes already has a Go Discord adapter on the shared gateway chassis; this page tracks donor material still worth porting or hardening.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and target behavior were verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` and `gormes/docs/content/upstream-hermes/user-guide/messaging/discord.md`.

That makes Discord a real donor candidate. The PicoClaw implementation already covers:

- session startup and shutdown around a live gateway connection
- DM versus guild-channel ingress
- mention-only group behavior
- reply and quoted-message shaping
- continuous typing loops
- basic media intake and send
- adjacent voice and TTS support

Keep the boundary explicit: PicoClaw only donates Discord channel-edge mechanics. Gormes runtime, session rules, and kernel integration remain authoritative.

## Why This Adapter Is Reusable

PicoClaw's Discord adapter is reusable because the interesting logic is transport-specific and mostly self-contained.

- Startup is clean: fetch bot identity, register the message handler, start the voice-control listener, then open the session.
- Group-trigger behavior is already expressed as transport-edge policy through `ShouldRespondInGroup`.
- Typing is implemented as a per-channel loop with stop handles, which matches the shared adapter pattern document and is easy to recast inside Gormes.
- Reply handling and Discord-specific reference expansion are local to the adapter, not smeared through a framework.

The voice and TTS surface in `picoclaw/pkg/channels/discord/voice.go` is broader than the minimum Phase 2.B.2 requirement, but it is adjacent rather than contaminating the base text adapter.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/discord/discord.go`
- `picoclaw/pkg/channels/discord/voice.go`
- `picoclaw/pkg/channels/discord/discord_test.go`
- `picoclaw/pkg/channels/discord/discord_resolve_test.go`
- `picoclaw/docs/channels/discord/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/discord.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

Copy candidates:

- Session startup and shutdown shape from `picoclaw/pkg/channels/discord/discord.go`: get bot identity before open, register handlers early, and stop typing loops before session close.
- Group-trigger logic from `handleMessage`, especially DM-always-respond versus guild mention filtering.
- Typing loop handling from `startTyping`, `stopTyping`, and `StartTyping`. Discord requires repeated `ChannelTyping` calls, and PicoClaw already treats the stop function as an adapter concern.
- Reply behavior from `sendChunk`, including `MessageReference` use when replying to a specific inbound message.
- Reference resolution tests from `picoclaw/pkg/channels/discord/discord_resolve_test.go`; they capture channel mention expansion and same-guild-only message-link expansion.

Rebuild in Gormes-native form:

- Inbound publish path. Replace PicoClaw's `HandleInboundContext` bus call with direct Gormes kernel event submission and Gormes session-key derivation.
- Session model. Follow the upstream Hermes Discord behavior docs and Gormes phase plan, not PicoClaw's exact chat ID and metadata layout.
- Voice/TTS. Keep `voice.go` as optional follow-on scope. Do not block the base text adapter on voice parity.
- Media store plumbing. PicoClaw's attachment handling depends on its media-store abstractions; Gormes should re-express that on top of its own storage and tool surfaces.

## Gormes Mapping

- The donor's `Start` and `Stop` methods map into the current Gormes Discord adapter lifecycle: one adapter, one kernel-facing render loop, explicit shutdown.
- `handleMessage` should map to Gormes inbound policy documented in `gormes/docs/content/upstream-hermes/user-guide/messaging/discord.md`: respond to DMs, require mention in server channels by default, preserve thread isolation, and keep session identity explicit.
- `sendChunk` maps to Gormes outbound delivery, especially reply threading and per-message send timeout.
- `startTyping` and `stopTyping` map cleanly to shared adapter UX helpers from `gateway-donor-map/shared-adapter-patterns.md`.
- `voice.go` is best treated as a later `internal/channels/discord/voice.go` companion rather than part of the shipped text adapter.

## Implementation Notes

- Build the first Gormes Discord port around text and attachments only. Voice receive, streamed TTS playback, and interruption logic can be a second pass.
- Preserve PicoClaw's startup ordering. Fetching bot identity before opening the session avoids racey mention detection.
- Port the typing loop behavior exactly in spirit: immediate `ChannelTyping`, repeat on a timer, cap lifetime, and make stop idempotent.
- Carry over the same-guild restriction from `resolveDiscordRefs`; cross-guild link expansion is a privacy bug.
- Keep `sendChunk` timeout behavior. Discord send calls should not block the render loop indefinitely.
- Use the tests as specification. `discord_test.go` validates proxy handling; `discord_resolve_test.go` validates link and channel resolution rules.

## Risks / Mismatches

- PicoClaw responds through a bus-centric adapter runtime; Gormes needs a kernel-centric gateway adapter. The state machine is portable, the call graph is not.
- Upstream Hermes user docs describe per-user session isolation in shared channels and thread-aware behavior. PicoClaw's donor code captures some transport mechanics, but not that full session policy.
- Voice/TTS materially increases scope: voice state mapping, Opus receive, interruption, and playback pipelines are all real features. Treat them as non-blocking adjacencies.
- Discord libraries and intents can drift over time. The donor is structurally useful, but any future implementation still needs current library verification when Phase 2.B.2 starts.

## Port Order Recommendation

1. Keep the current text adapter as the baseline and tighten mention filtering, reply delivery, and typing-loop tests first.
2. Port the reference-resolution tests and group-trigger behavior next, because those are high-regression surfaces.
3. Add attachment intake and outbound media once the text adapter is stable.
4. Defer voice join, speech receive, and TTS playback to an explicit follow-on scope.
5. Only then consider control-plane features such as slash-command parity or richer guild metadata handling.

## Code References

- `picoclaw/pkg/channels/discord/discord.go`: `Start`, `Stop`, `Send`, `SendMedia`, `sendChunk`, `handleMessage`, `startTyping`, `stopTyping`, `StartTyping`, `resolveDiscordRefs`, `stripBotMention`, `applyDiscordProxy`.
- `picoclaw/pkg/channels/discord/voice.go`: `handleVoiceCommand`, `receiveVoice`, `listenVoiceControl`, `playTTS`, `streamOggOpusToDiscord`.
- `picoclaw/pkg/channels/discord/discord_test.go`
- `picoclaw/pkg/channels/discord/discord_resolve_test.go`
- `picoclaw/docs/channels/discord/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/discord.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `copy candidate`.
