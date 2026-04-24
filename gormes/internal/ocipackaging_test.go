package internal_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOCIImageDockerfile guards the Phase 5.P "OCI image" seam: Gormes must
// ship a root Dockerfile that builds the static Go binary and exposes the
// same `/opt/data` volume layout as the upstream Python image, so operators
// who migrate from Hermes keep a single mount point for persistent state.
//
// The test intentionally asserts *structural* invariants (multi-stage build,
// non-root user, CGO disabled, XDG_DATA_HOME honored, docker volume, etc.)
// rather than the exact base image pins, because those will drift over time
// while the volume + entrypoint contract must remain stable.
func TestOCIImageDockerfile(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	dockerfilePath := filepath.Join(gormesRoot, "Dockerfile")

	raw, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read %s: %v", dockerfilePath, err)
	}
	content := string(raw)

	// Multi-stage build: a dedicated Go builder stage keeps the runtime image
	// small and reproducible, mirroring the upstream layered-install pattern.
	if !strings.Contains(content, "AS builder") && !strings.Contains(content, "AS build") {
		t.Errorf("Dockerfile is missing a multi-stage `AS builder` (or `AS build`) stage")
	}

	// Go toolchain base: the builder stage must use an official `golang`
	// image so the port stays self-contained without a host Go toolchain.
	if !strings.Contains(content, "FROM golang:") {
		t.Errorf("Dockerfile builder stage must start `FROM golang:` to build Gormes without host tooling")
	}

	// CGO_ENABLED=0 matches the Makefile's static-binary contract and keeps
	// the produced image portable across libc variants.
	if !strings.Contains(content, "CGO_ENABLED=0") {
		t.Errorf("Dockerfile must build with CGO_ENABLED=0 to ship a static binary")
	}

	// Build target: the `./cmd/gormes` package is the single operator entry
	// point; if the Dockerfile builds something else the image would ship the
	// wrong binary.
	if !strings.Contains(content, "./cmd/gormes") {
		t.Errorf("Dockerfile must `go build ./cmd/gormes` to produce the operator binary")
	}

	// Non-root runtime user named `gormes` — the upstream image uses a
	// dedicated `hermes` user for the volume; gormes mirrors that invariant
	// so mount permissions stay predictable.
	if !strings.Contains(content, "useradd") || !strings.Contains(content, "gormes") {
		t.Errorf("Dockerfile must create a non-root `gormes` user via useradd")
	}

	// Install prefix: the operator binary and bundled docker assets live
	// under `/opt/gormes` (mirrors upstream `/opt/hermes`).
	if !strings.Contains(content, "/opt/gormes") {
		t.Errorf("Dockerfile must install Gormes under /opt/gormes (mirrors upstream /opt/hermes layout)")
	}

	// Volume layout: `/opt/data` is the shared persistent mount. Operators
	// bind-mount this once and keep sessions/memory/skills across restarts.
	if !strings.Contains(content, `VOLUME [ "/opt/data" ]`) &&
		!strings.Contains(content, `VOLUME ["/opt/data"]`) {
		t.Errorf("Dockerfile must declare VOLUME /opt/data to mirror upstream volume layout")
	}

	// XDG_DATA_HOME=/opt/data is the bridge that steers Gormes's XDG-based
	// paths (checkpoints, skills, plugins, mcp-tokens, logs, learning, …)
	// into the mounted volume. Without it the volume mount would be empty
	// and Gormes would write to /root/.local/share.
	if !strings.Contains(content, "XDG_DATA_HOME=/opt/data") &&
		!strings.Contains(content, `XDG_DATA_HOME="/opt/data"`) {
		t.Errorf("Dockerfile must set ENV XDG_DATA_HOME=/opt/data so Gormes state lands in the volume")
	}

	// GORMES_HOME=/opt/data mirrors the upstream `HERMES_HOME` alias so
	// downstream scripts and docs can reference a single canonical env var.
	if !strings.Contains(content, "GORMES_HOME=/opt/data") &&
		!strings.Contains(content, `GORMES_HOME="/opt/data"`) {
		t.Errorf("Dockerfile must set ENV GORMES_HOME=/opt/data (mirrors upstream HERMES_HOME)")
	}

	// Entrypoint: the mounted-volume bootstrap runs before the binary.
	// Hard-code the install path so `docker run image gormes doctor …`
	// style overrides still flow through the entrypoint.
	if !strings.Contains(content, "/opt/gormes/docker/entrypoint.sh") {
		t.Errorf("Dockerfile ENTRYPOINT must be /opt/gormes/docker/entrypoint.sh")
	}
}

