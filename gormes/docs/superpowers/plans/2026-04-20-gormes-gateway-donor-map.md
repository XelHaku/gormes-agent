# Gateway Donor Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a new `building-gormes/gateway-donor-map/` documentation section that turns PicoClaw's Go channel adapters into prescriptive donor dossiers for future Gormes ports.

**Architecture:** Build one Hugo section with a hub page, one shared-patterns page, and one dossier per channel. Every dossier uses the same fixed template, keeps Hermes/Gormes architecture authoritative, and ends in an explicit reuse recommendation. Surface the new section from existing gateway docs so contributors can find it from the roadmap.

**Tech Stack:** Hugo markdown under `gormes/docs/content`, existing docs navigation patterns, donor references from `<picoclaw donor repo>`, verification via `cd gormes && go test ./docs -run TestHugoBuild -count=1`.

---

## Prerequisites

- Approved spec: `gormes/docs/superpowers/specs/2026-04-20-gormes-gateway-donor-map-design.md`
- Donor repo available locally at `<picoclaw donor repo>`
- Gormes repo root: `<repo>`
- Hugo smoke test already exists in `gormes/docs/build_test.go`

## File Structure Map

```text
gormes/docs/content/building-gormes/
├── _index.md                                        # MODIFY — link the new donor-map section from the Building Gormes landing page
├── core-systems/gateway.md                          # MODIFY — add a See also link to the donor-map section
├── architecture_plan/phase-2-gateway.md            # MODIFY — add a roadmap-side link to the donor-map section
└── gateway-donor-map/
    ├── _index.md                                   # NEW — hub page and triage table
    ├── shared-adapter-patterns.md                  # NEW — cross-channel reusable mechanics
    ├── telegram.md                                 # NEW — shipped baseline, donor notes only
    ├── discord.md                                  # NEW
    ├── slack.md                                    # NEW
    ├── whatsapp.md                                 # NEW
    ├── matrix.md                                   # NEW
    ├── irc.md                                      # NEW
    ├── line.md                                     # NEW
    ├── onebot.md                                   # NEW
    ├── qq.md                                       # NEW
    ├── wecom.md                                    # NEW
    ├── weixin.md                                   # NEW
    ├── feishu.md                                   # NEW
    ├── dingtalk.md                                 # NEW
    ├── vk.md                                       # NEW
    └── webhook.md                                  # NEW
```

## Locked Dossier Template

Every channel page must use this exact section order:

```md
## Status
## Why This Adapter Is Reusable
## Picoclaw Donor Files
## What To Copy vs What To Rebuild
## Gormes Mapping
## Implementation Notes
## Risks / Mismatches
## Port Order Recommendation
## Code References
```

Use repo-relative donor paths like ``picoclaw/pkg/channels/discord/discord.go`` inside the prose and bullet lists.

---

### Task 1: Scaffold The Donor-Map Section And Surface It In Existing Docs

**Files:**
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/_index.md`
- Modify: `gormes/docs/content/building-gormes/_index.md`
- Modify: `gormes/docs/content/building-gormes/core-systems/gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`

- [ ] **Step 1: Create `gateway-donor-map/_index.md` with the hub-page scaffold**

Write this exact starting structure, then fill the prose with the approved boundary from the spec:

```md
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
```

- [ ] **Step 2: Surface the new section from `building-gormes/_index.md`**

Insert this bullet under `## Contents` after `Porting a Subsystem`:

```md
- [Gateway Donor Map](./gateway-donor-map/) — prescriptive PicoClaw-to-Gormes channel reuse dossiers
```

- [ ] **Step 3: Add a See-also link to `core-systems/gateway.md`**

Append this sentence after the existing closing paragraph:

```md
For donor-code reconnaissance against PicoClaw's Go adapters, see [Gateway Donor Map](../gateway-donor-map/).
```

- [ ] **Step 4: Add a roadmap-side link to `architecture_plan/phase-2-gateway.md`**

Append this note after the Phase 2 ledger table:

```md
For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).
```

- [ ] **Step 5: Run the docs build smoke test**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok` and no broken internal-link failures.

- [ ] **Step 6: Commit the scaffold**

Run:

```bash
git add gormes/docs/content/building-gormes/_index.md \
  gormes/docs/content/building-gormes/core-systems/gateway.md \
  gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md \
  gormes/docs/content/building-gormes/gateway-donor-map/_index.md
