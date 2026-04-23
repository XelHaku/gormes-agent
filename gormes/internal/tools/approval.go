package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ErrDangerousAction marks tool inputs that are blocked before execution.
var ErrDangerousAction = errors.New("tools: dangerous action blocked")

type dangerousActionMatch struct {
	Field       string
	Command     string
	Description string
}

// DangerousActionError captures the blocked command detail while preserving
// ErrDangerousAction for errors.Is checks.
type DangerousActionError struct {
	ToolName string
	Match    dangerousActionMatch
}

func (e DangerousActionError) Error() string {
	toolName := strings.TrimSpace(e.ToolName)
	if toolName == "" {
		toolName = "unknown"
	}
	field := strings.TrimSpace(e.Match.Field)
	if field == "" {
		field = "command"
	}
	command := strings.TrimSpace(e.Match.Command)
	if command == "" {
		command = "<empty>"
	}
	return fmt.Sprintf("%s for tool %q: %s in %s (%s)", ErrDangerousAction.Error(), toolName, e.Match.Description, field, command)
}

func (e DangerousActionError) Unwrap() error { return ErrDangerousAction }

type dangerousPattern struct {
	re          *regexp.Regexp
	description string
}

var dangerousCommandPatterns = []dangerousPattern{
	{
		re:          regexp.MustCompile(`(?i)\brm\s+(?:-[[:alnum:]-]*r[[:alnum:]-]*f[[:alnum:]-]*|-[[:alnum:]-]*f[[:alnum:]-]*r[[:alnum:]-]*|--recursive(?:\s+--force)?|--force(?:\s+--recursive)?)\b`),
		description: "recursive delete",
	},
	{
		re:          regexp.MustCompile(`(?i)\b(?:bash|sh|zsh|ksh)\s+-[[:alpha:]]*c\b`),
		description: "shell execution via -c flag",
	},
	{
		re:          regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\r\n]*\|\s*(?:bash|sh|zsh|ksh)\b`),
		description: "remote content piped to shell",
	},
}

var commandLikeKeys = map[string]struct{}{
	"bash_command":  {},
	"cmd":           {},
	"command":       {},
	"command_line":  {},
	"script":        {},
	"shell":         {},
	"shell_command": {},
	"sh_command":    {},
}

// GuardDangerousInput scans command-like JSON fields and blocks known
// destructive shell patterns before tool execution.
func GuardDangerousInput(toolName string, args json.RawMessage) error {
	if len(args) == 0 || strings.TrimSpace(string(args)) == "" {
		return nil
	}

	var decoded any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return nil
	}

	match, ok := findDangerousAction(decoded, "")
	if !ok {
		return nil
	}
	return DangerousActionError{ToolName: toolName, Match: match}
}

func findDangerousAction(value any, path string) (dangerousActionMatch, bool) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if isCommandLikeKey(key) {
				if cmd, ok := typed[key].(string); ok {
					if match, ok := detectDangerousCommand(nextPath, cmd); ok {
						return match, true
					}
				}
			}
			if match, ok := findDangerousAction(typed[key], nextPath); ok {
				return match, true
			}
		}
	case []any:
		for i, item := range typed {
			nextPath := fmt.Sprintf("%s[%d]", path, i)
			if path == "" {
				nextPath = fmt.Sprintf("[%d]", i)
			}
			if match, ok := findDangerousAction(item, nextPath); ok {
				return match, true
			}
		}
	}
	return dangerousActionMatch{}, false
}

func isCommandLikeKey(key string) bool {
	_, ok := commandLikeKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func detectDangerousCommand(field, command string) (dangerousActionMatch, bool) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return dangerousActionMatch{}, false
	}
	for _, pattern := range dangerousCommandPatterns {
		if pattern.re.MatchString(trimmed) {
			return dangerousActionMatch{
				Field:       field,
				Command:     trimmed,
				Description: pattern.description,
			}, true
		}
	}
	return dangerousActionMatch{}, false
}
