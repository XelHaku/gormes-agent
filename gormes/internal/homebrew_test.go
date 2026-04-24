package internal_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHomebrewFormula guards the Phase 5.P "Homebrew" seam: the Go port
// must ship its own Homebrew formula at `packaging/homebrew/gormes.rb` that
// builds the gormes binary from source with the Go toolchain — not a Python
// virtualenv like the upstream `hermes-agent.rb`. The invariants below lock
// the packaging contract so `brew install gormes` stays a thin wrapper over
// the same `./cmd/gormes` build the Makefile and Dockerfile already use.
func TestHomebrewFormula(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	formulaPath := filepath.Join(gormesRoot, "packaging", "homebrew", "gormes.rb")

	raw, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("read %s: %v", formulaPath, err)
	}
	content := string(raw)

	// Formula class + base: Homebrew loads formulae by class name, and the
	// gormes-native formula must be named `Gormes` so the tap can install as
	// `brew install gormes` rather than the upstream `hermes-agent`.
	if !strings.Contains(content, "class Gormes < Formula") {
		t.Errorf("gormes.rb must declare `class Gormes < Formula` (gormes-native, not hermes-agent)")
	}

	// Identity: homepage + desc + license are required by `brew audit --strict`
	// for any formula that wants to ship via a tap or homebrew-core.
	for _, need := range []string{"desc ", "homepage ", "license "} {
		if !strings.Contains(content, need) {
			t.Errorf("gormes.rb must declare %s line for brew audit --strict compliance", strings.TrimSpace(need))
		}
	}

	// Source: the formula must point at the TrebuchetDynamics/gormes-agent
	// release assets so bumps flow through the same GitHub release pipeline
	// the OCI image already uses. We assert on the repo path, not a specific
	// tag, because the url will be rev-bumped every release.
	if !strings.Contains(content, "TrebuchetDynamics/gormes-agent") {
		t.Errorf("gormes.rb `url` must point at the TrebuchetDynamics/gormes-agent release assets")
	}

	// Go-native build: the Go port replaces the upstream Python virtualenv
	// plumbing. `depends_on \"go\" => :build` pulls the toolchain only at
	// build time, matching the Makefile's host-agnostic contract.
	if !strings.Contains(content, `depends_on "go" => :build`) {
		t.Errorf(`gormes.rb must declare depends_on "go" => :build for a Go-native formula`)
	}

	// Explicitly forbid the upstream Python virtualenv plumbing — if someone
	// copy-pastes `hermes-agent.rb` without porting it, brew will happily
	// install a broken formula that pip-installs nothing. These guards catch
	// that regression at the formula-review stage.
	for _, forbidden := range []string{
		"Language::Python::Virtualenv",
		"virtualenv_create",
		"pip_install",
		"pypi_packages",
		"python@3",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("gormes.rb must not contain upstream Python plumbing %q — this is the Go port", forbidden)
		}
	}

	// Static-binary build: CGO_ENABLED=0 matches the Makefile's
	// `-trimpath -ldflags=-s -w` static-binary contract so brew-installed
	// gormes behaves identically to `make build`.
	if !strings.Contains(content, "CGO_ENABLED") {
		t.Errorf("gormes.rb must set CGO_ENABLED to build a static gormes binary (matches Makefile)")
	}
	if !strings.Contains(content, `system "go", "build"`) {
		t.Errorf(`gormes.rb must invoke system "go", "build" so the formula builds via the Go toolchain`)
	}
	if !strings.Contains(content, "./cmd/gormes") {
		t.Errorf("gormes.rb must build `./cmd/gormes` (the operator entry point, not a subpackage)")
	}

	// Managed-install contract: mirrors the upstream `HERMES_MANAGED` env.
	// The wrapper exports `GORMES_MANAGED=homebrew` so runtime + docs can
	// detect managed installs and route upgrades through `brew upgrade`
	// rather than a self-update path.
	if !strings.Contains(content, "GORMES_MANAGED") {
		t.Errorf("gormes.rb wrapper must export GORMES_MANAGED (mirrors upstream HERMES_MANAGED contract)")
	}
	if !strings.Contains(content, `"homebrew"`) {
		t.Errorf("gormes.rb must set GORMES_MANAGED=\"homebrew\" so the runtime can identify managed installs")
	}
	if !strings.Contains(content, "write_env_script") {
		t.Errorf("gormes.rb must wrap the binary with write_env_script to pin GORMES_MANAGED")
	}

	// The wrapped binary must land under `bin/gormes` so users invoke
	// `gormes` (not `gormes-agent` or `hermes`). The upstream wrapper
	// iterated over multiple legacy names; the Go port has exactly one.
	if !strings.Contains(content, "bin/gormes") && !strings.Contains(content, `bin/"gormes"`) {
		t.Errorf("gormes.rb must install its user-facing wrapper at `bin/gormes`")
	}

	// Smoke test: `brew test gormes` runs the `test do` block. The upstream
	// assertion checks `hermes version`; the Go port checks `gormes version`
	// and expects the binary to print something containing "gormes" so the
	// cobra wiring in `cmd/gormes/version.go` stays load-bearing.
	if !strings.Contains(content, "test do") {
		t.Errorf("gormes.rb must declare a `test do` block for brew test")
	}
	if !strings.Contains(content, "gormes version") {
		t.Errorf("gormes.rb test block must invoke `gormes version` to smoke-test the binary")
	}
}

// TestHomebrewFormulaReadme guards the operator runbook that ships alongside
// the formula. Without it, future maintainers have no single place to look
// for "how do I bump this?" — the upstream README documents the release
// flow and the gormes port must mirror that so the packaging handoff stays
// self-service.
func TestHomebrewFormulaReadme(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	gormesRoot := filepath.Dir(filepath.Dir(file))
	readmePath := filepath.Join(gormesRoot, "packaging", "homebrew", "README.md")

	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	content := string(raw)

	// The README must point at the right formula file so operators don't
	// accidentally edit the upstream hermes formula when bumping gormes.
	if !strings.Contains(content, "gormes.rb") {
		t.Errorf("packaging/homebrew/README.md must reference gormes.rb (the formula file) by name")
	}

	// The README must reference the brew audit + brew test loop so the
	// release runbook is captured in one place.
	if !strings.Contains(content, "brew audit") {
		t.Errorf("packaging/homebrew/README.md must document the `brew audit` verification step")
	}
	if !strings.Contains(content, "brew test") {
		t.Errorf("packaging/homebrew/README.md must document the `brew test` verification step")
	}

	// GORMES_MANAGED is the managed-install contract — if the README doesn't
	// mention it, future maintainers could drop the env wrapper during a
	// refresh and silently break the contract.
	if !strings.Contains(content, "GORMES_MANAGED") {
		t.Errorf("packaging/homebrew/README.md must document the GORMES_MANAGED contract so the env wrapper survives future bumps")
	}
}
