Homebrew packaging notes for Gormes (Go port).

Use `packaging/homebrew/gormes.rb` as a tap or `homebrew-core` starting point.
This formula is the Go-native replacement for the upstream
`packaging/homebrew/hermes-agent.rb` — it builds from source with the Go
toolchain and does not install a Python virtualenv.

## Key choices

- Stable builds target the semver-named source tarball attached to each
  GitHub release (e.g. `gormes-0.2.0.tar.gz`), not the CalVer tag tarball.
  Release tooling in `scripts/release.py` publishes the asset.
- `depends_on "go" => :build` pulls the Go toolchain only at build time;
  the shipped binary is fully static (`CGO_ENABLED=0` + `-trimpath
  -ldflags="-s -w"`) so runtime has no dependency on the host toolchain.
- The formula builds `./cmd/gormes` from the `gormes/` subdirectory of the
  repo (the Go module root) so the same invocation works whether the user
  is building from a tag tarball or a `head` git archive.
- The install step wraps the binary with `write_env_script` to export
  `GORMES_MANAGED=homebrew` and `GORMES_BUNDLED_SOUL=<pkgshare>/SOUL.md`,
  mirroring the upstream `HERMES_MANAGED` contract. Downstream docs and any
  future `gormes update` subcommand can branch on `GORMES_MANAGED` to tell
  operators to run `brew upgrade gormes` instead of self-upgrading.
- The bundled `docker/SOUL.md` persona template is installed alongside the
  binary so first-run brew installs have the same persona seed as the OCI
  image bootstrap.

## Typical update flow

1. Bump the formula `url`, `version`, and `sha256` to the new release asset.
2. If new build-time Go toolchain requirements land, update the
   `depends_on "go" => :build` stanza accordingly.
3. Keep the `GORMES_MANAGED=homebrew` wrapper intact — removing it breaks
   the managed-install contract that future self-update paths depend on.
4. Verify:
   - `brew audit --new --strict gormes`
   - `brew test gormes`
   - Manually run `gormes version` and `gormes doctor` against a clean
     `GORMES_HOME` to confirm no regressions.

## Why not port the upstream Python formula as-is?

The upstream `hermes-agent.rb` uses `Language::Python::Virtualenv` with a
large `resources` list so `brew` pip-installs the Hermes Python package
tree on install. The Go port drops that entire machinery: there is no
Python runtime, no `pypi_packages`, and no `pip_install resources`. The
formula in this directory therefore reads very differently from the
upstream — any bump that would reintroduce Python plumbing should instead
be reconciled against the actual Go module structure.
