# Gormes Installer Parity Design

**Status:** Draft
**Author:** Codex
**Date:** 2026-04-23

## Context

Gormes currently exposes a minimal Unix installer at `gormes/www.gormes.ai/internal/site/install.sh`. That script only verifies `go`, checks for Go 1.25+, picks a bin directory, and runs `go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest`.

That is not the installer shape the user wants. The target user experience is the Hermes installer model:

1. One command starts a full managed install.
2. Rerunning that command behaves like an update.
3. The installer owns a managed local checkout.
4. The installer auto-installs core prerequisites where practical.
5. The user ends up with a stable global command.
6. Windows has first-class installer entrypoints instead of a Unix-only dead end.

At the same time, Gormes is not Hermes. The runtime is Go-native, source-backed for now, and should not bootstrap Python/Node donor surfaces just to imitate Hermes. The installer must feel analogous to Hermes while remaining honest about what Gormes actually ships today.

## Goals

1. Replace the current one-shot `go install` flow with a Hermes-style managed installer for Gormes.
2. Support Linux, macOS, Android/Termux, WSL-style Unix shells, and native Windows.
3. Make rerunning install behave like update: reuse the managed checkout, update it, rebuild, and refresh the published `gormes` command.
4. Auto-install core prerequisites where practical, with privileged prompts only when needed.
5. Publish a stable global `gormes` command for the current user.
6. Keep the installer Gormes-only. It must not install Hermes or provision Python donor runtimes.
7. Define a soft-fail summary model for non-core helper steps without weakening core install guarantees.

## Non-goals

- Shipping release-based installers or release artifact download logic. This design stays source-backed until release infrastructure exists.
- Installing Hermes, `api_server`, or any Python bridge runtime.
- Provisioning future roadmap features that are not shipped in Gormes today, such as browser automation, TTS, vision, or image generation.
- Reworking Gormes runtime state layout. This spec only covers installer-managed source/build locations plus command publication.
- Designing package-manager-native distribution channels like Homebrew taps, winget manifests, or apt repos. Those can come later once release artifacts exist.

## Decision Summary

The accepted installer contract is:

1. **Hermes-parity installer shell with Go internals.** Match Hermes installer UX and update semantics, but build Go binaries instead of Python/Node app environments.
2. **Full shipped install.** Install everything required for the currently shipped Gormes experience on that platform, not unshipped roadmap extras.
3. **Soft-fail helpers.** If a non-core helper step fails, finish the core install and print a clear success/skipped/fix summary.
4. **Gormes-only.** Do not install Hermes.
5. **Managed checkout + rerun-as-update.** The installer owns a local checkout and updates it in place on reruns.
6. **Automatic prerequisite install.** Mirror Hermes: auto-install what is practical, ask for `sudo`/admin only when necessary, fall back to manual instructions only if automation fails.
7. **Stable global command.** The installer must make `gormes` globally runnable for the current user.
8. **Hermes-style platform split.** Use Unix `install.sh`, Windows `install.ps1`, and `install.cmd` as the CMD wrapper.
9. **Whole-repo checkout.** Clone the whole repository into the managed install directory and build from its `gormes/` subdirectory.
10. **Hermes-analogy install roots.** Keep a Hermes-style managed install home for source/build ownership rather than using the runtime XDG directories directly.

## Installer Surfaces

The public installer surfaces become:

```text
https://gormes.ai/install.sh
https://gormes.ai/install.ps1
https://gormes.ai/install.cmd
```

Source files live under `gormes/www.gormes.ai/internal/site/` and are embedded into the site binary, analogous to the current embedded `install.sh`.

Expected file additions/changes:

```text
gormes/www.gormes.ai/internal/site/install.sh
gormes/www.gormes.ai/internal/site/install.ps1
gormes/www.gormes.ai/internal/site/install.cmd
gormes/www.gormes.ai/internal/site/assets.go
gormes/www.gormes.ai/internal/site/server.go
gormes/www.gormes.ai/internal/site/assets_test.go
gormes/www.gormes.ai/internal/site/static_export_test.go
gormes/README.md
README.md
```

`install.sh` remains the Unix/Termux entrypoint. On native Windows-like shells (`MINGW`, `MSYS`, `CYGWIN`) it should behave like Hermes and direct the user to the PowerShell installer instead of pretending the Unix path is supported there.

## Installer Interface

The platform entrypoints may use platform-native flag syntax, but they must expose the same logical controls:

1. **target branch** — default `main`
2. **managed install root override** — for advanced users and tests
3. **managed checkout directory override** — for advanced users and tests

The default path should remain the Hermes-analogy managed install location. Overrides are escape hatches, not the primary documented flow.

