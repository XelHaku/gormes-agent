---
title: "Wire Doctor"
weight: 60
---

# Wire Doctor

`gormes doctor` validates the local stack before a live turn burns tokens.

## Online mode

```bash
gormes doctor
```

Checks:

- Hermes backend reachable at configured endpoint (2s timeout)
- Tool registry built and every tool passes schema validation
- Config file parses, state dirs writable

## Offline mode

```bash
gormes doctor --offline
```

Skips the Hermes reachability check. Useful for CI or pre-flight when you want to verify the local tool surface without a live backend.

## Reading the output

```
[PASS] api_server: reachable at http://127.0.0.1:8642
[PASS] tool registry: 6 tools, all schemas valid
[PASS] config: loaded from ~/.config/gormes/config.toml
```

Any `[FAIL]` line names the subsystem and the error. `doctor` exits non-zero on failure so you can wire it into scripts.
