# syntax=docker/dockerfile:1.6
#
# Gormes — Go-native operator console for Hermes.
#
# This image mirrors the upstream Hermes volume contract (`/opt/data`) so that
# operators migrating from the Python image can keep their existing bind mount
# and still get sessions, memory, skills, plugins, and MCP tokens in the same
# place.  The only runtime requirement is the single static `gormes` binary
# plus the bootstrap `entrypoint.sh`; there is no Python or Node runtime.

# ---------- Stage 1: Go builder ----------
FROM golang:1.26-bookworm AS builder

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS=-trimpath

WORKDIR /src

# Copy module graph first so dependency layers cache across source edits.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the Gormes source tree.  .dockerignore excludes `bin/`,
# `.git`, and local caches so the build context stays lean.
COPY . .

# Build the single operator binary.  `-s -w` strips the symbol/debug tables
# to match the Makefile's release build flags.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gormes ./cmd/gormes

# ---------- Stage 2: Runtime ----------
FROM debian:13.4-slim AS runtime

# Gormes is a Go binary but still benefits from a minimal set of host tools:
# - ca-certificates: HTTPS to LLM + MCP endpoints
# - tini: PID-1 signal reaping for container stop/restart
# - gosu: drop root while preserving host-mapped UIDs for bind mounts
# - git, ripgrep, ffmpeg: matches upstream image's operator tool surface
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates tini gosu git ripgrep ffmpeg procps && \
    rm -rf /var/lib/apt/lists/*

# Non-root user for runtime; UID can be overridden via GORMES_UID at runtime
# so bind-mounted host volumes stay owned by the host user (mirrors upstream
# `hermes` user / HERMES_UID contract).
RUN useradd -u 10000 -m -d /opt/data gormes

# Install Gormes under /opt/gormes so the operator-visible layout mirrors
# the upstream /opt/hermes install prefix.
WORKDIR /opt/gormes

# Ship the static binary plus the on-disk docker bootstrap assets.  The
# entrypoint copies SOUL.md into the volume on first run, matching the
# upstream persona-file bootstrap.
COPY --from=builder /out/gormes /opt/gormes/bin/gormes
COPY docker/entrypoint.sh /opt/gormes/docker/entrypoint.sh
COPY docker/SOUL.md      /opt/gormes/docker/SOUL.md
RUN chmod 0755 /opt/gormes/docker/entrypoint.sh /opt/gormes/bin/gormes && \
    ln -s /opt/gormes/bin/gormes /usr/local/bin/gormes && \
    chown -R gormes:gormes /opt/gormes

# Volume contract — identical to the upstream Hermes image.  Operators who
# already bind-mount `/opt/data` from a Hermes container keep the same
# mount invocation when they switch to Gormes.
ENV GORMES_HOME=/opt/data \
    XDG_DATA_HOME=/opt/data
VOLUME [ "/opt/data" ]

ENTRYPOINT [ "/usr/bin/tini", "--", "/opt/gormes/docker/entrypoint.sh" ]
CMD [ "gormes" ]
