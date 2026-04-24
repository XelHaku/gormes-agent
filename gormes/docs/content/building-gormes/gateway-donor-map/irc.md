---
title: "IRC"
weight: 60
---

# IRC

IRC is the clearest example in this donor set of a low-priority adapter with some real protocol-specific donor value, but not enough to justify first-class Gormes investment unless there is a specific operator demand.

## Status

`gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` does not list IRC as a planned Gormes gateway platform. This dossier exists only because PicoClaw ships an IRC edge and future contributors may wonder whether it is a useful donor.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- The IRC recommendation here is based on donor code plus current Gormes in-tree planning docs, not on an existing Gormes IRC implementation.

Keep the boundary explicit: PicoClaw provides an IRC edge example only. Gormes architecture remains authoritative, and IRC should not distort gateway priorities just because a donor exists.

## Why This Adapter Is Reusable

The reusable part of PicoClaw's IRC adapter is narrow:

- connection setup with TLS, SASL, NickServ, and channel joins
- simple `PRIVMSG` ingest
- line-oriented outbound send
- IRCv3 `TAGMSG` typing indicator support
- bot-mention parsing through nick-prefix conventions

This is useful as protocol reconnaissance and as a small donor for IRC-specific edge mechanics, but it is not a deep donor. The implementation is intentionally small, does not cover media, does not handle richer channel metadata, and relies on IRC social conventions rather than strong platform primitives.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/irc/irc.go`
- `picoclaw/pkg/channels/irc/handler.go`
- `picoclaw/pkg/channels/irc/irc_test.go`
- `picoclaw/docs/guides/chat-apps.md`

## What To Copy vs What To Rebuild

Copy candidates:

- IRC connection bootstrap around nick, user, real name, TLS, SASL, and requested caps
- `onConnect` join logic and NickServ fallback
- `nickMentionedAt`, `isBotMentioned`, and `stripBotMention` for IRC-specific prefix detection
- the basic distinction between direct messages and channel messages in `onPrivmsg`
- line-by-line outbound send and optional IRCv3 typing via `TAGMSG`

Rebuild in Gormes-native form:

- session identity. PicoClaw uses sender nick for DMs and channel name for rooms; Gormes would need to decide how to cope with nick changes, bouncers, and multi-network deployments.
- reliability expectations. There is no webhook or durable platform API here; reconnect, netsplit, backlog, and identity issues would need explicit design.
- command UX. IRC commands are mostly conversational conventions, not stable app-level affordances like slash commands or structured mentions.

## Gormes Mapping

- `Start` and `onConnect` could seed a future low-priority `internal/irc` adapter if Gormes ever wanted one.
- `onPrivmsg` is the main donor for inbound parsing: map DM vs channel, allow-list sender, check mention or prefix policy, then publish a normalized inbound event.
- `Send` maps to the simplest possible outbound transport, but the 400-character `MaxMessageLength` and line-splitting behavior should stay adapter-local.
- `StartTyping` is a nice optional capability if the target IRC network supports `message-tags`, but it should remain best-effort and non-essential.

## Implementation Notes

- Treat IRC as an edge case and a fallback donor, not as a reference adapter for more modern transports.
- If a future porter truly needs IRC, start by preserving the mention-detection tests. That is the main protocol-specific behavior with reusable value.
- Do not let IRC's weak identity model leak into shared Gormes session assumptions.
- The donor code is good enough for a proof-of-concept adapter if Gormes ever needs IRC, but not strong enough to justify early roadmap priority by itself.

## Risks / Mismatches

- IRC identity is nick-based and therefore much less stable than Telegram IDs, Slack user IDs, or Matrix MXIDs.
- Channel behavior depends heavily on server capabilities and network policy. TLS, SASL, `message-tags`, and reconnect behavior are not uniform.
- There is no meaningful media or rich formatting story in this donor.
- The business value is likely low compared with planned Gormes adapters already listed in the subsystem inventory.

## Port Order Recommendation

1. Do not treat IRC as a normal Phase 2 gateway target.
2. If there is a real operator need, port only the minimal DM and channel text path first.
3. Add mention parsing and allow-list behavior next.
4. Add typing support only if the target IRC network proves it supports the required caps.

## Code References

- `picoclaw/pkg/channels/irc/irc.go`: `NewIRCChannel`, `Start`, `Stop`, `Send`, `StartTyping`, `extractHost`.
- `picoclaw/pkg/channels/irc/handler.go`: `onConnect`, `onPrivmsg`, `nickMentionedAt`, `isBotMentioned`, `stripBotMention`.
- `picoclaw/pkg/channels/irc/irc_test.go`
- Generic context only: `picoclaw/docs/guides/chat-apps.md`

Recommendation: `adapt pattern only`.
