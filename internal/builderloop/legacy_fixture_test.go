package builderloop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLegacyShellMarkedVendored(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	if err != nil {
		t.Fatalf("read .gitattributes: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if line == "testdata/legacy-shell/** linguist-vendored" {
			return
		}
	}

	t.Fatal(".gitattributes missing testdata/legacy-shell/** linguist-vendored")
}
