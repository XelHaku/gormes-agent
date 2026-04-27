package cli

import "testing"

func TestOnboardingSeen_MalformedConfigUnseen(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]any
	}{
		{
			name: "missing onboarding",
			cfg:  map[string]any{},
		},
		{
			name: "non-map onboarding",
			cfg: map[string]any{
				"onboarding": "corrupted",
			},
		},
		{
			name: "non-map seen",
			cfg: map[string]any{
				"onboarding": map[string]any{
					"seen": "corrupted",
				},
			},
		},
		{
			name: "false openclaw residue flag",
			cfg: map[string]any{
				"onboarding": map[string]any{
					"seen": map[string]any{
						OpenClawResidueCleanupFlag: false,
					},
				},
			},
		},
		{
			name: "unrelated seen flag",
			cfg: map[string]any{
				"onboarding": map[string]any{
					"seen": map[string]any{
						"busy_input_prompt": true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if OnboardingSeen(tt.cfg) {
				t.Fatalf("OnboardingSeen(%#v) = true, want false", tt.cfg)
			}
		})
	}
}

func TestOnboardingSeen_OpenClawResidueTrue(t *testing.T) {
	cfg := map[string]any{
		"onboarding": map[string]any{
			"seen": map[string]any{
				OpenClawResidueCleanupFlag: true,
			},
		},
	}

	if !OnboardingSeen(cfg) {
		t.Fatalf("OnboardingSeen(%#v) = false, want true", cfg)
	}
}

func TestMarkOnboardingSeen_InMemoryPreservesOtherFlags(t *testing.T) {
	cfg := map[string]any{
		"onboarding": map[string]any{
			"seen": map[string]any{
				"busy_input_prompt": true,
			},
		},
	}

	got := MarkOnboardingSeen(cfg)
	onboarding, ok := got["onboarding"].(map[string]any)
	if !ok {
		t.Fatalf("MarkOnboardingSeen(%#v) onboarding = %T, want map[string]any", cfg, got["onboarding"])
	}
	seen, ok := onboarding["seen"].(map[string]any)
	if !ok {
		t.Fatalf("MarkOnboardingSeen(%#v) onboarding.seen = %T, want map[string]any", cfg, onboarding["seen"])
	}

	if seen[OpenClawResidueCleanupFlag] != true {
		t.Fatalf("seen[%q] = %#v, want true", OpenClawResidueCleanupFlag, seen[OpenClawResidueCleanupFlag])
	}
	if seen["busy_input_prompt"] != true {
		t.Fatalf("seen[busy_input_prompt] = %#v, want preserved true", seen["busy_input_prompt"])
	}
}
