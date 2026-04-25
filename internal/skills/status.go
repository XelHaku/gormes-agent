package skills

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type SkillStatusCode string

const (
	SkillStatusAvailable           SkillStatusCode = "available"
	SkillStatusDisabled            SkillStatusCode = "disabled"
	SkillStatusUnsupported         SkillStatusCode = "unsupported"
	SkillStatusMissingPrerequisite SkillStatusCode = "missing-prerequisite"
	SkillStatusPreprocessingFailed SkillStatusCode = "preprocessing-failed"
)

type SkillStatus struct {
	Name   string
	Path   string
	Status SkillStatusCode
	Reason string
}

type RuntimeOptions struct {
	DisabledSkillNames map[string]bool
	Platform           string
	Env                map[string]string
	Preprocess         PreprocessOptions
}

func prepareSkills(ctx context.Context, in []Skill, opts RuntimeOptions) ([]Skill, []SkillStatus) {
	prepared := make([]Skill, 0, len(in))
	statuses := make([]SkillStatus, 0, len(in))
	for _, skill := range in {
		status := SkillStatus{Name: skill.Name, Path: skill.Path, Status: SkillStatusAvailable}

		switch {
		case isSkillDisabled(skill, opts.DisabledSkillNames):
			status.Status = SkillStatusDisabled
			status.Reason = "skill disabled"
		case !skillMatchesPlatform(skill, opts.Platform):
			status.Status = SkillStatusUnsupported
			status.Reason = "skill unsupported on platform " + resolvedPlatform(opts.Platform)
		case len(missingRequiredEnv(skill, opts.Env)) > 0:
			missing := missingRequiredEnv(skill, opts.Env)
			status.Status = SkillStatusMissingPrerequisite
			status.Reason = "missing environment variables: " + strings.Join(missing, ", ")
		default:
			preprocessOpts := opts.Preprocess
			if preprocessOpts.SkillDir == "" && skill.Path != "" {
				preprocessOpts.SkillDir = filepath.Dir(skill.Path)
			}
			body, err := PreprocessSkillContent(ctx, skill.Body, preprocessOpts)
			if err != nil {
				status.Status = SkillStatusPreprocessingFailed
				status.Reason = err.Error()
			} else {
				skill.Body = body
				prepared = append(prepared, skill)
			}
		}
		statuses = append(statuses, status)
	}
	return prepared, statuses
}

func isSkillDisabled(skill Skill, disabled map[string]bool) bool {
	if len(disabled) == 0 {
		return false
	}
	name := strings.TrimSpace(skill.Name)
	return disabled[name] || disabled[strings.ToLower(name)]
}

func skillMatchesPlatform(skill Skill, platform string) bool {
	if len(skill.Platforms) == 0 {
		return true
	}
	current := resolvedPlatform(platform)
	for _, allowed := range skill.Platforms {
		if platformMatches(current, allowed) {
			return true
		}
	}
	return false
}

func resolvedPlatform(platform string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		platform = runtime.GOOS
	}
	return platform
}

func platformMatches(current, allowed string) bool {
	current = normalizePlatform(current)
	allowed = normalizePlatform(allowed)
	return current != "" && current == allowed
}

func normalizePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "darwin", "mac", "macos", "osx":
		return "macos"
	case "linux":
		return "linux"
	case "windows", "win", "win32":
		return "windows"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func missingRequiredEnv(skill Skill, env map[string]string) []string {
	if len(skill.RequiredEnvVars) == 0 {
		return nil
	}
	missing := make([]string, 0, len(skill.RequiredEnvVars))
	for _, name := range skill.RequiredEnvVars {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		value, ok := lookupEnv(name, env)
		if !ok || strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func lookupEnv(name string, env map[string]string) (string, bool) {
	if env != nil {
		value, ok := env[name]
		return value, ok
	}
	return os.LookupEnv(name)
}
