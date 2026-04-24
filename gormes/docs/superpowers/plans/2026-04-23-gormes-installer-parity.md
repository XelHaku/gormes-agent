# Gormes Installer Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Gormes' current `go install`-only installer with Hermes-style managed installers for Unix and Windows, including rerun-as-update behavior, stable global command publication, and truthful docs/site coverage.

**Architecture:** Keep the install surface under `gormes/www.gormes.ai/internal/site/` and continue serving embedded installer assets from the landing-page module. The Unix path stays in `install.sh`, Windows gets `install.ps1` plus `install.cmd`, and behavior tests live alongside the site package as Go tests that execute the scripts under temporary fake toolchains. The installer owns a managed checkout under a Hermes-analogy install home and always builds from the repo's `gormes/` subdirectory.

**Tech Stack:** Go 1.25+, POSIX `sh`, PowerShell, `git`, user-scoped PATH publication, Go `httptest`, Go `os/exec`, Playwright for landing-page smoke tests.

**Reference spec:** `gormes/docs/superpowers/specs/2026-04-23-gormes-installer-parity-design.md`

**Baseline commit (spec):** `c7b9cf43`

---

## File Structure

**New files:**

```text
gormes/www.gormes.ai/internal/site/install.ps1
gormes/www.gormes.ai/internal/site/install.cmd
gormes/www.gormes.ai/internal/site/install_unix_test.go
gormes/www.gormes.ai/internal/site/install_windows_test.go
```

**Modified files:**

```text
gormes/www.gormes.ai/internal/site/install.sh
gormes/www.gormes.ai/internal/site/assets.go
gormes/www.gormes.ai/internal/site/server.go
gormes/www.gormes.ai/internal/site/assets_test.go
gormes/www.gormes.ai/internal/site/static_export_test.go
gormes/www.gormes.ai/internal/site/content.go
gormes/www.gormes.ai/tests/home.spec.mjs
gormes/www.gormes.ai/README.md
gormes/README.md
README.md
```

**Responsibility map:**

- `install.sh`: Unix / macOS / Linux / Termux / WSL installer, source-backed, rerun-as-update, user-bin publication.
- `install.ps1`: Native Windows installer with the same contract.
- `install.cmd`: CMD wrapper that launches the PowerShell installer.
- `assets.go`: embed/load/export all installer assets, not just `install.sh`.
- `server.go`: route `/install.sh`, `/install.ps1`, `/install.cmd`.
- `install_unix_test.go`: behavior tests for Unix installer path selection, managed checkout, update flow, publish + verify path.
- `install_windows_test.go`: content/smoke tests for PowerShell + CMD installer surfaces.
- `content.go` + `home.spec.mjs`: public install-section copy and browser assertions.
- `README.md`, `gormes/README.md`, `gormes/www.gormes.ai/README.md`: user-facing install/update guidance.

---

## Conventions Used In Every Task

- Keep installers user-scoped. No root-owned target paths.
- Do not install Hermes, Python, or Node unless a currently shipped Gormes feature truly needs it.
- Preserve a previously working published `gormes` command when an update fails.
- Every installer script must support test mode:
  - Unix: `GORMES_INSTALL_TEST_MODE=1`
  - Windows: `$env:GORMES_INSTALL_TEST_MODE='1'`
- Every task ends with a commit.
- Commit message format:
  - `test(www): ...`
  - `feat(www): ...`
  - `docs(gormes): ...`

---

## Task 1: Generalize Embedded Installer Assets And Routes

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/assets.go`
- Modify: `gormes/www.gormes.ai/internal/site/server.go`
- Modify: `gormes/www.gormes.ai/internal/site/assets_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/static_export_test.go`

- [ ] **Step 1.1: Write the failing Go tests for `/install.ps1` and `/install.cmd`**

Add these test bodies to `gormes/www.gormes.ai/internal/site/assets_test.go`:

```go
func TestServer_ServesPowerShellInstaller(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/install.ps1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"LOCALAPPDATA",
		"gormes-agent",
		"winget",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install.ps1 missing %q\n%s", want, body)
		}
	}
}

