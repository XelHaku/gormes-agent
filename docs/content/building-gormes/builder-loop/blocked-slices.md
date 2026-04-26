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
| 2 / 2.B.5 | BlueBubbles iMessage session-context prompt guidance | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound formatting splits blank-line paragraphs into separate iMessage sends, so prompt guidance has a matching delivery contract. | - |
| 2 / 2.B.5 | Slack rich-text quotes/lists + link-unfurl ingress | Slack config + cmd/gormes gateway registration | Slack gateway.Channel adapter shim and Slack config + cmd/gormes gateway registration are validated on main., internal/slack.Event currently carries only Text, so the first implementation step is adding fixture-only block/attachment payload fields to the Slack client seam without changing real Socket Mode network behavior., Command routing must continue to call gateway.ParseInboundText on the original Slack text; only EventSubmit text is augmented before entering gateway.InboundEvent. | Slack thread-parent context + team-scoped cache key |
| 2 / 2.B.5 | Slack thread-parent context + team-scoped cache key | Slack rich-text quotes/lists + link-unfurl ingress | Slack rich-text quotes/lists + link-unfurl ingress is validated, so Event and Channel write-scope conflicts are resolved first., gateway.InboundEvent can be extended with a small ReplyToText field without changing non-Slack adapters., The row uses fake Slack thread-replies data only; no live conversations.replies call is required in tests. | - |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | 2.E.2 | 2.E.2 is complete and the shared CommandDef registry is stable for gateway commands. | Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard |
| 4 / 4.A | Bedrock stale-client eviction + retry classification | Bedrock SigV4 + credential seam | A Bedrock client/cache seam exists behind the provider adapter and can be exercised without live AWS credentials. | - |
| 4 / 4.A | Codex OAuth state + stale-token relogin | Token vault, Multi-account auth, Codex Responses pure conversion harness, Codex Responses assistant content role types | Gormes has an XDG-scoped token vault and account-selection seam for provider credentials. | - |
| 4 / 4.G | Anthropic OAuth/keychain credential discovery | Token vault | Token vault owns XDG-scoped credential files and can expose provider auth status without live credentials. | - |
| 5 / 5.A | Home Assistant HASS_TOKEN platform-toolset carveout | Platform toolset config persistence + MCP sentinel | Platform toolset config persistence + MCP sentinel is validated on main, so default-off filtering, platform default supersets, and restricted toolsets already have pure fixtures., The upstream parity manifest already contains the homeassistant toolset and HASS_TOKEN requirement; the row only changes runtime default resolution when the env var is present., The test can use t.Setenv("HASS_TOKEN", "fake-test-token") and t.Setenv("HASS_TOKEN", "") without live Home Assistant calls. | - |
| 5 / 5.J | Subagent dangerous-command non-interactive approval policy | Recoverable dangerous patterns + blocked-result schema, Approval mode config normalization | Dangerous-command detection and approval-mode config normalization are fixture-locked for local tools. | - |
| 5 / 5.N | Autoloop recent-failure detail excerpts | Planner audit blank-subphase control-plane bucket | Planner audit blank-subphase control-plane bucket is complete and already attributes empty Task worker_failed events to control-plane/backend., builderloop.LedgerEvent already exposes Detail, ExitError, StdoutTail, and StderrTail from worker_failed records; the row only surfaces a bounded excerpt in planner audit output., This row is planner-control-plane only; no worker backend execution, candidate selection, or promotion semantics change is needed. | - |
| 5 / 5.N | Cron context_from output chaining | Cronjob tool API + schedule parser parity | Cronjob tool API + schedule parser parity has a create/update/list surface over the Go cron store, or this slice owns the minimal ContextFrom field and prompt-builder fixture without exposing a public tool yet. | - |
| 5 / 5.P | Unix installer root/FHS layout policy | Unix installer (install.sh) source-backed update flow | Unix installer (install.sh) source-backed update flow has canonical scripts under scripts/ and a byte-equal served site copy. | Installer site asset/route coverage |
| 5 / 5.Q | API server health + cron admin endpoints | Cronjob tool API + schedule parser parity, Cron prompt/script safety + pre-run script contract, Cron multi-target delivery + media/live-adapter fallback | Cronjob tool API + schedule parser parity, Cron prompt/script safety + pre-run script contract, and Cron multi-target delivery + media/live-adapter fallback have stable native store contracts. | - |
| 6 / 6.B | LLM-assisted pattern distillation | Portable SKILL.md format | Portable SKILL.md format is validated, so generated drafts can carry review state and remain out of prompt injection until approved., The row can use fake model/review prompt fixtures; no live provider, skill install, or prompt injection is required. | - |
| 6 / 6.C | Portable SKILL.md format | Phase 2.G skills runtime | Phase 2.G skills runtime is complete and the parser/store seam is stable enough for versioned metadata. | LLM-assisted pattern distillation, Hybrid lexical + semantic lookup, Skill effectiveness scoring |
| 6 / 6.D | Hybrid lexical + semantic lookup | Portable SKILL.md format | Portable SKILL.md format is validated so skills carry review state, triggers, exclusions, and provenance metadata., Phase 3 semantic embedding lookup remains optional and can be stubbed in unit fixtures without Ollama. | Source-aware retrieval damping fixtures, Code Cathedral II code-context retrieval fixtures, Skill effectiveness scoring |
| 6 / 6.D | Source-aware retrieval damping fixtures | Hybrid lexical + semantic lookup | Hybrid lexical + semantic lookup is validated and returns score explanations that can carry source-tier evidence., The worker can use synthetic skill/memory sources and fake scores; no GBrain SQL engine, PGLite, Postgres, or tree-sitter runtime is required. | Code Cathedral II code-context retrieval fixtures, Skill effectiveness scoring |
| 6 / 6.D | Code Cathedral II code-context retrieval fixtures | Hybrid lexical + semantic lookup | Hybrid lexical + semantic lookup is validated and can accept optional external evidence in fixtures. | - |
| 7 / 7.C | Matrix E2EE device-id crypto-store binding | Matrix real client/bootstrap layer | Matrix real client/bootstrap layer has auth, sync/invite handling, room-kind policy, and a fakeable E2EE bootstrap seam. | - |
<!-- PROGRESS:END -->
