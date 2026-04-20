#!/usr/bin/env bash
# update-readme.sh — substitutes benchmark values into README.md from benchmarks.json
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GORMES_DIR="$(dirname "$SCRIPT_DIR")"
BENCHMARKS_FILE="$GORMES_DIR/benchmarks.json"
README_FILE="$GORMES_DIR/../README.md"

if [[ ! -f "$BENCHMARKS_FILE" ]]; then
    echo "update-readme: benchmarks.json not found — skipping" >&2
    exit 0
fi

size_mb=$(python3 -c "import json; print(json.load(open('$BENCHMARKS_FILE'))['binary']['size_mb'])")

sed -i -E "s/~[0-9.]+ MB/~${size_mb} MB/g" "$README_FILE"

echo "README.md updated with binary size: ${size_mb} MB"
