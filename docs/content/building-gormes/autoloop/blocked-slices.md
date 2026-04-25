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
| 2 / 2.B.3 | Slack config + cmd/gormes gateway registration | Slack gateway.Channel adapter shim | Slack gateway.Channel adapter shim runs through the shared Manager lifecycle in tests. | - |
| 2 / 2.B.5 | BlueBubbles iMessage session-context prompt guidance | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound formatting splits blank-line paragraphs into separate iMessage sends, so prompt guidance has a matching delivery contract. | - |
| 2 / 2.F.3 | Runtime status JSON + PID/process validation | `gormes gateway status` read-only command | `gormes gateway status` read-only command has established the operator-facing status view and can consume PID validation evidence., The slice can use a fake process table and temp gateway_state.json/PID paths; no live system process control is required. | Token-scoped gateway locks, Gateway /restart command + takeover markers |
| 2 / 2.F.3 | Token-scoped gateway locks | Runtime status JSON + PID/process validation | Runtime status JSON + PID/process validation can identify stale or live processes without invoking live service managers. | Gateway /restart command + takeover markers, Slack config + cmd/gormes gateway registration |
| 2 / 2.F.3 | Gateway /restart command + takeover markers | Drain-timeout resume_pending recovery, Runtime status JSON + PID/process validation, Token-scoped gateway locks | Drain-timeout resume_pending recovery, Runtime status JSON + PID/process validation, and Token-scoped gateway locks are validated on main. | - |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | 2.E.2 | 2.E.2 is complete and the shared CommandDef registry is stable for gateway commands. | Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard |
| 4 / 4.A | Bedrock stale-client eviction + retry classification | Bedrock SigV4 + credential seam | A Bedrock client/cache seam exists behind the provider adapter and can be exercised without live AWS credentials. | - |
| 4 / 4.A | Codex OAuth state + stale-token relogin | Token vault, Multi-account auth, Codex Responses pure conversion harness | Gormes has an XDG-scoped token vault and account-selection seam for provider credentials. | - |
| 4 / 4.G | Anthropic OAuth/keychain credential discovery | Token vault | Token vault owns XDG-scoped credential files and can expose provider auth status without live credentials. | - |
| 5 / 5.J | Subagent dangerous-command non-interactive approval policy | Dangerous-command detector + blocked-result schema, Approval mode config normalization | Dangerous-command detection and approval-mode config normalization are fixture-locked for local tools. | - |
| 5 / 5.N | Cron context_from output chaining | Cronjob tool API + schedule parser parity | Cronjob tool API + schedule parser parity has a create/update/list surface over the Go cron store, or this slice owns the minimal ContextFrom field and prompt-builder fixture without exposing a public tool yet. | - |
| 5 / 5.O | Busy command guard for compression and long CLI actions | CLI command registry parity + active-turn busy policy | The CLI command registry has a shared active-turn/busy policy surface. | - |
| 5 / 5.O | Oneshot stdout-only kernel execution | Top-level oneshot flag and model/provider resolver | Top-level oneshot flag and model/provider resolver is validated on main., A fake hermes.Client can stream one assistant final response through the existing kernel without live provider credentials. | - |
| 5 / 5.P | Unix installer root/FHS layout policy | Unix installer (install.sh) source-backed update flow | Unix installer (install.sh) source-backed update flow has canonical scripts under scripts/ and a byte-equal served site copy. | Installer site asset/route coverage |
| 5 / 5.Q | Dashboard PTY chat sidecar contract | PTY bridge protocol adapter, SSE streaming to Bubble Tea TUI | PTY bridge behavior and TUI gateway event streaming are each fixture-locked. | - |
| 6 / 6.C | Portable SKILL.md format | Phase 2.G skills runtime | Phase 2.G skills runtime is complete and the parser/store seam is stable enough for versioned metadata. | LLM-assisted pattern distillation, Hybrid lexical + semantic lookup, Skill effectiveness scoring |
| 7 / 7.C | Matrix E2EE device-id crypto-store binding | Matrix real client/bootstrap layer | Matrix real client/bootstrap layer has auth, sync/invite handling, room-kind policy, and a fakeable E2EE bootstrap seam. | - |
<!-- PROGRESS:END -->
