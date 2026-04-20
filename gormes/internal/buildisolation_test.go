package internal_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestTUIBinaryHasNoTelegramDep guards the Operational Moat: cmd/gormes
// (the TUI) must never transitively depend on telegram-bot-api or on the
// internal/telegram adapter package. If either appears in the TUI's dep
// graph, the binary size jumps and the per-binary-per-platform promise
// breaks.
//
// Runs `go list -deps ./cmd/gormes` from the gormes module root and
// inspects every dependency path.
func TestTUIBinaryHasNoTelegramDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cmd/gormes")
	cmd.Dir = ".." // run from gormes/ so ./cmd/gormes resolves
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	deps := strings.Split(out.String(), "\n")
	for _, d := range deps {
		if strings.Contains(d, "go-telegram-bot-api") ||
			strings.Contains(d, "/internal/telegram") {
			t.Errorf("cmd/gormes transitively depends on %q — Operational Moat violated", d)
		}
	}
}

// TestKernelHasNoSessionDep guards the Phase 2.C boundary: internal/kernel
// must never transitively import internal/session or go.etcd.io/bbolt.
// If either appears in the kernel's dep graph, persistence has leaked into
// the turn-loop and the single-owner isolation is compromised.
func TestKernelHasNoSessionDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./internal/kernel")
	cmd.Dir = ".." // run from gormes/
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "go.etcd.io/bbolt") ||
			strings.Contains(d, "/internal/session") {
			t.Errorf("internal/kernel transitively depends on %q — Phase 2.C isolation violated", d)
		}
	}
}

// TestKernelHasNoMemoryDep guards the Phase 3.A boundary: internal/kernel
// must never transitively import internal/memory or github.com/ncruces/go-sqlite3.
// If either appears in the kernel's dep graph, persistence has leaked into
// the turn loop and the 250ms StoreAckDeadline is structurally at risk.
func TestKernelHasNoMemoryDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./internal/kernel")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "ncruces/go-sqlite3") ||
			strings.Contains(d, "/internal/memory") {
			t.Errorf("internal/kernel transitively depends on %q — Phase 3.A isolation violated", d)
		}
	}
}

// TestTUIBinaryHasNoSqliteDep guards the Operational Moat: cmd/gormes
// (the TUI) must never transitively depend on ncruces/go-sqlite3 or on
// the internal/memory package. If either appears, the TUI's <10 MB
// binary budget is at risk and the dual-store architecture is breached.
func TestTUIBinaryHasNoSqliteDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cmd/gormes")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "ncruces/go-sqlite3") ||
			strings.Contains(d, "/internal/memory") {
			t.Errorf("cmd/gormes transitively depends on %q — Dual-Store Architecture violated", d)
		}
	}
}
