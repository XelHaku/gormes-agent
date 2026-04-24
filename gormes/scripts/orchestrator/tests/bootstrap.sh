#!/usr/bin/env bash
# Bootstrap pinned bats-core and helper libs into ./vendor/.
# Idempotent: skips download when vendored binaries already exist.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_DIR="$SCRIPT_DIR/vendor"

BATS_CORE_VERSION="1.11.0"
BATS_ASSERT_VERSION="2.1.0"
BATS_SUPPORT_VERSION="0.3.0"

BATS_CORE_SHA256="aeff09fdc8b0c88b3087c99de00cf549356d7a2f6a69e3fcec5e0e861d2f9063"
BATS_ASSERT_SHA256="98ca3b685f8b8993e48ec057565e6e2abcc541034ed5b0e81f191505682037fd"
BATS_SUPPORT_SHA256="7815237aafeb42ddcc1b8c698fc5808026d33317d8701d5ec2396e9634e2918f"

fetch_and_extract() {
  local name="$1"
  local version="$2"
  local sha256="$3"
  local dest_name="$4"

  if [[ "${BATS_OFFLINE:-0}" == "1" ]]; then
    echo "ERROR: $name not vendored and BATS_OFFLINE=1" >&2
    return 1
  fi

  local url="https://github.com/bats-core/${name}/archive/refs/tags/v${version}.tar.gz"
  local tmp_tarball
  tmp_tarball="$(mktemp)"
  trap 'rm -f "$tmp_tarball"' RETURN

  echo "==> Downloading ${name} v${version}"
  curl -fsSL "$url" -o "$tmp_tarball"

  local got
  got="$(sha256sum "$tmp_tarball" | awk '{print $1}')"
  if [[ "$got" != "$sha256" ]]; then
    echo "ERROR: SHA256 mismatch for ${name} v${version}" >&2
    echo "  expected: $sha256" >&2
    echo "  got:      $got" >&2
    rm -f "$tmp_tarball"
    exit 1
  fi

  local extract_tmp
  extract_tmp="$(mktemp -d)"
  tar -xzf "$tmp_tarball" -C "$extract_tmp"

  rm -rf "$VENDOR_DIR/$dest_name"
  mv "$extract_tmp/${name}-${version}" "$VENDOR_DIR/$dest_name"
  rm -rf "$extract_tmp"
  rm -f "$tmp_tarball"
  trap - RETURN
}

mkdir -p "$VENDOR_DIR"

if [[ ! -x "$VENDOR_DIR/bats-core/bin/bats" ]]; then
  fetch_and_extract "bats-core" "$BATS_CORE_VERSION" "$BATS_CORE_SHA256" "bats-core"
fi

if [[ ! -f "$VENDOR_DIR/bats-assert/load.bash" ]]; then
  fetch_and_extract "bats-assert" "$BATS_ASSERT_VERSION" "$BATS_ASSERT_SHA256" "bats-assert"
fi

if [[ ! -f "$VENDOR_DIR/bats-support/load.bash" ]]; then
  fetch_and_extract "bats-support" "$BATS_SUPPORT_VERSION" "$BATS_SUPPORT_SHA256" "bats-support"
fi

echo "==> bats harness vendored at $VENDOR_DIR"
