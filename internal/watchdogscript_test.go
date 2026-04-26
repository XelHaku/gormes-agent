package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOrchestratorWatchdogRestartsInactiveServiceAndRunsChecks(t *testing.T) {
	repoRoot := testRepoRoot(t)
	tmpRepo := t.TempDir()
	copyFile(t,
		filepath.Join(repoRoot, "scripts", "orchestrator", "watchdog.sh"),
		filepath.Join(tmpRepo, "scripts", "orchestrator", "watchdog.sh"),
		0o755,
	)

	logDir := filepath.Join(tmpRepo, "logs")
	binDir := installWatchdogFakeBin(t, tmpRepo)
	cmd := exec.Command("bash", "scripts/orchestrator/watchdog.sh")
	cmd.Dir = tmpRepo
	cmd.Env = overlayEnv(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+filepath.Join(tmpRepo, "home"),
		"XDG_RUNTIME_DIR="+filepath.Join(tmpRepo, "runtime"),
		"WATCHDOG_LOG="+logDir,
		"WATCHDOG_SERVICE_STATE=inactive",
		"WATCHDOG_GIT_DIRTY=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("watchdog failed: %v\noutput:\n%s", err, string(out))
	}

	systemctlLog := readOptionalFile(t, filepath.Join(logDir, "systemctl"))
	for _, want := range []string{
		"--user is-active --quiet gormes-orchestrator.service",
		"--user reset-failed gormes-orchestrator.service",
		"--user restart gormes-orchestrator.service",
	} {
		if !strings.Contains(systemctlLog, want) {
			t.Fatalf("systemctl log = %q, want %q", systemctlLog, want)
		}
	}

	goLog := readOptionalFile(t, filepath.Join(logDir, "go"))
	for _, want := range []string{
		"run ./cmd/builder-loop doctor",
		"run ./cmd/planner-loop doctor",
		"run ./cmd/builder-loop audit",
	} {
		if !strings.Contains(goLog, want) {
			t.Fatalf("go log = %q, want %q", goLog, want)
		}
	}

	gitLog := readOptionalFile(t, filepath.Join(logDir, "git"))
	for _, want := range []string{
		"status --porcelain",
		"add -A",
		"commit -m",
	} {
		if !strings.Contains(gitLog, want) {
			t.Fatalf("git log = %q, want %q", gitLog, want)
		}
	}
}

func TestOrchestratorWatchdogDoesNotRestartActiveService(t *testing.T) {
	repoRoot := testRepoRoot(t)
	tmpRepo := t.TempDir()
	copyFile(t,
		filepath.Join(repoRoot, "scripts", "orchestrator", "watchdog.sh"),
		filepath.Join(tmpRepo, "scripts", "orchestrator", "watchdog.sh"),
		0o755,
	)

	logDir := filepath.Join(tmpRepo, "logs")
	binDir := installWatchdogFakeBin(t, tmpRepo)
	cmd := exec.Command("bash", "scripts/orchestrator/watchdog.sh")
	cmd.Dir = tmpRepo
	cmd.Env = overlayEnv(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+filepath.Join(tmpRepo, "home"),
		"XDG_RUNTIME_DIR="+filepath.Join(tmpRepo, "runtime"),
		"WATCHDOG_LOG="+logDir,
		"WATCHDOG_SERVICE_STATE=active",
		"WATCHDOG_GIT_DIRTY=",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("watchdog failed: %v\noutput:\n%s", err, string(out))
	}

	systemctlLog := readOptionalFile(t, filepath.Join(logDir, "systemctl"))
	if strings.Contains(systemctlLog, "--user restart gormes-orchestrator.service") ||
		strings.Contains(systemctlLog, "--user reset-failed gormes-orchestrator.service") {
		t.Fatalf("systemctl log = %q, want no restart/reset for active service", systemctlLog)
	}
	if !strings.Contains(systemctlLog, "--user is-active --quiet gormes-orchestrator.service") {
		t.Fatalf("systemctl log = %q, want active check", systemctlLog)
	}
}

func installWatchdogFakeBin(t *testing.T, repo string) string {
	t.Helper()
	binDir := filepath.Join(repo, "bin")
	writeFile(t, filepath.Join(binDir, "systemctl"), []byte(`#!/usr/bin/env sh
set -eu
mkdir -p "$WATCHDOG_LOG"
printf '%s\n' "$*" >> "$WATCHDOG_LOG/systemctl"
if [ "$*" = "--user is-active --quiet gormes-orchestrator.service" ]; then
  if [ "${WATCHDOG_SERVICE_STATE:-active}" = "active" ]; then
    exit 0
  fi
  exit 3
fi
exit 0
`), 0o755)
	writeFile(t, filepath.Join(binDir, "go"), []byte(`#!/usr/bin/env sh
set -eu
mkdir -p "$WATCHDOG_LOG"
printf '%s\n' "$*" >> "$WATCHDOG_LOG/go"
exit 0
`), 0o755)
	writeFile(t, filepath.Join(binDir, "git"), []byte(`#!/usr/bin/env sh
set -eu
mkdir -p "$WATCHDOG_LOG"
printf '%s\n' "$*" >> "$WATCHDOG_LOG/git"
case "$*" in
  "rev-parse --is-inside-work-tree")
    exit 0
    ;;
  "status --porcelain")
    if [ -n "${WATCHDOG_GIT_DIRTY:-}" ]; then
      printf ' M docs/content/building-gormes/architecture_plan/progress.json\n'
    fi
    exit 0
    ;;
esac
exit 0
`), 0o755)
	return binDir
}

func readOptionalFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err == nil {
		return string(raw)
	}
	if os.IsNotExist(err) {
		return ""
	}
	t.Fatalf("ReadFile(%s) error = %v", path, err)
	return ""
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	return filepath.Dir(filepath.Dir(file))
}
