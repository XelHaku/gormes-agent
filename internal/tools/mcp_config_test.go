package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
)

func TestMCPConfigParsesStdioServerDefaultsAndSampling(t *testing.T) {
	resolved, err := ParseMCPConfigYAML([]byte(`
mcp_servers:
  github:
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-github"
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
      DEBUG: "true"
    timeout: 90
    connect_timeout: 15
    sampling:
      enabled: true
      model: gemini-3-flash
      max_tokens_cap: 4096
      timeout: 30
      max_rpm: 10
      allowed_models:
        - gemini-3-flash
      max_tool_rounds: 5
      log_level: debug
  defaulted:
    command: uvx
`), MCPConfigOptions{LookupEnv: lookupMCPTestEnv(map[string]string{
		"GITHUB_TOKEN": "ghp_secret123",
	})})
	if err != nil {
		t.Fatalf("ParseMCPConfigYAML: %v", err)
	}

	github := mustMCPServer(t, resolved, "github")
	if github.Transport != MCPTransportStdio {
		t.Fatalf("github transport = %q, want stdio", github.Transport)
	}
	if github.Command != "npx" {
		t.Fatalf("github command = %q, want npx", github.Command)
	}
	if got, want := strings.Join(github.Args, " "), "-y @modelcontextprotocol/server-github"; got != want {
		t.Fatalf("github args = %q, want %q", got, want)
	}
	if github.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_secret123" {
		t.Fatalf("github token env was not interpolated: %#v", github.Env)
	}
	if github.Env["DEBUG"] != "true" {
		t.Fatalf("github DEBUG env = %q, want true", github.Env["DEBUG"])
	}
	if github.Timeout != 90*time.Second || github.ConnectTimeout != 15*time.Second {
		t.Fatalf("github timeouts = %s/%s, want 90s/15s", github.Timeout, github.ConnectTimeout)
	}
	if !github.Sampling.Enabled || github.Sampling.Model != "gemini-3-flash" {
		t.Fatalf("github sampling = %#v, want enabled gemini-3-flash", github.Sampling)
	}
	if github.Sampling.MaxTokensCap != 4096 || github.Sampling.Timeout != 30*time.Second ||
		github.Sampling.MaxRPM != 10 || github.Sampling.MaxToolRounds != 5 ||
		github.Sampling.LogLevel != "debug" {
		t.Fatalf("github sampling numeric fields = %#v", github.Sampling)
	}
	if got, want := strings.Join(github.Sampling.AllowedModels, ","), "gemini-3-flash"; got != want {
		t.Fatalf("github allowed models = %q, want %q", got, want)
	}

	defaulted := mustMCPServer(t, resolved, "defaulted")
	if defaulted.Timeout != 120*time.Second || defaulted.ConnectTimeout != 60*time.Second {
		t.Fatalf("defaulted timeouts = %s/%s, want Hermes defaults 120s/60s", defaulted.Timeout, defaulted.ConnectTimeout)
	}

	status := mustMCPStatus(t, resolved, "github")
	if status.Status != MCPConfigStatusReady {
		t.Fatalf("github status = %q, want ready (%s)", status.Status, status.Reason)
	}
	if got := status.Env["GITHUB_PERSONAL_ACCESS_TOKEN"]; got != RedactedMCPConfigValue {
		t.Fatalf("redacted github token env = %q, want %q", got, RedactedMCPConfigValue)
	}
	if strings.Contains(resolved.RedactedStatusText(), "ghp_secret123") {
		t.Fatalf("redacted status leaked token: %s", resolved.RedactedStatusText())
	}
}