func TestServer_ServesCmdWrapper(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/install.cmd", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "install.ps1") {
		t.Fatalf("install.cmd missing PowerShell handoff\n%s", body)
	}
}
```

Extend `gormes/www.gormes.ai/internal/site/static_export_test.go` with:

```go
for _, name := range []string{"install.sh", "install.ps1", "install.cmd"} {
	if _, err := os.Stat(filepath.Join(root, name)); err != nil {
		t.Fatalf("export missing %s: %v", name, err)
	}
}
```

- [ ] **Step 1.2: Run the site-package tests and verify they fail**

Run:

```bash
cd gormes/www.gormes.ai
go test ./internal/site -run 'TestServer_Serves(PowerShellInstaller|CmdWrapper)|TestExportDir_WritesStaticSite' -v
```

Expected:

- `FAIL` because `/install.ps1` and `/install.cmd` are not routed
- `FAIL` because exported `dist/` only contains `install.sh`

- [ ] **Step 1.3: Implement multi-asset embed/load/export and HTTP routes**

Update `gormes/www.gormes.ai/internal/site/assets.go`:

```go
//go:embed templates/*.tmpl templates/partials/*.tmpl static/* install.sh install.ps1 install.cmd
var siteFS embed.FS

type Site struct {
	page       LandingPage
	templates  *template.Template
	static     fs.FS
	installers map[string][]byte
}

func readInstallerAssets() (map[string][]byte, error) {
	assets := map[string][]byte{}
	for _, name := range []string{"install.sh", "install.ps1", "install.cmd"} {
		body, err := siteFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		assets[name] = body
	}
	return assets, nil
}

func (s *Site) InstallScript(name string) []byte {
	return s.installers[name]
}
```

Update `loadSite()` and `ExportDir()` accordingly:

```go
installers, err := readInstallerAssets()
if err != nil {
	return nil, err
}

return &Site{
	page:       DefaultPage(),
	templates:  templates,
	static:     files,
	installers: installers,
}, nil
```

```go
for _, name := range []string{"install.sh", "install.ps1", "install.cmd"} {
	mode := os.FileMode(0o644)
	if name == "install.sh" {
		mode = 0o755
	}
	if err := os.WriteFile(filepath.Join(root, name), s.installers[name], mode); err != nil {
		return err
	}
}
```

Update `gormes/www.gormes.ai/internal/site/server.go`:

```go
func NewServer() (http.Handler, error) {
	site, err := loadSite()
	if err != nil {
		return nil, err
	}

	srv := &Server{site: site}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(site.static)))
	mux.HandleFunc("/install.sh", srv.handleInstall("install.sh", "text/x-shellscript; charset=utf-8"))
	mux.HandleFunc("/install.ps1", srv.handleInstall("install.ps1", "text/plain; charset=utf-8"))
	mux.HandleFunc("/install.cmd", srv.handleInstall("install.cmd", "text/plain; charset=utf-8"))
	mux.HandleFunc("/", srv.handleIndex)
	return mux, nil
}

func (s *Server) handleInstall(name string, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(s.site.InstallScript(name))
	}
}
```

- [ ] **Step 1.4: Re-run the targeted tests and verify they pass**

Run:

```bash
cd gormes/www.gormes.ai
go test ./internal/site -run 'TestServer_Serves(PowerShellInstaller|CmdWrapper|InstallScript)|TestExportDir_WritesStaticSite' -v
```

Expected:

- `PASS` for all targeted installer route/export tests

- [ ] **Step 1.5: Commit**

```bash
git add gormes/www.gormes.ai/internal/site/assets.go \
        gormes/www.gormes.ai/internal/site/server.go \
        gormes/www.gormes.ai/internal/site/assets_test.go \
        gormes/www.gormes.ai/internal/site/static_export_test.go
git commit -m "feat(www): serve all installer assets"
```

---

## Task 2: Refactor `install.sh` Into A Testable Unix Installer Skeleton

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/install.sh`
- Create: `gormes/www.gormes.ai/internal/site/install_unix_test.go`

- [ ] **Step 2.1: Write the failing Unix installer tests**

Create `gormes/www.gormes.ai/internal/site/install_unix_test.go`:

```go
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runInstallSH(t *testing.T, body string, env ...string) (string, error) {
	t.Helper()
	script := filepath.Join(".", "install.sh")
	cmd := exec.Command("sh", "-c", `. "`+script+`"; `+body)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), append([]string{"GORMES_INSTALL_TEST_MODE=1"}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallSH_DefaultManagedPaths(t *testing.T) {
	home := t.TempDir()
	out, err := runInstallSH(t, `printf '%s|%s|%s\n' "$(managed_home_dir)" "$(managed_checkout_dir)" "$(pick_bin_dir)"`, "HOME="+home)
	if err != nil {
		t.Fatalf("runInstallSH: %v\n%s", err, out)
	}
	want := home + "/.gormes|" + home + "/.gormes/gormes-agent|" + home + "/.local/bin"
	if strings.TrimSpace(out) != want {
		t.Fatalf("paths = %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestInstallSH_TermuxUsesPrefixBin(t *testing.T) {
	home := t.TempDir()
	out, err := runInstallSH(t, `printf '%s\n' "$(pick_bin_dir)"`,
		"HOME="+home,
		"PREFIX=/data/data/com.termux/files/usr",
		"TERMUX_VERSION=0.118.0",
	)
	if err != nil {
		t.Fatalf("runInstallSH: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "/data/data/com.termux/files/usr/bin" {
		t.Fatalf("pick_bin_dir = %q", strings.TrimSpace(out))
	}
}

func TestInstallSH_WindowsShellHintMentionsPowerShell(t *testing.T) {
	_, err := runInstallSH(t, `UNAME=MSYS_NT-10.0 check_platform`)
	if err == nil {
		t.Fatal("expected check_platform to fail for Windows-like shell")
	}
}
```

