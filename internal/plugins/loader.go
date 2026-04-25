package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultGormesVersion = "1.0.0"

var pluginNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type rawPluginManifest struct {
	Name           string          `yaml:"name"`
	Label          string          `yaml:"label"`
	Version        string          `yaml:"version"`
	Description    string          `yaml:"description"`
	Author         string          `yaml:"author"`
	Kind           string          `yaml:"kind"`
	RequiresGormes string          `yaml:"requires_gormes"`
	RequiresEnv    stringList      `yaml:"requires_env"`
	RequiresAuth   stringList      `yaml:"requires_auth"`
	Auth           stringList      `yaml:"auth"`
	ProvidesTools  stringList      `yaml:"provides_tools"`
	ProvidesHooks  stringList      `yaml:"provides_hooks"`
	Hooks          stringList      `yaml:"hooks"`
	Capabilities   []rawCapability `yaml:"capabilities"`
}

type rawCapability struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

type stringList []string

func (s *stringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		item := strings.TrimSpace(value.Value)
		if item != "" {
			*s = append((*s)[:0], item)
		}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, node := range value.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				if item := strings.TrimSpace(node.Value); item != "" {
					out = append(out, item)
				}
			case yaml.MappingNode:
				if item := stringFromMapping(node, "source", "name", "provider", "env"); item != "" {
					out = append(out, item)
				}
			}
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("expected string or list")
	}
}

func stringFromMapping(node *yaml.Node, keys ...string) string {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		for _, want := range keys {
			if key == want && node.Content[i+1].Kind == yaml.ScalarNode {
				return strings.TrimSpace(node.Content[i+1].Value)
			}
		}
	}
	return ""
}

// LoadDir reads plugin metadata from a directory without importing or invoking
// any plugin runtime file.
func LoadDir(dir string, opts LoadOptions) PluginStatus {
	opts = normalizeLoadOptions(opts)
	baseName := filepath.Base(filepath.Clean(dir))
	status := PluginStatus{
		Name:   baseName,
		Source: opts.Source,
		State:  StateDisabled,
	}

	pluginPath := firstExisting(filepath.Join(dir, "plugin.yaml"), filepath.Join(dir, "plugin.yml"))
	dashboardPath := filepath.Join(dir, "dashboard", "manifest.json")
	hasPluginManifest := pluginPath != ""
	hasDashboardManifest := fileExists(dashboardPath)

	if !hasPluginManifest && !hasDashboardManifest {
		status.State = StateInvalid
		status.Evidence = append(status.Evidence, evidence(EvidenceMissingRequiredField, "plugin.yaml", "plugin.yaml or dashboard/manifest.json is required"))
		return status
	}

	var manifest Manifest
	if hasPluginManifest {
		raw, parseEvidence, ok := parsePluginManifest(pluginPath)
		if !ok {
			status.State = StateMalformed
			status.Evidence = append(status.Evidence, parseEvidence)
			return status
		}
		manifest = manifestFromRaw(raw)
	}

	var dashboard *DashboardManifest
	if hasDashboardManifest {
		parsed, parseEvidence, ok := parseDashboardManifest(dashboardPath)
		if !ok {
			status.State = StateMalformed
			status.Evidence = append(status.Evidence, parseEvidence)
			return status
		}
		dashboard = parsed
		mergeDashboardMetadata(&manifest, dashboard, hasPluginManifest)
	}

	status.Manifest = manifest
	status.Name = nonEmpty(manifest.Name, baseName)
	status.Version = manifest.Version
	status.Label = manifest.Label
	status.Description = manifest.Description
	status.Dashboard = dashboard

	validation := validateManifest(manifest, dashboard, hasPluginManifest, hasDashboardManifest, opts.CurrentGormesVersion)
	credentialEvidence := missingCredentialEvidence(manifest, opts)
	status.Evidence = append(status.Evidence, validation...)
	status.Evidence = append(status.Evidence, credentialEvidence...)

	if len(validation) > 0 {
		status.State = StateInvalid
		status.Capabilities = capabilityStatuses(manifest, status.Name, StateInvalid, validation)
		return sortPluginStatus(status)
	}

	status.Evidence = append(status.Evidence, evidence(EvidenceExecutionDisabled, "runtime", "plugin runtime execution is disabled for metadata-only discovery"))
	status.State = StateDisabled
	capEvidence := append([]Evidence(nil), credentialEvidence...)
	capEvidence = append(capEvidence, evidence(EvidenceExecutionDisabled, "runtime", "plugin runtime execution is disabled for metadata-only discovery"))
	status.Capabilities = capabilityStatuses(manifest, status.Name, StateDisabled, capEvidence)
	status.Manifest.Capabilities = append([]Capability(nil), manifest.Capabilities...)
	return sortPluginStatus(status)
}

