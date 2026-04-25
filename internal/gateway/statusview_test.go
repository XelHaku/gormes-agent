package gateway

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderStatusSummary_DeterministicChannelRows(t *testing.T) {
	summary := StatusSummary{
		Channels: []StatusChannel{
			{Name: "telegram", Detail: "allowed_chat_id=42"},
			{Name: "slack", Detail: "allowed_channel_id=C999"},
			{Name: "discord", Detail: "allowed_channel_id=D123"},
		},
		Runtime: RuntimeStatus{
			PID:          4242,
			GatewayState: GatewayStateRunning,
			ActiveAgents: 2,
			Platforms: map[string]PlatformRuntimeStatus{
				"telegram": {State: PlatformStateRunning},
				"discord":  {State: PlatformStateFailed, ErrorMessage: "discord: open session: denied"},
				"slack":    {State: PlatformStateStopped},
			},
		},
		Pairing: PairingStatus{
			Platforms: []PairingPlatformStatus{
				{Platform: "telegram", State: PairingPlatformStatePaired, PendingCount: 1, ApprovedCount: 1},
				{Platform: "slack", State: PairingPlatformStateUnpaired, PendingCount: 1, ApprovedCount: 0},
				{Platform: "discord", State: PairingPlatformStatePaired, PendingCount: 0, ApprovedCount: 1},
			},
			Pending: []PairingPendingRecord{
				{Platform: "telegram", UserID: "telegram-user", Code: "TGREADY", AgeSeconds: 60},
				{Platform: "slack", UserID: "slack-user", Code: "SLREADY", AgeSeconds: 120},
			},
			Approved: []PairingApprovedRecord{
				{Platform: "telegram", UserID: "telegram-owner"},
				{Platform: "discord", UserID: "discord-owner", UserName: "Grace"},
			},
			Degraded: []PairingDegradedEvidence{
				{Platform: "discord", Reason: PairingDegradedLockedOut, Message: "platform locked after repeated invalid pairing approvals"},
			},
		},
	}

	got := RenderStatusSummary(summary)
	want := strings.Join([]string{
		"Gateway status",
		"runtime: running (pid=4242 active_agents=2)",
		"channels:",
		"- discord: lifecycle=failed error=\"discord: open session: denied\"; pairing=paired pending=0 approved=1; target=allowed_channel_id=D123",
		"- slack: lifecycle=stopped; pairing=unpaired pending=1 approved=0; target=allowed_channel_id=C999",
		"- telegram: lifecycle=running; pairing=paired pending=1 approved=1; target=allowed_chat_id=42",
		"pairing:",
		"- pending slack user=slack-user code=SLREADY age=120s",
		"- pending telegram user=telegram-user code=TGREADY age=60s",
		"- approved discord user=discord-owner name=Grace",
		"- approved telegram user=telegram-owner",
		"degraded:",
		"- pairing discord locked_out: platform locked after repeated invalid pairing approvals",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderStatusSummary() mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestRenderStatusSummary_NoChannelsAndMissingState(t *testing.T) {
	got := RenderStatusSummary(StatusSummary{
		Pairing: PairingStatus{
			Degraded: []PairingDegradedEvidence{
				{Reason: PairingDegradedMissing, Message: "pairing state is missing"},
			},
		},
	})
	want := strings.Join([]string{
		"Gateway status",
		"runtime: missing",
		"channels: none configured",
		"degraded:",
		"- pairing missing: pairing state is missing",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderStatusSummary() mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestRenderStatusSummary_DoesNotMutateInputOrdering(t *testing.T) {
	channels := []StatusChannel{
		{Name: "telegram", Detail: "allowed_chat_id=42"},
		{Name: "discord", Detail: "allowed_channel_id=D123"},
	}
	original := append([]StatusChannel(nil), channels...)

	_ = RenderStatusSummary(StatusSummary{Channels: channels})

	if !reflect.DeepEqual(channels, original) {
		t.Fatalf("RenderStatusSummary mutated channels: got %+v want %+v", channels, original)
	}
}
