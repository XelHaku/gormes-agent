package skills

import (
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	invalidCommandCharsRE = regexp.MustCompile(`[^a-z0-9-]`)
	multiHyphenRE         = regexp.MustCompile(`-{2,}`)
)

type SkillSlashCommand struct {
	Command     string
	Name        string
	Description string
	SkillDir    string
	Skill       Skill
}

type SlashMessageOptions struct {
	RuntimeNote string
}

func (r *Runtime) SkillSlashCommands(ctx context.Context, opts RuntimeOptions) ([]SkillSlashCommand, []SkillStatus, error) {
	if r == nil || r.store == nil {
		return nil, nil, nil
	}
	snapshot, err := r.store.SnapshotActive()
	if err != nil {
		return nil, nil, err
	}
	prepared, statuses := prepareSkills(ctx, snapshot.Skills, opts)
	return BuildSkillSlashCommands(prepared), statuses, nil
}

func BuildSkillSlashCommands(skills []Skill) []SkillSlashCommand {
	if len(skills) == 0 {
		return nil
	}
	commands := make([]SkillSlashCommand, 0, len(skills))
	seen := map[string]bool{}
	for _, skill := range skills {
		commandName := normalizeSkillCommandName(skill.Name)
		if commandName == "" {
			continue
		}
		command := "/" + commandName
		if seen[command] {
			continue
		}
		seen[command] = true
		commands = append(commands, SkillSlashCommand{
			Command:     command,
			Name:        skill.Name,
			Description: slashCommandDescription(skill),
			SkillDir:    skillDir(skill),
			Skill:       skill,
		})
	}
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Command != commands[j].Command {
			return commands[i].Command < commands[j].Command
		}
		return commands[i].Name < commands[j].Name
	})
	return commands
}

func ResolveSkillSlashCommand(commands []SkillSlashCommand, raw string) (SkillSlashCommand, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.TrimPrefix(key, "/")
	if i := strings.IndexAny(key, " \t\r\n"); i >= 0 {
		key = key[:i]
	}
	key = strings.ReplaceAll(key, "_", "-")
	if key == "" {
		return SkillSlashCommand{}, false
	}
	key = "/" + key
	for _, command := range commands {
		if command.Command == key {
			return command, true
		}
	}
	return SkillSlashCommand{}, false
}

func BuildSkillSlashCommandMessage(command SkillSlashCommand, userInstruction string, opts SlashMessageOptions) string {
	parts := []string{
		`[SYSTEM: The user has invoked the "` + command.Name + `" skill, indicating they want you to follow its instructions. The full skill content is loaded below.]`,
		"",
		strings.TrimSpace(command.Skill.Body),
	}

	if command.SkillDir != "" {
		parts = append(parts,
			"",
			"[Skill directory: "+command.SkillDir+"]",
			"Resolve any relative paths in this skill against that directory before reading files or running scripts.",
		)
	}

	if instruction := strings.TrimSpace(userInstruction); instruction != "" {
		parts = append(parts,
			"",
			"The user has provided the following instruction alongside the skill invocation: "+instruction,
		)
	}
	if note := strings.TrimSpace(opts.RuntimeNote); note != "" {
		parts = append(parts,
			"",
			"[Runtime note: "+note+"]",
		)
	}

	return strings.Join(parts, "\n")
}

func normalizeSkillCommandName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	name = invalidCommandCharsRE.ReplaceAllString(name, "")
	name = multiHyphenRE.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

func slashCommandDescription(skill Skill) string {
	if description := strings.TrimSpace(skill.Description); description != "" {
		return description
	}
	return "Invoke the " + skill.Name + " skill"
}

func skillDir(skill Skill) string {
	if skill.Path == "" {
		return ""
	}
	return filepath.Dir(skill.Path)
}