Replace the top of `install.sh` so tests can override `uname`:

```sh
platform_name() {
  if [ -n "${UNAME:-}" ]; then
    printf '%s\n' "$UNAME"
    return
  fi
  uname -s
}
```

- [ ] **Step 2.2: Run the new Unix tests and verify they fail**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstallSH_(DefaultManagedPaths|TermuxUsesPrefixBin|WindowsShellHintMentionsPowerShell)' -v
```

Expected:

- `FAIL` because `managed_home_dir`, `managed_checkout_dir`, and test-mode sourcing do not exist yet

- [ ] **Step 2.3: Implement test-mode gating and core path helpers in `install.sh`**

Rewrite the script skeleton around named functions:

```sh
#!/bin/sh
set -eu

BRANCH="${GORMES_BRANCH:-main}"
INSTALL_HOME="${GORMES_INSTALL_HOME:-${HOME}/.gormes}"
INSTALL_DIR="${GORMES_INSTALL_DIR:-${INSTALL_HOME}/gormes-agent}"

log()  { printf '[gormes] %s\n' "$*" >&2; }
fail() { printf '[gormes] error: %s\n' "$*" >&2; exit 1; }

platform_name() {
  if [ -n "${UNAME:-}" ]; then
    printf '%s\n' "$UNAME"
    return
  fi
  uname -s
}

is_termux() {
  case "${PREFIX:-}" in
    */com.termux/files/usr) return 0 ;;
  esac
  [ -n "${TERMUX_VERSION:-}" ]
}

managed_home_dir() {
  printf '%s\n' "$INSTALL_HOME"
}

managed_checkout_dir() {
  printf '%s\n' "$INSTALL_DIR"
}

pick_bin_dir() {
  if is_termux; then
    printf '%s/bin\n' "${PREFIX:-/data/data/com.termux/files/usr}"
    return
  fi
  printf '%s/.local/bin\n' "$HOME"
}

check_platform() {
  case "$(platform_name)" in
    Linux*|Darwin*) return 0 ;;
    MINGW*|MSYS*|CYGWIN*)
      fail "native Windows shell detected — use PowerShell: irm https://gormes.ai/install.ps1 | iex"
      ;;
    *)
      fail "unsupported OS: $(platform_name)"
      ;;
  esac
}

