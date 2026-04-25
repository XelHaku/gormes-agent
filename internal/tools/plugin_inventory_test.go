package tools

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
)

func TestRegistryRecordsDisabledPluginInventoryWithoutRegisteringTools(t *testing.T) {
	registry := NewRegistry()
	registry.RecordPluginInventory(plugins.Inventory{
		Capabilities: []plugins.CapabilityStatus{
			{
				Plugin: "spotify",
				Kind:   plugins.CapabilityTool,
				Name:   "spotify_playback",
				State:  plugins.StateDisabled,
				Evidence: []plugins.Evidence{{
					Code:  plugins.EvidenceMissingCredential,
					Field: "providers.spotify",
				}},
			},
			{
				Plugin: "example",
				Kind:   plugins.CapabilityDashboard,
				Name:   "example",
				State:  plugins.StateDisabled,
				Evidence: []plugins.Evidence{{
					Code:  plugins.EvidenceExecutionDisabled,
					Field: "runtime",
				}},
			},
			{
				Plugin: "example",
				Kind:   plugins.CapabilityBackendRoute,
				Name:   "/api/plugins/example/",
				State:  plugins.StateDisabled,
				Evidence: []plugins.Evidence{{
					Code:  plugins.EvidenceExecutionDisabled,
					Field: "runtime",
				}},
			},
		},
	})

	if _, ok := registry.Get("spotify_playback"); ok {
		t.Fatal("disabled plugin tool was registered as executable")
	}
	if descriptors := registry.Descriptors(); len(descriptors) != 0 {
		t.Fatalf("descriptors = %+v, want no executable plugin descriptors", descriptors)
	}

	rows := registry.DisabledPluginCapabilities()
	if len(rows) != 3 {
		t.Fatalf("disabled capability rows = %+v, want 3", rows)
	}
	if rows[0].Kind != plugins.CapabilityBackendRoute || rows[0].Name != "/api/plugins/example/" {
		t.Fatalf("rows are not deterministically sorted: %+v", rows)
	}
	if rows[2].Kind != plugins.CapabilityTool || rows[2].Name != "spotify_playback" {
		t.Fatalf("tool row missing from inventory: %+v", rows)
	}
	if rows[2].Evidence[0].Code != plugins.EvidenceMissingCredential {
		t.Fatalf("tool row evidence = %+v, want missing credential", rows[2].Evidence)
	}
}