func TestMCPConfigParsesHTTPConfigAndRedactsHeaders(t *testing.T) {
	resolved, err := ParseMCPConfigJSON([]byte(`{
	  "mcp_servers": {
	    "remote": {
	      "url": "https://mcp.example.test/mcp",
	      "headers": {
	        "Authorization": "Bearer ${REMOTE_TOKEN}",
	        "X-API-Token": "token=raw-secret",
	        "X-Trace": "trace-1"
	      },
	      "timeout": 180,
	      "connect_timeout": 20
	    }
	  }
	}`), MCPConfigOptions{LookupEnv: lookupMCPTestEnv(map[string]string{
		"REMOTE_TOKEN": "sk-live-secret",
	})})
	if err != nil {
		t.Fatalf("ParseMCPConfigJSON: %v", err)
	}

	remote := mustMCPServer(t, resolved, "remote")
	if remote.Transport != MCPTransportHTTP {
		t.Fatalf("remote transport = %q, want http", remote.Transport)
	}
	if remote.URL != "https://mcp.example.test/mcp" {
		t.Fatalf("remote URL = %q", remote.URL)
	}
	if remote.Headers["Authorization"] != "Bearer sk-live-secret" {
		t.Fatalf("remote Authorization header was not interpolated: %#v", remote.Headers)
	}
	if remote.Timeout != 180*time.Second || remote.ConnectTimeout != 20*time.Second {
		t.Fatalf("remote timeouts = %s/%s, want 180s/20s", remote.Timeout, remote.ConnectTimeout)
	}

	status := mustMCPStatus(t, resolved, "remote")
	if status.Headers["Authorization"] != RedactedMCPConfigValue {
		t.Fatalf("Authorization redaction = %q, want %q", status.Headers["Authorization"], RedactedMCPConfigValue)
	}
	if status.Headers["X-API-Token"] != RedactedMCPConfigValue {
		t.Fatalf("token-like header redaction = %q, want %q", status.Headers["X-API-Token"], RedactedMCPConfigValue)
	}
	if status.Headers["X-Trace"] != "trace-1" {
		t.Fatalf("non-secret header = %q, want trace-1", status.Headers["X-Trace"])
	}
	if report := resolved.RedactedStatusText(); strings.Contains(report, "sk-live-secret") || strings.Contains(report, "raw-secret") {
		t.Fatalf("redacted status leaked header secrets: %s", report)
	}
}

func TestMCPConfigRejectsInvalidEnvAndTransportBeforeRuntime(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantReason  MCPConfigStatus
		wantMessage string
	}{
		{
			name: "invalid env name",
			body: `
mcp_servers:
  broken:
    command: npx
    env:
      BAD-NAME: value
`,
			wantReason:  MCPConfigStatusInvalidEnv,
			wantMessage: "invalid env variable name",
		},
		{
			name: "both command and url",
			body: `
mcp_servers:
  broken:
    command: npx
    url: https://mcp.example.test/mcp
`,
			wantReason:  MCPConfigStatusInvalidTransport,
			wantMessage: "both command and url",
		},
		{
			name: "neither command nor url",
			body: `
mcp_servers:
  broken:
    enabled: true
`,
			wantReason:  MCPConfigStatusInvalidTransport,
			wantMessage: "requires command or url",
		},
		{
			name: "missing env interpolation",
			body: `
mcp_servers:
  broken:
    command: npx
    env:
      API_KEY: "${MISSING_MCP_KEY}"
`,
			wantReason:  MCPConfigStatusInvalidEnv,
			wantMessage: "missing environment variable MISSING_MCP_KEY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ParseMCPConfigYAML([]byte(tc.body), MCPConfigOptions{
				LookupEnv: lookupMCPTestEnv(nil),
			})
			if err == nil {
				t.Fatalf("ParseMCPConfigYAML succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("error = %q, want %q", err.Error(), tc.wantMessage)
			}
			if strings.Contains(err.Error(), "value") {
				t.Fatalf("error leaked config value: %s", err.Error())
			}
			if len(resolved.Servers) != 0 {
				t.Fatalf("resolved valid servers = %#v, want none", resolved.Servers)
			}
			status := mustMCPStatus(t, resolved, "broken")
			if status.Status != tc.wantReason {
				t.Fatalf("broken status = %q, want %q (%s)", status.Status, tc.wantReason, status.Reason)
			}
		})
	}
}