if [ "${GORMES_INSTALL_TEST_MODE:-0}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi
```

Keep `main()` temporarily minimal in this task; do not implement full install flow yet.

- [ ] **Step 2.4: Re-run the Unix tests and verify they pass**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstallSH_(DefaultManagedPaths|TermuxUsesPrefixBin|WindowsShellHintMentionsPowerShell)' -v
```

Expected:

- `PASS` for the path helper tests
- the Windows-shell test should now fail for the right reason and contain the PowerShell hint

- [ ] **Step 2.5: Commit**

```bash
git add gormes/www.gormes.ai/internal/site/install.sh \
        gormes/www.gormes.ai/internal/site/install_unix_test.go
git commit -m "test(www): add unix installer test seams"
```

---

## Task 3: Implement Unix Managed Install, Update, Publish, And Verify

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/install.sh`
- Modify: `gormes/www.gormes.ai/internal/site/install_unix_test.go`

- [ ] **Step 3.1: Write failing first-install and rerun-update tests**

Append these tests to `install_unix_test.go`:

```go
func TestInstallSH_TermuxPackageInstallCommand(t *testing.T) {
	home := t.TempDir()
	out, err := runInstallSH(t, `printf '%s\n' "$(package_install_command git go)"`,
		"HOME="+home,
		"PREFIX=/data/data/com.termux/files/usr",
		"TERMUX_VERSION=0.118.0",
	)
	if err != nil {
		t.Fatalf("runInstallSH: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "pkg install -y git golang" {
		t.Fatalf("package_install_command = %q", strings.TrimSpace(out))
	}
}

func writeFakeCommand(t *testing.T, dir string, name string, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func TestInstallSH_FirstInstallBuildsAndPublishes(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	fakebin := filepath.Join(root, "fakebin")
	if err := os.MkdirAll(fakebin, 0o755); err != nil {
		t.Fatalf("MkdirAll(fakebin): %v", err)
	}

	writeFakeCommand(t, fakebin, "git", `#!/bin/sh
set -eu
for last; do :; done
case "$1" in
  clone)
    mkdir -p "$last/.git" "$last/gormes/cmd/gormes" "$last/gormes/bin"
    exit 0
    ;;
  fetch|checkout|pull|stash|rev-parse|status)
    [ "$1" = "rev-parse" ] && printf 'refs/stash\n'
    exit 0
    ;;
esac
exit 0
`)

	writeFakeCommand(t, fakebin, "go", `#!/bin/sh
set -eu
if [ "$1" = "env" ] && [ "$2" = "GOVERSION" ]; then
  printf 'go1.25.1\n'
  exit 0
fi
if [ "$1" = "build" ]; then
  out=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then
      out="$2"
      shift 2
      continue
    fi
    shift
  done
  mkdir -p "$(dirname "$out")"
  cat >"$out" <<'EOF'
#!/bin/sh
case "$1" in
  version) printf 'gormes test build\n' ;;
  doctor) exit 0 ;;
  *) exit 0 ;;
esac
EOF
  chmod +x "$out"
  exit 0
fi
exit 1
`)

	script := filepath.Join(".", "install.sh")
	cmd := exec.Command("sh", script)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+fakebin+":"+os.Getenv("PATH"),
		"GORMES_INSTALL_HOME="+filepath.Join(home, ".gormes"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".local", "bin", "gormes")); err != nil {
		t.Fatalf("published gormes missing: %v\n%s", err, out)
	}
}

func TestInstallSH_RerunUpdatesExistingCheckout(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	installHome := filepath.Join(home, ".gormes")
	installDir := filepath.Join(installHome, "gormes-agent")
	fakebin := filepath.Join(root, "fakebin")
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(filepath.Join(installDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}
	if err := os.MkdirAll(fakebin, 0o755); err != nil {
		t.Fatalf("MkdirAll(fakebin): %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(logDir): %v", err)
	}

	writeFakeCommand(t, fakebin, "git", `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$LOG_DIR/git.log"
case "$1" in
  status) exit 0 ;;
  rev-parse) printf 'refs/stash\n'; exit 0 ;;
  fetch|checkout|pull|stash) exit 0 ;;
esac
exit 0
`)

	writeFakeCommand(t, fakebin, "go", `#!/bin/sh
set -eu
if [ "$1" = "env" ] && [ "$2" = "GOVERSION" ]; then
  printf 'go1.25.1\n'
  exit 0
fi
if [ "$1" = "build" ]; then
  out=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then out="$2"; shift 2; continue; fi
    shift
  done
  mkdir -p "$(dirname "$out")"
  printf '#!/bin/sh\nexit 0\n' >"$out"
  chmod +x "$out"
  exit 0
fi
exit 1
`)

	script := filepath.Join(".", "install.sh")
	cmd := exec.Command("sh", script)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+fakebin+":"+os.Getenv("PATH"),
		"LOG_DIR="+logDir,
		"GORMES_INSTALL_HOME="+installHome,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	body, err := os.ReadFile(filepath.Join(logDir, "git.log"))
	if err != nil {
		t.Fatalf("ReadFile(git.log): %v", err)
	}
	for _, want := range []string{"fetch origin", "checkout main", "pull --ff-only origin main"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("git.log missing %q\n%s", want, body)
		}
	}
}
```

- [ ] **Step 3.2: Run the Unix installer behavior tests and verify they fail**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstallSH_(TermuxPackageInstallCommand|FirstInstallBuildsAndPublishes|RerunUpdatesExistingCheckout)' -v
```

Expected:

- `FAIL` because `install.sh` does not yet clone/update/build/publish/verify

- [ ] **Step 3.3: Implement the Unix installer flow in `install.sh`**

Add the core behavior functions:

```sh
detect_distro() {
  if is_termux; then
    printf '%s\n' termux
    return
  fi
  if [ -n "${DISTRO:-}" ]; then
    printf '%s\n' "$DISTRO"
    return
  fi
  if [ -f /etc/os-release ]; then
    DISTRO_ID=$(sed -n 's/^ID=//p' /etc/os-release | tr -d '"')
    [ -n "$DISTRO_ID" ] && { printf '%s\n' "$DISTRO_ID"; return; }
  fi
  case "$(platform_name)" in
    Darwin*) printf '%s\n' macos ;;
    *) printf '%s\n' unknown ;;
  esac
}

package_install_command() {
  distro="$(detect_distro)"
  case "$distro" in
    termux) printf 'pkg install -y git golang\n' ;;
    macos) printf 'brew install git go\n' ;;
    ubuntu|debian) printf 'sudo apt install -y git golang-go\n' ;;
    fedora) printf 'sudo dnf install -y git golang\n' ;;
    arch) printf 'sudo pacman -S --noconfirm git go\n' ;;
    *) return 1 ;;
  esac
}

install_prereqs() {
  distro="$(detect_distro)"
  case "$distro" in
    termux)
      pkg install -y git golang
      ;;
    macos)
      command -v brew >/dev/null 2>&1 || return 1
      brew install git go
      ;;
    ubuntu|debian)
      command -v sudo >/dev/null 2>&1 || return 1
      sudo apt update
      sudo apt install -y git golang-go
      ;;
    fedora)
      command -v sudo >/dev/null 2>&1 || return 1
      sudo dnf install -y git golang
      ;;
    arch)
      command -v sudo >/dev/null 2>&1 || return 1
      sudo pacman -S --noconfirm git go
      ;;
    *)
      return 1
      ;;
  esac
}

check_go_version() {
  goversion=$(go env GOVERSION 2>/dev/null || go version | awk '{print $3}')
  case "$goversion" in
    go1.2[5-9]*|go1.[3-9][0-9]*|go[2-9]*) ;;
    *) fail "Go 1.25+ required today; found ${goversion}" ;;
  esac
}

ensure_git() {
  command -v git >/dev/null 2>&1 && return 0
  install_prereqs || fail "git missing and automatic install failed; try: $(package_install_command git go 2>/dev/null || printf 'install git manually')"
}

ensure_go() {
  command -v go >/dev/null 2>&1 || install_prereqs || fail "go missing and automatic install failed; try: $(package_install_command git go 2>/dev/null || printf 'install Go 1.25+ manually')"
  check_go_version
}

ensure_checkout_parent() {
  mkdir -p "$(managed_home_dir)"
}

prepare_checkout() {
  ensure_checkout_parent
  if [ -d "$(managed_checkout_dir)/.git" ]; then
    cd "$(managed_checkout_dir)"
    autostash_ref=""
    if [ -n "$(git status --porcelain 2>/dev/null || true)" ]; then
      stash_name="gormes-install-autostash-$(date -u +%Y%m%d-%H%M%S)"
      git stash push --include-untracked -m "$stash_name" >/dev/null 2>&1 || true
      autostash_ref="$(git rev-parse --verify refs/stash 2>/dev/null || true)"
    fi
    git fetch origin
    git checkout "$BRANCH"
    git pull --ff-only origin "$BRANCH"
    if [ -n "${autostash_ref:-}" ]; then
      git stash apply "$autostash_ref" >/dev/null 2>&1 || fail "update succeeded but local changes could not be restored"
      git stash drop "$autostash_ref" >/dev/null 2>&1 || true
    fi
    return
  fi
  if [ -e "$(managed_checkout_dir)" ]; then
    fail "managed checkout exists but is not a git repository: $(managed_checkout_dir)"
  fi
  git clone --branch "$BRANCH" https://github.com/TrebuchetDynamics/gormes-agent.git "$(managed_checkout_dir)"
}

build_gormes() {
  cd "$(managed_checkout_dir)/gormes"
  mkdir -p bin
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/gormes ./cmd/gormes
}

publish_binary() {
  BIN_DIR="$(pick_bin_dir)"
  mkdir -p "$BIN_DIR"
  TARGET="$(managed_checkout_dir)/gormes/bin/gormes"
  TMP_LINK="$BIN_DIR/.gormes.tmp.$$"
  ln -sfn "$TARGET" "$TMP_LINK"
  mv -f "$TMP_LINK" "$BIN_DIR/gormes"
}

verify_install() {
  PUBLISHED="$(pick_bin_dir)/gormes"
  [ -x "$PUBLISHED" ] || fail "published gormes command missing: $PUBLISHED"
  "$PUBLISHED" version >/dev/null 2>&1 || fail "gormes version failed"
  "$PUBLISHED" doctor --offline >/dev/null 2>&1 || log "doctor skipped or unavailable in current build"
}

main() {
  check_platform
  ensure_git
  ensure_go
  prepare_checkout
  build_gormes
  publish_binary
  verify_install
  log "installed $(pick_bin_dir)/gormes"
}
```

Keep PATH printing in place after `publish_binary()` if `pick_bin_dir` is not already in `PATH`.

- [ ] **Step 3.4: Re-run the Unix tests and a shell syntax check**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstallSH_(TermuxPackageInstallCommand|FirstInstallBuildsAndPublishes|RerunUpdatesExistingCheckout|DefaultManagedPaths|TermuxUsesPrefixBin|WindowsShellHintMentionsPowerShell)' -v
sh -n install.sh
```

Expected:

- all targeted Unix installer tests `PASS`
- `sh -n install.sh` exits `0`

- [ ] **Step 3.5: Commit**

```bash
git add gormes/www.gormes.ai/internal/site/install.sh \
        gormes/www.gormes.ai/internal/site/install_unix_test.go
git commit -m "feat(www): implement unix managed installer"
```

---

## Task 4: Add Native Windows Installers And Smoke Tests

**Files:**
- Create: `gormes/www.gormes.ai/internal/site/install.ps1`
- Create: `gormes/www.gormes.ai/internal/site/install.cmd`
- Create: `gormes/www.gormes.ai/internal/site/install_windows_test.go`

- [ ] **Step 4.1: Write failing Windows installer tests**

Create `gormes/www.gormes.ai/internal/site/install_windows_test.go`:

```go
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPS1_ContainsManagedInstallContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(".", "install.ps1"))
	if err != nil {
		t.Fatalf("ReadFile(install.ps1): %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`$env:LOCALAPPDATA\gormes`,
		"gormes-agent",
		"winget install Git.Git",
		"winget install GoLang.Go",
		"choco install git",
		"choco install golang",
		"git fetch origin",
		"Copy-Item",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("install.ps1 missing %q", want)
		}
	}
}

