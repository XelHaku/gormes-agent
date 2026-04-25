package apiserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
)

func TestDashboardStatusExposesDisabledPluginCapabilityInventory(t *testing.T) {
	inventory := plugins.Inventory{
		Plugins: []plugins.PluginStatus{{
			Name:        "spotify",
			Version:     "1.0.0",
			Label:       "Spotify",
			Description: "Native Spotify integration",
			State:       plugins.StateDisabled,
			Evidence: []plugins.Evidence{{
				Code:  plugins.EvidenceMissingCredential,
				Field: "providers.spotify",
			}},
		}},
		Capabilities: []plugins.CapabilityStatus{
			{
				Plugin: "spotify",
				Kind:   plugins.CapabilityTool,
				Name:   "spotify_search",
				State:  plugins.StateDisabled,
				Evidence: []plugins.Evidence{{
					Code:  plugins.EvidenceMissingCredential,
					Field: "providers.spotify",
				}},
			},
			{
				Plugin: "spotify",
				Kind:   plugins.CapabilityBackendRoute,
				Name:   "/api/plugins/spotify/",
				State:  plugins.StateDisabled,
				Evidence: []plugins.Evidence{{
					Code:  plugins.EvidenceExecutionDisabled,
					Field: "runtime",
				}},
			},
		},
	}
	srv := NewServer(Config{ModelName: "gormes-agent", PluginInventory: inventory})

	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	var got struct {
		Panels map[string]struct {
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"panels"`
		Plugins plugins.Inventory `json:"plugins"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if got.Panels["plugins"].State != plugins.StateDisabled {
		t.Fatalf("plugin panel = %+v, want disabled", got.Panels["plugins"])
	}
	if len(got.Plugins.Capabilities) != 2 {
		t.Fatalf("plugin capabilities = %+v, want disabled inventory rows", got.Plugins.Capabilities)
	}
	if got.Plugins.Capabilities[0].Name != "/api/plugins/spotify/" || got.Plugins.Capabilities[0].State != plugins.StateDisabled {
		t.Fatalf("first capability row = %+v, want sorted backend route disabled", got.Plugins.Capabilities[0])
	}
	if got.Plugins.Plugins[0].Evidence[0].Code != plugins.EvidenceMissingCredential {
		t.Fatalf("plugin evidence = %+v, want missing credential", got.Plugins.Plugins[0].Evidence)
	}
}

func TestDashboardPluginsEndpointReturnsMetadataOnlyDisabledRows(t *testing.T) {
	inventory := plugins.Inventory{
		Plugins: []plugins.PluginStatus{{
			Name:                "strike-freedom-cockpit",
			Version:             "1.0.0",
			Label:               "Strike Freedom Cockpit",
			Description:         "MS-STATUS sidebar + header crest",
			State:               plugins.StateDisabled,
			RuntimeCodeExecuted: false,
			Dashboard: &plugins.DashboardManifest{
				Name:        "strike-freedom-cockpit",
				Label:       "Strike Freedom Cockpit",
				Description: "MS-STATUS sidebar + header crest",
				Entry:       "dist/index.js",
				Tab: plugins.DashboardTab{
					Path:   "/strike-freedom-cockpit",
					Hidden: true,
				},
				Slots: []string{"sidebar", "header-left", "footer-right"},
			},
			Capabilities: []plugins.CapabilityStatus{{
				Plugin: "strike-freedom-cockpit",
				Kind:   plugins.CapabilityDashboard,
				Name:   "strike-freedom-cockpit",
				State:  plugins.StateDisabled,
			}},
			Evidence: []plugins.Evidence{{
				Code:  plugins.EvidenceExecutionDisabled,
				Field: "runtime",
			}},
		}},
	}
	srv := NewServer(Config{PluginInventory: inventory})

	resp := getJSON(t, srv.Handler(), "/api/dashboard/plugins", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	var got struct {
		Runtime struct {
			State            string `json:"state"`
			ReactViteRuntime string `json:"react_vite_runtime"`
		} `json:"runtime"`
		Plugins []plugins.PluginStatus `json:"plugins"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode plugins: %v", err)
	}
	if got.Runtime.State != plugins.StateDisabled || got.Runtime.ReactViteRuntime != "absent" {
		t.Fatalf("runtime = %+v, want disabled absent runtime", got.Runtime)
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("plugins = %+v, want one disabled metadata row", got.Plugins)
	}
	if got.Plugins[0].State != plugins.StateDisabled || got.Plugins[0].RuntimeCodeExecuted {
		t.Fatalf("plugin row = %+v, want disabled without runtime execution", got.Plugins[0])
	}
	if got.Plugins[0].Dashboard == nil || !got.Plugins[0].Dashboard.Tab.Hidden || len(got.Plugins[0].Dashboard.Slots) != 3 {
		t.Fatalf("dashboard manifest row = %+v, want hidden slot metadata", got.Plugins[0].Dashboard)
	}
}
