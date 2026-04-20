#!/usr/bin/env bash
# record-benchmark.sh — measures bin/gormes and records metrics to benchmarks.json
# Called automatically by 'make build' after producing the binary.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GORMES_DIR="$(dirname "$SCRIPT_DIR")"
BENCHMARKS_FILE="$GORMES_DIR/benchmarks.json"
BINARY_PATH="${BINARY_PATH:-"$GORMES_DIR/bin/gormes"}"

# ── validation ────────────────────────────────────────────────────────────────
if [[ ! -f "$BINARY_PATH" ]]; then
    echo "record-benchmark: binary not found at $BINARY_PATH — skipping" >&2
    exit 0  # don't fail the build if called before binary exists
fi

# ── measure ─────────────────────────────────────────────────────────────────
size_bytes=$(stat -c%s "$BINARY_PATH")
size_mb=$(awk "BEGIN {printf \"%.1f\", $size_bytes / 1048576}")
today=$(date +%Y-%m-%d)

# ── read existing benchmarks.json ─────────────────────────────────────────────
if [[ -f "$BENCHMARKS_FILE" ]]; then
    # Use python3 for reliable JSON manipulation (available everywhere)
    python3 << PYEOF
import json, sys
with open("$BENCHMARKS_FILE") as f:
    data = json.load(f)

size_bytes = $size_bytes
size_mb = $size_mb

# Update binary metrics
data["binary"]["size_bytes"] = size_bytes
data["binary"]["size_mb"] = str(size_mb)
data["binary"]["last_measured"] = "$today"

# Check if last history entry is from today (avoid duplicate entries)
history = data.get("history", [])
if not history or history[0].get("date") != "$today":
    # Prepend new entry (most recent first)
    import subprocess, datetime
    commit = subprocess.check_output(
        ["git", "rev-parse", "--short", "HEAD"],
        text=True
    ).strip()
    phase = open("$GORMES_DIR/docs/ARCH_PLAN.md").read().split("## Phase")[1].split("\n")[0].strip() if __import__("os").path.exists("$GORMES_DIR/docs/ARCH_PLAN.md") else "unknown"
    data["history"].insert(0, {
        "date": "$today",
        "size_mb": size_mb,
        "commit": commit,
        "phase": "Phase " + phase
    })

with open("$BENCHMARKS_FILE", "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print(f"benchmarks.json updated: {size_mb} MB")
PYEOF
else
    # Create new file
    cat > "$BENCHMARKS_FILE" << EOF
{
  "binary": {
    "name": "gormes",
    "path": "bin/gormes",
    "size_bytes": $size_bytes,
    "size_mb": "$size_mb",
    "build_flags": "CGO_ENABLED=0 -trimpath -ldflags=\\"-s -w\\"",
    "linker": "static",
    "stripped": true,
    "go_version": "1.25+",
    "last_measured": "$today"
  },
  "properties": {
    "cgo": false,
    "dependencies": "zero (no dynamic library deps)",
    "platforms": ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
  },
  "history": [
    {
      "date": "$today",
      "size_mb": $size_mb,
      "phase": "unknown"
    }
  ]
}
EOF
    echo "benchmarks.json created: $size_mb MB"
fi

# ── copy to Hugo data directory for docs site ────────────────────────────────
docs_data="$GORMES_DIR/docs/data"
mkdir -p "$docs_data"
cp "$BENCHMARKS_FILE" "$docs_data/benchmarks.json"
echo "copied to docs/data/benchmarks.json"