git commit -m "docs(gormes): scaffold gateway donor map section"
```

---

### Task 2: Write `shared-adapter-patterns.md`

**Files:**
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md`

Primary donor files to cite in this page:

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

- [ ] **Step 1: Create the page with the approved sections**

Start from this exact scaffold:

```md
---
title: "Shared Adapter Patterns"
weight: 10
---

# Shared Adapter Patterns

This page captures the PicoClaw mechanics that are reusable across more than one channel adapter. Copy transport-edge ideas from here; do not import PicoClaw's product architecture.

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

## The Reusable Shape

## Message Splitting And Outbound Limits

## Typing, Placeholder, And Reaction Mechanics

## Manager / Worker / Rate-Limit Patterns

## Webhook And Dynamic-Mux Patterns

## What Gormes Should Not Import Blindly

## Code References
```

- [ ] **Step 2: Fill the page with concrete conclusions**

Make sure the finished prose explicitly covers:

- `BaseChannel` options that generalize cleanly to Gormes
- the `Manager.preSend` / placeholder-edit flow as a reusable pattern
- `split.go` as the source for per-channel outbound length policy
- `dynamic_mux.go` as a webhook-family pattern, not a universal gateway model
- PicoClaw coupling that should stay out of Gormes, especially manager/bus assumptions

- [ ] **Step 3: Verify the page builds**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok`

- [ ] **Step 4: Commit the shared-patterns page**

Run:

```bash
git add gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md
git commit -m "docs(gormes): add shared gateway donor patterns"
```

---

### Task 3: Write The Core Chat Dossiers (`telegram`, `discord`, `slack`, `whatsapp`)

**Files:**
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/telegram.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/discord.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/slack.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/whatsapp.md`

- [ ] **Step 1: Write `telegram.md` as the shipped-baseline dossier**

Use this donor set:

- `picoclaw/pkg/channels/telegram/telegram.go`
- `picoclaw/pkg/channels/telegram/command_registration.go`
- `picoclaw/pkg/channels/telegram/parse_markdown_to_md_v2.go`
- `picoclaw/pkg/channels/telegram/parser_markdown_to_html.go`
- `picoclaw/pkg/channels/telegram/telegram_dispatch_test.go`
- `picoclaw/pkg/channels/telegram/telegram_group_command_filter_test.go`
- `picoclaw/docs/channels/telegram/README.md`
- `gormes/internal/telegram/bot.go`
- `gormes/internal/telegram/render.go`
- `gormes/docs/content/using-gormes/telegram-adapter.md`

The page must say that Gormes already shipped Telegram, so the recommendation is about **delta reuse** rather than greenfield porting. Focus on markdown rendering, command registration, group-trigger behavior, and anything PicoClaw handles that Gormes does not yet.

- [ ] **Step 2: Write `discord.md`**

Use this donor set:

- `picoclaw/pkg/channels/discord/discord.go`
- `picoclaw/pkg/channels/discord/voice.go`
- `picoclaw/pkg/channels/discord/discord_test.go`
- `picoclaw/pkg/channels/discord/discord_resolve_test.go`
- `picoclaw/docs/channels/discord/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/discord.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

The page must cover Discord session startup, mention/group-trigger behavior, typing loop handling, voice/TTS as adjacent but non-blocking scope, and how much of the donor surface is cleanly portable into a future `internal/discord/`.

- [ ] **Step 3: Write `slack.md`**

Use this donor set:

- `picoclaw/pkg/channels/slack/slack.go`
- `picoclaw/pkg/channels/slack/slack_test.go`
- `picoclaw/docs/channels/slack/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/slack.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

The page must cover Socket Mode startup, `pendingAcks`, thread timestamp handling, media upload shape, and whether PicoClaw's `socketmode` choice should become Gormes's default path.

- [ ] **Step 4: Write `whatsapp.md`**

Use this donor set:

- `picoclaw/pkg/channels/whatsapp/whatsapp.go`
- `picoclaw/pkg/channels/whatsapp/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native_stub.go`
- `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/whatsapp.md`