## Managed Install Layout

### Unix / macOS / Linux / WSL / Termux

Managed installer home:

```text
~/.gormes/
```

Managed checkout root:

```text
~/.gormes/gormes-agent
```

Command publication:

- Standard Unix/macOS/Linux/WSL: `~/.local/bin/gormes`
- Termux: `$PREFIX/bin/gormes`

### Windows

Managed installer home:

```text
%LOCALAPPDATA%\gormes\
```

Managed checkout root:

```text
%LOCALAPPDATA%\gormes\gormes-agent
```

Published command directory:

```text
%LOCALAPPDATA%\gormes\bin\
```

Published command:

```text
%LOCALAPPDATA%\gormes\bin\gormes.exe
```

### Separation From Runtime State

This managed installer home is **not** the same thing as Gormes runtime state. The installer uses Hermes-analogy paths because the user explicitly wants Hermes-like install/update behavior. Gormes runtime config/data can still remain Go-native and XDG-oriented elsewhere in the product.

That distinction avoids conflating:

- installer-managed source/build material, and
- runtime-managed config/session/memory state.

## Core Install Contract

Core install success means all of the following are true:

1. The managed checkout exists and is in a usable state.
2. The required build prerequisites exist.
3. The Gormes binary was built successfully from the managed checkout's `gormes/` subdirectory.
4. The stable global `gormes` command now points at the latest successful build.
5. A lightweight smoke verification passes, such as `gormes version` or `gormes doctor --offline`.

If those five conditions are satisfied, the install succeeds even if non-core helper steps fail.

## What "Full Shipped Install" Means Right Now

The installer should automatically provision everything needed for the currently shipped Gormes path:

1. `git`
2. `go` at the project's supported version floor (`1.25+`)
3. whatever minimal OS tools are needed to clone, build, and publish the binary
4. the managed checkout
5. the built `gormes` command
6. post-build verification

It should **not** attempt to provision Hermes-era optional stacks that Gormes has not shipped in Go yet, including:

- browser automation
- Playwright/browser engines
- TTS / transcription / voice helpers
- image-generation runtimes
- future skill/plugin/MCP ecosystems that are not part of today's shipped path

If any of those become part of the default shipped Gormes path later, they can graduate into the core install set in a later installer iteration.

## Platform Behavior

### Unix `install.sh`

Responsibilities:

1. Detect platform: Linux, macOS, WSL-style Unix shell, or Termux.
2. Reject native Windows shells and direct the user to `install.ps1`.
3. Detect and, when possible, install `git` and `go`.
4. Create or update the managed checkout.
5. Build from `<managed checkout>/gormes`.
6. Publish the stable `gormes` command into the user bin directory.
7. Verify the install.
8. Print a final summary showing:
   - core success status
   - helper steps skipped/failed
   - PATH notes
   - rerun/update hints

#### Unix prerequisite strategy

- **Termux:** use `pkg install` for `git` and `golang`.
- **macOS:** prefer `brew` when present; otherwise fall back to user-facing instructions or a later managed archive path if implementation work chooses to support it.
- **Linux distro with known package manager:** prefer `apt`, `dnf`, or `pacman`, prompting for `sudo` only when needed.
- **Unknown Unix / no successful package-manager path:** present precise manual next steps and stop only if the core install cannot continue.

### Windows `install.ps1`

Responsibilities mirror Unix:

1. Detect/update/install `git` and `go`.
2. Create or update the managed checkout.
3. Build from `<managed checkout>\gormes`.
4. Copy the built binary into `%LOCALAPPDATA%\gormes\bin\gormes.exe`.
5. Ensure `%LOCALAPPDATA%\gormes\bin` is on the user's PATH.
6. Run a smoke verification.
7. Print the same final summary shape as Unix.

#### Windows prerequisite strategy

- Prefer `winget` when available.
- Fall back to `choco` if present.
- If package-manager installation fails, allow direct-download fallback where practical for the managed user-space toolchain path.
- If automation still fails, emit exact manual instructions and fail only if the core install cannot proceed.

### Windows `install.cmd`

This remains a thin CMD wrapper that launches `install.ps1`, analogous to Hermes. Its job is user ergonomics, not install logic.

## Update Model

Rerunning install behaves like update.

### Existing managed checkout

If the managed checkout already exists and is a git repository:

1. enter the checkout
2. detect local modifications
3. preserve local modifications before updating
4. fetch origin
5. switch to the target branch
6. pull/update
7. rebuild
8. republish the stable command only after the rebuild succeeds

### Local modifications

Hermes-style preservation is required. The installer must not blindly wipe local edits in the managed checkout.

Required behavior:

