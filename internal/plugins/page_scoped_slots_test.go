package plugins

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

// Test-only copy of Hermes' documented page-scoped slots. LoadDir must keep
// accepting arbitrary slot names as inert metadata; this is not a validator.
var hermesKnownPageScopedSlotCatalogue = []string{
	"sessions:top",
	"sessions:bottom",
	"analytics:top",
	"analytics:bottom",
	"logs:top",
	"logs:bottom",
	"cron:top",
	"cron:bottom",
	"skills:top",
	"skills:bottom",
	"config:top",
	"config:bottom",
	"env:top",
	"env:bottom",
	"docs:top",
	"docs:bottom",
	"chat:top",
	"chat:bottom",
}

func TestLoadDirPreservesHermesPageScopedSlotNamesAsInertMetadata(t *testing.T) {
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
	dir := writePluginFixture(t, "page-scoped-dashboard", map[string]string{
		filepath.Join("dashboard", "manifest.json"): dashboardManifestFixture(t, "page-scoped-dashboard", declaredSlots),
	})

	status := LoadDir(dir, LoadOptions{Source: SourceUser, CurrentGormesVersion: "1.0.0"})

	if status.State != StateDisabled {
		t.Fatalf("state = %q, want disabled; evidence=%+v", status.State, status.Evidence)
	}
	if status.RuntimeCodeExecuted {
		t.Fatal("LoadDir executed dashboard plugin runtime code")
	}
	if status.Dashboard == nil {
		t.Fatal("dashboard manifest missing")
	}
	if !slices.Equal(status.Dashboard.Slots, declaredSlots) {
		t.Fatalf("dashboard slots = %#v, want %#v", status.Dashboard.Slots, declaredSlots)
	}
	for _, slot := range declaredSlots {
		if !slices.Contains(hermesKnownPageScopedSlotCatalogue, slot) {
			t.Fatalf("test fixture slot %q is not documented in the Hermes page-scoped catalogue", slot)
		}
	}
	assertCapability(t, status.Capabilities, CapabilityDashboard, "page-scoped-dashboard", StateDisabled)
	assertNoBackendRouteCapability(t, status.Capabilities)
}

func TestLoadDirKeepsEmptyAndUnknownPageScopedSlotNamesInert(t *testing.T) {
	declaredSlots := []string{"", "future-page:middle", "chat:bottom"}
	dir := writePluginFixture(t, "future-page-slots", map[string]string{
		filepath.Join("dashboard", "manifest.json"): dashboardManifestFixture(t, "future-page-slots", declaredSlots),
	})

	status := LoadDir(dir, LoadOptions{Source: SourceUser, CurrentGormesVersion: "1.0.0"})

	if status.State != StateDisabled {
		t.Fatalf("state = %q, want disabled; evidence=%+v", status.State, status.Evidence)
	}
	if status.Dashboard == nil {
		t.Fatal("dashboard manifest missing")
	}
	if !slices.Equal(status.Dashboard.Slots, declaredSlots) {
		t.Fatalf("dashboard slots = %#v, want inert metadata %#v", status.Dashboard.Slots, declaredSlots)
	}
	assertCapability(t, status.Capabilities, CapabilityDashboard, "future-page-slots", StateDisabled)
	assertNoBackendRouteCapability(t, status.Capabilities)
}

func dashboardManifestFixture(t *testing.T, name string, slots []string) string {
	t.Helper()
	slotJSON, err := json.Marshal(slots)
	if err != nil {
		t.Fatal(err)
	}
	return `{
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
}

func assertNoBackendRouteCapability(t *testing.T, capabilities []CapabilityStatus) {
	t.Helper()
	for _, capability := range capabilities {
		if capability.Kind == CapabilityBackendRoute {
			t.Fatalf("backend route capability = %+v, want page-scoped slots to remain non-executable metadata", capability)
		}
	}
}
