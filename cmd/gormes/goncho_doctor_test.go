package main

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
)

func TestDoctorGonchoConfigOutputIncludesEffectiveSettingsAndRedactsSecrets(t *testing.T) {
	cfg := config.Config{
		Hermes: config.HermesCfg{APIKey: "sk-do-not-print"},
		Goncho: config.GonchoCfg{
			Enabled:                      true,
			Workspace:                    "ops-workspace",
			ObserverPeer:                 "ops-observer",
			RecentMessages:               5,
			MaxMessageSize:               25_000,
			MaxFileSize:                  5_242_880,
			GetContextMaxTokens:          100_000,
			ReasoningEnabled:             true,
			PeerCardEnabled:              true,
			SummaryEnabled:               true,
			DreamEnabled:                 false,
			DeriverWorkers:               1,
			RepresentationBatchMaxTokens: 1024,
			DialecticDefaultLevel:        "low",
		},
	}

	out := doctorGonchoConfig(cfg).Format()

	for _, want := range []string{
		"Goncho config",
		"workspace=ops-workspace",
		"observer_peer=ops-observer",
		"enabled=true",
		"recent_messages=5",
		"max_message_size=25000",
		"max_file_size=5242880",
		"get_context_max_tokens=100000",
		"reasoning_enabled=true",
		"peer_card_enabled=true",
		"summary_enabled=true",
		"dream_enabled=false",
		"feature_disabled:dream",
		"deriver_workers=1",
		"representation_batch_max_tokens=1024",
		"dialectic_default_level=low",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor Goncho output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-do-not-print") {
		t.Fatalf("doctor Goncho output leaked secret:\n%s", out)
	}
}
