---
title: "Shared Adapter Patterns"
weight: 10
---

# Shared Adapter Patterns

This page captures the PicoClaw mechanics that are reusable across more than one channel adapter. Copy transport-edge ideas from here; do not import PicoClaw's product architecture.

## Provenance

- Donor inspected for this page: external sibling repo `<picoclaw donor repo>`.
- Donor commit pinned for this research: `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- Upstream donor repo: `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.

## Donor Files

- `picoclaw/pkg/channels/base.go`
- `picoclaw/pkg/channels/interfaces.go`
- `picoclaw/pkg/channels/manager.go`
- `picoclaw/pkg/channels/manager_channel.go`
- `picoclaw/pkg/channels/split.go`
- `picoclaw/pkg/channels/marker.go`
- `picoclaw/pkg/channels/dynamic_mux.go`
- `picoclaw/pkg/channels/media.go`
- `picoclaw/pkg/channels/registry.go`
- `picoclaw/pkg/channels/voice_capabilities.go`
- `picoclaw/pkg/channels/webhook.go`

If you are porting a new adapter, start with `picoclaw/pkg/channels/base.go`, `picoclaw/pkg/channels/interfaces.go`, `picoclaw/pkg/channels/manager.go`, and `picoclaw/pkg/channels/split.go` before drilling into channel-specific donor files.

## The Reusable Shape

`BaseChannel` is the cleanest donor abstraction in PicoClaw. It centralizes allow-list checks, group-trigger policy, reasoning-channel IDs, and max-message-length declarations without forcing one platform SDK on every channel. That shape maps cleanly to Gormes: keep per-channel transport code thin and push shared policy down into reusable gateway helpers.

## Message Splitting And Outbound Limits

`pkg/channels/split.go` is worth borrowing conceptually. The important idea is not the exact function signature; it is the policy:

- enforce per-channel rune limits up front;
- prefer newline or whitespace split points;
- preserve fenced code blocks instead of corrupting them mid-stream;
- leave enough headroom for markup expansion on channels like Telegram.

That belongs in shared Gormes gateway tooling, not reimplemented independently per adapter.

## Typing, Placeholder, And Reaction Mechanics

`Manager.preSend` in `pkg/channels/manager.go` is the most reusable UX pattern in the donor repo. It treats outbound delivery as a staged cleanup pipeline:

- stop typing indicators;
- undo temporary reactions;
- delete or edit placeholders when the real response is ready;
- fall back to a normal send when edit-in-place is not supported.

Gormes should keep this sequence, but house it inside Hermes-style session and kernel boundaries instead of importing PicoClaw's manager as-is.

## Manager / Worker / Rate-Limit Patterns

PicoClaw's manager keeps one worker queue per channel and attaches per-channel rate limiters (`telegram`, `discord`, `slack`, `matrix`, `line`, `qq`, `irc`). That is useful as evidence that rate policy belongs in shared gateway orchestration rather than hidden inside every adapter.

The transferable part is:

- central rate shaping;
- channel-scoped work queues;
- typed capability checks such as editor/deleter/media sender support.

The non-transferable part is the full PicoClaw manager/bus ownership model. Gormes already has its own kernel and session boundaries, so reuse the pattern, not the runtime.

## Webhook And Dynamic-Mux Patterns

`pkg/channels/dynamic_mux.go` and `pkg/channels/webhook.go` are valuable for webhook-family channels such as LINE, WeCom, WeiXin, and Feishu. The key lesson is that webhook ingress needs a shared registration and routing layer so each provider does not reinvent HTTP path dispatch, signature verification hooks, or listener startup.

That does not mean every Gormes adapter should route through a universal webhook abstraction. Long-poll, socket-mode, and DingTalk Stream Mode channels still deserve simpler dedicated loops.

## What Gormes Should Not Import Blindly

Do not pull these pieces over wholesale:

- PicoClaw's bus ownership and manager lifecycle contracts
- assumptions that placeholders, reactions, or streaming all terminate through the same manager path
- bridge- or provider-specific config layout that does not match Gormes config boundaries
- product-level runtime decisions that belong to Hermes/Gormes rather than the channel edge

## Code References

- `picoclaw/pkg/channels/base.go`
- `picoclaw/pkg/channels/manager.go`
- `picoclaw/pkg/channels/split.go`
- `picoclaw/pkg/channels/dynamic_mux.go`
- `picoclaw/pkg/channels/webhook.go`
- `picoclaw/pkg/channels/media.go`
- `picoclaw/pkg/channels/voice_capabilities.go`

## Landed Gormes Mappings

- PicoClaw Discord startup, mention-gate, and typing references:
  `picoclaw/pkg/channels/discord/discord.go`
  → `gormes/internal/channels/discord/real_client.go`
  → `gormes/internal/channels/discord/bot.go`
  → `gormes/cmd/gormes/gateway.go`

- PicoClaw Slack Socket Mode, ACK, and thread target references (partially landed, not yet on the shared gateway entrypoint):
  `picoclaw/pkg/channels/slack/slack.go`
  → `gormes/internal/slack/real_client.go`
  → `gormes/internal/slack/bot.go`

- Gormes-only shared runtime extraction:
  `gormes/cmd/gormes/gateway.go`
  → `gormes/internal/gateway/manager.go`
  → `gormes/internal/channels/telegram/bot.go`

- Gormes-only threaded text contract freeze ahead of Matrix/Mattermost:
  `gormes/internal/slack/bot.go`
  → `gormes/internal/gateway/event.go`
  → `gormes/internal/gateway/delivery.go`
  → `gormes/internal/channels/threadtext/contract.go`

- PicoClaw DingTalk Stream Mode and session-webhook reply references (bot seam and runtime contract landed; real SDK binding still planned):
  `picoclaw/pkg/channels/dingtalk/dingtalk.go`
  → `gormes/internal/channels/dingtalk/bot.go`
  → `gormes/internal/channels/dingtalk/runtime.go`
