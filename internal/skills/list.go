package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SkillStatusEnabled SkillStatusCode = "enabled"
)

type SkillRow struct {
	Name     string
	Category string
	Source   string
	Trust    string
	Path     string
	Status   SkillStatusCode
}

type ListOptions struct {
	Source      string
	EnabledOnly bool
}

type skillListMeta struct {
	Category string `json:"category"`
	Source   string `json:"source"`
	Trust    string `json:"trust"`
}

func ListInstalledSkills(opts ListOptions, disabled map[string]struct{}) []SkillRow {
	rows := installedSkillRows()
	source := normalizedListSource(opts.Source)
	disabledNames := normalizedDisabledSet(disabled)

	out := make([]SkillRow, 0, len(rows))
	for _, row := range rows {
		if source != "all" && row.Source != source {
			continue
		}
		if _, ok := disabledNames[strings.ToLower(strings.TrimSpace(row.Name))]; ok {
			row.Status = SkillStatusDisabled
		} else {
			row.Status = SkillStatusEnabled
		}
		if opts.EnabledOnly && row.Status == SkillStatusDisabled {
			continue
		}
		out = append(out, row)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func installedSkillRows() []SkillRow {
	store := NewStore(defaultSkillsRoot(), 0)
	snapshot, err := store.SnapshotActive()
	if err != nil {
		return nil
	}

	rows := make([]SkillRow, 0, len(snapshot.Skills))
	for _, skill := range snapshot.Skills {
		row := SkillRow{
			Name: skill.Name,
			Path: skill.Path,
		}
		meta := readSkillListMeta(skill.Path)
		row.Category = firstNonBlank(meta.Category, categoryFromSkillPath(store.ActiveDir(), skill.Path))
		row.Source = normalizeInstalledSource(firstNonBlank(meta.Source, sourceFromSkillPath(store.ActiveDir(), skill.Path)))
		row.Trust = firstNonBlank(meta.Trust, defaultTrustForSource(row.Source))
		rows = append(rows, row)
	}
	return rows
}

func readSkillListMeta(skillPath string) skillListMeta {
	if skillPath == "" {
		return skillListMeta{}
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(skillPath), "meta.json"))
	if err != nil {
		return skillListMeta{}
	}
	var meta skillListMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return skillListMeta{}
	}
	return meta
}

func defaultSkillsRoot() string {
	if root := strings.TrimSpace(os.Getenv("GORMES_SKILLS_ROOT")); root != "" {
		return root
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "gormes", "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".local", "share", "gormes", "skills")
	}
	return filepath.Join(home, ".local", "share", "gormes", "skills")
}

func categoryFromSkillPath(activeDir, skillPath string) string {
	relDir := relativeSkillDir(activeDir, skillPath)
	if relDir == "" || relDir == "." {
		return ""
	}
	parts := splitPath(relDir)
	if len(parts) <= 1 {
		return ""
	}
	return filepath.Join(parts[:len(parts)-1]...)
}

func sourceFromSkillPath(activeDir, skillPath string) string {
	parts := splitPath(relativeSkillDir(activeDir, skillPath))
	if len(parts) > 1 {
		return parts[0]
	}
	return "local"
}

func relativeSkillDir(activeDir, skillPath string) string {
	if activeDir == "" || skillPath == "" {
		return ""
	}
	rel, err := filepath.Rel(activeDir, filepath.Dir(skillPath))
	if err != nil {
		return ""
	}
	return rel
}

func splitPath(path string) []string {
	if path == "" || path == "." {
		return nil
	}
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == filepath.Separator
	})
	out := parts[:0]
	for _, part := range parts {
		if part != "" && part != "." {
			out = append(out, part)
		}
	}
	return out
}

func normalizedDisabledSet(disabled map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(disabled))
	for name := range disabled {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

func normalizedListSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return "all"
	}
	switch source {
	case "all", "hub", "builtin", "local":
		return source
	default:
		return source
	}
}

func normalizeInstalledSource(source string) string {
	source = normalizedListSource(source)
	switch source {
	case "hub", "builtin", "local":
		return source
	default:
		return "local"
	}
}

func defaultTrustForSource(source string) string {
	switch source {
	case "hub":
		return "community"
	case "builtin":
		return "builtin"
	default:
		return "local"
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
