#!/bin/sh
# install.sh — bootstrap Gormes on Linux, macOS, or WSL2.
#
# Usage:
#   curl -fsSL https://gormes.ai/install.sh | sh
#
# Environment overrides:
#   GORMES_REPO     — GitHub repo hosting releases (default: TrebuchetDynamics/gormes-agent)
#   GORMES_VERSION  — release tag to install (default: latest)
#   GORMES_PREFIX   — install prefix (default: $HOME/.local, falls back to /usr/local)
#
# Native Windows is not supported. Install WSL2 and rerun inside it.

set -eu

REPO="${GORMES_REPO:-TrebuchetDynamics/gormes-agent}"
VERSION="${GORMES_VERSION:-latest}"
PREFIX="${GORMES_PREFIX:-}"

log()  { printf '[gormes] %s\n' "$*" >&2; }
fail() { printf '[gormes] error: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"; }

detect_os() {
  case "$(uname -s)" in
    Linux*)   printf 'linux\n' ;;
    Darwin*)  printf 'darwin\n' ;;
    MINGW*|MSYS*|CYGWIN*)
      fail "native Windows is not supported — install WSL2 and rerun this script inside it" ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   printf 'amd64\n' ;;
    aarch64|arm64)  printf 'arm64\n' ;;
    armv7l|armv7)   printf 'armv7\n' ;;
    *) fail "unsupported CPU: $(uname -m)" ;;
  esac
}

pick_prefix() {
  if [ -n "$PREFIX" ]; then
    printf '%s\n' "$PREFIX"
    return
  fi
  if [ -w "${HOME:-/nonexistent}" ] 2>/dev/null; then
    printf '%s/.local\n' "$HOME"
    return
  fi
  printf '/usr/local\n'
}

resolve_tag() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s\n' "$VERSION"
    return
  fi
  api="https://api.github.com/repos/${REPO}/releases/latest"
  tag=$(curl -fsSL "$api" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1 || true)
  [ -n "$tag" ] || fail "could not resolve latest release from ${api} — set GORMES_VERSION or check ${REPO}"
  printf '%s\n' "$tag"
}

main() {
  need curl
  need tar
  need uname

  OS=$(detect_os)
  ARCH=$(detect_arch)
  TAG=$(resolve_tag)
  PREFIX_DIR=$(pick_prefix)
  BIN_DIR="${PREFIX_DIR}/bin"

  ASSET="gormes_${TAG#v}_${OS}_${ARCH}.tar.gz"
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

  log "target ${OS}/${ARCH}, version ${TAG}"
  log "fetching ${URL}"

  TMP=$(mktemp -d)
  trap 'rm -rf "$TMP"' EXIT

  if ! curl -fsSL "$URL" -o "${TMP}/gormes.tgz"; then
    fail "download failed — the release asset may not exist yet.\n    build from source instead:  go install github.com/${REPO}@${TAG}"
  fi

  tar -xzf "${TMP}/gormes.tgz" -C "$TMP"

  mkdir -p "$BIN_DIR"
  if [ ! -w "$BIN_DIR" ]; then
    fail "cannot write to ${BIN_DIR} — set GORMES_PREFIX to a writable path"
  fi

  install -m 0755 "${TMP}/gormes" "${BIN_DIR}/gormes"
  log "installed ${BIN_DIR}/gormes"

  case ":${PATH:-}:" in
    *":${BIN_DIR}:"*) ;;
    *)
      log "note: ${BIN_DIR} is not in your PATH"
      log "add it:  export PATH=\"${BIN_DIR}:\$PATH\""
      ;;
  esac

  log "verify:  gormes version"
  log "doctor:  gormes doctor --offline"
}

main "$@"
