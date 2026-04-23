# Gormes Phase 2 Finish Sprint

> Execution: strict TDD (`RED -> GREEN -> REFACTOR`), atomic commits, no stacking on a red suite.

**Goal:** Finish the remaining Phase 2 gateway/runtime backlog from the audited roadmap state and leave Phase 2 closed in docs, tests, and generated progress surfaces.

## Verified baseline

The audited repo already has these Phase 2 substrates landed:

- Shared gateway chassis plus Telegram, Discord, and Slack.
- Session context + delivery routing.
- Cron delivery bridge.
- Deterministic subagent runtime + child Hermes stream loop.
- Slash-command registry + BOOT hook.
- Contract-first connector seams for Signal, Feishu, WeCom/WeiXin, QQ, DingTalk runtime bootstrap, and the shared threaded-text contract.

The remaining queue is now narrower than the old sprint plan implied and should stay decomposed:

- **2.B.3 (P1):** Slack shared-gateway closeout: migrate the existing `internal/slack` bot onto `gateway.Channel`/`Manager`, shared command parsing, config loading, and `cmd/gormes gateway` registration.
- **2.F.3 (P2):** pairing-state read model, status readout, runtime-status convergence.
- **2.F.4 (P3):** home-channel rules, notify-to routing, channel directory, manager remember-source, mirror/sticker cache.
- **2.B.4 (P1):** WhatsApp runtime selection + pairing/reconnect/send lifecycle.
- **2.B.6 (P2):** Signal transport/bootstrap layer.
- **2.B.8 (P4):** Matrix seam, Mattermost seam, then real client/bootstrap layers.
- **2.B.10 (P4):** Feishu, WeCom/WeiXin, and QQ transport/bootstrap layers.

---

## Phase 2 Definition of Done

1. Every Phase 2 item in `progress.json` is `complete`.
2. `go test ./... -count=1` passes twice in a row.
3. `go run ./cmd/progress-gen -validate` passes.
4. `go test ./docs -count=1` passes and the Phase 2 docs match implementation.

---

## Execution order (strict)

1. **P2-0** Finish `2.B.3` Slack shared-gateway closeout so the P1 shared-chassis claim is true again.
2. **P2-A** Finish the `2.F.3` pairing/status read model before widening any operator UX.
3. **P2-B** Finish `2.F.4` home-channel rules + notify-to routing before directory state starts driving delivery.
4. **P2-C** Finish `2.F.4` channel directory + remember-source before cache/mirror extras.
5. **P2-D** Finish `2.B.4` WhatsApp runtime selection + send lifecycle on top of the already-landed ingress seam.
6. **P2-E** Finish `2.B.6` Signal transport plus `2.B.8` Matrix/Mattermost platform seams.
7. **P2-F** Finish `2.B.8` real client/bootstrap layers plus `2.B.10` remaining transport bootstraps.
8. **P2-G** Finish `2.F.4` mirror/sticker cache tail and Phase 2 docs/ledger closeout.

---

## Slice P2-0 — 2.B.3 Slack Shared-Gateway Closeout

**Targets**
- `2.B.3` Gateway command wiring
- `2.B.3` Shared gateway registration + config wiring

**Files (expected)**
- `internal/slack/*`
- `internal/gateway/*`
- `internal/config/*`
- `cmd/gormes/*`

**Verify**
```bash
go test ./internal/slack ./internal/gateway ./internal/config ./cmd/gormes -count=1
```

**Commit strategy**
- One command-parser migration commit, then one `cmd/gormes gateway` registration/config commit.

---

## Slice P2-A — 2.F.3 Pairing/State Model

**Targets**
- `2.F.3` XDG pairing store
- `2.F.3` `gormes gateway status` operator readout
- `2.F.3` Runtime status convergence + channel lifecycle writers

**Files (expected)**
- `internal/gateway/*`
- `cmd/gormes/*`
- `internal/config/*`

**Verify**
```bash
go test ./internal/gateway ./cmd/gormes ./internal/config -count=1
```

**Commit strategy**
- One read-model commit, then one status/lifecycle commit.

---

## Slice P2-B — 2.F.4 Routing Rules

**Targets**
- `2.F.4` Home channel ownership rules
- `2.F.4` Notify-to delivery routing

**Files**
- `internal/gateway/*`
- `internal/session/*`
- `cmd/gormes/*`

**Verify**
```bash
go test ./internal/gateway ./internal/session ./cmd/gormes -count=1
```

**Commit**
`feat(gateway): add home-channel and notify-to routing rules`

---

## Slice P2-C — 2.F.4 Directory Surfaces

**Targets**
- `2.F.4` Channel directory persistence + lookup contract
- `2.F.4` Manager remember-source hook

**Files**
- `internal/gateway/*`
- `internal/session/*`
- `cmd/gormes/*`

**Verify**
```bash
go test ./internal/gateway ./internal/session ./cmd/gormes -count=1
```

**Commit**
`feat(gateway): add channel-directory and remember-source surfaces`

---

## Slice P2-D — 2.B.4 WhatsApp Runtime Closeout

**Targets**
- `2.B.4` Bridge-vs-native runtime decision
- `2.B.4` Pairing, reconnect, and send contract

**Files**
- `internal/channels/whatsapp/*`
- `internal/gateway/*`
- `internal/config/*`

**Verify**
```bash
go test ./internal/channels/whatsapp ./internal/gateway ./internal/config -count=1
```

**Commit strategy**
- One runtime-selection commit, then one send/lifecycle commit.

---

## Slice P2-E — Signal + Threaded-Text Platform Seams

**Targets**
- `2.B.6` Signal transport/bootstrap layer
- `2.B.8` Matrix shared-chassis bot seam
- `2.B.8` Mattermost shared-chassis bot seam

**Files**
- `internal/channels/signal/*`
- `internal/channels/matrix/*`
- `internal/channels/mattermost/*`
- `internal/channels/threadtext/*`
- `internal/gateway/*`

**Verify**
```bash
go test ./internal/channels/signal ./internal/channels/threadtext ./internal/channels/matrix ./internal/channels/mattermost ./internal/gateway -count=1
```

**Commit strategy**
- One platform seam per commit.

---

## Slice P2-F — Remaining Transport/Bootstrap Layers

**Targets**
- `2.B.8` Matrix real client/bootstrap layer
- `2.B.8` Mattermost REST/WS bootstrap layer
- `2.B.10` Feishu transport/bootstrap layer
- `2.B.10` WeCom + WeiXin transport/bootstrap layer
- `2.B.10` QQ Bot transport/bootstrap layer

**Verify**
```bash
go test ./internal/channels/... ./internal/gateway ./internal/config ./cmd/gormes -count=1
```

**Commit strategy**
- One transport family per commit.

---

## Slice P2-G — Operator Tail + Docs Closeout

**Targets**
- `2.F.4` Mirror + sticker cache surfaces
- Phase 2 docs + ledger closeout

**Files**
- `internal/gateway/*`
- `docs/content/building-gormes/architecture_plan/progress.json`
- `docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- `docs/content/building-gormes/architecture_plan/_index.md`

**Verify**
```bash
go run ./cmd/progress-gen -write
go run ./cmd/progress-gen -validate
go test ./docs -count=1
go test ./... -count=1
go test ./... -count=1
```

**Commit**
`docs(phase2): finalize gateway/runtime phase closeout`

---

## Global Guardrail

After every slice:

```bash
go test ./... -count=1
```

If red, land the immediate fix before starting the next slice.
