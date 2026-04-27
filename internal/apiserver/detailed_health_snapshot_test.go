package apiserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDetailedHealthSnapshot_AllSystemsReady(t *testing.T) {
	snapshot := DetailedHealthSnapshot(DetailedHealthSnapshotInput{
		Provider: DetailedHealthProviderInput{
			Name:       "openai",
			Model:      "gpt-4.1",
			Configured: true,
		},
		ResponseStore: DetailedHealthResponseStoreInput{
			Enabled:      true,
			Stored:       3,
			MaxStored:    100,
			LRUEvictions: 1,
		},
		RunEvents: DetailedHealthRunEventsInput{
			Available:     true,
			Active:        2,
			OrphanedSwept: 1,
			TTLSeconds:    300,
		},
		Gateway: DetailedHealthGatewayInput{
			Available:    true,
			State:        "running",
			ActiveAgents: 1,
			Platforms:    map[string]string{"telegram": "running"},
			ProxyState:   "ready",
		},
		Cron: DetailedHealthCronInput{
			Available:     true,
			Enabled:       true,
			Jobs:          4,
			Paused:        1,
			LastRunStatus: "success",
		},
	})

	assertSectionReady(t, "provider", snapshot.Provider.Status, snapshot.Provider.Evidence)
	assertSectionReady(t, "response_store", snapshot.ResponseStore.Status, snapshot.ResponseStore.Evidence)
	assertSectionReady(t, "run_events", snapshot.RunEvents.Status, snapshot.RunEvents.Evidence)
	assertSectionReady(t, "gateway", snapshot.Gateway.Status, snapshot.Gateway.Evidence)
	assertSectionReady(t, "cron", snapshot.Cron.Status, snapshot.Cron.Evidence)

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode snapshot object: %v", err)
	}
	for _, field := range []string{"provider", "response_store", "run_events", "gateway", "cron"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("snapshot JSON missing stable field %q: %s", field, raw)
		}
	}
	if snapshot.Provider.Name != "openai" || snapshot.Provider.Model != "gpt-4.1" {
		t.Fatalf("provider section = %+v, want provider name/model", snapshot.Provider)
	}
	if !snapshot.ResponseStore.StoreEnabled || snapshot.ResponseStore.Stored != 3 || snapshot.ResponseStore.MaxStored != 100 {
		t.Fatalf("response_store section = %+v, want store stats", snapshot.ResponseStore)
	}
	if !snapshot.RunEvents.Available || snapshot.RunEvents.TTLSeconds != 300 {
		t.Fatalf("run_events section = %+v, want run stream stats", snapshot.RunEvents)
	}
	if !snapshot.Gateway.Available || snapshot.Gateway.GatewayState != "running" || snapshot.Gateway.Platforms["telegram"] != "running" {
		t.Fatalf("gateway section = %+v, want gateway runtime stats", snapshot.Gateway)
	}
	if !snapshot.Cron.Available || !snapshot.Cron.Enabled || snapshot.Cron.Jobs != 4 || snapshot.Cron.Paused != 1 {
		t.Fatalf("cron section = %+v, want cron read-model stats", snapshot.Cron)
	}
}

func TestDetailedHealthSnapshot_DegradedEvidence(t *testing.T) {
	snapshot := DetailedHealthSnapshot(DetailedHealthSnapshotInput{
		Provider: DetailedHealthProviderInput{
			Name:       "openai",
			Model:      "gpt-4.1",
			Configured: false,
		},
		ResponseStore: DetailedHealthResponseStoreInput{
			Enabled: false,
		},
		RunEvents: DetailedHealthRunEventsInput{
			Available: false,
		},
		Gateway: DetailedHealthGatewayInput{
			Available: false,
			State:     "startup_failed",
		},
		Cron: DetailedHealthCronInput{
			Available: false,
			Enabled:   true,
		},
	})

	assertSectionDegradedWithEvidence(t, "provider", snapshot.Provider.Status, snapshot.Provider.Evidence, "provider_unconfigured")
	assertSectionDegradedWithEvidence(t, "response_store", snapshot.ResponseStore.Status, snapshot.ResponseStore.Evidence, "response_store_disabled")
	assertSectionDegradedWithEvidence(t, "run_events", snapshot.RunEvents.Status, snapshot.RunEvents.Evidence, "run_events_unavailable")
	assertSectionDegradedWithEvidence(t, "gateway", snapshot.Gateway.Status, snapshot.Gateway.Evidence, "gateway_unavailable")
	assertSectionDegradedWithEvidence(t, "cron", snapshot.Cron.Status, snapshot.Cron.Evidence, "cron_unavailable")
}