func TestInstallCMD_WrapsPowerShellInstaller(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(".", "install.cmd"))
	if err != nil {
		t.Fatalf("ReadFile(install.cmd): %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "install.ps1") {
		t.Fatalf("install.cmd missing install.ps1 handoff")
	}
}

func TestInstallPS1_LoadsInPwshIfPresent(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not installed")
	}
	script := filepath.Join(".", "install.ps1")
	cmd := exec.Command("pwsh", "-NoProfile", "-Command", "$env:GORMES_INSTALL_TEST_MODE='1'; . '"+script+"'; Write-Output (Get-ManagedHome)")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh load failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "gormes") {
		t.Fatalf("pwsh output missing managed home\n%s", out)
	}
}
```

- [ ] **Step 4.2: Run the Windows installer tests and verify they fail**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstall(PS1|CMD)' -v
```

Expected:

- `FAIL` because `install.ps1` and `install.cmd` do not exist yet

- [ ] **Step 4.3: Implement `install.ps1` and `install.cmd`**

Write `gormes/www.gormes.ai/internal/site/install.ps1`:

```powershell
param(
    [string]$Branch = "main",
    [string]$InstallHome = "$env:LOCALAPPDATA\gormes",
    [string]$InstallDir = "$env:LOCALAPPDATA\gormes\gormes-agent"
)

$ErrorActionPreference = "Stop"

function Get-ManagedHome { $InstallHome }
function Get-ManagedCheckoutDir { $InstallDir }
function Get-PublishedBinDir { Join-Path $InstallHome "bin" }

function Ensure-Git {
    if (Get-Command git -ErrorAction SilentlyContinue) { return }
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install Git.Git --silent --accept-package-agreements --accept-source-agreements
        return
    }
    if (Get-Command choco -ErrorAction SilentlyContinue) {
        choco install git -y
        return
    }
    throw "git missing and automatic install failed"
}

function Ensure-Go {
    if (Get-Command go -ErrorAction SilentlyContinue) { return }
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install GoLang.Go --silent --accept-package-agreements --accept-source-agreements
        return
    }
    if (Get-Command choco -ErrorAction SilentlyContinue) {
        choco install golang -y
        return
    }
    throw "go missing and automatic install failed"
}

function Test-GoVersion {
    $version = (& go env GOVERSION 2>$null)
    if (-not $version) { $version = (& go version 2>$null) }
    if ($version -notmatch "go1\.(2[5-9]|[3-9][0-9])|go[2-9]") {
        throw "Go 1.25+ required today; found $version"
    }
}

function Install-Repository {
    if (Test-Path "$InstallDir\.git") {
        Push-Location $InstallDir
        git fetch origin
        git checkout $Branch
        git pull --ff-only origin $Branch
        Pop-Location
        return
    }
    if (Test-Path $InstallDir) {
        throw "managed checkout exists but is not a git repository: $InstallDir"
    }
    git clone --branch $Branch https://github.com/TrebuchetDynamics/gormes-agent.git $InstallDir
}

function Build-Gormes {
    Push-Location (Join-Path $InstallDir "gormes")
    New-Item -ItemType Directory -Force -Path "bin" | Out-Null
    go build -trimpath -ldflags="-s -w" -o "bin\gormes.exe" .\cmd\gormes
    Pop-Location
}

function Publish-Gormes {
    $binDir = Get-PublishedBinDir
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    Copy-Item (Join-Path $InstallDir "gormes\bin\gormes.exe") (Join-Path $binDir "gormes.exe") -Force
}

function Ensure-UserPathContainsBin {
    $binDir = Get-PublishedBinDir
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$binDir*") {
        [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(';') + ";" + $binDir).TrimStart(';'), "User")
    }
}

function Verify-Gormes {
    $published = Join-Path (Get-PublishedBinDir) "gormes.exe"
    if (-not (Test-Path $published)) { throw "published gormes.exe missing" }
    & $published version | Out-Null
}

function Invoke-Main {
    Ensure-Git
    Ensure-Go
    Test-GoVersion
    Install-Repository
    Build-Gormes
    Publish-Gormes
    Ensure-UserPathContainsBin
    Verify-Gormes
}

if (-not $env:GORMES_INSTALL_TEST_MODE) {
    Invoke-Main
}
```

