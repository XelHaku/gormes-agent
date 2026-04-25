package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse converts a SKILL.md document into a typed Skill.
func Parse(raw []byte, maxBytes int) (Skill, error) {
	doc := string(raw)
	if maxBytes > 0 && len(raw) > maxBytes {
		return Skill{}, fmt.Errorf("skill document too large: %d > %d bytes", len(raw), maxBytes)
	}

	doc = strings.TrimPrefix(doc, "\ufeff")
	doc = strings.ReplaceAll(doc, "\r\n", "\n")

	lines := strings.Split(doc, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Skill{}, fmt.Errorf("skill frontmatter must start with ---")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return Skill{}, fmt.Errorf("skill frontmatter closing --- not found")
	}

	var skill Skill
	skill.RawBytes = len(raw)

	frontmatter := parseFrontmatter(strings.Join(lines[1:end], "\n"))
	skill.Name = frontmatterString(frontmatter, "name")
	skill.Description = frontmatterString(frontmatter, "description")
	skill.Platforms = frontmatterStringList(frontmatter["platforms"])
	skill.RequiredEnvVars = requiredEnvVars(frontmatter)

	skill.Body = strings.Trim(strings.Join(lines[end+1:], "\n"), "\n")
	if err := skill.Validate(maxBytes); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

func trimScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func parseFrontmatter(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := yaml.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}

	for _, line := range strings.Split(raw, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = trimScalar(value)
	}
	return out
}

func frontmatterString(frontmatter map[string]any, key string) string {
	value, ok := frontmatter[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func frontmatterStringList(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = appendStringValue(out, item)
		}
		return dedupeStrings(out)
	case []string:
		return dedupeStrings(v)
	case string:
		return parseInlineStringList(v)
	default:
		return appendStringValue(nil, v)
	}
}

func requiredEnvVars(frontmatter map[string]any) []string {
	var out []string
	out = append(out, frontmatterStringList(frontmatter["required_environment_variables"])...)
	if prereqs, ok := frontmatter["prerequisites"].(map[string]any); ok {
		out = append(out, frontmatterStringList(prereqs["env_vars"])...)
	}
	return dedupeStrings(out)
}

func appendStringValue(out []string, value any) []string {
	switch v := value.(type) {
	case nil:
		return out
	case map[string]any:
		if name := frontmatterString(v, "name"); name != "" {
			return append(out, name)
		}
		if envVar := frontmatterString(v, "env_var"); envVar != "" {
			return append(out, envVar)
		}
		return out
	default:
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
			return append(out, s)
		}
		return out
	}
}

func parseInlineStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "\"'")
		if part != "" {
			out = append(out, part)
		}
	}
	return dedupeStrings(out)
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