1. If the checkout is dirty, autostash tracked and untracked changes before update.
2. Attempt to reapply them after the repository update.
3. If reapply succeeds, continue and note that local changes were restored.
4. If reapply fails, preserve the stash and fail the update path with clear recovery instructions.
5. Do not break the previously published working `gormes` command when an update fails after a prior successful install.

### Non-git managed directory

If the managed install directory exists but is not a git repository, stop with a clear message telling the user either to remove it or rerun with an explicit managed checkout override.

## Source Acquisition

Because release artifacts do not exist yet, the installer remains source-backed.

Default source acquisition path:

1. try SSH clone
2. fall back to HTTPS clone
3. on Windows, allow a ZIP fallback if git clone is failing for platform-specific file-I/O reasons, then initialize a repo for future updates

That matches Hermes' update/install spirit while still accommodating Windows-specific failure modes.

## Build and Publication Model

### Build root

The installer clones the whole repository, then builds from:

```text
<managed checkout>/gormes
```

Build artifact source path:

```text
<managed checkout>/gormes/bin/gormes
```

### Stable published command

The published command is the user-facing stable path. The installer should treat it as the thing that must remain healthy.

- On Unix, prefer a symlink from the user bin dir to the managed build artifact.
- On Windows, prefer copying the built binary into the published bin dir to avoid symlink privilege issues.

Publication must be atomic enough that a failed build/update does not replace a previously working published binary with a broken one.

## PATH Handling

Hermes-style behavior applies here too:

1. Prefer user-scoped command directories.
2. Detect whether the command directory is already on PATH.
3. If it is not on PATH, print the exact export/addition command.
4. On Windows, update the user's PATH when possible.
5. Treat PATH persistence problems as helper failures, not core install failures, as long as the published binary exists and the installer can show the direct command path.

## Failure Model

### Core failures

The installer must fail when:

- the managed checkout cannot be created or updated
- `git` cannot be obtained and no usable repo exists
- `go` cannot be obtained at a supported version
- the build fails
- command publication fails
- smoke verification fails

### Helper failures

The installer should **not** fail the overall run when a non-core helper step fails. Instead it should finish with a structured summary:

```text
Core install: succeeded
Published command: succeeded
Verification: succeeded
Helpers skipped/failed:
- PATH not updated automatically
- shell completion not installed
How to fix:
- export PATH=...
- rerun installer after ...
```

Right now helper failures are expected to be relatively small, because the design explicitly avoids provisioning unshipped optional ecosystems.

## Verification

Minimum verification contract:

1. Confirm the published `gormes` binary exists.
2. Run `gormes version` or equivalent.
3. Run `gormes doctor --offline` if the current binary supports it in a non-destructive way.

If verification fails, the install/update is considered failed even if the build step technically completed.

## Documentation Surface

Public install docs should be updated to match the new contract:

1. Unix/macOS/Linux/Termux command remains `curl -fsSL https://gormes.ai/install.sh | sh`
2. Windows gets first-class PowerShell guidance and `install.cmd` fallback guidance
3. docs should describe rerun-as-update behavior explicitly
4. docs should explain that the installer is source-backed for now
5. docs should avoid promising Hermes installation or full runtime parity that Gormes does not yet have

## Testing

The implementation should ship with automated coverage for both the website asset surface and the installer decision logic.

### Site/asset tests

Add/extend Go tests to verify:

1. `/install.sh`, `/install.ps1`, and `/install.cmd` are embedded and served.
2. static export writes all installer assets.
3. the landing/install copy references the correct commands and current repo/module path.

### Unix installer tests

The shell installer should be structured into testable functions, with environment-variable seams for:

- detected OS/distro
- package-manager presence
- `git`/`go` presence
- managed checkout root
- published bin dir
- smoke-check commands

Targeted tests should cover:

1. first install on Unix
2. rerun/update path
3. dirty checkout autostash path
4. Termux package-manager flow
5. native Windows-shell redirect message
6. helper-failure summary without core failure

### Windows installer tests

The PowerShell installer should likewise be factored for testability. At minimum, cover:

1. first install
2. rerun/update path
3. PATH publication/update
4. `winget`/`choco`/fallback decision order
5. helper-failure summary without core failure

The specific Windows test harness can be decided in the implementation plan, but the script must be decomposed enough that this coverage is realistic.

## Rollout Notes

This installer redesign is intentionally transitional:

- it gives Gormes a Hermes-like onboarding/update surface immediately
- it keeps the implementation honest by staying source-backed
- it does not tie Gormes to Python/Hermes donor install flows
- it can later swap the source-build phase for release-download logic without changing the user-facing contract much

That makes it the right bridge between today's source-backed Go port and the future pure-Gormes release model.
