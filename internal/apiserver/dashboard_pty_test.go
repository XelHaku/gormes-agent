package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestDashboardStatus_DefaultsToAPIOnlyWhenPtyAndSidecarUnconfigured pins the
// degraded mode contract: when no embedded chat transport is wired up the
// dashboard reports `chat` (the API-only path) as enabled, while the new
// pty_chat and chat_sidecar panels report disabled with descriptive reasons.
func TestDashboardStatus_DefaultsToAPIOnlyWhenPtyAndSidecarUnconfigured(t *testing.T) {
	srv := NewServer(Config{
		ModelName: "gormes-agent",
		Loop:      &dashboardContractLoop{},
	})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	got := decodeDashboardChatPanels(t, status.Body.Bytes())

	if got["chat"].State != "enabled" {
		t.Fatalf("chat panel = %+v, want enabled API-only fallback", got["chat"])
	}
	pty := got["pty_chat"]
	if pty.State != "disabled" || pty.Category != dashboardPanelOptional || !strings.Contains(pty.Reason, "PTY") {
		t.Fatalf("pty_chat panel = %+v, want disabled optional PTY", pty)
	}
	side := got["chat_sidecar"]
	if side.State != "disabled" || side.Category != dashboardPanelOptional || !strings.Contains(side.Reason, "sidecar") {
		t.Fatalf("chat_sidecar panel = %+v, want disabled optional sidecar", side)
	}
}

// TestDashboardStatus_ReportsPtyAndSidecarAvailable confirms that wiring both
// PTY and sidecar availability into Config exposes them as enabled panels with
// distinct endpoint hints, separate from the API-only chat panel.
func TestDashboardStatus_ReportsPtyAndSidecarAvailable(t *testing.T) {
	srv := NewServer(Config{
		ModelName: "gormes-agent",
		Loop:      &dashboardContractLoop{},
		ChatTransport: ChatTransportStatus{
			PTYAvailable:     true,
			SidecarAvailable: true,
		},
	})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	got := decodeDashboardChatPanels(t, status.Body.Bytes())

	if got["chat"].State != "enabled" {
		t.Fatalf("chat panel = %+v, want enabled", got["chat"])
	}
	pty := got["pty_chat"]
	if pty.State != "enabled" || pty.Category != dashboardPanelOptional {
		t.Fatalf("pty_chat panel = %+v, want enabled optional", pty)
	}
	if len(pty.Endpoints) == 0 {
		t.Fatalf("pty_chat endpoints = %v, want at least one transport hint", pty.Endpoints)
	}
	side := got["chat_sidecar"]
	if side.State != "enabled" || side.Category != dashboardPanelOptional {
		t.Fatalf("chat_sidecar panel = %+v, want enabled optional", side)
	}
	if len(side.Endpoints) == 0 {
		t.Fatalf("chat_sidecar endpoints = %v, want at least one event channel hint", side.Endpoints)
	}
}

// TestDashboardStatus_PtyUnavailableDegradesToAPIOnly proves the degraded mode:
// when PTY is reported unavailable the chat panel stays enabled (the dashboard
// can fall back to the OpenAI-compatible chat surface) while pty_chat and the
// chat_sidecar panel both report disabled with reasons.
func TestDashboardStatus_PtyUnavailableDegradesToAPIOnly(t *testing.T) {
	srv := NewServer(Config{
		ModelName: "gormes-agent",
		Loop:      &dashboardContractLoop{},
		ChatTransport: ChatTransportStatus{
			PTYAvailable:     false,
			PTYReason:        "pseudo-terminals are unavailable on windows",
			SidecarAvailable: true,
		},
	})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	got := decodeDashboardChatPanels(t, status.Body.Bytes())

	if got["chat"].State != "enabled" {
		t.Fatalf("chat panel = %+v, want enabled API-only fallback", got["chat"])
	}
	pty := got["pty_chat"]
	if pty.State != "disabled" || !strings.Contains(pty.Reason, "windows") {
		t.Fatalf("pty_chat panel = %+v, want disabled with PTY reason surfaced", pty)
	}
	side := got["chat_sidecar"]
	if side.State != "disabled" || !strings.Contains(side.Reason, "PTY") {
		t.Fatalf("chat_sidecar panel = %+v, want disabled because PTY is the sidecar's host", side)
	}
}

// TestDashboardStatus_SidecarUnavailableKeepsPty pins the third acceptance
// criterion: a sidecar publish failure must not kill PTY, so the API surface
// must continue to report pty_chat enabled while chat_sidecar drops to
// disabled with a sidecar-specific reason.
func TestDashboardStatus_SidecarUnavailableKeepsPty(t *testing.T) {
	srv := NewServer(Config{
		ModelName: "gormes-agent",
		Loop:      &dashboardContractLoop{},
		ChatTransport: ChatTransportStatus{
			PTYAvailable:     true,
			SidecarAvailable: false,
			SidecarReason:    "sidecar publisher queue is full",
		},
	})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	got := decodeDashboardChatPanels(t, status.Body.Bytes())

	if got["chat"].State != "enabled" {
		t.Fatalf("chat panel = %+v, want enabled", got["chat"])
	}
	pty := got["pty_chat"]
	if pty.State != "enabled" {
		t.Fatalf("pty_chat panel = %+v, want enabled even when sidecar is failing", pty)
	}
	side := got["chat_sidecar"]
	if side.State != "disabled" || !strings.Contains(side.Reason, "queue is full") {
		t.Fatalf("chat_sidecar panel = %+v, want disabled with publisher reason", side)
	}
}

// TestDashboardChatTransport_PanelEndpointsAreSeparate guards the contract that
// PTY byte transport and structured tool-event publication never share the
// same endpoint identifier, which is what keeps the two transports separable
// in the API server's dashboard contract.
func TestDashboardChatTransport_PanelEndpointsAreSeparate(t *testing.T) {
	srv := NewServer(Config{
		ModelName: "gormes-agent",
		Loop:      &dashboardContractLoop{},
		ChatTransport: ChatTransportStatus{
			PTYAvailable:     true,
			SidecarAvailable: true,
		},
	})
	status := getJSON(t, srv.Handler(), "/api/status", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", status.Code, status.Body.String())
	}

	got := decodeDashboardChatPanels(t, status.Body.Bytes())

	pty := got["pty_chat"].Endpoints
	side := got["chat_sidecar"].Endpoints
	if len(pty) == 0 || len(side) == 0 {
		t.Fatalf("expected non-empty endpoint hints for both transports; pty=%v sidecar=%v", pty, side)
	}
	overlap := make(map[string]bool, len(pty))
	for _, ep := range pty {
		overlap[ep] = true
	}
	for _, ep := range side {
		if overlap[ep] {
			t.Fatalf("transport endpoint %q appears in both pty_chat and chat_sidecar; transports must stay separate", ep)
		}
	}
}

func decodeDashboardChatPanels(t *testing.T, body []byte) map[string]struct {
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
	Category  string   `json:"category"`
	Endpoints []string `json:"endpoints"`
} {
	t.Helper()
	var got struct {
		Panels map[string]struct {
			State     string   `json:"state"`
			Reason    string   `json:"reason"`
			Category  string   `json:"category"`
			Endpoints []string `json:"endpoints"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode dashboard status: %v", err)
	}
	return got.Panels
}
