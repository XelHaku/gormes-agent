package tools

import "testing"

func TestBrowserSSRFGuard_CoercesQuotedFalseValues(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want bool
	}{
		{name: "double_quoted_false", raw: `"false"`, want: false},
		{name: "single_quoted_false", raw: `'false'`, want: false},
		{name: "numeric_zero", raw: 0, want: false},
		{name: "no", raw: "no", want: false},
		{name: "off", raw: "off", want: false},
		{name: "true", raw: "true", want: true},
		{name: "yes", raw: "yes", want: true},
		{name: "on", raw: "on", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CoerceBrowserSSRFGuardBool(tt.raw, true)
			if got.Value != tt.want {
				t.Fatalf("CoerceBrowserSSRFGuardBool(%#v).Value = %v, want %v", tt.raw, got.Value, tt.want)
			}
			if got.Evidence != "" {
				t.Fatalf("CoerceBrowserSSRFGuardBool(%#v).Evidence = %q, want empty", tt.raw, got.Evidence)
			}
		})
	}
}

func TestBrowserSSRFGuard_PrivateURLBlockedWhenCloudWouldReceiveIt(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "localhost", rawURL: "http://localhost:3000/dashboard"},
		{name: "rfc1918", rawURL: "http://192.168.1.10/admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckBrowserSSRFGuard("task-1", tt.rawURL, BrowserSSRFGuardOptions{
				CloudConfigured:         true,
				AllowPrivateURLs:        false,
				AutoLocalForPrivateURLs: false,
			})
			if got.Allowed {
				t.Fatalf("CheckBrowserSSRFGuard(%q).Allowed = true, want false", tt.rawURL)
			}
			if got.Evidence != "private_url_blocked" {
				t.Fatalf("CheckBrowserSSRFGuard(%q).Evidence = %q, want private_url_blocked", tt.rawURL, got.Evidence)
			}
		})
	}
}

func TestBrowserSSRFGuard_PublicURLAllowed(t *testing.T) {
	got := CheckBrowserSSRFGuard("task-2", "https://example.com/docs", BrowserSSRFGuardOptions{
		CloudConfigured:         true,
		AllowPrivateURLs:        false,
		AutoLocalForPrivateURLs: false,
	})
	if !got.Allowed {
		t.Fatalf("CheckBrowserSSRFGuard(public).Allowed = false, want true")
	}
	if got.Evidence == "private_url_blocked" {
		t.Fatalf("CheckBrowserSSRFGuard(public).Evidence = private_url_blocked, want no private block evidence")
	}
}
