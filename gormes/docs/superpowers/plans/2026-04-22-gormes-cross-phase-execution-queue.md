# Gormes Cross-Phase Execution Queue

This file converts the per-phase sprint plans into one conservative execution queue for a single primary engineer working in strict TDD on `main`.

## Planning assumptions

- Estimates are **engineering time**, not calendar time.
- Every slice follows the existing sprint rule: do not stack work on a red suite.
- Phase 1 is already complete and sits off the critical path as a maintenance lane.
- Phase 6 depends specifically on the **Phase 5.F skills plumbing** inside the broader Phase 5 purge; it does not need the entire Phase 5 tail to be fully closed before starting.

## Hard dependency gates

| Gate | Meaning | Must be true before moving on |
|---|---|---|
| G0 | Baseline stays green | `go test ./internal/progress -count=1` and `go run ./cmd/progress-gen --validate` remain green before starting a new tranche |
| G1 | Runtime realism landed | Phase 2 `P2-A` complete: real child Hermes stream loop is merged and `go test ./internal/subagent ./internal/hermes ./internal/kernel -count=1 -race` is green |
| G2 | Shared gateway lifecycle/operator plane landed | Phase 2 remaining `2.B.3` + `2.F.3` + `2.F.4` slices are complete: Slack is registered through the shared manager/command parser, pairing/status read model is green, and home-channel/operator surfaces are green |
| G3 | Gateway routing substrate landed | Phase 2 `P2-D` complete: session context, delivery routing, and stream fan-out are stable and tested |
| G4 | Primary adapter wave landed | Phase 2 `P2-E` complete: WhatsApp, Signal, and Email/SMS are on the shared contract |
| G5 | Phase 2 closed | Phase 2 `P2-F` + `P2-G` complete: long-tail adapters/docs done, `go test ./... -count=1` passes twice, and `go run ./cmd/progress-gen --validate` is green |
| G6 | Memory observability spine landed | Phase 3 `P3-A` + `P3-B` complete: session mirror, tool audit, memory status, and decay surfaces are green |
| G7 | Phase 3 closed | Phase 3 `P3-C` + `P3-D` + `P3-E` complete: export, cross-chat synthesis, insights, lineage search, and docs are green |
| G8 | Native agent foundation landed | Phase 4 `P4-A` + `P4-B` + `P4-C` complete: provider adapters, context engine, prompt builder, and model routing are stable |
| G9 | Phase 4 closed | Phase 4 `P4-D` + `P4-E` complete: telemetry, titles, credentials, resilience, and docs are green |
| G10 | Learning-loop prereq landed | Phase 5 `P5-A` + the **skills-plumbing portion** of `P5-C` are complete: core tool/runtime surface plus remaining skills-system port are stable |
| G11 | Phase 5 closed | Phase 5 `P5-B` + remaining `P5-C` + `P5-D` + `P5-E` complete: Python is out of the runtime path and packaging/operator parity is green |
| G12 | Phase 6 closed | Phase 6 `P6-A`..`P6-D` complete and the learning loop is auditable, reversible, and operator-visible |

## Conservative single-stream queue

| Queue | Tranche | Source slices | Estimate | Blocked by | Exit gate |
|---|---|---|---|---|---|
| 0 | Phase 1 maintenance lane | Phase 1 `P1-A`..`P1-D` | 2-4 days | None | Optional; does not block the critical path unless a Phase 1 regression appears |
| 1 | Runtime realism | Phase 2 `P2-A` | 3-5 days | G0 | G1 |
| 2 | Shared gateway lifecycle/operator plane | Phase 2 remaining `2.B.3` + `2.F.3` + `2.F.4` slices | 8-13 days | G1 | G2 |
| 3 | Routing substrate | Phase 2 `P2-D` | 4-6 days | G2 | G3 |
| 4 | Adapter wave 1 | Phase 2 `P2-E` | 8-12 days | G3 | G4 |
| 5 | Adapter wave 2 + closeout | Phase 2 `P2-F` + `P2-G` | 11-17 days | G4 | G5 |
| 6 | Memory observability + decay | Phase 3 `P3-A` + `P3-B` | 8-10 days | G5 | G6 |
| 7 | Memory export + identity + lineage | Phase 3 `P3-C` + `P3-D` + `P3-E` | 10-14 days | G6 | G7 |
| 8 | Native agent foundation | Phase 4 `P4-A` + `P4-B` + `P4-C` | 15-24 days | G5 and G7 | G8 |
| 9 | Runtime intelligence/resilience | Phase 4 `P4-D` + `P4-E` | 10-14 days | G8 | G9 |
| 10 | Purge foundation + skills plumbing | Phase 5 `P5-A` + skills-plumbing portion of `P5-C` | 15-20 days | G9 | G10 |
| 11 | Remaining purge + packaging | Phase 5 `P5-B` + remaining `P5-C` + `P5-D` + `P5-E` | 24-35 days | G10 | G11 |
| 12 | Native learning loop | Phase 6 `P6-A`..`P6-D` | 10-16 days | G10 | G12 |

## Critical-path summary

- **Immediate next slice:** Queue 2, the remaining Phase 2 shared-gateway/lifecycle/operator plane (`2.B.3` Slack closeout, then `2.F.3` + `2.F.4`).
- **Architecture unlock:** Queue 8. Phase 4 should not start before Phase 2 is closed and Phase 3 memory closeout is stable.
- **Learning-loop unlock:** Queue 10. Phase 6 can start once the **Phase 5.F skills plumbing** inside `P5-C` is real, even if the rest of Phase 5 is still finishing.
- **Conservative total critical path:** about **103-147 engineering days** for Queues 1-12, or roughly **21-30 engineer-weeks**. Phase 1 maintenance adds **2-4 days** only if regression cleanup is needed.

## Practical execution rule

If this stays single-threaded, do not jump phases early. The correct order is:

1. Close Phase 2 completely.
2. Close Phase 3 completely.
3. Land the Phase 4 native-brain foundation and close Phase 4.
4. Land the Phase 5 purge foundation up through skills plumbing.
5. Start Phase 6.
6. Finish the remaining Phase 5 purge tail if it is still open.

If a second engineer is available, the only safe early overlap is:

- Phase 1 maintenance alongside anything.
- Phase 6 starting after **G10** while the non-skills Phase 5 tail continues.

Everything else should stay serialized because the later phases depend on the exact runtime, routing, memory, and skills contracts produced by the earlier ones.
