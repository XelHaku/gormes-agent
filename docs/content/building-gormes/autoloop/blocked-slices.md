---
title: "Blocked Slices"
weight: 40
aliases:
  - /building-gormes/blocked-slices/
---

# Blocked Slices

This page is generated from canonical `progress.json` rows that declare
`blocked_by`.

Use it to avoid assigning work before the dependency chain is ready.

<!-- PROGRESS:START kind=blocked-slices -->
| Phase | Slice | Blocked by | Ready when | Unblocks |
|---|---|---|---|---|
| 2 / 2.B.4 | WhatsApp reconnect backoff + send retry policy | WhatsApp outbound pairing gate + raw peer mapping | Outbound pairing gates and raw peer mapping are fixture-locked and transport senders can be faked. | - |
| 2 / 2.B.5 | BlueBubbles iMessage session-context prompt guidance | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound formatting splits blank-line paragraphs into separate iMessage sends, so prompt guidance has a matching delivery contract. | - |
| 2 / 2.E.3 | Durable worker supervisor status seam | Durable job backpressure + timeout audit | Backpressure and timeout audit fields are visible through the durable ledger status surface. | - |
| 2 / 2.F.3 | Unauthorized DM pairing response contract | Pairing approval + rate-limit semantics | Pairing approval, rate limiting, and allowlist checks are fixture-locked. | - |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | 2.E.2 | 2.E.2 is complete and the shared CommandDef registry is stable for gateway commands. | Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard |
| 4 / 4.A | Bedrock stale-client eviction + retry classification | Bedrock SigV4 + credential seam | A Bedrock client/cache seam exists behind the provider adapter and can be exercised without live AWS credentials. | - |
| 4 / 4.A | Codex OAuth state + stale-token relogin | Token vault, Multi-account auth, Codex Responses pure conversion harness | Gormes has an XDG-scoped token vault and account-selection seam for provider credentials. | - |
| 4 / 4.G | Anthropic OAuth/keychain credential discovery | Token vault | Token vault owns XDG-scoped credential files and can expose provider auth status without live credentials. | - |
| 5 / 5.I | Dashboard theme/plugin extension status contract | Plugin SDK | Plugin SDK manifest loading is fixture-locked and Dashboard API client contract already exposes disabled/degraded panel status. | - |
| 5 / 5.I | First-party Spotify plugin fixture | Plugin SDK | Plugin manifest loading and capability registration are fixture-locked by the Plugin SDK slice. | - |
| 5 / 5.J | Subagent dangerous-command non-interactive approval policy | Dangerous-command detector + blocked-result schema, Approval mode config normalization | Dangerous-command detection and approval-mode config normalization are fixture-locked for local tools. | - |
| 5 / 5.O | Busy command guard for compression and long CLI actions | CLI command registry parity + active-turn busy policy | The CLI command registry has a shared active-turn/busy policy surface. | - |
| 5 / 5.Q | Dashboard PTY chat sidecar contract | PTY bridge protocol adapter, SSE streaming to Bubble Tea TUI | PTY bridge behavior and TUI gateway event streaming are each fixture-locked. | - |
| 6 / 6.C | Portable SKILL.md format | Phase 2.G skills runtime | Phase 2.G skills runtime is complete and the parser/store seam is stable enough for versioned metadata. | LLM-assisted pattern distillation, Hybrid lexical + semantic lookup, Skill effectiveness scoring |
| 7 / 7.C | Matrix E2EE device-id crypto-store binding | Matrix real client/bootstrap layer | Matrix real client/bootstrap layer has auth, sync/invite handling, room-kind policy, and a fakeable E2EE bootstrap seam. | - |
<!-- PROGRESS:END -->
