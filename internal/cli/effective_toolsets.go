package cli

import (
	"fmt"
	"sort"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

const PlatformToolsetIssueDuplicateToolsetKey PlatformToolsetIssueKind = "duplicate_toolset_key"

// EffectiveToolsetOption is one deterministic row for future setup/tools
// pickers. Built-in rows are sourced from the upstream parity manifest; plugin
// rows remain disabled metadata and never imply handler registration.
type EffectiveToolsetOption struct {
	Key         string
	Label       string
	Description string
	Source      string
	Plugin      string
	State       string
}

// EffectiveToolsetReport records the picker rows plus degraded-mode evidence
// for duplicate plugin/built-in declarations.
type EffectiveToolsetReport struct {
	Options []EffectiveToolsetOption
	Issues  []PlatformToolsetIssue
}

var configurableBuiltinToolsetOrder = []string{
	"web",
	"browser",
	"terminal",
	"file",
	"code_execution",
	"vision",
	"image_gen",
	"moa",
	"tts",
	"skills",
	"todo",
	"memory",
	"session_search",
	"clarify",
	"delegation",
	"cronjob",
	"messaging",
	"rl",
	"homeassistant",
	"spotify",
	"discord",
	"discord_admin",
}

var builtinToolsetLabels = map[string]string{
	"browser":        "Browser Automation",
	"clarify":        "Clarifying Questions",
	"code_execution": "Code Execution",
	"cronjob":        "Cron Jobs",
	"delegation":     "Task Delegation",
	"discord":        "Discord (read/participate)",
	"discord_admin":  "Discord Server Admin",
	"file":           "File Operations",
	"homeassistant":  "Home Assistant",
	"image_gen":      "Image Generation",
	"memory":         "Memory",
	"messaging":      "Cross-Platform Messaging",
	"moa":            "Mixture of Agents",
	"rl":             "RL Training",
	"session_search": "Session Search",
	"skills":         "Skills",
	"spotify":        "Spotify",
	"terminal":       "Terminal & Processes",
	"todo":           "Task Planning",
	"tts":            "Text-to-Speech",
	"vision":         "Vision / Image Analysis",
	"web":            "Web Search & Scraping",
}

// EffectiveToolsetPickerOptions merges built-in picker rows with inert plugin
// toolset metadata, deduping by key so bundled plugins cannot duplicate
// first-party toolsets such as spotify.
func EffectiveToolsetPickerOptions(inventory plugins.Inventory) (EffectiveToolsetReport, error) {
	manifest, err := tools.LoadUpstreamToolParityManifest()
	if err != nil {
		return EffectiveToolsetReport{}, err
	}
	return EffectiveToolsetPickerOptionsFromManifest(manifest, inventory), nil
}

// EffectiveToolsetPickerOptionsFromManifest is the deterministic pure helper
// behind EffectiveToolsetPickerOptions.
func EffectiveToolsetPickerOptionsFromManifest(manifest tools.UpstreamToolParityManifest, inventory plugins.Inventory) EffectiveToolsetReport {
	report := EffectiveToolsetReport{}
	seen := make(map[string]EffectiveToolsetOption)

	for _, key := range configurableBuiltinToolsetOrder {
		row, ok := manifest.Toolset(key)
		if !ok {
			continue
		}
		option := EffectiveToolsetOption{
			Key:         row.Name,
			Label:       builtinToolsetLabel(row.Name),
			Description: row.Description,
			Source:      row.Source,
		}
		report.Options = append(report.Options, option)
		seen[option.Key] = option
	}

	for _, option := range pluginToolsetOptions(inventory) {
		if existing, duplicate := seen[option.Key]; duplicate {
			report.Issues = append(report.Issues, PlatformToolsetIssue{
				Kind:    PlatformToolsetIssueDuplicateToolsetKey,
				Toolset: option.Key,
				Detail:  fmt.Sprintf("plugin %s (%s) declares a toolset already provided by %s; keeping the first option", option.Plugin, option.Source, existing.Source),
			})
			continue
		}
		report.Options = append(report.Options, option)
		seen[option.Key] = option
	}

	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Kind != report.Issues[j].Kind {
			return report.Issues[i].Kind < report.Issues[j].Kind
		}
		if report.Issues[i].Toolset != report.Issues[j].Toolset {
			return report.Issues[i].Toolset < report.Issues[j].Toolset
		}
		return report.Issues[i].Detail < report.Issues[j].Detail
	})
	return report
}

func builtinToolsetLabel(key string) string {
	if label := builtinToolsetLabels[key]; label != "" {
		return label
	}
	return key
}

func pluginToolsetOptions(inventory plugins.Inventory) []EffectiveToolsetOption {
	var options []EffectiveToolsetOption
	seen := make(map[string]bool)
	for _, status := range inventory.Plugins {
		toolsetDescriptions := make(map[string]string)
		for _, tool := range status.Tools {
			if tool.Toolset == "" {
				continue
			}
			if toolsetDescriptions[tool.Toolset] == "" {
				toolsetDescriptions[tool.Toolset] = tool.Description
			}
		}
		for toolset, toolDescription := range toolsetDescriptions {
			dedupeKey := string(status.Source) + "\x00" + status.Name + "\x00" + toolset
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			options = append(options, EffectiveToolsetOption{
				Key:         toolset,
				Label:       firstNonEmpty(status.Label, status.Manifest.Label, status.Name, toolset),
				Description: firstNonEmpty(status.Description, status.Manifest.Description, toolDescription),
				Source:      string(status.Source),
				Plugin:      status.Name,
				State:       status.State,
			})
		}
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Key != options[j].Key {
			return options[i].Key < options[j].Key
		}
		if options[i].Source != options[j].Source {
			return options[i].Source < options[j].Source
		}
		return options[i].Plugin < options[j].Plugin
	})
	return options
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
