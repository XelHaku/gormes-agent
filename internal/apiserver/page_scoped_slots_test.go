package apiserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
)

func TestDashboardPluginsEndpointPreservesPageScopedSlotMetadata(t *testing.T) {
	declaredSlots := []string{
		"sessions:top",
		"analytics:bottom",
		"logs:top",
		"skills:bottom",
		"config:top",
		"env:bottom",
		"docs:top",
		"cron:bottom",
		"chat:top",
	}
	dir := writeDashboardPluginFixture(t, "page-scoped-dashboard", declaredSlots)
	status := plugins.LoadDir(dir, plugins.LoadOptions{Source: plugins.SourceUser, CurrentGormesVersion: "1.0.0"})
	inventory := plugins.BuildInventory([]plugins.PluginStatus{status})
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
		Plugins       []plugins.PluginStatus     `json:"plugins"`
		BackendRoutes []plugins.CapabilityStatus `json:"backend_routes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode dashboard plugin inventory: %v; body=%s", err, resp.Body.String())
	}

	if got.Runtime.State != plugins.StateDisabled || got.Runtime.ReactViteRuntime != "absent" {
		t.Fatalf("runtime = %+v, want disabled absent React/Vite runtime", got.Runtime)
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("plugins = %+v, want one disabled metadata row", got.Plugins)
	}
	plugin := got.Plugins[0]
	if plugin.State != plugins.StateDisabled || plugin.RuntimeCodeExecuted {
		t.Fatalf("plugin row = %+v, want disabled without runtime execution", plugin)
	}
	if plugin.Dashboard == nil {
		t.Fatal("dashboard manifest missing from plugin row")
	}
	if !slices.Equal(plugin.Dashboard.Slots, declaredSlots) {
		t.Fatalf("dashboard slots = %#v, want %#v", plugin.Dashboard.Slots, declaredSlots)
	}
	if len(got.BackendRoutes) != 0 {
		t.Fatalf("backend routes = %+v, want no executable backend route for page-scoped slot metadata", got.BackendRoutes)
	}
}

func writeDashboardPluginFixture(t *testing.T, name string, slots []string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name, "dashboard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	slotJSON, err := json.Marshal(slots)
	if err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name": "` + name + `",
  "label": "Page Scoped Dashboard",
  "description": "Synthetic metadata-only fixture for Hermes page-scoped slots",
  "version": "1.0.0",
  "tab": {
    "path": "/` + name + `",
    "hidden": true
  },
  "slots": ` + string(slotJSON) + `,
  "entry": "dist/index.js"
}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(dir)
}
