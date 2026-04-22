---
title: "Gateway Donor Map"
weight: 45
---

# Gateway Donor Map

PicoClaw is useful to Gormes as a donor repo for Go channel-edge work, not as the architecture to copy wholesale. Hermes/Gormes still owns the kernel, session model, agent behavior, and the Operative System AI product direction.

## Provenance

- Donor inspected for this section: external sibling repo `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- Donor commit pinned for this research: `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- Upstream donor repo: `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed on this page is relative to that donor root, not relative to the Gormes repo.

## What This Section Is For

- Turn PicoClaw's finished Go adapters into concrete porting notes for Gormes Phase 2 channel work
- Separate transport-edge reuse from PicoClaw-specific runtime coupling
- Point contributors at the files worth mining first before they open a spec or `internal/<channel>/` port PR

## How To Use This Section

1. Read [Shared Adapter Patterns](./shared-adapter-patterns/) first.
2. Cross-check the channel with the current [gateway core-system doc](../core-systems/gateway/) and the [Phase 2 gateway ledger](../architecture_plan/phase-2-gateway/).
3. Lift transport-edge ideas only. Rebuild them inside Gormes's kernel/session boundaries.

## Verified Shared Donor Files

These PicoClaw files are the highest-leverage shared references across more than one adapter:

- `picoclaw/pkg/channels/base.go` for allow-lists, group-trigger handling, max-message-length policy, and reasoning-channel plumbing
- `picoclaw/pkg/channels/manager.go` for placeholder-edit, typing-stop, reaction-undo, and per-channel rate-limit orchestration
- `picoclaw/pkg/channels/split.go` for outbound chunking that preserves fenced code blocks
- `picoclaw/pkg/channels/dynamic_mux.go` and `picoclaw/pkg/channels/webhook.go` for webhook-family fan-in patterns
- `picoclaw/pkg/channels/media.go` and `picoclaw/pkg/channels/voice_capabilities.go` for optional capability boundaries

## Channel Triage

| Channel | Recommendation | Donor Surface | Dossier |
|---|---|---|---|
| Telegram | `adapt pattern only` | `pkg/channels/telegram/` | [Telegram](./telegram/) |
| Discord | `copy candidate` | `pkg/channels/discord/` | [Discord](./discord/) |
| Slack | `copy candidate` | `pkg/channels/slack/` | [Slack](./slack/) |
| WhatsApp | `adapt pattern only` | `pkg/channels/whatsapp/`, `pkg/channels/whatsapp_native/` | [WhatsApp](./whatsapp/) |
| Matrix | `adapt pattern only` | `pkg/channels/matrix/` | [Matrix](./matrix/) |
| IRC | `adapt pattern only` | `pkg/channels/irc/` | [IRC](./irc/) |
| LINE | `adapt pattern only` | `pkg/channels/line/` | [LINE](./line/) |
| OneBot | `adapt pattern only` | `pkg/channels/onebot/` | [OneBot](./onebot/) |
| QQ | `copy candidate` | `pkg/channels/qq/` | [QQ](./qq/) |
| WeCom | `copy candidate` | `pkg/channels/wecom/` | [WeCom](./wecom/) |
| WeiXin | `adapt pattern only` | `pkg/channels/weixin/` | [WeiXin](./weixin/) |
| Feishu | `adapt pattern only` | `pkg/channels/feishu/` | [Feishu](./feishu/) |
| DingTalk | `adapt pattern only` | `pkg/channels/dingtalk/` | [DingTalk](./dingtalk/) |
| VK | `not worth reusing` | `pkg/channels/vk/` | [VK](./vk/) |
| Webhook | `adapt pattern only` | `pkg/channels/webhook.go` | [Webhook](./webhook/) |

## Current Gormes Anchors

- [Using Gormes: Telegram Adapter](../../using-gormes/telegram-adapter/) for the shipped baseline
- [Gateway](../core-systems/gateway/) for the architecture Gormes keeps authoritative
- [Phase 2 Gateway](../architecture_plan/phase-2-gateway/) for the roadmap order and shipped/planned status
- [Subsystem Inventory](../architecture_plan/subsystem-inventory/) for the current port ledger

## Planned Channel Dossiers

The next pass should split this donor map into dedicated per-channel dossiers for Telegram, Discord, Slack, WhatsApp, Matrix, IRC, LINE, OneBot, QQ, WeCom, WeiXin, Feishu, DingTalk, VK, and webhook-family adapters.