The page must compare PicoClaw's bridge-based path with its optional `whatsapp_native` build-tag path. Call out which pieces are reusable for Gormes and which ones are bridge- or build-tag-specific.

- [ ] **Step 5: Verify this batch builds**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok`

- [ ] **Step 6: Commit the core-chat batch**

Run:

```bash
git add gormes/docs/content/building-gormes/gateway-donor-map/telegram.md \
  gormes/docs/content/building-gormes/gateway-donor-map/discord.md \
  gormes/docs/content/building-gormes/gateway-donor-map/slack.md \
  gormes/docs/content/building-gormes/gateway-donor-map/whatsapp.md
git commit -m "docs(gormes): add core gateway donor dossiers"
```

---

### Task 4: Write The Matrix / Community / Webhook Dossiers

**Files:**
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/matrix.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/irc.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/line.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/onebot.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/qq.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/webhook.md`

- [ ] **Step 1: Write `matrix.md`**

Use this donor set:

- `picoclaw/pkg/channels/matrix/matrix.go`
- `picoclaw/pkg/channels/matrix/matrix_test.go`
- `picoclaw/docs/channels/matrix/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/matrix.md`

Focus on how much of the Matrix session/auth/send flow is generic enough for a future Gormes adapter, and whether PicoClaw's implementation looks like a direct donor or mainly a shape reference.

- [ ] **Step 2: Write `irc.md`**

Use this donor set:

- `picoclaw/pkg/channels/irc/irc.go`
- `picoclaw/pkg/channels/irc/handler.go`
- `picoclaw/pkg/channels/irc/irc_test.go`
- `picoclaw/docs/guides/chat-apps.md`

Focus on command parsing, IRC-specific event handling, and whether the channel is worth porting directly versus documenting as a lower-priority edge path.

- [ ] **Step 3: Write `line.md`**

Use this donor set:

- `picoclaw/pkg/channels/line/line.go`
- `picoclaw/pkg/channels/line/line_test.go`
- `picoclaw/docs/channels/line/README.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

The page must call out that LINE is a webhook-first channel and explain how PicoClaw's HTTP handling should map, or not map, into Gormes's gateway shape.

- [ ] **Step 4: Write `onebot.md`**

Use this donor set:

- `picoclaw/pkg/channels/onebot/onebot.go`
- `picoclaw/docs/channels/onebot/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/qqbot.md`

Focus on bridge/protocol assumptions and what a Gormes porter should steal from PicoClaw versus what should stay behind.

- [ ] **Step 5: Write `qq.md`**

Use this donor set:

- `picoclaw/pkg/channels/qq/qq.go`
- `picoclaw/pkg/channels/qq/audio_duration.go`
- `picoclaw/pkg/channels/qq/botgo_logger.go`
- `picoclaw/pkg/channels/qq/qq_test.go`
- `picoclaw/docs/channels/qq/README.md`

Focus on official bot API flow, sender/channel identity handling, audio/media side paths, and how much of the SDK choice and runtime shape look reusable.

- [ ] **Step 6: Write `webhook.md`**

Use this donor set:

- `picoclaw/pkg/channels/webhook.go`
- `picoclaw/pkg/channels/dynamic_mux.go`
- `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/webhooks.md`

This page must explain that PicoClaw's generic webhook path is useful as an ingress pattern, but not as a replacement for Gormes's gateway architecture.

- [ ] **Step 7: Verify this batch builds**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok`

- [ ] **Step 8: Commit the community/webhook batch**

Run:

```bash
git add gormes/docs/content/building-gormes/gateway-donor-map/matrix.md \
  gormes/docs/content/building-gormes/gateway-donor-map/irc.md \
  gormes/docs/content/building-gormes/gateway-donor-map/line.md \
  gormes/docs/content/building-gormes/gateway-donor-map/onebot.md \
  gormes/docs/content/building-gormes/gateway-donor-map/qq.md \
  gormes/docs/content/building-gormes/gateway-donor-map/webhook.md
git commit -m "docs(gormes): add secondary gateway donor dossiers"
```

---

### Task 5: Write The Enterprise / China-Facing Dossiers