func TestMCPConfigReportsMissingSDKWithoutPartialServers(t *testing.T) {
	runtimeAvailable := false
	resolved, err := ParseMCPConfigYAML([]byte(`
mcp_servers:
  remote:
    url: https://mcp.example.test/mcp
    headers:
      Authorization: "Bearer ${REMOTE_TOKEN}"
`), MCPConfigOptions{
		LookupEnv:          lookupMCPTestEnv(map[string]string{"REMOTE_TOKEN": "sk-live-secret"}),
		RuntimeAvailable:   &runtimeAvailable,
		RuntimeUnavailable: "MCP SDK unavailable",
	})
	if err == nil {
		t.Fatalf("ParseMCPConfigYAML succeeded, want missing SDK error")
	}
	if !strings.Contains(err.Error(), "MCP SDK unavailable") {
		t.Fatalf("error = %q, want missing SDK reason", err.Error())
	}
	if strings.Contains(err.Error(), "sk-live-secret") {
		t.Fatalf("missing SDK error leaked header secret: %s", err.Error())
	}
	if len(resolved.Servers) != 0 {
		t.Fatalf("resolved servers = %#v, want none while runtime is unavailable", resolved.Servers)
	}
	status := mustMCPStatus(t, resolved, "remote")
	if status.Status != MCPConfigStatusMissingSDK {
		t.Fatalf("remote status = %q, want %q", status.Status, MCPConfigStatusMissingSDK)
	}
}

func TestHonchoMCPConfigKeepsHONCHOAPIURLServerLocal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HONCHO_API_URL", "http://global.example.invalid")

	resolved, err := ParseMCPConfigYAML([]byte(`
mcp_servers:
  honcho:
    command: bunx
    args:
      - wrangler
      - dev
      - mcp/src/index.ts
    env:
      AUTH_HEADER: "Bearer ${HONCHO_KEY}"
      USER_NAME: operator
      HONCHO_API_URL: http://127.0.0.1:28000
`), MCPConfigOptions{LookupEnv: lookupMCPTestEnv(map[string]string{
		"HONCHO_KEY": "honcho-secret",
	})})
	if err != nil {
		t.Fatalf("ParseMCPConfigYAML: %v", err)
	}

	honcho := mustMCPServer(t, resolved, "honcho")
	if got := honcho.Env["HONCHO_API_URL"]; got != "http://127.0.0.1:28000" {
		t.Fatalf("HONCHO_API_URL server env = %q, want self-hosted MCP worker URL", got)
	}
	status := mustMCPStatus(t, resolved, "honcho")
	if _, ok := status.Headers["HONCHO_API_URL"]; ok {
		t.Fatalf("HONCHO_API_URL appeared as request header in %#v", status.Headers)
	}
	if strings.Contains(resolved.RedactedStatusText(), "honcho-secret") {
		t.Fatalf("redacted status leaked Honcho auth header: %s", resolved.RedactedStatusText())
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	runtime := cfg.Goncho.RuntimeConfig()
	if runtime.WorkspaceID != goncho.DefaultWorkspaceID {
		t.Fatalf("Goncho workspace = %q, want default %q", runtime.WorkspaceID, goncho.DefaultWorkspaceID)
	}
	if strings.Contains(strings.ToLower(runtime.WorkspaceID), "honcho") {
		t.Fatalf("Goncho workspace unexpectedly reflected Honcho MCP env: %#v", runtime)
	}
}

func lookupMCPTestEnv(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func mustMCPServer(t *testing.T, resolved MCPConfigResolution, name string) MCPServerDefinition {
	t.Helper()
	server, ok := resolved.Server(name)
	if !ok {
		t.Fatalf("missing MCP server %q in %#v", name, resolved.Servers)
	}
	return server
}

func mustMCPStatus(t *testing.T, resolved MCPConfigResolution, name string) MCPServerStatus {
	t.Helper()
	status, ok := resolved.Status(name)
	if !ok {
		t.Fatalf("missing MCP status %q in %#v", name, resolved.Statuses)
	}
	return status
}
