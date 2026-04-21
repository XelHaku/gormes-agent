---
title: "Gateway Donor Map"
weight: 45
---

# Gateway Donor Map

PicoClaw is useful to Gormes as a donor repo for Go channel-edge work, not as the architecture to copy wholesale. Hermes/Gormes still owns the kernel, session model, agent behavior, and the Operative System AI product direction.

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

| Channel | Recommendation | Why It Matters | First Donor Files |
|---|---|---|---|
| Telegram | adapt pattern only | Gormes already ships Telegram, but PicoClaw still has useful markdown, command-registration, and group-trigger handling details | `pkg/channels/telegram/telegram.go`, `pkg/channels/telegram/command_registration.go`, `pkg/channels/telegram/parse_markdown_to_md_v2.go` |
| Discord | copy candidate | Clean Go adapter with startup, mention handling, typing loops, and a bounded voice sidecar | `pkg/channels/discord/discord.go`, `pkg/channels/discord/voice.go` |
| Slack | copy candidate | Socket Mode lifecycle, thread timestamp handling, reaction ACK flow, and file-upload shape are already solved in Go | `pkg/channels/slack/slack.go` |
| WhatsApp | adapt pattern only | The bridge path is useful, but it is shaped around PicoClaw's bridge contract rather than a Gormes-native session model | `pkg/channels/whatsapp/whatsapp.go`, `pkg/channels/whatsapp_native/whatsapp_native.go` |
| Matrix | adapt pattern only | Good session/auth/send reference, but likely needs a cleaner Gormes wrapper than a straight lift | `pkg/channels/matrix/matrix.go` |
| LINE / WeCom / WeiXin / Feishu / DingTalk | adapt pattern only | Webhook-family channels are valuable mainly because PicoClaw already solved HTTP ingress and provider-specific request plumbing | `pkg/channels/line/line.go`, `pkg/channels/wecom/wecom.go`, `pkg/channels/weixin/weixin.go`, `pkg/channels/feishu/feishu_64.go`, `pkg/channels/dingtalk/dingtalk.go` |
| QQ / OneBot | adapt pattern only | Useful for identity mapping and protocol edges, but they carry more provider-specific assumptions | `pkg/channels/qq/qq.go`, `pkg/channels/onebot/onebot.go` |
| IRC / VK / Webhook | not worth reusing directly | These are still informative for edge behavior, but they are lower-priority ports for the current Gormes roadmap | `pkg/channels/irc/irc.go`, `pkg/channels/vk/vk.go`, `pkg/channels/webhook.go` |

## Current Gormes Anchors

- [Using Gormes: Telegram Adapter](../../using-gormes/telegram-adapter/) for the shipped baseline
- [Gateway](../core-systems/gateway/) for the architecture Gormes keeps authoritative
- [Phase 2 Gateway](../architecture_plan/phase-2-gateway/) for the roadmap order and shipped/planned status
- [Subsystem Inventory](../architecture_plan/subsystem-inventory/) for the current port ledger

## Planned Channel Dossiers

The next pass should split this donor map into dedicated per-channel dossiers for Telegram, Discord, Slack, WhatsApp, Matrix, IRC, LINE, OneBot, QQ, WeCom, WeiXin, Feishu, DingTalk, VK, and webhook-family adapters.