**Files:**
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/wecom.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/weixin.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/feishu.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/dingtalk.md`
- Create: `gormes/docs/content/building-gormes/gateway-donor-map/vk.md`

- [ ] **Step 1: Write `wecom.md`**

Use this donor set:

- `picoclaw/pkg/channels/wecom/wecom.go`
- `picoclaw/pkg/channels/wecom/protocol.go`
- `picoclaw/pkg/channels/wecom/media.go`
- `picoclaw/pkg/channels/wecom/reqid_store.go`
- `picoclaw/pkg/channels/wecom/wecom_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/wecom.go`
- `picoclaw/web/backend/api/wecom.go`
- `picoclaw/docs/channels/wecom/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/wecom.md`

The page must separate transport protocol handling from PicoClaw-specific auth/bootstrap glue and explain whether WeCom is mostly donor code or mostly donor ideas.

- [ ] **Step 2: Write `weixin.md`**

Use this donor set:

- `picoclaw/pkg/channels/weixin/weixin.go`
- `picoclaw/pkg/channels/weixin/api.go`
- `picoclaw/pkg/channels/weixin/auth.go`
- `picoclaw/pkg/channels/weixin/media.go`
- `picoclaw/pkg/channels/weixin/state.go`
- `picoclaw/pkg/channels/weixin/types.go`
- `picoclaw/pkg/channels/weixin/weixin_test.go`
- `picoclaw/cmd/picoclaw/internal/auth/weixin.go`
- `picoclaw/web/backend/api/weixin.go`
- `picoclaw/docs/channels/weixin/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/weixin.md`

The page must cover Tencent iLink auth, native QR-login flow, media handling, and how much of the donor code is channel-specific enough that Gormes should adapt patterns rather than copy code.

- [ ] **Step 3: Write `feishu.md`**

Use this donor set:

- `picoclaw/pkg/channels/feishu/common.go`
- `picoclaw/pkg/channels/feishu/feishu_32.go`
- `picoclaw/pkg/channels/feishu/feishu_64.go`
- `picoclaw/pkg/channels/feishu/feishu_reply.go`
- `picoclaw/pkg/channels/feishu/token_cache.go`
- `picoclaw/pkg/channels/feishu/feishu_reply_test.go`
- `picoclaw/docs/channels/feishu/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/feishu.md`

Focus on the split between shared helpers and protocol/version-specific files, and make the dossier useful to someone deciding whether Feishu is a reasonable Phase 2 adapter target.

- [ ] **Step 4: Write `dingtalk.md`**

Use this donor set:

- `picoclaw/pkg/channels/dingtalk/dingtalk.go`
- `picoclaw/pkg/channels/dingtalk/dingtalk_test.go`
- `picoclaw/docs/channels/dingtalk/README.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/dingtalk.md`

Focus on stream-mode startup, why it matters that the channel does not require the shared webhook server for inbound delivery, and whether the donor code looks close enough to copy.

- [ ] **Step 5: Write `vk.md`**

Use this donor set:

- `picoclaw/pkg/channels/vk/vk.go`
- `picoclaw/pkg/channels/vk/vk_test.go`
- `picoclaw/docs/channels/vk/README.md`

This page must be blunt about roadmap fit. If the donor is technically fine but strategically low-value for Gormes, say so.

- [ ] **Step 6: Verify this batch builds**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok`

- [ ] **Step 7: Commit the enterprise/China batch**

Run:

```bash
git add gormes/docs/content/building-gormes/gateway-donor-map/wecom.md \
  gormes/docs/content/building-gormes/gateway-donor-map/weixin.md \
  gormes/docs/content/building-gormes/gateway-donor-map/feishu.md \
  gormes/docs/content/building-gormes/gateway-donor-map/dingtalk.md \
  gormes/docs/content/building-gormes/gateway-donor-map/vk.md
git commit -m "docs(gormes): add enterprise gateway donor dossiers"
```

---

### Task 6: Final Triage Table, Consistency Sweep, And Verification

**Files:**
- Modify: `gormes/docs/content/building-gormes/gateway-donor-map/_index.md`
- Modify: every dossier under `gormes/docs/content/building-gormes/gateway-donor-map/` if consistency fixes are needed

- [ ] **Step 1: Add the final triage table to `_index.md`**

