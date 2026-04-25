package repoctl

import (
	"os"
	"strings"
	"testing"
)

func TestMakefileUsesAutoloopRepoHelpers(t *testing.T) {
	raw, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(raw)

	for _, forbidden := range []string{
		"bash scripts/record-benchmark.sh",
		"bash scripts/record-progress.sh",
		"bash scripts/update-readme.sh",
		"go run ./cmd/repoctl",
		"go run ./cmd/progress-gen",
	} {
		if strings.Contains(makefile, forbidden) {
			t.Fatalf("Makefile still calls %q", forbidden)
		}
	}

	for _, required := range []string{
		"go run ./cmd/builder-loop repo benchmark record",
		"go run ./cmd/builder-loop progress write",
		"go run ./cmd/builder-loop repo readme update",
	} {
		if !strings.Contains(makefile, required) {
			t.Fatalf("Makefile missing %q", required)
		}
	}
}
