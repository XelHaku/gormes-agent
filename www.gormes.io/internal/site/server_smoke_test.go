package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_IndexRenders200(t *testing.T) {
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
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "Gormes") {
		t.Fatalf("body %q does not mention Gormes", rr.Body.String())
	}
}
