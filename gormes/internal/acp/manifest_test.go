package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryManifest_AdvertisesGormesACPCommand(t *testing.T) {
	path := filepath.Join("..", "..", "acp_registry", "agent.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	var manifest struct {
		Name         string `json:"name"`
		Distribution struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("json.Unmarshal(agent.json): %v", err)
	}

	if manifest.Name != "gormes" {
		t.Fatalf("manifest name = %q, want gormes", manifest.Name)
	}
	if manifest.Distribution.Command != "gormes" {
		t.Fatalf("manifest distribution.command = %q, want gormes", manifest.Distribution.Command)
	}
	if len(manifest.Distribution.Args) != 1 || manifest.Distribution.Args[0] != "acp" {
		t.Fatalf("manifest distribution.args = %#v, want [\"acp\"]", manifest.Distribution.Args)
	}
}
