package repoctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateReadmeSizeFromBenchmark(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{"size_mb":"16.2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReadme(ReadmeOptions{Root: root}); err != nil {
		t.Fatalf("UpdateReadme: %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
	}
}

func TestUpdateReadmeSizeFromNumericBenchmark(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{"size_mb":16.2}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReadme(ReadmeOptions{Root: root}); err != nil {
		t.Fatalf("UpdateReadme: %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
	}
}

func TestUpdateReadmeSkipsMissingBenchmarks(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	original := "Binary size: ~99.9 MB\n"
	if err := os.WriteFile(readme, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReadme(ReadmeOptions{Root: root}); err != nil {
		t.Fatalf("UpdateReadme: %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Fatalf("README changed:\n%s", raw)
	}
}
