package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_ServesEmbeddedCSS(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/static/site.css", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("content-type = %q, want text/css", ct)
	}
	if !strings.Contains(rr.Body.String(), "--page-bg") {
		t.Fatalf("css is missing expected design variables")
	}
}

func TestServer_IndexLinksCSSAndAvoidsScripts(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `href="/static/site.css"`) {
		t.Fatalf("index is missing stylesheet link\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Fatalf("index must not require JavaScript\n%s", body)
	}
}

func TestServer_UnknownRoutesReturn404(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
