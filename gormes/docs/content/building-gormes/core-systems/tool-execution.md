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
- **Fail-closed dangerous action gate** — command-like tool args are scanned for destructive shell payloads before `Tool.Execute` runs
- **Wire Doctor** — `gormes doctor --offline` validates the registry before a live turn burns tokens

## Status

✅ Shipped (Phase 2.A) with Phase 5.J guardrails. The current registry has the minimal tool surface, and dangerous shell payloads in command-like JSON fields now fail closed before execution in both the kernel and the in-process executor. Porting the broader upstream terminal/code-exec surfaces remains Phase 5.K. See [Phase 2](../architecture_plan/phase-2-gateway/).
