package cli

import "testing"

// Mirrors hermes_cli/platforms.py::PLATFORMS. The upstream OrderedDict is the
// authoritative sequence — TUI menus rely on deterministic iteration order,
// so any drift from this list is a port-parity regression.
var wantPlatforms = []struct {
	key            string
	label          string
	defaultToolset string
}{
	{"cli", "🖥️  CLI", "hermes-cli"},
	{"telegram", "📱 Telegram", "hermes-telegram"},
	{"discord", "💬 Discord", "hermes-discord"},
	{"slack", "💼 Slack", "hermes-slack"},
	{"whatsapp", "📱 WhatsApp", "hermes-whatsapp"},
	{"signal", "📡 Signal", "hermes-signal"},
	{"bluebubbles", "💙 BlueBubbles", "hermes-bluebubbles"},
	{"email", "📧 Email", "hermes-email"},
	{"homeassistant", "🏠 Home Assistant", "hermes-homeassistant"},
	{"mattermost", "💬 Mattermost", "hermes-mattermost"},
	{"matrix", "💬 Matrix", "hermes-matrix"},
	{"dingtalk", "💬 DingTalk", "hermes-dingtalk"},
	{"feishu", "🪽 Feishu", "hermes-feishu"},
	{"wecom", "💬 WeCom", "hermes-wecom"},
	{"wecom_callback", "💬 WeCom Callback", "hermes-wecom-callback"},
	{"weixin", "💬 Weixin", "hermes-weixin"},
	{"qqbot", "💬 QQBot", "hermes-qqbot"},
	{"webhook", "🔗 Webhook", "hermes-webhook"},
	{"api_server", "🌐 API Server", "hermes-api-server"},
}

func TestPlatforms_OrderedSequenceMatchesUpstream(t *testing.T) {
	if got, want := len(Platforms), len(wantPlatforms); got != want {
		t.Fatalf("len(Platforms) = %d, want %d (upstream OrderedDict length)", got, want)
	}
	for i, want := range wantPlatforms {
		got := Platforms[i]
		if got.Key != want.key {
			t.Errorf("Platforms[%d].Key = %q, want %q", i, got.Key, want.key)
		}
		if got.Label != want.label {
			t.Errorf("Platforms[%d].Label = %q, want %q", i, got.Label, want.label)
		}
		if got.DefaultToolset != want.defaultToolset {
			t.Errorf("Platforms[%d].DefaultToolset = %q, want %q", i, got.DefaultToolset, want.defaultToolset)
		}
	}
}

func TestPlatformLabel_KnownKeyReturnsLabel(t *testing.T) {
	if got, want := PlatformLabel("telegram", "fallback"), "📱 Telegram"; got != want {
		t.Errorf("PlatformLabel(telegram) = %q, want %q", got, want)
	}
	if got, want := PlatformLabel("api_server", "fallback"), "🌐 API Server"; got != want {
		t.Errorf("PlatformLabel(api_server) = %q, want %q", got, want)
	}
}

func TestPlatformLabel_UnknownKeyReturnsDefault(t *testing.T) {
	if got, want := PlatformLabel("not-a-platform", "—"), "—"; got != want {
		t.Errorf("PlatformLabel(unknown, —) = %q, want %q", got, want)
	}
	// Upstream platform_label(key, default="") returns the empty string when
	// no default is supplied. PlatformLabel's two-arg signature makes the
	// caller pass "" explicitly — the contract is the same.
	if got := PlatformLabel("not-a-platform", ""); got != "" {
		t.Errorf("PlatformLabel(unknown, \"\") = %q, want empty string", got)
	}
}

func TestPlatformByKey_HitAndMiss(t *testing.T) {
	info, ok := PlatformByKey("discord")
	if !ok {
		t.Fatalf("PlatformByKey(discord) ok = false, want true")
	}
	if info.DefaultToolset != "hermes-discord" {
		t.Errorf("PlatformByKey(discord).DefaultToolset = %q, want hermes-discord", info.DefaultToolset)
	}
	if _, ok := PlatformByKey("no-such-key"); ok {
		t.Error("PlatformByKey(no-such-key) ok = true, want false")
	}
}

func TestPlatforms_NoDuplicateKeys(t *testing.T) {
	seen := make(map[string]struct{}, len(Platforms))
	for _, p := range Platforms {
		if _, dup := seen[p.Key]; dup {
			t.Fatalf("duplicate platform key %q", p.Key)
		}
		seen[p.Key] = struct{}{}
	}
}
