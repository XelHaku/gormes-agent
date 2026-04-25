package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
)

func TestDashboardExtensionStatusDistinguishesThemesPluginsAndBackendRoutes(t *testing.T) {
	inventory := plugins.Inventory{
		Plugins: []plugins.PluginStatus{{
			Name:        "strike-freedom-cockpit",
			Version:     "1.0.0",
			Label:       "Strike Freedom Cockpit",
			Description: "Cockpit dashboard shell extension",
			State:       plugins.StateDisabled,
			Dashboard: &plugins.DashboardManifest{
				Name:        "strike-freedom-cockpit",
				Label:       "Strike Freedom Cockpit",
				Description: "Cockpit dashboard shell extension",
				Version:     "1.0.0",
				Entry:       "dist/index.js",
				CSS:         "dist/style.css",
				API:         "plugin_api.py",
				Tab: plugins.DashboardTab{
					Path:   "/strike-freedom-cockpit",
					Hidden: true,
				},
				Slots: []string{"sidebar", "header-left", "overlay"},
			},
			Capabilities: []plugins.CapabilityStatus{
				{
					Plugin: "strike-freedom-cockpit",
					Kind:   plugins.CapabilityDashboard,
					Name:   "strike-freedom-cockpit",
					State:  plugins.StateDisabled,
					Evidence: []plugins.Evidence{{
						Code:  plugins.EvidenceExecutionDisabled,
						Field: "runtime",
					}},
				},
				{
					Plugin: "strike-freedom-cockpit",
					Kind:   plugins.CapabilityBackendRoute,
					Name:   "/api/plugins/strike-freedom-cockpit/",
					State:  plugins.StateDisabled,
					Evidence: []plugins.Evidence{{
						Code:  plugins.EvidenceExecutionDisabled,
						Field: "runtime",
					}},
				},
			},
			Evidence: []plugins.Evidence{{
				Code:  plugins.EvidenceExecutionDisabled,
				Field: "runtime",
			}},
		}},
	}
	inventory = plugins.BuildInventory(inventory.Plugins)
	srv := NewServer(Config{ModelName: "gormes-agent", PluginInventory: inventory})

	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	var got struct {
		Panels map[string]struct {
			State    string `json:"state"`
			Reason   string `json:"reason"`
			Category string `json:"category"`
		} `json:"panels"`
		UpstreamReactRuntime struct {
			State    string `json:"state"`
			Required bool   `json:"required"`
		} `json:"upstream_react_runtime"`
		DashboardExtensions struct {
			Runtime struct {
				State            string             `json:"state"`
				ReactViteRuntime string             `json:"react_vite_runtime"`
				Evidence         []plugins.Evidence `json:"evidence"`
			} `json:"runtime"`
			Themes struct {
				State    string             `json:"state"`
				Active   string             `json:"active"`
				Evidence []plugins.Evidence `json:"evidence"`
			} `json:"themes"`
			UIPlugins     []plugins.PluginStatus     `json:"ui_plugins"`
			BackendRoutes []plugins.CapabilityStatus `json:"backend_routes"`
		} `json:"dashboard_extensions"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	if got.Panels["chat"].Category != "built_in" {
		t.Fatalf("chat panel = %+v, want built_in category", got.Panels["chat"])
	}
	if got.Panels["plugins"].Category != "optional_extension" || got.Panels["plugins"].State != plugins.StateDisabled {
		t.Fatalf("plugins panel = %+v, want disabled optional_extension", got.Panels["plugins"])
	}
	if got.UpstreamReactRuntime.Required || got.UpstreamReactRuntime.State != "absent" {
		t.Fatalf("upstream runtime = %+v, want absent and not required", got.UpstreamReactRuntime)
	}
	if got.DashboardExtensions.Runtime.State != plugins.StateDisabled || got.DashboardExtensions.Runtime.ReactViteRuntime != "absent" {
		t.Fatalf("extension runtime = %+v, want disabled absent React/Vite runtime", got.DashboardExtensions.Runtime)
	}
	assertExtensionEvidence(t, got.DashboardExtensions.Runtime.Evidence, plugins.EvidenceExecutionDisabled, "react_vite_runtime")
	if got.DashboardExtensions.Themes.State != plugins.StateUnavailable || got.DashboardExtensions.Themes.Active != "" {
		t.Fatalf("theme inventory = %+v, want unavailable without active theme", got.DashboardExtensions.Themes)
	}
	assertExtensionEvidence(t, got.DashboardExtensions.Themes.Evidence, plugins.EvidenceThemeRuntimeUnavailable, "dashboard.theme")
	if len(got.DashboardExtensions.UIPlugins) != 1 || got.DashboardExtensions.UIPlugins[0].Name != "strike-freedom-cockpit" {
		t.Fatalf("ui plugins = %+v, want dashboard plugin row", got.DashboardExtensions.UIPlugins)
	}
	if got.DashboardExtensions.UIPlugins[0].RuntimeCodeExecuted {
		t.Fatalf("ui plugin row executed runtime code: %+v", got.DashboardExtensions.UIPlugins[0])
	}
	if len(got.DashboardExtensions.BackendRoutes) != 1 || got.DashboardExtensions.BackendRoutes[0].Name != "/api/plugins/strike-freedom-cockpit/" {
		t.Fatalf("backend routes = %+v, want plugin backend route inventory", got.DashboardExtensions.BackendRoutes)
	}
	assertExtensionEvidence(t, got.DashboardExtensions.BackendRoutes[0].Evidence, plugins.EvidenceExecutionDisabled, "runtime")
}

func TestDashboardPluginsEndpointReturnsUnavailableRuntimeEvidenceWhenEmpty(t *testing.T) {
	srv := NewServer(Config{ModelName: "gormes-agent"})

	resp := getJSON(t, srv.Handler(), "/api/dashboard/plugins", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	var got struct {
		Runtime struct {
			State            string             `json:"state"`
			Reason           string             `json:"reason"`
			ReactViteRuntime string             `json:"react_vite_runtime"`
			Evidence         []plugins.Evidence `json:"evidence"`
		} `json:"runtime"`
		Themes struct {
			State    string             `json:"state"`
			Evidence []plugins.Evidence `json:"evidence"`
		} `json:"themes"`
		Plugins       []plugins.PluginStatus     `json:"plugins"`
		Capabilities  []plugins.CapabilityStatus `json:"capabilities"`
		BackendRoutes []plugins.CapabilityStatus `json:"backend_routes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode dashboard plugin inventory: %v; body=%s", err, resp.Body.String())
	}

	if got.Runtime.State != plugins.StateDisabled || got.Runtime.ReactViteRuntime != "absent" {
		t.Fatalf("runtime = %+v, want disabled absent runtime", got.Runtime)
	}
	if !strings.Contains(got.Runtime.Reason, "React/Vite") {
		t.Fatalf("runtime reason = %q, want React/Vite degradation", got.Runtime.Reason)
	}
	assertExtensionEvidence(t, got.Runtime.Evidence, plugins.EvidenceExecutionDisabled, "react_vite_runtime")
	if got.Themes.State != plugins.StateUnavailable {
		t.Fatalf("themes = %+v, want unavailable", got.Themes)
	}
	assertExtensionEvidence(t, got.Themes.Evidence, plugins.EvidenceThemeRuntimeUnavailable, "dashboard.theme")
	if len(got.Plugins) != 0 || len(got.Capabilities) != 0 || len(got.BackendRoutes) != 0 {
		t.Fatalf("empty inventory = plugins:%+v capabilities:%+v backend:%+v", got.Plugins, got.Capabilities, got.BackendRoutes)
	}
}

func assertExtensionEvidence(t *testing.T, evidence []plugins.Evidence, code, field string) {
	t.Helper()
	for _, ev := range evidence {
		if ev.Code == code && ev.Field == field {
			return
		}
	}
	t.Fatalf("missing evidence code=%q field=%q in %+v", code, field, evidence)
}
