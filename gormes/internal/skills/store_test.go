package skills

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSkillStoreSnapshotLoadsOnlyActiveSkills(t *testing.T) {
	root := t.TempDir()
	writeSkillDoc(t, filepath.Join(root, "active", "careful-review", "SKILL.md"), "careful-review", "Review carefully", "Follow the review checklist.")
	writeSkillDoc(t, filepath.Join(root, "candidates", "cand-1", "SKILL.md"), "candidate-only", "Should stay inactive", "Do not load me.")

	store := NewStore(root, 8*1024)
	snap, err := store.SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() error = %v", err)
	}
	if len(snap.Skills) != 1 {
		t.Fatalf("len(Skills) = %d, want 1", len(snap.Skills))
	}
	if snap.Skills[0].Name != "careful-review" {
		t.Fatalf("Skills[0].Name = %q, want %q", snap.Skills[0].Name, "careful-review")
	}
}

func TestSkillStoreSnapshotIsImmutableAfterLoad(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "active", "careful-review", "SKILL.md")
	writeSkillDoc(t, path, "careful-review", "Review carefully", "Follow the review checklist.")

	store := NewStore(root, 8*1024)
	snap, err := store.SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() error = %v", err)
	}

	writeSkillDoc(t, path, "careful-review-v2", "Review even more carefully", "Follow the v2 checklist.")

	if got := snap.Skills[0].Description; got != "Review carefully" {
		t.Fatalf("snapshot description mutated to %q, want original value", got)
	}
	if got := snap.Skills[0].Body; got != "Follow the review checklist." {
		t.Fatalf("snapshot body mutated to %q, want original value", got)
	}

	fresh, err := store.SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() fresh error = %v", err)
	}
	if len(fresh.Skills) != 1 || fresh.Skills[0].Name != "careful-review-v2" {
		t.Fatalf("fresh snapshot = %#v, want updated skill", fresh.Skills)
	}
}

func TestRuntimeBuildSkillBlockSelectsAndRendersActiveSkills(t *testing.T) {
	root := t.TempDir()
	writeSkillDoc(t, filepath.Join(root, "active", "careful-review", "SKILL.md"), "careful-review", "Review carefully", "Follow the review checklist.")
	writeSkillDoc(t, filepath.Join(root, "active", "review-tests", "SKILL.md"), "review-tests", "Review tests and failure modes", "Check assertions before implementation.")
	writeSkillDoc(t, filepath.Join(root, "candidates", "cand-1", "SKILL.md"), "candidate-only", "Should stay inactive", "Do not load me.")

	runtime := NewRuntime(root, 8*1024, 2, "")
	block, names, err := runtime.BuildSkillBlock(context.Background(), "please review this carefully and check tests")
	if err != nil {
		t.Fatalf("BuildSkillBlock() error = %v", err)
	}

	wantNames := []string{"review-tests", "careful-review"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("names = %#v, want %#v", names, wantNames)
	}
	wantBlock := "<skills>\n## review-tests\nReview tests and failure modes\n\nCheck assertions before implementation.\n\n## careful-review\nReview carefully\n\nFollow the review checklist.\n</skills>"
	if block != wantBlock {
		t.Fatalf("BuildSkillBlock() = %q, want %q", block, wantBlock)
	}
}

func writeSkillDoc(t *testing.T, path, name, description, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	raw := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