func TestDetailedHealthSnapshot_RedactsSecrets(t *testing.T) {
	snapshot := DetailedHealthSnapshot(DetailedHealthSnapshotInput{
		Provider: DetailedHealthProviderInput{
			Name:              "openai",
			Model:             "gpt-4.1",
			Configured:        false,
			APIKey:            "sk-provider-secret",
			RawRequestPayload: `{"messages":[{"content":"raw request secret"}]}`,
		},
		ResponseStore: DetailedHealthResponseStoreInput{
			Enabled: false,
		},
		RunEvents: DetailedHealthRunEventsInput{
			Available: false,
		},
		Gateway: DetailedHealthGatewayInput{
			Available: false,
			State:     "startup_failed",
			Token:     "gateway-token-secret",
		},
		Cron: DetailedHealthCronInput{
			Available:    false,
			Enabled:      true,
			ScriptBodies: []string{"curl https://example.invalid?token=cron-script-secret"},
		},
	})

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	body := string(raw)
	for _, secret := range []string{
		"sk-provider-secret",
		"raw request secret",
		"gateway-token-secret",
		"cron-script-secret",
		"curl https://example.invalid",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("snapshot leaked secret %q in JSON: %s", secret, body)
		}
	}
	assertSectionDegradedWithEvidence(t, "provider", snapshot.Provider.Status, snapshot.Provider.Evidence, "provider_unconfigured")
	assertSectionDegradedWithEvidence(t, "cron", snapshot.Cron.Status, snapshot.Cron.Evidence, "cron_unavailable")
}

func TestDetailedHealthSnapshot_IsPureValueModel(t *testing.T) {
	platforms := map[string]string{"telegram": "running"}
	scriptBodies := []string{"do not mutate"}
	input := DetailedHealthSnapshotInput{
		Provider: DetailedHealthProviderInput{
			Name:       "native",
			Model:      "gormes-agent",
			Configured: true,
		},
		ResponseStore: DetailedHealthResponseStoreInput{
			Enabled: true,
		},
		RunEvents: DetailedHealthRunEventsInput{
			Available: true,
		},
		Gateway: DetailedHealthGatewayInput{
			Available: true,
			State:     "running",
			Platforms: platforms,
		},
		Cron: DetailedHealthCronInput{
			Available:    true,
			Enabled:      true,
			ScriptBodies: scriptBodies,
		},
	}

	first := DetailedHealthSnapshot(input)
	second := DetailedHealthSnapshot(input)

	if first.Gateway.Platforms["telegram"] != "running" || second.Gateway.Platforms["telegram"] != "running" {
		t.Fatalf("snapshots lost gateway platform state: first=%+v second=%+v", first.Gateway.Platforms, second.Gateway.Platforms)
	}
	first.Gateway.Platforms["telegram"] = "mutated"
	if second.Gateway.Platforms["telegram"] != "running" || input.Gateway.Platforms["telegram"] != "running" {
		t.Fatalf("snapshot shared mutable gateway platform map: first=%+v second=%+v input=%+v", first.Gateway.Platforms, second.Gateway.Platforms, input.Gateway.Platforms)
	}
	if len(input.Cron.ScriptBodies) != 1 || input.Cron.ScriptBodies[0] != "do not mutate" {
		t.Fatalf("snapshot mutated cron script bodies: %+v", input.Cron.ScriptBodies)
	}
}

func assertSectionReady(t *testing.T, name, status string, evidence []DetailedHealthEvidence) {
	t.Helper()
	if status != "ready" {
		t.Fatalf("%s status = %q, want ready", name, status)
	}
	if len(evidence) != 0 {
		t.Fatalf("%s evidence = %+v, want empty for ready section", name, evidence)
	}
}

func assertSectionDegradedWithEvidence(t *testing.T, name, status string, evidence []DetailedHealthEvidence, code string) {
	t.Helper()
	if status != "degraded" {
		t.Fatalf("%s status = %q, want degraded", name, status)
	}
	for _, item := range evidence {
		if item.Code == code {
			return
		}
	}
	t.Fatalf("%s evidence = %+v, want code %q", name, evidence, code)
}
