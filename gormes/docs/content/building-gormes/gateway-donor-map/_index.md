---
title: "Gateway Donor Map"
weight: 45
---

# Gateway Donor Map

PicoClaw is a donor repo for Go channel-edge work, not the source of truth for Gormes architecture. Gormes architecture remains authoritative, and Hermes stays the legacy reference for the older product direction and runtime model.

## Provenance

- Donor inspected for this section: external sibling repo `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- Donor commit pinned for this research: `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- Upstream donor repo: `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed on this page or in the linked dossiers is relative to that donor root, not relative to the Gormes repo.

## What This Section Is For

- Turn PicoClaw's Go messaging adapters into Gormes porting notes
- Separate transport-edge reuse from PicoClaw-specific runtime coupling
- Give every planned adapter a hard recommendation: `copy candidate`, `adapt pattern only`, or `not worth reusing`

## How To Use This Section

1. Read [Shared Adapter Patterns](./shared-adapter-patterns/) first.
2. Open the relevant channel dossier.
3. Use the donor files and Gormes mapping notes to draft the implementation spec or PR.

## Triage View

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