Write `gormes/www.gormes.ai/internal/site/install.cmd`:

```bat
@echo off
setlocal
powershell -ExecutionPolicy ByPass -NoProfile -Command "irm https://gormes.ai/install.ps1 | iex"
if %ERRORLEVEL% NEQ 0 exit /b %ERRORLEVEL%
```

This task must land the actual Windows prerequisite strategy skeleton: prefer `winget`, fall back to `choco`, then fail with a clear manual-install message if neither path works.

- [ ] **Step 4.4: Re-run the Windows installer tests**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
go test -run 'TestInstall(PS1|CMD)' -v
```

Expected:

- `PASS` for content tests
- `PASS` or `SKIP` for the `pwsh` load test depending on local tool availability

- [ ] **Step 4.5: Commit**

```bash
git add gormes/www.gormes.ai/internal/site/install.ps1 \
        gormes/www.gormes.ai/internal/site/install.cmd \
        gormes/www.gormes.ai/internal/site/install_windows_test.go
git commit -m "feat(www): add windows installer assets"
```

---

## Task 5: Update Public Install Copy, Browser Assertions, And READMEs

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/content.go`
- Modify: `gormes/www.gormes.ai/tests/home.spec.mjs`
- Modify: `gormes/www.gormes.ai/internal/site/static_export_test.go`
- Modify: `gormes/www.gormes.ai/README.md`
- Modify: `gormes/README.md`
- Modify: `README.md`

