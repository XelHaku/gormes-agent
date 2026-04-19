package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_RendersApprovedPhase1Story(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	wants := []string{
		"The Agent That GOes With You.",
		"Open Source • MIT License • Phase 1 Go Port",
		"API_SERVER_ENABLED=true hermes gateway start",
		"./bin/gormes",
		"Phase 1 uses your existing Hermes backend.",
		"The Port Is Already Moving",
		"Help Finish the Port",
		"Same agent. Same memory. Same workflows.",
	}

	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, body)
		}
	}
}