Replace the simple link table with a richer table in this format, using the recommendation chosen in each finished dossier:

```md
## Triage View

| Channel | Recommendation | Donor Surface | Dossier |
|---|---|---|---|
| Telegram | `adapt pattern only` | `pkg/channels/telegram/` | [Telegram](./telegram/) |
| Discord | `copy candidate` or `adapt pattern only` as justified in the dossier | `pkg/channels/discord/` | [Discord](./discord/) |
| Slack | `copy candidate` or `adapt pattern only` as justified in the dossier | `pkg/channels/slack/` | [Slack](./slack/) |
| WhatsApp | `adapt pattern only` or `not worth reusing` as justified in the dossier | `pkg/channels/whatsapp/`, `pkg/channels/whatsapp_native/` | [WhatsApp](./whatsapp/) |
| Matrix | recommendation from the dossier | `pkg/channels/matrix/` | [Matrix](./matrix/) |
| IRC | recommendation from the dossier | `pkg/channels/irc/` | [IRC](./irc/) |
| LINE | recommendation from the dossier | `pkg/channels/line/` | [LINE](./line/) |
| OneBot | recommendation from the dossier | `pkg/channels/onebot/` | [OneBot](./onebot/) |
| QQ | recommendation from the dossier | `pkg/channels/qq/` | [QQ](./qq/) |
| WeCom | recommendation from the dossier | `pkg/channels/wecom/` | [WeCom](./wecom/) |
| WeiXin | recommendation from the dossier | `pkg/channels/weixin/` | [WeiXin](./weixin/) |
| Feishu | recommendation from the dossier | `pkg/channels/feishu/` | [Feishu](./feishu/) |
| DingTalk | recommendation from the dossier | `pkg/channels/dingtalk/` | [DingTalk](./dingtalk/) |
| VK | recommendation from the dossier | `pkg/channels/vk/` | [VK](./vk/) |
| Webhook | recommendation from the dossier | `pkg/channels/webhook.go` | [Webhook](./webhook/) |
```

Use the final, concrete label in the finished file. Do not leave the prose `recommendation from the dossier`; that phrase is for this plan only.

- [ ] **Step 2: Run a heading-consistency sweep**

Run:

```bash
cd gormes && for f in docs/content/building-gormes/gateway-donor-map/*.md; do \
  case "$f" in \
    docs/content/building-gormes/gateway-donor-map/_index.md|docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md) continue ;; \
  esac; \
  printf "%s\n" "$f"; \
  rg -n '^## (Status|Why This Adapter Is Reusable|Picoclaw Donor Files|What To Copy vs What To Rebuild|Gormes Mapping|Implementation Notes|Risks / Mismatches|Port Order Recommendation|Code References)$' "$f"; \
  printf '\n'; \
done
```

Expected: every dossier file prints all nine required section headings. If a page is missing one, fix it immediately.

- [ ] **Step 3: Run the final Hugo smoke test**

Run:

```bash
cd gormes && go test ./docs -run TestHugoBuild -count=1
```

Expected: `ok`

- [ ] **Step 4: Inspect the final diff**

Run:

```bash
git diff --stat -- gormes/docs/content/building-gormes gormes/docs/superpowers
git diff -- gormes/docs/content/building-gormes/gateway-donor-map
```

Expected: only the planned docs files and the approved spec/plan files show changes.

- [ ] **Step 5: Commit the finished donor map**

Run:

```bash
git add gormes/docs/content/building-gormes \
  gormes/docs/superpowers/specs/2026-04-20-gormes-gateway-donor-map-design.md \
  gormes/docs/superpowers/plans/2026-04-20-gormes-gateway-donor-map.md
git commit -m "docs(gormes): add picoclaw gateway donor map"
```

---

## Self-Review Checklist

- **Spec coverage:** The plan covers the hub page, shared-patterns page, every channel dossier named in the spec, and the required navigation surface in existing docs.
- **Placeholder scan:** No `TBD`, `TODO`, or empty file references remain. The only variable left during execution is the final recommendation label per dossier, which the steps require the implementer to choose explicitly after reviewing the cited donor files.
- **Consistency:** Every dossier uses the same section order, the same donor-path style, and the same Hugo verification command.
