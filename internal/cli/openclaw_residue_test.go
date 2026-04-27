package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectOpenClawResidue_DirectoryOnly(t *testing.T) {
	home := t.TempDir()

	if DetectOpenClawResidue(home) {
		t.Fatalf("DetectOpenClawResidue(%q) = true before .openclaw exists, want false", home)
	}

	openclawPath := filepath.Join(home, ".openclaw")
	if err := os.WriteFile(openclawPath, []byte("not a workspace"), 0o600); err != nil {
		t.Fatalf("write regular .openclaw file: %v", err)
	}
	if DetectOpenClawResidue(home) {
		t.Fatalf("DetectOpenClawResidue(%q) = true for regular .openclaw file, want false", home)
	}

	if err := os.Remove(openclawPath); err != nil {
		t.Fatalf("remove regular .openclaw file: %v", err)
	}
	if err := os.Mkdir(openclawPath, 0o700); err != nil {
		t.Fatalf("create .openclaw directory: %v", err)
	}
	if !DetectOpenClawResidue(home) {
		t.Fatalf("DetectOpenClawResidue(%q) = false for .openclaw directory, want true", home)
	}
}

func TestDetectOpenClawResidue_UnreadableHomeReturnsFalse(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0); err != nil {
		t.Fatalf("make home unreadable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(home, 0o700); err != nil {
			t.Fatalf("restore home permissions: %v", err)
		}
	})

	if DetectOpenClawResidue(home) {
		t.Fatalf("DetectOpenClawResidue(%q) = true for unreadable home, want false", home)
	}
}

func TestOpenClawResidueHint_MentionsInjectedCleanupCommand(t *testing.T) {
	const cleanupCommand = "gormes openclaw cleanup --archive"

	got := OpenClawResidueHint(cleanupCommand)
	for _, want := range []string{"~/.openclaw", cleanupCommand} {
		if !strings.Contains(got, want) {
			t.Fatalf("OpenClawResidueHint(%q) missing %q:\n%s", cleanupCommand, want, got)
		}
	}
	if strings.Contains(got, "hermes claw cleanup") {
		t.Fatalf("OpenClawResidueHint(%q) mentions Hermes cleanup command:\n%s", cleanupCommand, got)
	}
}
