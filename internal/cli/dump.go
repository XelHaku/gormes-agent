package cli

import (
	"sort"
	"strings"
)

// DumpInput is the pure input model for a deterministic support-summary dump.
// Callers populate the fields from runtime sources at the boundary so
// RenderDumpSummary can stay file/clock/env/network inert.
type DumpInput struct {
	Version         string
	OS              string
	Arch            string
	ProfileName     string
	Toolsets        []string
	SecretsLikeKeys []string
}

// RenderDumpSummary returns a deterministic, plain-text summary of the
// supplied DumpInput. Lines are emitted in the fixed order
// version, os, arch, profile, toolsets and every literal occurrence of any
// SecretsLikeKeys entry is replaced with "[redacted]" before returning.
// Empty scalar fields render as "unknown" and an empty Toolsets renders as
// "(none)".
func RenderDumpSummary(in DumpInput) string {
	var b strings.Builder
	writeDumpLine(&b, "version", scalarOrUnknown(in.Version))
	writeDumpLine(&b, "os", scalarOrUnknown(in.OS))
	writeDumpLine(&b, "arch", scalarOrUnknown(in.Arch))
	writeDumpLine(&b, "profile", scalarOrUnknown(in.ProfileName))
	writeDumpLine(&b, "toolsets", toolsetsValue(in.Toolsets))
	return redactSecrets(b.String(), in.SecretsLikeKeys)
}

func writeDumpLine(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

func scalarOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func toolsetsValue(toolsets []string) string {
	if len(toolsets) == 0 {
		return "(none)"
	}
	return strings.Join(toolsets, ", ")
}

// redactSecrets replaces every literal occurrence of each non-empty secret
// with "[redacted]". Secrets are processed longest-first (then lexicographic)
// so that overlapping secrets cannot leave a partial match. Empty secrets are
// skipped because replacing the empty string would inject the marker between
// every byte of the output.
func redactSecrets(text string, secrets []string) string {
	if len(secrets) == 0 {
		return text
	}
	ordered := make([]string, 0, len(secrets))
	for _, s := range secrets {
		if s != "" {
			ordered = append(ordered, s)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if len(ordered[i]) != len(ordered[j]) {
			return len(ordered[i]) > len(ordered[j])
		}
		return ordered[i] < ordered[j]
	})
	for _, s := range ordered {
		text = strings.ReplaceAll(text, s, "[redacted]")
	}
	return text
}