func parsePluginManifest(path string) (rawPluginManifest, Evidence, bool) {
	var raw rawPluginManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return raw, evidence(EvidenceMalformedManifest, "plugin.yaml", err.Error()), false
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return raw, evidence(EvidenceMalformedManifest, "plugin.yaml", err.Error()), false
	}
	return raw, Evidence{}, true
}

func parseDashboardManifest(path string) (*DashboardManifest, Evidence, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, evidence(EvidenceMalformedManifest, "dashboard/manifest.json", err.Error()), false
	}
	var dashboard DashboardManifest
	if err := json.Unmarshal(data, &dashboard); err != nil {
		return nil, evidence(EvidenceMalformedManifest, "dashboard/manifest.json", err.Error()), false
	}
	if dashboard.Version == "" {
		dashboard.Version = "0.0.0"
	}
	if dashboard.Icon == "" {
		dashboard.Icon = "Puzzle"
	}
	if dashboard.Tab.Position == "" {
		dashboard.Tab.Position = "end"
	}
	return &dashboard, Evidence{}, true
}

func manifestFromRaw(raw rawPluginManifest) Manifest {
	manifest := Manifest{
		Name:           strings.TrimSpace(raw.Name),
		Version:        strings.TrimSpace(raw.Version),
		Label:          strings.TrimSpace(raw.Label),
		Description:    strings.TrimSpace(raw.Description),
		Author:         strings.TrimSpace(raw.Author),
		Kind:           strings.TrimSpace(raw.Kind),
		RequiresGormes: strings.TrimSpace(raw.RequiresGormes),
		RequiresEnv:    cleanStrings([]string(raw.RequiresEnv)),
		RequiresAuth:   cleanStrings(append([]string(raw.RequiresAuth), []string(raw.Auth)...)),
	}
	if manifest.Kind == "" {
		manifest.Kind = "standalone"
	}
	for i, tool := range cleanStrings([]string(raw.ProvidesTools)) {
		manifest.Capabilities = append(manifest.Capabilities, Capability{Kind: CapabilityTool, Name: tool, SourceField: fmt.Sprintf("provides_tools[%d]", i)})
	}
	hooks := cleanStrings(append([]string(raw.ProvidesHooks), []string(raw.Hooks)...))
	for i, hook := range hooks {
		manifest.Capabilities = append(manifest.Capabilities, Capability{Kind: CapabilityHook, Name: hook, SourceField: fmt.Sprintf("hooks[%d]", i)})
	}
	for i, cap := range raw.Capabilities {
		manifest.Capabilities = append(manifest.Capabilities, Capability{
			Kind:        CapabilityKind(strings.TrimSpace(cap.Kind)),
			Name:        strings.TrimSpace(cap.Name),
			SourceField: fmt.Sprintf("capabilities[%d].kind", i),
		})
	}
	return manifest
}

func mergeDashboardMetadata(manifest *Manifest, dashboard *DashboardManifest, hasPluginManifest bool) {
	if manifest.Name == "" {
		manifest.Name = strings.TrimSpace(dashboard.Name)
	}
	if manifest.Version == "" {
		if hasPluginManifest {
			manifest.Version = strings.TrimSpace(manifest.Version)
		} else {
			manifest.Version = nonEmpty(strings.TrimSpace(dashboard.Version), "0.0.0")
		}
	}
	if manifest.Label == "" {
		manifest.Label = strings.TrimSpace(dashboard.Label)
	}
	if manifest.Description == "" {
		manifest.Description = strings.TrimSpace(dashboard.Description)
	}
	if manifest.Kind == "" {
		manifest.Kind = "standalone"
	}
	if dashboard.Name != "" {
		manifest.Capabilities = append(manifest.Capabilities, Capability{Kind: CapabilityDashboard, Name: strings.TrimSpace(dashboard.Name), SourceField: "dashboard"})
	}
	if dashboard.API != "" {
		name := strings.TrimSpace(dashboard.Name)
		if name == "" {
			name = strings.TrimSpace(manifest.Name)
		}
		manifest.Capabilities = append(manifest.Capabilities, Capability{Kind: CapabilityBackendRoute, Name: "/api/plugins/" + name + "/", SourceField: "dashboard.api"})
	}
}

