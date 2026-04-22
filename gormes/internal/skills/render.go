package skills

import (
	"strconv"
	"strings"
)

func RenderBlock(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<skills>\n")
	for i, skill := range skills {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## ")
		b.WriteString(skill.Name)
		b.WriteByte('\n')
		b.WriteString(skill.Description)
		b.WriteString("\n\n")
		b.WriteString(skill.Body)
		b.WriteByte('\n')
	}
	b.WriteString("</skills>")
	return b.String()
}

func RenderDocument(skill Skill) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(strconv.Quote(strings.TrimSpace(skill.Name)))
	b.WriteByte('\n')
	b.WriteString("description: ")
	b.WriteString(strconv.Quote(strings.TrimSpace(skill.Description)))
	b.WriteString("\n---\n\n")
	b.WriteString(strings.TrimSpace(skill.Body))
	b.WriteByte('\n')
	return b.String()
}
