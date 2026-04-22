package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDraftCandidateWritesInactiveArtifact(t *testing.T) {
	store := NewStore(t.TempDir(), 8*1024)

	meta, err := store.DraftCandidate(CandidateDraft{
		Slug:         "delegate-review",
		Goal:         "review the runtime changes",
		Summary:      "review completed successfully",
		SourceRunID:  "sa_123",
		ChildAgentID: "sa_123",
		ToolNames:    []string{"echo", "now"},
	})
	if err != nil {
		t.Fatalf("DraftCandidate() error = %v", err)
	}
	if meta.Status != CandidateStatusCandidate {
		t.Fatalf("Status = %q, want %q", meta.Status, CandidateStatusCandidate)
	}
	if meta.CandidateID == "" {
		t.Fatal("CandidateID is empty")
	}

	raw, err := os.ReadFile(filepath.Join(store.CandidateDir(), meta.CandidateID, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(SKILL.md): %v", err)
	}
	skill, err := Parse(raw, 8*1024)
	if err != nil {
		t.Fatalf("Parse(candidate SKILL.md): %v", err)
	}
	if skill.Name != "delegate-review" {
		t.Fatalf("candidate skill name = %q, want %q", skill.Name, "delegate-review")
	}

	gotMeta := readCandidateMeta(t, filepath.Join(store.CandidateDir(), meta.CandidateID, "meta.json"))
	if gotMeta.SourceRunID != "sa_123" {
		t.Fatalf("SourceRunID = %q, want %q", gotMeta.SourceRunID, "sa_123")
	}
	if len(gotMeta.ToolNames) != 2 {
		t.Fatalf("ToolNames = %#v, want 2 names", gotMeta.ToolNames)
	}
}

func TestStorePromoteCandidateMakesSkillVisibleOnNextSnapshot(t *testing.T) {
	store := NewStore(t.TempDir(), 8*1024)

	meta, err := store.DraftCandidate(CandidateDraft{
		Slug:         "delegate-review",
		Goal:         "review the runtime changes",
		Summary:      "review completed successfully",
		SourceRunID:  "sa_456",
		ChildAgentID: "sa_456",
		ToolNames:    []string{"echo"},
	})
	if err != nil {
		t.Fatalf("DraftCandidate() error = %v", err)
	}

	before, err := store.SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() before promotion: %v", err)
	}
	if len(before.Skills) != 0 {
		t.Fatalf("len(before.Skills) = %d, want 0", len(before.Skills))
	}

	active, err := store.PromoteCandidate(meta.CandidateID)
	if err != nil {
		t.Fatalf("PromoteCandidate() error = %v", err)
	}
	if active.SourceCandidateID != meta.CandidateID {
		t.Fatalf("SourceCandidateID = %q, want %q", active.SourceCandidateID, meta.CandidateID)
	}
	if active.Status != ActiveStatus {
		t.Fatalf("active status = %q, want %q", active.Status, ActiveStatus)
	}

	after, err := store.SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() after promotion: %v", err)
	}
	if len(after.Skills) != 1 {
		t.Fatalf("len(after.Skills) = %d, want 1", len(after.Skills))
	}
	if after.Skills[0].Name != "delegate-review" {
		t.Fatalf("after.Skills[0].Name = %q, want %q", after.Skills[0].Name, "delegate-review")
	}

	gotCandidateMeta := readCandidateMeta(t, filepath.Join(store.CandidateDir(), meta.CandidateID, "meta.json"))
	if gotCandidateMeta.Status != CandidateStatusPromoted {
		t.Fatalf("candidate status = %q, want %q", gotCandidateMeta.Status, CandidateStatusPromoted)
	}

	gotActiveMeta := readActiveMeta(t, filepath.Join(store.ActiveDir(), "delegate-review", "meta.json"))
	if gotActiveMeta.SourceCandidateID != meta.CandidateID {
		t.Fatalf("SourceCandidateID = %q, want %q", gotActiveMeta.SourceCandidateID, meta.CandidateID)
	}
	if gotActiveMeta.Status != ActiveStatus {
		t.Fatalf("active status = %q, want %q", gotActiveMeta.Status, ActiveStatus)
	}
}

func readCandidateMeta(t *testing.T, path string) CandidateMetadata {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	var meta CandidateMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", path, err)
	}
	return meta
}

func readActiveMeta(t *testing.T, path string) ActiveMetadata {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	var meta ActiveMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", path, err)
	}
	return meta
}
