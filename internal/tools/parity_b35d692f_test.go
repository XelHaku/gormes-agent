package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolParityManifestHermesB35D692F(t *testing.T) {
	manifest, err := LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}

	fixture := loadParityFixture(t)

	if got, want := manifest.Source.Donor, "hermes"; got != want {
		t.Fatalf("source donor = %q, want %q", got, want)
	}
	if got, want := manifest.Source.Commit, "b35d692f"; !strings.HasPrefix(got, want) {
		t.Fatalf("source commit = %q, want prefix %q", got, want)
	}
	for _, input := range []string{
		"toolsets.py",
		"model_tools.py",
		"tools/discord_tool.py",
		"tools/cronjob_tools.py",
		"tests/tools/test_discord_tool.py",
		"tests/hermes_cli/test_tools_config.py",
	} {
		assertContains(t, manifest.Source.InputFiles, input)
	}

	if got, stale := len(manifest.Tools), 55; got == stale {
		t.Fatalf("tool rows still equal stale pre-b35d692f count %d", stale)
	}
	if _, ok := manifest.Tool("discord_server"); ok {
		t.Fatal("legacy discord_server row must not remain in the b35d692f manifest")
	}

	discord := mustTool(t, manifest, "discord")
	if got, want := discord.Toolset, "discord"; got != want {
		t.Fatalf("discord toolset = %q, want %q", got, want)
	}
	assertContains(t, discord.RequiredEnv, "DISCORD_BOT_TOKEN")
	assertSchemaActionEnum(t, discord.Schema, []string{"search_members", "fetch_messages", "create_thread"})
	assertDynamicSchemaProvenance(t, discord.SchemaProvenance, "discord", "tools/discord_tool.py:get_dynamic_schema_core")

	discordAdmin := mustTool(t, manifest, "discord_admin")
	if got, want := discordAdmin.Toolset, "discord_admin"; got != want {
		t.Fatalf("discord_admin toolset = %q, want %q", got, want)
	}
	assertContains(t, discordAdmin.RequiredEnv, "DISCORD_BOT_TOKEN")
	assertSchemaActionEnum(t, discordAdmin.Schema, []string{
		"list_guilds",
		"server_info",
		"list_channels",
		"channel_info",
		"list_roles",
		"member_info",
		"list_pins",
		"pin_message",
		"unpin_message",
		"add_role",
		"remove_role",
	})
	assertDynamicSchemaProvenance(t, discordAdmin.SchemaProvenance, "discord_admin", "tools/discord_tool.py:get_dynamic_schema_admin")

	cronjob := fixture.mustTool(t, "cronjob")
	contextFrom := schemaProperty(t, cronjob.Schema, "context_from")
	if got, want := contextFrom["type"], "array"; got != want {
		t.Fatalf("cronjob context_from type = %v, want %q", got, want)
	}
	description, _ := contextFrom["description"].(string)
	if !strings.Contains(description, "On update, pass an empty array to clear") {
		t.Fatalf("cronjob context_from description does not capture update-clear semantics: %q", description)
	}
	cronjobRow := mustTool(t, manifest, "cronjob")
	if got := cronjobRow.DescriptorMetadata.UpdateClearSemantics["context_from"]; !strings.Contains(got, "empty array") {
		t.Fatalf("cronjob context_from descriptor metadata = %q, want empty-array clear semantics", got)
	}

	for _, toolset := range fixture.Toolsets {
		assertNotContains(t, toolset.DirectTools, "discord_server")
		assertNotContains(t, toolset.ResolvedTools, "discord_server")
	}

	hermesDiscord := fixture.mustToolset(t, "hermes-discord")
	assertContains(t, hermesDiscord.DirectTools, "discord")
	assertContains(t, hermesDiscord.DirectTools, "discord_admin")
	assertContains(t, hermesDiscord.ResolvedTools, "discord")
	assertContains(t, hermesDiscord.ResolvedTools, "discord_admin")

	gateway := fixture.mustToolset(t, "hermes-gateway")
	assertContains(t, gateway.ResolvedTools, "discord")
	assertContains(t, gateway.ResolvedTools, "discord_admin")

	for _, name := range []string{"discord", "discord_admin"} {
		toolset, ok := manifest.Toolset(name)
		if !ok {
			t.Fatalf("missing toolset parity row for %s", name)
		}
		assertContains(t, toolset.PlatformRestrictions.AllowedPlatforms, "discord")
		if toolset.PlatformRestrictions.DefaultEnabled == nil || *toolset.PlatformRestrictions.DefaultEnabled {
			t.Fatalf("%s default_enabled = %v, want false", name, toolset.PlatformRestrictions.DefaultEnabled)
		}
	}

	feishu := fixture.mustToolset(t, "hermes-feishu")
	for _, name := range []string{
		"feishu_doc_read",
		"feishu_drive_list_comments",
		"feishu_drive_list_comment_replies",
		"feishu_drive_reply_comment",
		"feishu_drive_add_comment",
	} {
		assertContains(t, feishu.DirectTools, name)
		assertContains(t, feishu.ResolvedTools, name)
	}

	for _, generic := range []string{"hermes-cli", "hermes-cron"} {
		toolset := fixture.mustToolset(t, generic)
		for _, platformOnly := range []string{
			"discord",
			"discord_admin",
			"feishu_doc_read",
			"feishu_drive_list_comments",
			"feishu_drive_list_comment_replies",
			"feishu_drive_reply_comment",
			"feishu_drive_add_comment",
		} {
			assertNotContains(t, toolset.DirectTools, platformOnly)
			assertNotContains(t, toolset.ResolvedTools, platformOnly)
		}
	}
}

