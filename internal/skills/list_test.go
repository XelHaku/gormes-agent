package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListInstalledSkills_StatusColumnPopulated(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GORMES_SKILLS_ROOT", root)
	writeListSkillDoc(t, root, "x", "hub-skill", "hub", "community")
	writeListSkillDoc(t, root, "x", "builtin-skill", "builtin", "builtin")
	writeListSkillDoc(t, root, "x", "local-skill", "local", "local")

	rows := ListInstalledSkills(ListOptions{}, nil)

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3: %#v", len(rows), rows)
	}
	for _, row := range rows {
		if row.Status != "enabled" {
			t.Fatalf("row %q Status = %q, want enabled", row.Name, row.Status)
		}
	}
}

func TestListInstalledSkills_DisabledRowsCarryDisabledStatus(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GORMES_SKILLS_ROOT", root)
	writeListSkillDoc(t, root, "x", "hub-skill", "hub", "community")
	writeListSkillDoc(t, root, "x", "local-skill", "local", "local")

	rows := ListInstalledSkills(ListOptions{}, map[string]struct{}{"hub-skill": {}})

	got := rowStatuses(rows)
	want := map[string]string{
		"hub-skill":   "disabled",
		"local-skill": "enabled",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %#v, want %#v", got, want)
	}
}

func TestListInstalledSkills_EnabledOnlyFilter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GORMES_SKILLS_ROOT", root)
	writeListSkillDoc(t, root, "x", "hub-skill", "hub", "community")
	writeListSkillDoc(t, root, "x", "builtin-skill", "builtin", "builtin")
	writeListSkillDoc(t, root, "x", "local-skill", "local", "local")

	rows := ListInstalledSkills(ListOptions{EnabledOnly: true}, map[string]struct{}{"hub-skill": {}})

	got := rowNames(rows)
	want := []string{"builtin-skill", "local-skill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
}

func TestListInstalledSkills_SourceFilterRespected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GORMES_SKILLS_ROOT", root)
	writeListSkillDoc(t, root, "x", "hub-skill", "hub", "community")
	writeListSkillDoc(t, root, "x", "builtin-skill", "builtin", "builtin")
	writeListSkillDoc(t, root, "x", "local-skill", "local", "local")

	tests := map[string][]string{
		"hub":     {"hub-skill"},
		"builtin": {"builtin-skill"},
		"local":   {"local-skill"},
	}
	for source, want := range tests {
		t.Run(source, func(t *testing.T) {
			rows := ListInstalledSkills(ListOptions{Source: source}, nil)
			if got := rowNames(rows); !reflect.DeepEqual(got, want) {
				t.Fatalf("names = %#v, want %#v", got, want)
			}
		})
	}
}

func writeListSkillDoc(t *testing.T, root, category, name, source, trust string) {
	t.Helper()
	dir := filepath.Join(root, "active", category, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
	raw := "---\nname: " + name + "\ndescription: " + name + " description\n---\n\nUse " + name + "."
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md): %v", err)
	}
	meta := `{"source":"` + source + `","trust":"` + trust + `"}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(meta.json): %v", err)
	}
}

func rowNames(rows []SkillRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	return out
}

func rowStatuses(rows []SkillRow) map[string]string {
	out := make(map[string]string)
	for _, row := range rows {
		out[row.Name] = string(row.Status)
	}
	return out
}
