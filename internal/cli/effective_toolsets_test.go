package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

func TestEffectiveToolsetPickerDedupesBundledPluginKeys(t *testing.T) {
	bundledSpotify := loadToolsetPluginFixture(t, "spotify", plugins.SourceBundled, map[string]string{
		"plugin.yaml": `name: spotify
label: Plugin Spotify
version: 1.0.0
description: plugin metadata must not replace the built-in spotify picker row
provides_tools:
  - spotify_playback
`,
		"__init__.py": `from plugins.spotify.tools import SPOTIFY_SCHEMA, _handle_spotify_playback

_TOOLS = (
    ("spotify_playback", SPOTIFY_SCHEMA, _handle_spotify_playback, "music"),
)

def register(ctx) -> None:
    for name, schema, handler, emoji in _TOOLS:
        ctx.register_tool(name=name, toolset="spotify", schema=schema, handler=handler, emoji=emoji)
`,
		"tools.py": `raise RuntimeError("metadata loader must not execute plugin runtime")

SPOTIFY_SCHEMA = {
    "name": "spotify_playback",
    "description": "plugin metadata description must not win",
    "parameters": {"type": "object", "properties": {}, "required": []},
}

def _handle_spotify_playback(args):
    raise RuntimeError("handler must not run")
`,
	})
	userAmbient := loadToolsetPluginFixture(t, "ambient", plugins.SourceUser, map[string]string{
		"plugin.yaml": `name: ambient
label: Ambient Controls
version: 1.0.0
description: User ambient audio controls
provides_tools:
  - ambient_play
`,
		"__init__.py": `from plugins.ambient.tools import AMBIENT_SCHEMA, _handle_ambient_play

_TOOLS = (
    ("ambient_play", AMBIENT_SCHEMA, _handle_ambient_play, "sound"),
)

def register(ctx) -> None:
    for name, schema, handler, emoji in _TOOLS:
        ctx.register_tool(name=name, toolset="ambient_audio", schema=schema, handler=handler, emoji=emoji)
`,
		"tools.py": `raise RuntimeError("metadata loader must not execute plugin runtime")

AMBIENT_SCHEMA = {
    "name": "ambient_play",
    "description": "Start ambient audio playback.",
    "parameters": {"type": "object", "properties": {}, "required": []},
}

def _handle_ambient_play(args):
    raise RuntimeError("handler must not run")
`,
	})

	if bundledSpotify.RuntimeCodeExecuted || userAmbient.RuntimeCodeExecuted {
		t.Fatalf("plugin metadata discovery executed runtime code: spotify=%v ambient=%v", bundledSpotify.RuntimeCodeExecuted, userAmbient.RuntimeCodeExecuted)
	}

	report, err := EffectiveToolsetPickerOptions(plugins.BuildInventory([]plugins.PluginStatus{bundledSpotify, userAmbient}))
	if err != nil {
		t.Fatalf("EffectiveToolsetPickerOptions: %v", err)
	}

	keys := effectiveToolsetKeys(report.Options)
	if len(keys) != len(uniqueStrings(keys)) {
		t.Fatalf("duplicate effective toolset keys: %v", keys)
	}

	spotify := requireEffectiveToolsetOption(t, report.Options, "spotify")
	manifest, err := tools.LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}
	builtinSpotify, ok := manifest.Toolset("spotify")
	if !ok {
		t.Fatal("missing built-in spotify toolset fixture row")
	}
	if spotify.Label != "Spotify" {
		t.Fatalf("spotify label = %q, want built-in label Spotify", spotify.Label)
	}
	if spotify.Description != builtinSpotify.Description {
		t.Fatalf("spotify description = %q, want built-in description %q", spotify.Description, builtinSpotify.Description)
	}
	if spotify.Source != builtinSpotify.Source {
		t.Fatalf("spotify source = %q, want built-in source %q", spotify.Source, builtinSpotify.Source)
	}
	if spotify.Plugin != "" || spotify.State != "" {
		t.Fatalf("spotify option used plugin metadata: %+v", spotify)
	}
	assertEffectiveToolsetIssue(t, report.Issues, PlatformToolsetIssueDuplicateToolsetKey, "spotify")

	ambient := requireEffectiveToolsetOption(t, report.Options, "ambient_audio")
	if ambient.Label != "Ambient Controls" {
		t.Fatalf("ambient label = %q, want plugin label", ambient.Label)
	}
	if ambient.Description != "User ambient audio controls" {
		t.Fatalf("ambient description = %q, want plugin description", ambient.Description)
	}
	if ambient.Source != string(plugins.SourceUser) || ambient.Plugin != "ambient" || ambient.State != plugins.StateDisabled {
		t.Fatalf("ambient plugin row metadata = %+v", ambient)
	}
	if optionIndex(report.Options, "ambient_audio") <= optionIndex(report.Options, "spotify") {
		t.Fatalf("plugin toolset was not appended after built-in rows: %v", keys)
	}

	cfg := PlatformToolsetConfig{PlatformToolsets: map[string][]string{
		"cli": {"spotify", "spotify", "ambient_audio"},
	}}
	status, err := cfg.PlatformStatus("cli")
	if err != nil {
		t.Fatalf("PlatformStatus: %v", err)
	}
	if countString(status.RuntimeToolsets, "spotify") != 1 {
		t.Fatalf("runtime spotify count = %d in %v, want one", countString(status.RuntimeToolsets, "spotify"), status.RuntimeToolsets)
	}

	saveReport, err := cfg.SavePlatformSelection("cli", []string{"spotify", "spotify", "ambient_audio"})
	if err != nil {
		t.Fatalf("SavePlatformSelection: %v", err)
	}
	wantPersisted := []string{"ambient_audio", "spotify"}
	if !reflect.DeepEqual(saveReport.PersistedToolsets, wantPersisted) {
		t.Fatalf("persisted toolsets = %v, want %v", saveReport.PersistedToolsets, wantPersisted)
	}
}

func loadToolsetPluginFixture(t *testing.T, name string, source plugins.Source, files map[string]string) plugins.PluginStatus {
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
	return plugins.LoadDir(dir, plugins.LoadOptions{
		Source:               source,
		CurrentGormesVersion: "1.0.0",
		EnvLookup:            func(string) bool { return false },
		AuthLookup:           func(string) bool { return false },
	})
}

func effectiveToolsetKeys(options []EffectiveToolsetOption) []string {
	out := make([]string, 0, len(options))
	for _, option := range options {
		out = append(out, option.Key)
	}
	return out
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

func requireEffectiveToolsetOption(t *testing.T, options []EffectiveToolsetOption, key string) EffectiveToolsetOption {
	t.Helper()
	for _, option := range options {
		if option.Key == key {
			return option
		}
	}
	t.Fatalf("missing effective toolset option %q in %+v", key, options)
	return EffectiveToolsetOption{}
}

func optionIndex(options []EffectiveToolsetOption, key string) int {
	for i, option := range options {
		if option.Key == key {
			return i
		}
	}
	return -1
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func assertEffectiveToolsetIssue(t *testing.T, issues []PlatformToolsetIssue, kind PlatformToolsetIssueKind, toolset string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Kind == kind && issue.Toolset == toolset {
			return
		}
	}
	t.Fatalf("missing issue kind=%s toolset=%s in %#v", kind, toolset, issues)
}
