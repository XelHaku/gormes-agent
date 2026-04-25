package plugins

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectPluginsEnabledFromEnv returns true only for explicit project-plugin
// opt-in. GORMES_ENABLE_PROJECT_PLUGINS is native; HERMES_ENABLE_PROJECT_PLUGINS
// is accepted for parity with the donor manifests.
func ProjectPluginsEnabledFromEnv() bool {
	return envTruthy(os.Getenv("GORMES_ENABLE_PROJECT_PLUGINS")) || envTruthy(os.Getenv("HERMES_ENABLE_PROJECT_PLUGINS"))
}

// Discover scans configured roots for plugin manifests without executing
// plugin runtime code. Project roots are ignored unless explicitly enabled.
func Discover(roots DiscoveryRoots, opts DiscoverOptions) Inventory {
	loadOpts := LoadOptions{
		CurrentGormesVersion: opts.CurrentGormesVersion,
		EnvLookup:            opts.EnvLookup,
		AuthLookup:           opts.AuthLookup,
	}
	var statuses []PluginStatus
	for _, root := range roots.Bundled {
		loadOpts.Source = SourceBundled
		statuses = append(statuses, discoverRoot(root, loadOpts)...)
	}
	for _, root := range roots.User {
		loadOpts.Source = SourceUser
		statuses = append(statuses, discoverRoot(root, loadOpts)...)
	}

	inventory := BuildInventory(statuses)
	if roots.Project != "" {
		if opts.EnableProjectPlugins {
			loadOpts.Source = SourceProject
			inventory = BuildInventory(append(inventory.Plugins, discoverRoot(roots.Project, loadOpts)...))
			inventory.ProjectDiscoveryEnabled = true
		} else {
			inventory.ProjectDiscoveryEnabled = false
			inventory.Evidence = append(inventory.Evidence, Evidence{
				Code:    EvidenceProjectPluginsDisabled,
				Field:   "project",
				Message: "project plugin discovery requires an explicit config or environment gate",
			})
		}
	}
	return sortInventory(inventory)
}

// BuildInventory flattens plugin statuses into deterministic plugin and
// capability rows.
func BuildInventory(statuses []PluginStatus) Inventory {
	inventory := Inventory{Plugins: append([]PluginStatus(nil), statuses...)}
	for _, status := range statuses {
		inventory.Capabilities = append(inventory.Capabilities, status.Capabilities...)
	}
	return sortInventory(inventory)
}

func discoverRoot(root string, opts LoadOptions) []PluginStatus {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var statuses []PluginStatus
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(root, entry.Name())
		if hasManifest(child) {
			statuses = append(statuses, LoadDir(child, opts))
			continue
		}
		nested, err := os.ReadDir(child)
		if err != nil {
			continue
		}
		for _, nestedEntry := range nested {
			if !nestedEntry.IsDir() {
				continue
			}
			nestedChild := filepath.Join(child, nestedEntry.Name())
			if hasManifest(nestedChild) {
				statuses = append(statuses, LoadDir(nestedChild, opts))
			}
		}
	}
	return statuses
}

func hasManifest(dir string) bool {
	return fileExists(filepath.Join(dir, "plugin.yaml")) ||
		fileExists(filepath.Join(dir, "plugin.yml")) ||
		fileExists(filepath.Join(dir, "dashboard", "manifest.json"))
}

func sortInventory(inventory Inventory) Inventory {
	sort.Slice(inventory.Plugins, func(i, j int) bool {
		if inventory.Plugins[i].Name != inventory.Plugins[j].Name {
			return inventory.Plugins[i].Name < inventory.Plugins[j].Name
		}
		return inventory.Plugins[i].Source < inventory.Plugins[j].Source
	})
	sortCapabilityStatuses(inventory.Capabilities)
	sort.Slice(inventory.Evidence, func(i, j int) bool {
		if inventory.Evidence[i].Code != inventory.Evidence[j].Code {
			return inventory.Evidence[i].Code < inventory.Evidence[j].Code
		}
		return inventory.Evidence[i].Field < inventory.Evidence[j].Field
	})
	return inventory
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
