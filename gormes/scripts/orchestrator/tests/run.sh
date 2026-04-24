#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BATS="$SCRIPT_DIR/vendor/bats-core/bin/bats"

if [[ ! -x "$BATS" ]]; then
  bash "$SCRIPT_DIR/bootstrap.sh"
fi

tiers=("$@")
[[ ${#tiers[@]} -eq 0 ]] && tiers=(unit)

declare -a bats_paths=()
for tier in "${tiers[@]}"; do
  case "$tier" in
    unit|integration)
      bats_paths+=("$SCRIPT_DIR/$tier")
      ;;
    *)
      echo "ERROR: unknown tier '$tier'" >&2
      exit 1
      ;;
  esac
done

exec "$BATS" --print-output-on-failure --timing "${bats_paths[@]}"