type parityFixture struct {
	Source struct {
		Donor      string   `json:"donor"`
		Commit     string   `json:"commit"`
		InputFiles []string `json:"input_files"`
	} `json:"source"`
	Tools    []parityFixtureTool    `json:"tools"`
	Toolsets []parityFixtureToolset `json:"toolsets"`
}

type parityFixtureTool struct {
	Name               string `json:"name"`
	Schema             json.RawMessage
	SchemaProvenance   schemaProvenance   `json:"schema_provenance"`
	DescriptorMetadata descriptorMetadata `json:"descriptor_metadata"`
}

type schemaProvenance struct {
	Kind                 string   `json:"kind"`
	StaticSchemaSource   string   `json:"static_schema_source"`
	RuntimeSchemaSources []string `json:"runtime_schema_sources"`
	CapabilityFilters    []string `json:"capability_filters"`
	ConfigFilters        []string `json:"config_filters"`
	UnavailableWhenEmpty bool     `json:"unavailable_when_empty"`
}

type descriptorMetadata struct {
	UpdateClearSemantics map[string]string `json:"update_clear_semantics"`
}

type parityFixtureToolset struct {
	Name                 string               `json:"name"`
	DirectTools          []string             `json:"direct_tools"`
	ResolvedTools        []string             `json:"resolved_tools"`
	PlatformRestrictions platformRestrictions `json:"platform_restrictions"`
}

type platformRestrictions struct {
	AllowedPlatforms []string `json:"allowed_platforms"`
	DefaultEnabled   *bool    `json:"default_enabled"`
	Source           string   `json:"source"`
}

func loadParityFixture(t *testing.T) parityFixture {
	t.Helper()
	var fixture parityFixture
	if err := json.Unmarshal(upstreamToolParityManifestJSON, &fixture); err != nil {
		t.Fatalf("unmarshal parity fixture: %v", err)
	}
	return fixture
}

func (f parityFixture) mustTool(t *testing.T, name string) parityFixtureTool {
	t.Helper()
	for _, tool := range f.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool parity row for %s", name)
	return parityFixtureTool{}
}

func (f parityFixture) mustToolset(t *testing.T, name string) parityFixtureToolset {
	t.Helper()
	for _, toolset := range f.Toolsets {
		if toolset.Name == name {
			return toolset
		}
	}
	t.Fatalf("missing toolset parity row for %s", name)
	return parityFixtureToolset{}
}

func assertSchemaActionEnum(t *testing.T, raw json.RawMessage, want []string) {
	t.Helper()
	action := schemaProperty(t, raw, "action")
	got, ok := action["enum"].([]any)
	if !ok {
		t.Fatalf("action enum = %#v, want array", action["enum"])
	}
	if len(got) != len(want) {
		t.Fatalf("action enum length = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i, value := range got {
		if value != want[i] {
			t.Fatalf("action enum[%d] = %v, want %q; full enum=%v", i, value, want[i], got)
		}
	}
}

func assertDynamicSchemaProvenance(t *testing.T, provenance ToolSchemaProvenance, name string, dynamicSource string) {
	t.Helper()
	if got, want := provenance.Kind, "dynamic-runtime"; got != want {
		t.Fatalf("%s schema provenance kind = %q, want %q", name, got, want)
	}
	assertContains(t, provenance.RuntimeSchemaSources, "model_tools.py:get_tool_definitions")
	assertContains(t, provenance.RuntimeSchemaSources, dynamicSource)
	if !provenance.UnavailableWhenEmpty {
		t.Fatalf("%s schema provenance should record drop-when-empty dynamic schema behavior", name)
	}
}

func schemaProperty(t *testing.T, raw json.RawMessage, name string) map[string]any {
	t.Helper()
	var schema struct {
		Parameters struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	property, ok := schema.Parameters.Properties[name]
	if !ok {
		t.Fatalf("schema missing property %q", name)
	}
	return property
}

func assertNotContains(t *testing.T, values []string, unwanted string) {
	t.Helper()
	for _, value := range values {
		if value == unwanted {
			t.Fatalf("%v unexpectedly contains %q", values, unwanted)
		}
	}
}
