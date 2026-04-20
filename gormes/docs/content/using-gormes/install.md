---
title: "Install"
weight: 20
---

# Install

Gormes is a single static Go binary (~17 MB). Zero CGO, no Python runtime on the host.

## Recommended: curl pipe

```bash
curl -fsSL https://gormes.ai/install.sh | sh
```

Installs via `go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest` into `$HOME/go/bin/gormes`.

Requires Go 1.25+. On Ubuntu: `sudo apt install golang-1.25`. On macOS: `brew install go`.

## Go install directly

```bash
go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest
```

## Platform matrix

| Platform | Status |
|---|---|
| Linux x86_64 | ✅ tested |
| Linux arm64 | ✅ tested |
| macOS arm64 (Apple Silicon) | ✅ tested |
| macOS Intel | 🟡 should work, not regression-tested |
| Windows (native) | ❌ not supported |
| Windows WSL2 | ✅ tested |
| Termux (Android) | ✅ tested |

## Verify

```bash
gormes version
gormes doctor --offline
```
