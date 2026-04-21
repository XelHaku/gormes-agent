package internal_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

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

// TestKernelHasNoMessagingSDKDeps guards the Phase 2.B boundary: internal/kernel
// must never transitively import transport SDKs. Discord and Slack stay at the
// adapter edge; if either SDK reaches kernel, channel runtime concerns leaked
// into the shared turn loop.
func TestKernelHasNoMessagingSDKDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./internal/kernel")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "bwmarrin/discordgo") ||
			strings.Contains(d, "slack-go/slack") {
			t.Errorf("internal/kernel transitively depends on %q — messaging SDK leaked into kernel", d)
		}
	}
}
