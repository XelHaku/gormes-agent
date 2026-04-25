package plugins

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadDirParsesManifestDashboardCapabilitiesAndRequirements(t *testing.T) {
	dir := writePluginFixture(t, "spotify", map[string]string{
		"plugin.yaml": `name: spotify
label: Spotify
version: 1.0.0
description: "Native Spotify integration using Spotify Web API + PKCE OAuth."
author: NousResearch
kind: backend
requires_gormes: ">=1.0.0 <2.0.0"
requires_env:
  - SPOTIFY_CLIENT_ID
requires_auth:
  - providers.spotify
provides_tools:
  - spotify_playback
  - spotify_devices
hooks:
  - post_tool_call
`,
		filepath.Join("dashboard", "manifest.json"): `{
  "name": "spotify",
  "label": "Spotify",
  "description": "Spotify dashboard plugin",
  "icon": "Sparkles",
  "version": "1.0.0",
  "tab": {
    "path": "/spotify",
    "position": "after:skills",
    "override": "/music",
    "hidden": true
  },
  "slots": ["sidebar", "header-left"],
  "entry": "dist/index.js",
  "css": "dist/style.css",
  "api": "plugin_api.py"
}`,
	})

	status := LoadDir(dir, LoadOptions{
		Source:               SourceBundled,
		CurrentGormesVersion: "1.4.0",
		EnvLookup:            func(name string) bool { return name == "SPOTIFY_CLIENT_ID" },
		AuthLookup:           func(name string) bool { return name == "providers.spotify" },
	})

	if status.State != StateDisabled {
		t.Fatalf("state = %q, want disabled; evidence=%+v", status.State, status.Evidence)
	}
	if status.RuntimeCodeExecuted {
		t.Fatal("LoadDir executed plugin runtime code")
	}
	if status.Manifest.Name != "spotify" || status.Manifest.Version != "1.0.0" || status.Manifest.Label != "Spotify" {
		t.Fatalf("manifest identity = %+v, want spotify/1.0.0/Spotify", status.Manifest)
	}
	if status.Manifest.Kind != "backend" || status.Manifest.Author != "NousResearch" {
		t.Fatalf("manifest kind/author = %q/%q", status.Manifest.Kind, status.Manifest.Author)
	}
	if !slices.Equal(status.Manifest.RequiresEnv, []string{"SPOTIFY_CLIENT_ID"}) {
		t.Fatalf("requires_env = %#v", status.Manifest.RequiresEnv)
	}
	if !slices.Equal(status.Manifest.RequiresAuth, []string{"providers.spotify"}) {
		t.Fatalf("requires_auth = %#v", status.Manifest.RequiresAuth)
	}

	assertCapability(t, status.Capabilities, CapabilityTool, "spotify_playback", StateDisabled)
	assertCapability(t, status.Capabilities, CapabilityTool, "spotify_devices", StateDisabled)
	assertCapability(t, status.Capabilities, CapabilityHook, "post_tool_call", StateDisabled)
	assertCapability(t, status.Capabilities, CapabilityDashboard, "spotify", StateDisabled)
	assertCapability(t, status.Capabilities, CapabilityBackendRoute, "/api/plugins/spotify/", StateDisabled)

	if status.Dashboard == nil {
		t.Fatal("dashboard manifest missing")
	}
	if status.Dashboard.Entry != "dist/index.js" || status.Dashboard.CSS != "dist/style.css" || status.Dashboard.API != "plugin_api.py" {
		t.Fatalf("dashboard assets = %+v", status.Dashboard)
	}
	if status.Dashboard.Tab.Override != "/music" || !status.Dashboard.Tab.Hidden {
		t.Fatalf("dashboard tab = %+v, want override /music and hidden", status.Dashboard.Tab)
	}
	if !slices.Equal(status.Dashboard.Slots, []string{"sidebar", "header-left"}) {
		t.Fatalf("dashboard slots = %#v", status.Dashboard.Slots)
	}
}

func TestLoadDirFailsClosedWithStructuredEvidence(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantState string
		wantCode  string
		wantField string
	}{
		{
			name: "malformed manifest",
			files: map[string]string{
				"plugin.yaml": "name: [broken\n",
			},
			wantState: StateMalformed,
			wantCode:  EvidenceMalformedManifest,
			wantField: "plugin.yaml",
		},
		{
			name: "invalid name",
			files: map[string]string{
				"plugin.yaml": "name: Bad Name\nversion: 1.0.0\n",
			},
			wantState: StateInvalid,
			wantCode:  EvidenceInvalidName,
			wantField: "name",
		},
		{
			name: "missing required field",
			files: map[string]string{
				"plugin.yaml": "name: missing-version\n",
			},
			wantState: StateInvalid,
			wantCode:  EvidenceMissingRequiredField,
			wantField: "version",
		},
		{
			name: "unsupported capability kind",
			files: map[string]string{
				"plugin.yaml": `name: weird
version: 1.0.0
capabilities:
  - kind: websocket
    name: live-feed
`,
			},
			wantState: StateInvalid,
			wantCode:  EvidenceUnsupportedCapabilityKind,
			wantField: "capabilities[0].kind",
		},
		{
			name: "incompatible version constraint",
			files: map[string]string{
				"plugin.yaml": "name: future\nversion: 1.0.0\nrequires_gormes: \">=9.0.0\"\n",
			},
			wantState: StateInvalid,
			wantCode:  EvidenceIncompatibleVersion,
			wantField: "requires_gormes",
		},
		{
			name: "dashboard missing required field",
			files: map[string]string{
				filepath.Join("dashboard", "manifest.json"): `{"name":"dash-only","tab":{"path":"/dash-only"},"entry":"dist/index.js"}`,
			},
			wantState: StateInvalid,
			wantCode:  EvidenceMissingRequiredField,
			wantField: "dashboard.label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writePluginFixture(t, strings.ReplaceAll(tt.name, " ", "-"), tt.files)
			status := LoadDir(dir, LoadOptions{Source: SourceUser, CurrentGormesVersion: "1.0.0"})
			if status.State != tt.wantState {
				t.Fatalf("state = %q, want %q; evidence=%+v", status.State, tt.wantState, status.Evidence)
			}
			assertEvidence(t, status.Evidence, tt.wantCode, tt.wantField)
			if status.RuntimeCodeExecuted {
				t.Fatal("invalid manifest path executed plugin runtime code")
			}
		})
	}
}

