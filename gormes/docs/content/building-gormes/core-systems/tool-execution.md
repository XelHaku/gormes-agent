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

✅ Shipped (Phase 2.A). The current registry has the minimal tool surface; porting the 61 upstream Python tools is Phase 5.A. See [Phase 2](../architecture_plan/phase-2-gateway/).
