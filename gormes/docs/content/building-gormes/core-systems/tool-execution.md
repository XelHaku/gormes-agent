---
title: "Tool Execution"
weight: 40
---

# Tool Execution

Typed Go interfaces. In-process registry. No Python bounce.

## The contract

```go
type Tool interface {
    Name() string
    Execute(ctx context.Context, input string) (string, error)
}
```

Every tool lives behind this interface. Schemas are Go structs — schema drift is a compile error, not a silent agent-loop failure.

## What you get

- **Deterministic execution** — no subprocess spawning for in-process tools
- **Bounded side effects** — ctx cancels; deadlines respected
- **Wire Doctor** — `gormes doctor --offline` validates the registry before a live turn burns tokens

## Status

✅ Shipped (Phase 2.A), with Phase 5.K now extending the registry to include a guarded `execute_code` tool. The current Go tool set still avoids broad terminal/file mutation surfaces: `execute_code` runs local `sh`/`python` snippets with timeout/output caps and pre-exec filesystem/network blocking, while the wider sandbox backend matrix stays in Phase 5.B and dangerous-action approval remains Phase 5.J. Cron job management remains an operator-tool parity task in Phase 5.N even though the Phase 2.D scheduler/audit bridge is shipped. See [Phase 2](../architecture_plan/phase-2-gateway/) and [Phase 5](../architecture_plan/phase-5-final-purge/).
