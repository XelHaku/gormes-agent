package tools

import "testing"

func TestIsPrivateBrowserHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "localhost", host: "localhost", want: true},
		{name: "localhost_with_trailing_dot", host: "LOCALHOST.", want: true},
		{name: "ipv4_loopback", host: "127.0.0.1", want: true},
		{name: "rfc1918_10", host: "10.20.30.40", want: true},
		{name: "rfc1918_172_lower_bound", host: "172.16.0.1", want: true},
		{name: "rfc1918_172_upper_bound", host: "172.31.255.254", want: true},
		{name: "rfc1918_192", host: "192.168.1.50", want: true},
		{name: "ipv4_link_local", host: "169.254.1.10", want: true},
		{name: "ipv6_loopback", host: "::1", want: true},
		{name: "mdns_local_suffix", host: "raspberrypi.local", want: true},
		{name: "lan_suffix", host: "printer.lan", want: true},
		{name: "internal_suffix", host: "db.internal", want: true},
		{name: "public_hostname", host: "github.com", want: false},
		{name: "public_ip_literal", host: "8.8.8.8", want: false},
		{name: "outside_172_private_lower", host: "172.15.255.255", want: false},
		{name: "outside_172_private_upper", host: "172.32.0.1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPrivateBrowserHost(tt.host); got != tt.want {
				t.Fatalf("IsPrivateBrowserHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestRouteBrowserNavigation_PrivateHostsUseLocalSidecar(t *testing.T) {
	tests := []struct {
		name    string
		taskID  string
		rawURL  string
		wantKey string
	}{
		{name: "localhost_default_task", rawURL: "http://localhost:3000/", wantKey: "default::local"},
		{name: "loopback_ipv4", taskID: "task-1", rawURL: "http://127.0.0.1:8080/", wantKey: "task-1::local"},
		{name: "rfc1918_10", taskID: "task-1", rawURL: "http://10.2.3.4/", wantKey: "task-1::local"},
		{name: "rfc1918_172_lower_bound", taskID: "task-1", rawURL: "http://172.16.0.10/", wantKey: "task-1::local"},
		{name: "rfc1918_172_upper_bound", taskID: "task-1", rawURL: "http://172.31.255.250/", wantKey: "task-1::local"},
		{name: "rfc1918_192", taskID: "task-1", rawURL: "http://192.168.1.50:8000/", wantKey: "task-1::local"},
		{name: "ipv4_link_local", taskID: "task-1", rawURL: "http://169.254.10.20/", wantKey: "task-1::local"},
		{name: "ipv6_loopback", taskID: "task-1", rawURL: "http://[::1]:3000/", wantKey: "task-1::local"},
		{name: "local_suffix", taskID: "task-1", rawURL: "http://raspberrypi.local/", wantKey: "task-1::local"},
		{name: "lan_suffix", taskID: "task-1", rawURL: "http://printer.lan/", wantKey: "task-1::local"},
		{name: "internal_suffix", taskID: "task-1", rawURL: "http://db.internal/", wantKey: "task-1::local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteBrowserNavigation(tt.taskID, tt.rawURL, true, true, false, false)
			want := BrowserRoute{
				SessionKey: tt.wantKey,
				ForceLocal: true,
				Reason:     "private_url_local_sidecar",
			}
			if got != want {
				t.Fatalf("RouteBrowserNavigation(%q, %q) = %#v, want %#v", tt.taskID, tt.rawURL, got, want)
			}
		})
	}
}

func TestRouteBrowserNavigation_PublicURLsUseCloudKey(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "public_hostname", rawURL: "https://github.com/x/y"},
		{name: "public_ip_literal", rawURL: "https://8.8.8.8/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteBrowserNavigation("task-2", tt.rawURL, true, true, false, false)
			want := BrowserRoute{SessionKey: "task-2"}
			if got != want {
				t.Fatalf("RouteBrowserNavigation(%q) = %#v, want %#v", tt.rawURL, got, want)
			}
		})
	}
}

func TestRouteBrowserNavigation_DisabledOrOverrideCases(t *testing.T) {
	tests := []struct {
		name                    string
		cloudConfigured         bool
		autoLocalForPrivateURLs bool
		cdpOverride             bool
		camofoxMode             bool
	}{
		{name: "no_cloud_provider", cloudConfigured: false, autoLocalForPrivateURLs: true},
		{name: "auto_local_disabled", cloudConfigured: true, autoLocalForPrivateURLs: false},
		{name: "cdp_override", cloudConfigured: true, autoLocalForPrivateURLs: true, cdpOverride: true},
		{name: "camofox_mode", cloudConfigured: true, autoLocalForPrivateURLs: true, camofoxMode: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteBrowserNavigation(
				"task-3",
				"http://localhost:3000/",
				tt.cloudConfigured,
				tt.autoLocalForPrivateURLs,
				tt.cdpOverride,
				tt.camofoxMode,
			)
			want := BrowserRoute{SessionKey: "task-3"}
			if got != want {
				t.Fatalf("RouteBrowserNavigation(%s) = %#v, want %#v", tt.name, got, want)
			}
		})
	}
}

func TestRouteBrowserNavigation_DefaultTaskID(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantKey string
	}{
		{name: "public_url_uses_default", rawURL: "https://github.com/", wantKey: "default"},
		{name: "private_url_uses_default_local_sidecar", rawURL: "http://localhost:3000/", wantKey: "default::local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteBrowserNavigation("", tt.rawURL, true, true, false, false)
			want := BrowserRoute{SessionKey: tt.wantKey}
			if tt.wantKey == "default::local" {
				want.ForceLocal = true
				want.Reason = "private_url_local_sidecar"
			}
			if got != want {
				t.Fatalf("RouteBrowserNavigation(empty task, %q) = %#v, want %#v", tt.rawURL, got, want)
			}
		})
	}
}
