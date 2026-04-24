package repoctl

import (
	"os"
	"strings"
	"testing"
)

func TestMakefileUsesRepoctl(t *testing.T) {
	raw, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(raw)

	for _, forbidden := range []string{
		"bash scripts/record-benchmark.sh",
		"bash scripts/record-progress.sh",
		"bash scripts/update-readme.sh",
	} {
		if strings.Contains(makefile, forbidden) {
			t.Fatalf("Makefile still calls %q", forbidden)
		}
	}

	for _, required := range []string{
		"go run ./cmd/repoctl benchmark record",
		"go run ./cmd/repoctl progress sync",
		"go run ./cmd/repoctl readme update",
	} {
		if !strings.Contains(makefile, required) {
			t.Fatalf("Makefile missing %q", required)
		}
	}
}
