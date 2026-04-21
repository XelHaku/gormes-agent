---
title: "Gateway Donor Map"
weight: 45
---

# Gateway Donor Map

PicoClaw is a donor repo for Go channel-edge work, not the source of truth for Gormes architecture. Hermes/Gormes still owns the kernel, session model, and the Operative System AI product direction.

## What This Section Is For

- Turn PicoClaw's Go messaging adapters into Gormes porting notes
- Separate transport-edge reuse from PicoClaw-specific runtime coupling
- Give every planned adapter a hard recommendation: `copy candidate`, `adapt pattern only`, or `not worth reusing`

## How To Use This Section

1. Read [Shared Adapter Patterns](./shared-adapter-patterns/) first.
2. Open the relevant channel dossier.
3. Use the donor files and Gormes mapping notes to draft the implementation spec or PR.

## Channel Dossiers

| Channel | Dossier |
|---|---|
| Telegram | [Telegram](./telegram/) |
| Discord | [Discord](./discord/) |
| Slack | [Slack](./slack/) |
| WhatsApp | [WhatsApp](./whatsapp/) |
| Matrix | [Matrix](./matrix/) |
| IRC | [IRC](./irc/) |
| LINE | [LINE](./line/) |
| OneBot | [OneBot](./onebot/) |
| QQ | [QQ](./qq/) |
| WeCom | [WeCom](./wecom/) |
| WeiXin | [WeiXin](./weixin/) |
| Feishu | [Feishu](./feishu/) |
| DingTalk | [DingTalk](./dingtalk/) |
| VK | [VK](./vk/) |
| Webhook | [Webhook](./webhook/) |