func TestLoadDirReportsMissingCredentialsAsDisabledEvidence(t *testing.T) {
	dir := writePluginFixture(t, "spotify", map[string]string{
		"plugin.yaml": `name: spotify
version: 1.0.0
requires_env:
  - SPOTIFY_CLIENT_ID
requires_auth:
  - providers.spotify
provides_tools:
  - spotify_search
`,
	})

	status := LoadDir(dir, LoadOptions{
		Source:               SourceUser,
		CurrentGormesVersion: "1.0.0",
		EnvLookup:            func(string) bool { return false },
		AuthLookup:           func(string) bool { return false },
	})

	if status.State != StateDisabled {
		t.Fatalf("state = %q, want disabled; evidence=%+v", status.State, status.Evidence)
	}
	assertEvidence(t, status.Evidence, EvidenceMissingCredential, "SPOTIFY_CLIENT_ID")
	assertEvidence(t, status.Evidence, EvidenceMissingCredential, "providers.spotify")
	capability := findCapability(status.Capabilities, CapabilityTool, "spotify_search")
	if capability == nil {
		t.Fatalf("missing spotify_search capability in %+v", status.Capabilities)
	}
	assertEvidence(t, capability.Evidence, EvidenceMissingCredential, "SPOTIFY_CLIENT_ID")
	assertEvidence(t, capability.Evidence, EvidenceExecutionDisabled, "runtime")
}

func TestDiscoverProjectPluginsRequireExplicitGate(t *testing.T) {
	root := t.TempDir()
	projectPlugin := filepath.Join(root, ".hermes", "plugins", "project-tool")
	if err := os.MkdirAll(projectPlugin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPlugin, "plugin.yaml"), []byte("name: project-tool\nversion: 1.0.0\nprovides_tools: [project_tool]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	disabled := Discover(DiscoveryRoots{Project: filepath.Join(root, ".hermes", "plugins")}, DiscoverOptions{
		CurrentGormesVersion: "1.0.0",
	})
	if len(disabled.Plugins) != 0 {
		t.Fatalf("project plugins discovered without gate: %+v", disabled.Plugins)
	}
	if disabled.ProjectDiscoveryEnabled {
		t.Fatal("project discovery reported enabled without gate")
	}
	assertEvidence(t, disabled.Evidence, EvidenceProjectPluginsDisabled, "project")

	t.Setenv("GORMES_ENABLE_PROJECT_PLUGINS", "1")
	enabled := Discover(DiscoveryRoots{Project: filepath.Join(root, ".hermes", "plugins")}, DiscoverOptions{
		CurrentGormesVersion: "1.0.0",
		EnableProjectPlugins: ProjectPluginsEnabledFromEnv(),
	})
	if !enabled.ProjectDiscoveryEnabled {
		t.Fatal("project discovery did not report enabled with env gate")
	}
	if len(enabled.Plugins) != 1 || enabled.Plugins[0].Name != "project-tool" {
		t.Fatalf("project plugins = %+v, want project-tool", enabled.Plugins)
	}
	assertCapability(t, enabled.Capabilities, CapabilityTool, "project_tool", StateDisabled)
}

func writePluginFixture(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func assertCapability(t *testing.T, capabilities []CapabilityStatus, kind CapabilityKind, name, state string) {
	t.Helper()
	capability := findCapability(capabilities, kind, name)
	if capability == nil {
		t.Fatalf("missing capability %s:%s in %+v", kind, name, capabilities)
	}
	if capability.State != state {
		t.Fatalf("capability %s:%s state = %q, want %q", kind, name, capability.State, state)
	}
}

func findCapability(capabilities []CapabilityStatus, kind CapabilityKind, name string) *CapabilityStatus {
	for i := range capabilities {
		if capabilities[i].Kind == kind && capabilities[i].Name == name {
			return &capabilities[i]
		}
	}
	return nil
}

func assertEvidence(t *testing.T, evidence []Evidence, code, field string) {
	t.Helper()
	for _, ev := range evidence {
		if ev.Code == code && ev.Field == field {
			return
		}
	}
	t.Fatalf("missing evidence code=%q field=%q in %+v", code, field, evidence)
}