func validateManifest(manifest Manifest, dashboard *DashboardManifest, hasPluginManifest, hasDashboardManifest bool, currentVersion string) []Evidence {
	var out []Evidence
	if manifest.Name == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "name", "plugin name is required"))
	} else if !pluginNameRE.MatchString(manifest.Name) {
		out = append(out, evidence(EvidenceInvalidName, "name", "plugin name must be lowercase alphanumeric with hyphens"))
	}
	if hasPluginManifest && manifest.Version == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "version", "plugin version is required"))
	}
	if manifest.RequiresGormes != "" {
		ok, err := versionSatisfies(currentVersion, manifest.RequiresGormes)
		if err != nil {
			out = append(out, evidence(EvidenceIncompatibleVersion, "requires_gormes", err.Error()))
		} else if !ok {
			out = append(out, evidence(EvidenceIncompatibleVersion, "requires_gormes", "current Gormes version does not satisfy plugin constraint"))
		}
	}
	for _, cap := range manifest.Capabilities {
		if !supportedCapabilityKind(cap.Kind) {
			field := nonEmpty(cap.SourceField, "capabilities.kind")
			out = append(out, evidence(EvidenceUnsupportedCapabilityKind, field, "unsupported capability kind "+string(cap.Kind)))
		}
		if strings.TrimSpace(cap.Name) == "" {
			out = append(out, evidence(EvidenceMissingRequiredField, nonEmpty(cap.SourceField, "capabilities.name"), "capability name is required"))
		}
	}
	if hasDashboardManifest {
		out = append(out, validateDashboardManifest(dashboard)...)
	}
	return out
}

func validateDashboardManifest(dashboard *DashboardManifest) []Evidence {
	var out []Evidence
	if dashboard == nil {
		return out
	}
	if strings.TrimSpace(dashboard.Name) == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "dashboard.name", "dashboard plugin name is required"))
	} else if !pluginNameRE.MatchString(dashboard.Name) {
		out = append(out, evidence(EvidenceInvalidName, "dashboard.name", "dashboard plugin name must be lowercase alphanumeric with hyphens"))
	}
	if strings.TrimSpace(dashboard.Label) == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "dashboard.label", "dashboard plugin label is required"))
	}
	if strings.TrimSpace(dashboard.Tab.Path) == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "dashboard.tab.path", "dashboard tab path is required"))
	}
	if strings.TrimSpace(dashboard.Entry) == "" {
		out = append(out, evidence(EvidenceMissingRequiredField, "dashboard.entry", "dashboard entry bundle path is required"))
	}
	return out
}

func missingCredentialEvidence(manifest Manifest, opts LoadOptions) []Evidence {
	var out []Evidence
	for _, name := range manifest.RequiresEnv {
		if !opts.EnvLookup(name) {
			out = append(out, evidence(EvidenceMissingCredential, name, "required environment variable is missing"))
		}
	}
	for _, name := range manifest.RequiresAuth {
		if !opts.AuthLookup(name) {
			out = append(out, evidence(EvidenceMissingCredential, name, "required auth credential is missing"))
		}
	}
	return out
}

func capabilityStatuses(manifest Manifest, pluginName, state string, ev []Evidence) []CapabilityStatus {
	out := make([]CapabilityStatus, 0, len(manifest.Capabilities))
	for _, cap := range manifest.Capabilities {
		if strings.TrimSpace(cap.Name) == "" {
			continue
		}
		out = append(out, CapabilityStatus{
			Plugin:   pluginName,
			Kind:     cap.Kind,
			Name:     cap.Name,
			State:    state,
			Evidence: cloneEvidence(ev),
		})
	}
	sortCapabilityStatuses(out)
	return out
}

func normalizeLoadOptions(opts LoadOptions) LoadOptions {
	if opts.Source == "" {
		opts.Source = SourceUser
	}
	if opts.CurrentGormesVersion == "" {
		opts.CurrentGormesVersion = defaultGormesVersion
	}
	if opts.EnvLookup == nil {
		opts.EnvLookup = func(name string) bool {
			_, ok := os.LookupEnv(name)
			return ok
		}
	}
	if opts.AuthLookup == nil {
		opts.AuthLookup = func(string) bool { return false }
	}
	return opts
}

func sortPluginStatus(status PluginStatus) PluginStatus {
	sort.Slice(status.Evidence, func(i, j int) bool {
		if status.Evidence[i].Code != status.Evidence[j].Code {
			return status.Evidence[i].Code < status.Evidence[j].Code
		}
		return status.Evidence[i].Field < status.Evidence[j].Field
	})
	sortCapabilityStatuses(status.Capabilities)
	return status
}

func supportedCapabilityKind(kind CapabilityKind) bool {
	switch kind {
	case CapabilityTool, CapabilityHook, CapabilityDashboard, CapabilityBackendRoute:
		return true
	default:
		return false
	}
}

func sortCapabilityStatuses(items []CapabilityStatus) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Plugin != items[j].Plugin {
			return items[i].Plugin < items[j].Plugin
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
}

func cleanStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func cloneEvidence(in []Evidence) []Evidence {
	return append([]Evidence(nil), in...)
}

func firstExisting(paths ...string) string {
	for _, path := range paths {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func evidence(code, field, message string) Evidence {
	return Evidence{Code: code, Field: field, Message: message}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
