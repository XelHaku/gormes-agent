package tools

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestUpstreamToolParityManifestCapturesRegistryInventory(t *testing.T) {
	manifest, err := LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}

	if got, want := len(manifest.Tools), 56; got != want {
		t.Fatalf("tool rows = %d, want %d", got, want)
	}
	if got, want := manifest.Source.Registry, "tools/registry.py"; got != want {
		t.Fatalf("registry source = %q, want %q", got, want)
	}
	if got, want := manifest.Source.Toolsets, "toolsets.py"; got != want {
		t.Fatalf("toolsets source = %q, want %q", got, want)
	}

	for _, name := range []string{
		"browser_cdp",
		"browser_dialog",
		"browser_navigate",
		"discord",
		"discord_admin",
		"execute_code",
		"image_generate",
		"mixture_of_agents",
		"rl_start_training",
		"text_to_speech",
		"web_search",
	} {
		row, ok := manifest.Tool(name)
		if !ok {
			t.Fatalf("missing tool parity row for %s", name)
		}
		if row.Toolset == "" {
			t.Fatalf("%s: empty toolset", name)
		}
		if len(row.Schema) == 0 || !json.Valid(row.Schema) {
			t.Fatalf("%s: invalid schema JSON: %s", name, row.Schema)
		}
		if row.ResultEnvelope.Encoding != "json-string" {
			t.Fatalf("%s: result envelope encoding = %q, want json-string", name, row.ResultEnvelope.Encoding)
		}
		if len(row.ResultEnvelope.ErrorFields) == 0 {
			t.Fatalf("%s: missing result error fields", name)
		}
		if len(row.TrustClasses) == 0 {
			t.Fatalf("%s: missing trust classes", name)
		}
		if row.DegradedStatus.StatusField == "" {
			t.Fatalf("%s: missing degraded-mode status field", name)
		}
	}

	moa := mustTool(t, manifest, "mixture_of_agents")
	assertContains(t, moa.RequiredEnv, "OPENROUTER_API_KEY")

	rl := mustTool(t, manifest, "rl_start_training")
	assertContains(t, rl.RequiredEnv, "TINKER_API_KEY")
	assertContains(t, rl.RequiredEnv, "WANDB_API_KEY")

	web := mustTool(t, manifest, "web_search")
	assertContains(t, web.RequiredEnv, "FIRECRAWL_API_KEY")
	assertContains(t, web.RequiredEnv, "TAVILY_API_KEY")

	image := mustTool(t, manifest, "image_generate")
	if !image.HasProviderPath("fal") {
		t.Fatalf("image_generate should capture the FAL provider path")
	}

	cdp := mustTool(t, manifest, "browser_cdp")
	if !cdp.HasProviderPath("cdp") {
		t.Fatalf("browser_cdp should capture the CDP provider-specific path")
	}

	executeCode := mustTool(t, manifest, "execute_code")
	assertContains(t, executeCode.ResultEnvelope.SuccessFields, "status")
	assertContains(t, executeCode.ResultEnvelope.SuccessFields, "output")

	cli, ok := manifest.Toolset("hermes-cli")
	if !ok {
		t.Fatal("missing hermes-cli toolset parity row")
	}
	assertContains(t, cli.ResolvedTools, "browser_cdp")
	assertContains(t, cli.ResolvedTools, "send_message")
	assertNotContains(t, cli.ResolvedTools, "discord")
	assertNotContains(t, cli.ResolvedTools, "discord_admin")

	gateway, ok := manifest.Toolset("hermes-gateway")
	if !ok {
		t.Fatal("missing hermes-gateway toolset parity row")
	}
	assertContains(t, gateway.Includes, "hermes-discord")
	assertContains(t, gateway.ResolvedTools, "discord")
	assertContains(t, gateway.ResolvedTools, "discord_admin")
	assertNotContains(t, gateway.ResolvedTools, "discord_server")
}

func TestToolParityDoctorReportsDisabledDependenciesSchemaDriftAndProviderPaths(t *testing.T) {
	manifest, err := LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}

	report := manifest.Doctor(ToolParityDoctorOptions{
		Env: map[string]string{},
		DisabledTools: map[string]string{
			"web_extract": "disabled by platform config",
		},
		LocalSchemas: map[string]json.RawMessage{
			"web_search": json.RawMessage(`{"name":"web_search","parameters":{"type":"object","properties":{},"required":[]}}`),
		},
	})

	assertIssue(t, report, ToolParityIssueDisabledTool, "web_extract")
	assertIssue(t, report, ToolParityIssueMissingDependency, "web_search")
	assertIssue(t, report, ToolParityIssueSchemaDrift, "web_search")
	assertIssue(t, report, ToolParityIssueUnavailableProviderPath, "browser_cdp")
}

func TestToolParityDoctorReportsB35D692FManifestDrift(t *testing.T) {
	manifest, err := LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}

	report := manifest.Doctor(ToolParityDoctorOptions{
		ExpectedSourceCommit: "55-tool-stale",
		RequiredTools:        []string{"discord", "discord_admin", "discord_server"},
		RequiredSchemaProperties: map[string][]string{
			"cronjob": {"context_from", "missing_context_from"},
		},
		RequiredToolsetTools: map[string][]string{
			"hermes-discord": {"discord", "discord_admin", "discord_server"},
		},
		ForbiddenToolsetTools: map[string][]string{
			"hermes-cli": {"discord"},
		},
	})

	assertIssue(t, report, ToolParityIssueStaleSourceCommit, "manifest")
	assertIssue(t, report, ToolParityIssueMissingToolParityRow, "discord_server")
	assertIssue(t, report, ToolParityIssueMissingSchemaProperty, "cronjob")
	assertIssue(t, report, ToolParityIssueToolsetMismatch, "discord_server")
}

func TestHandlerPortRequiresParityRow(t *testing.T) {
	manifest, err := LoadUpstreamToolParityManifest()
	if err != nil {
		t.Fatalf("LoadUpstreamToolParityManifest: %v", err)
	}

	if err := manifest.AssertHandlerPortAllowed("todo"); err != nil {
		t.Fatalf("known tool should be port-eligible after parity row exists: %v", err)
	}
	if err := manifest.AssertHandlerPortAllowed("future_tool_without_descriptor"); !errors.Is(err, ErrMissingToolParityRow) {
		t.Fatalf("unknown tool error = %v, want ErrMissingToolParityRow", err)
	}
}

func mustTool(t *testing.T, manifest UpstreamToolParityManifest, name string) UpstreamToolParityRow {
	t.Helper()
	row, ok := manifest.Tool(name)
	if !ok {
		t.Fatalf("missing tool parity row for %s", name)
	}
	return row
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%v does not contain %q", values, want)
}

func assertIssue(t *testing.T, report ToolParityDoctorReport, kind ToolParityIssueKind, tool string) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Kind == kind && issue.Tool == tool {
			return
		}
	}
	t.Fatalf("missing issue kind=%s tool=%s in %#v", kind, tool, report.Issues)
}
