package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	defaultInlineShellTimeout   = 10 * time.Second
	defaultInlineShellMaxOutput = 4000
)

var (
	templateVarRE  = regexp.MustCompile(`\$\{(HERMES_SKILL_DIR|HERMES_SESSION_ID|GORMES_SKILL_DIR|GORMES_SESSION_ID)\}`)
	inlineShellRE  = regexp.MustCompile("!`([^`\n]+)`")
	errBashMissing = errors.New("bash not found")
)

// PreprocessOptions controls deterministic SKILL.md preprocessing. The zero
// value substitutes template variables and leaves inline shell snippets literal.
type PreprocessOptions struct {
	SkillDir  string
	SessionID string

	DisableTemplateVars  bool
	InlineShell          bool
	InlineShellTimeout   time.Duration
	InlineShellMaxOutput int
}

// PreprocessSkillContent applies prompt-safe SKILL.md preprocessing. Inline
// shell snippets only run when explicitly enabled by the caller.
func PreprocessSkillContent(ctx context.Context, content string, opts PreprocessOptions) (string, error) {
	if content == "" {
		return content, nil
	}
	if !opts.DisableTemplateVars {
		content = substituteTemplateVars(content, opts)
	}
	if !opts.InlineShell || !strings.Contains(content, "!`") {
		return content, nil
	}
	return expandInlineShell(ctx, content, opts)
}

func substituteTemplateVars(content string, opts PreprocessOptions) string {
	return templateVarRE.ReplaceAllStringFunc(content, func(token string) string {
		match := templateVarRE.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		switch match[1] {
		case "HERMES_SKILL_DIR", "GORMES_SKILL_DIR":
			if opts.SkillDir != "" {
				return opts.SkillDir
			}
		case "HERMES_SESSION_ID", "GORMES_SESSION_ID":
			if opts.SessionID != "" {
				return opts.SessionID
			}
		}
		return token
	})
}

func expandInlineShell(ctx context.Context, content string, opts PreprocessOptions) (string, error) {
	var firstErr error
	rendered := inlineShellRE.ReplaceAllStringFunc(content, func(match string) string {
		if firstErr != nil {
			return match
		}
		submatch := inlineShellRE.FindStringSubmatch(match)
		if len(submatch) != 2 {
			return match
		}
		command := strings.TrimSpace(submatch[1])
		if command == "" {
			return ""
		}
		output, err := runInlineShell(ctx, command, opts)
		if err != nil {
			firstErr = err
			return match
		}
		return output
	})
	if firstErr != nil {
		return "", firstErr
	}
	return rendered, nil
}

func runInlineShell(ctx context.Context, command string, opts PreprocessOptions) (string, error) {
	timeout := opts.InlineShellTimeout
	if timeout <= 0 {
		timeout = defaultInlineShellTimeout
	}
	maxOutput := opts.InlineShellMaxOutput
	if maxOutput <= 0 {
		maxOutput = defaultInlineShellMaxOutput
	}

	shellCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(shellCtx, "bash", "-c", command)
	if opts.SkillDir != "" {
		cmd.Dir = opts.SkillDir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if shellCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("inline-shell timeout after %s: %s", timeout, command)
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", errBashMissing
	}
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("inline-shell error: %s", detail)
		}
		return "", fmt.Errorf("inline-shell error: %w", err)
	}

	output := strings.TrimRight(stdout.String(), "\n")
	if output == "" {
		output = strings.TrimRight(stderr.String(), "\n")
	}
	if len(output) > maxOutput {
		output = output[:maxOutput] + "...[truncated]"
	}
	return output, nil
}
