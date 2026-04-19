package site

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

func TestServer_RendersWithoutModuleRootTemplates(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	root := filepath.Join(filepath.Dir(file), "..", "..")
	templatesDir := filepath.Join(root, "templates")
	hiddenDir := templatesDir + ".hidden-for-test"

	if _, err := os.Stat(templatesDir); err == nil {
		if err := os.Rename(templatesDir, hiddenDir); err != nil {
			t.Fatalf("hide module-root templates: %v", err)
		}
		defer func() {
			if err := os.Rename(hiddenDir, templatesDir); err != nil {
				t.Fatalf("restore module-root templates: %v", err)
			}
		}()
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat module-root templates: %v", err)
	}

	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "The Agent That GOes With You.") {
		t.Fatalf("body missing embedded landing page content:\n%s", rr.Body.String())
	}
}
