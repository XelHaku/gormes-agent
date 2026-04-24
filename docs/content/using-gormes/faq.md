---
title: "FAQ"
weight: 70
---

# FAQ

### Do I need Hermes running?

Yes. Gormes is a Go frontend that talks to Hermes's `api_server` over HTTP. Without Hermes, only `--offline` mode (cosmetic smoke-tester) works.

### Can I use it without Python?

Not yet. Phase 4 makes Hermes optional; Phase 5 removes Python entirely. See the [Roadmap](../../building-gormes/architecture_plan/).

### Where does memory live?

`~/.hermes/memory/memory.db` (SQLite) with a human-readable mirror at `~/.hermes/memory/USER.md`. The mirror refreshes every 30 seconds.

### How do I back up memory?

Copy `~/.hermes/memory/memory.db` — it's a single SQLite file. USER.md regenerates from it.

### The install script installed Gormes to `$HOME/go/bin` but it's not on my PATH.

Add it: `export PATH="$HOME/go/bin:$PATH"` in your shell rc.

### How do I reset a session?

```bash
gormes --resume new
```

### Logs?

`~/.hermes/gormes.log` (current run) and `~/.hermes/crash-*.log` (panics). Crash logs are timestamped.
