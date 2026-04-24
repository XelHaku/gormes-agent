#!/bin/bash
# Docker/Podman entrypoint: bootstrap the Gormes data volume and hand off
# to the static `gormes` binary.
#
# The volume layout mirrors the upstream Hermes image so operators migrating
# from the Python runtime keep the same bind mount and see familiar buckets.
# Gormes's Go runtime keys persistent state off `XDG_DATA_HOME`, so we export
# that to `/opt/data` (inside <volume>/gormes/*) before exec'ing the binary.
set -e

GORMES_HOME="${GORMES_HOME:-/opt/data}"
INSTALL_DIR="/opt/gormes"

# Export the data-home alias + XDG override so every subprocess (including
# the gormes binary itself) writes to the mounted volume.
export GORMES_HOME
export XDG_DATA_HOME="${XDG_DATA_HOME:-$GORMES_HOME}"

# --- Privilege dropping via gosu ---
# When started as root (the default for Docker, or fakeroot in rootless
# Podman), optionally remap the gormes user/group to match host-side
# ownership, fix volume permissions, then re-exec as gormes.
if [ "$(id -u)" = "0" ]; then
    if [ -n "$GORMES_UID" ] && [ "$GORMES_UID" != "$(id -u gormes)" ]; then
        echo "Changing gormes UID to $GORMES_UID"
        usermod -u "$GORMES_UID" gormes
    fi

    if [ -n "$GORMES_GID" ] && [ "$GORMES_GID" != "$(id -g gormes)" ]; then
        echo "Changing gormes GID to $GORMES_GID"
        # -o allows non-unique GID (e.g. macOS GID 20 "staff" may already
        # exist as "dialout" in the Debian-based container image).
        groupmod -o -g "$GORMES_GID" gormes 2>/dev/null || true
    fi

    actual_gormes_uid=$(id -u gormes)
    if [ "$(stat -c %u "$GORMES_HOME" 2>/dev/null)" != "$actual_gormes_uid" ]; then
        echo "$GORMES_HOME is not owned by $actual_gormes_uid, fixing"
        # In rootless Podman the container's "root" is mapped to an
        # unprivileged host UID — chown will fail.  That's fine: the volume
        # is already owned by the mapped user on the host side.
        chown -R gormes:gormes "$GORMES_HOME" 2>/dev/null || \
            echo "Warning: chown failed (rootless container?) — continuing anyway"
    fi

    echo "Dropping root privileges"
    exec gosu gormes "$0" "$@"
fi

# --- Running as gormes from here ---
#
# Pre-create the operator-visible volume layout.  Gormes's Go runtime will
# create the XDG `gormes/` subtree (checkpoints/, skills/, plugins/,
# mcp-tokens/, learning/, logs/, ...) under $XDG_DATA_HOME on demand, but
# these top-level buckets stay pre-created so operators can bind extra
# paths (e.g. backups, scratch workspaces) into familiar locations.
#
# The "home/" subdirectory is a per-profile HOME for subprocesses
# (git, ssh, gh, npm …).  Without it those tools write to /root which is
# ephemeral and shared across profiles.
mkdir -p "$GORMES_HOME"/{cron,sessions,logs,hooks,memories,skills,plans,workspace,home}

# Bootstrap the persona file if the operator has not provided one.  Edits
# to this file are loaded live by the kernel; deleting it falls back to
# the default persona.
if [ ! -f "$GORMES_HOME/SOUL.md" ]; then
    cp "$INSTALL_DIR/docker/SOUL.md" "$GORMES_HOME/SOUL.md"
fi

# Resolve the gormes binary via the /usr/local/bin symlink so a `docker run
# image gormes doctor --offline` style command routes through here without
# the caller having to spell the install path.
if [ "$1" = "gormes" ] || [ -z "$1" ]; then
    shift || true
    exec /opt/gormes/bin/gormes "$@"
fi
exec "$@"