- [ ] **Step 5.1: Write the failing copy/docs assertions**

Update `gormes/www.gormes.ai/internal/site/static_export_test.go`:

```go
wants := []string{
	"curl -fsSL https://gormes.ai/install.sh | sh",
	"irm https://gormes.ai/install.ps1 | iex",
	"Rerun the installer to update",
}
```

Update `gormes/www.gormes.ai/tests/home.spec.mjs`:

```js
await expect(page.getByText('curl -fsSL https://gormes.ai/install.sh | sh')).toBeVisible();
await expect(page.getByText('irm https://gormes.ai/install.ps1 | iex')).toBeVisible();
await expect(page.getByText('Rerun the installer to update the managed Gormes checkout.')).toBeVisible();
await expect(page.locator('button.copy-btn')).toHaveCount(3);
```

Also update the mobile loop assertion from `2` copy buttons to `3`.

- [ ] **Step 5.2: Run the Go + browser tests and verify they fail**

Run:

```bash
cd gormes/www.gormes.ai
go test ./internal/site -run 'TestExportDir_WritesStaticSite' -v
npm run test:e2e -- --grep homepage
```

Expected:

- `FAIL` because the landing page still only shows the Unix installer path
- Playwright copy-button count and Windows command assertions fail

- [ ] **Step 5.3: Implement landing-page copy and README updates**

Update `gormes/www.gormes.ai/internal/site/content.go`:

