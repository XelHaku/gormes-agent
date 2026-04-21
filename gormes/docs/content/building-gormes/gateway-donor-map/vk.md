---
title: "VK"
weight: 140
---

# VK

VK is technically serviceable in PicoClaw, but it is a bad fit for the current Gormes roadmap. This page is intentionally blunt because a porter deciding where to spend Phase 2 effort needs that signal, not politeness.

## Status

VK does not appear in `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` as a planned gateway adapter. The donor exists in PicoClaw, but Gormes currently has no stated phase slot or operator-facing Hermes docs for VK.

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes planning status was verified in-tree against `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`.

Keep the boundary explicit: PicoClaw can still be studied as donor input, but Gormes architecture and roadmap priorities remain authoritative.

## Why This Adapter Is Reusable

The donor is understandable and reasonably small.

- `pkg/channels/vk/vk.go` covers Long Poll startup, inbound message handling, message splitting, reply-to support, and basic attachment labeling.
- The send path is straightforward and not deeply entangled with PicoClaw runtime internals.
- The tests verify config decoding, max message length, and some attachment formatting behavior.

That said, "technically reusable" is not the same as "worth reusing now."

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/vk/vk.go`
- `picoclaw/pkg/channels/vk/vk_test.go`
- `picoclaw/docs/channels/vk/README.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

## What To Copy vs What To Rebuild

If VK were ever prioritized, likely copy candidates would be:

- Long Poll startup and shutdown from `Start` and `Stop`.
- Basic inbound/outbound flow from `handleMessage` and `Send`.
- Message splitting and reply-to behavior in the send path.

Even then, several parts would still need rebuilding:

- Mention logic is unfinished as donor material: `isMentioned` always returns false and `stripBotMention` is effectively a no-op.
- Attachment handling is mostly placeholder text, not rich media ingestion.
- Gormes session policy and roadmap priorities would still need to be defined from scratch because there is no in-tree Hermes-facing VK product guidance.

## Gormes Mapping

- `Start` and `Send` could seed a future `internal/vk` adapter if priorities change.
- `handleMessage` shows the likely edge model: peer-based chat identity, optional group-trigger rules, and reply-to delivery by conversation message ID.
- The missing or stubbed mention behavior is the key warning sign. A porter would need to re-verify core group-chat semantics rather than trusting the donor.

## Implementation Notes

- Do not spend near-term Phase 2 time here unless roadmap priorities change materially.
- If VK ever becomes relevant, treat the donor as a bootstrap skeleton and verify mention/group behavior against current VK API behavior before writing production code.
- Do not oversell the attachment surface. The current donor mostly turns media into placeholders instead of delivering rich cross-modal behavior.

## Risks / Mismatches

- No current roadmap slot in Gormes.
- No in-tree Hermes-facing VK behavior docs to anchor product expectations.
- Mention handling is not convincingly implemented in the donor.
- Attachment support is thin and mostly textual placeholders.
- Strategic value looks lower than Feishu, WeCom, DingTalk, or even the more general western channels already on the map.

## Port Order Recommendation

1. Do not schedule VK while planned enterprise and mainstream adapters are still unbuilt.
2. Revisit only if product requirements or user demand make VK materially important.
3. If that happens, start with a fresh transport validation pass rather than assuming the donor is production-ready.

## Code References

- `picoclaw/pkg/channels/vk/vk.go`: `Start`, `handleMessage`, `Send`, `isMentioned`, `processAttachments`.
- `picoclaw/pkg/channels/vk/vk_test.go`
- `picoclaw/docs/channels/vk/README.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`

Recommendation: `not worth reusing`.
