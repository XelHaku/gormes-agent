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