```go
InstallSteps: []InstallStep{
	{Label: "1. UNIX / TERMUX", Command: "curl -fsSL https://gormes.ai/install.sh | sh"},
	{Label: "2. WINDOWS", Command: "irm https://gormes.ai/install.ps1 | iex"},
	{Label: "3. RUN", Command: "gormes"},
},
InstallFootnote:     "Rerun the installer to update the managed Gormes checkout.",
InstallFootnoteLink: "Source-backed for now.",
InstallFootnoteHref: "https://github.com/TrebuchetDynamics/gormes-agent/tree/main/gormes",
```

Update `gormes/README.md` install block to contain this shape:

```text
## Install

Unix / macOS / Linux / Termux:

curl -fsSL https://gormes.ai/install.sh | sh
gormes doctor --offline
gormes

Windows PowerShell:

irm https://gormes.ai/install.ps1 | iex
gormes doctor --offline
gormes

Rerun the installer to update the managed Gormes checkout.
```

Update root `README.md` quick start to contain this shape:

```text
# Unix / macOS / Linux / Termux
curl -fsSL https://gormes.ai/install.sh | sh

# Windows PowerShell
irm https://gormes.ai/install.ps1 | iex

gormes
```

Update `gormes/www.gormes.ai/README.md` to document that the site now serves `/install.sh`, `/install.ps1`, and `/install.cmd`.

- [ ] **Step 5.4: Re-run the site and browser verification**

Run:

```bash
cd gormes/www.gormes.ai
go test ./internal/site -run 'TestExportDir_WritesStaticSite' -v
npm run test:e2e -- --grep homepage
```

Expected:

- Go static-export test `PASS`
- homepage Playwright test `PASS`

- [ ] **Step 5.5: Commit**

```bash
git add gormes/www.gormes.ai/internal/site/content.go \
        gormes/www.gormes.ai/tests/home.spec.mjs \
        gormes/www.gormes.ai/internal/site/static_export_test.go \
        gormes/www.gormes.ai/README.md \
        gormes/README.md \
        README.md
git commit -m "docs(gormes): document managed installers"
```

---

## Task 6: Final Verification Sweep

**Files:**
- Modify if needed based on failures from the verification run

- [ ] **Step 6.1: Run the focused Go test suite**

Run:

```bash
cd gormes/www.gormes.ai
go test ./internal/site -v
```

Expected:

- `PASS` for route tests, export tests, Unix installer tests, and Windows installer tests

- [ ] **Step 6.2: Run the full landing-page verification**

Run:

```bash
cd gormes/www.gormes.ai
go test ./...
npm run test:e2e
```

Expected:

- all Go tests `PASS`
- Playwright homepage smoke `PASS`

- [ ] **Step 6.3: Run direct script smoke checks**

Run:

```bash
cd gormes/www.gormes.ai/internal/site
sh -n install.sh
if command -v pwsh >/dev/null 2>&1; then
  pwsh -NoProfile -Command "$env:GORMES_INSTALL_TEST_MODE='1'; . ./install.ps1; Write-Host (Get-ManagedHome)"
fi
```

Expected:

- Unix shell syntax check exits `0`
- PowerShell smoke prints a managed `gormes` path when `pwsh` is available

- [ ] **Step 6.4: Commit any verification fixes**

If verification required no code changes, skip this commit. If fixes were needed:

```bash
git add gormes/www.gormes.ai/internal/site \
        gormes/www.gormes.ai/tests \
        gormes/www.gormes.ai/README.md \
        gormes/README.md \
        README.md
git commit -m "test(www): close installer parity verification gaps"
```

---

## Self-Review Checklist

### Spec coverage

- Multi-entrypoint installer surface: Task 1, Task 4, Task 5
- Unix managed install/update behavior: Task 2, Task 3
- Windows first-class installer support: Task 1, Task 4, Task 5
- Stable global command + rerun-as-update: Task 3, Task 4
- Automatic prerequisite install: Task 3, Task 4
- Soft-fail helper philosophy: baked into script structure in Task 3/4, documented in Task 5
- Gormes-only truthfulness + no Hermes install: Task 5 docs/copy

### Placeholder scan

- No `TODO` / `TBD`
- Every task has exact file paths
- Every code-changing step includes concrete code blocks
- Every test step includes exact commands

### Type / API consistency

- Unix script helper names: `managed_home_dir`, `managed_checkout_dir`, `pick_bin_dir`, `check_platform`, `prepare_checkout`, `build_gormes`, `publish_binary`, `verify_install`
- PowerShell helper names: `Get-ManagedHome`, `Get-ManagedCheckoutDir`, `Get-PublishedBinDir`, `Install-Repository`, `Build-Gormes`, `Publish-Gormes`, `Verify-Gormes`