// TestOCIImageEntrypoint guards the bootstrap script that runs inside the
// image: volume-dir layout, XDG_DATA_HOME plumbing, and privilege drop must
// match the upstream contract so Hermes->Gormes migration remains a
// drop-in volume swap.
func TestOCIImageEntrypoint(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	entrypointPath := filepath.Join(gormesRoot, "docker", "entrypoint.sh")

	info, err := os.Stat(entrypointPath)
	if err != nil {
		t.Fatalf("stat %s: %v", entrypointPath, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s mode = %v, want executable", entrypointPath, info.Mode())
	}

	raw, err := os.ReadFile(entrypointPath)
	if err != nil {
		t.Fatalf("read %s: %v", entrypointPath, err)
	}
	content := string(raw)

	if !strings.HasPrefix(content, "#!/bin/bash") && !strings.HasPrefix(content, "#!/usr/bin/env bash") {
		t.Errorf("entrypoint.sh must start with a bash shebang")
	}
	if !strings.Contains(content, "set -e") {
		t.Errorf("entrypoint.sh must `set -e` so bootstrap failures short-circuit before exec")
	}

	// Volume home alias: defaulting GORMES_HOME to /opt/data lets operators
	// override the mount point without rebuilding the image.
	if !strings.Contains(content, `GORMES_HOME="${GORMES_HOME:-/opt/data}"`) {
		t.Errorf("entrypoint.sh must default GORMES_HOME to /opt/data")
	}

	// Gormes's Go runtime keys off XDG_DATA_HOME for every persistent path
	// (checkpoints, skills, plugins, mcp-tokens, learning, logs). Exporting
	// it inside the entrypoint is what actually steers writes into the
	// mounted volume — the Dockerfile ENV alone is not enough once the
	// entrypoint script runs with an override-friendly default.
	if !strings.Contains(content, "export XDG_DATA_HOME") {
		t.Errorf("entrypoint.sh must export XDG_DATA_HOME so Gormes state lands in the volume")
	}

	// Privilege drop via gosu, mirroring the upstream pattern. Required so
	// bind-mounted host volumes stay owned by the host UID.
	if !strings.Contains(content, "gosu gormes") {
		t.Errorf("entrypoint.sh must exec `gosu gormes` to drop root while preserving host UID")
	}
	if !strings.Contains(content, "GORMES_UID") {
		t.Errorf("entrypoint.sh must honor GORMES_UID to remap container UID for bind mounts")
	}

	// Volume layout: the upstream image bootstraps a canonical directory
	// tree so first-run operators find the expected buckets. Gormes
	// mirrors the same shape (no hermes-specific names, XDG-mounted
	// subtree lives under <volume>/gormes/ which the runtime creates
	// on demand — but the operator-visible buckets are pre-created here).
	//
	// The check is anchored against a bootstrap `mkdir -p "$GORMES_HOME"/`
	// line so matches can't false-positive against documentation comments.
	mkdirLine := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mkdir -p") && strings.Contains(trimmed, "$GORMES_HOME") {
			mkdirLine = trimmed
			break
		}
	}
	if mkdirLine == "" {
		t.Fatalf("entrypoint.sh must bootstrap $GORMES_HOME with an `mkdir -p` line before exec")
	}
	requiredDirs := []string{"sessions", "logs", "skills", "plans", "workspace", "home", "cron", "hooks", "memories"}
	for _, d := range requiredDirs {
		if !strings.Contains(mkdirLine, d) {
			t.Errorf("entrypoint.sh mkdir line is missing %q bucket (mirrors upstream layout): %s", d, mkdirLine)
		}
	}

	// Final exec must hand off to the gormes binary, not hermes.
	if !strings.Contains(content, "exec /opt/gormes/bin/gormes") &&
		!strings.Contains(content, "exec gormes") {
		t.Errorf("entrypoint.sh must `exec gormes` after bootstrap to hand control to the Go binary")
	}
}

// TestOCIImageSoul guards the persona file bootstrap. The upstream docker
// image ships a `docker/SOUL.md` so first-run volumes get a working
// persona template; gormes mirrors that so operators don't lose the
// upstream experience.
func TestOCIImageSoul(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	soulPath := filepath.Join(gormesRoot, "docker", "SOUL.md")

	raw, err := os.ReadFile(soulPath)
	if err != nil {
		t.Fatalf("read %s: %v", soulPath, err)
	}
	content := string(raw)

	// The persona file must clearly identify itself as the gormes soul
	// rather than silently mirroring the upstream hermes persona.
	if !strings.Contains(strings.ToLower(content), "gormes") {
		t.Errorf("docker/SOUL.md must mention Gormes — it is the gormes-native persona template")
	}
}

// TestOCIImageDockerignore keeps build-time image size bounded: node_modules,
// the bin/ output directory, and local caches must not leak into the build
// context (which would invalidate cached layers and ship stale binaries).
func TestOCIImageDockerignore(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	ignorePath := filepath.Join(gormesRoot, ".dockerignore")

	raw, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("read %s: %v", ignorePath, err)
	}
	content := string(raw)

	required := []string{".git", "bin"}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Errorf(".dockerignore must include %q to keep the build context lean", token)
		}
	}
}
