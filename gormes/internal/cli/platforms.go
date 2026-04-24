package cli

// PlatformInfo mirrors hermes_cli/platforms.py::PlatformInfo: the metadata a
// single platform entry exposes to TUI menus and default-toolset resolution.
type PlatformInfo struct {
	// Key is the canonical platform identifier (e.g. "telegram").
	Key string
	// Label is the human-readable label used in TUI menus, including the
	// upstream emoji prefix.
	Label string
	// DefaultToolset is the toolset identifier applied when a platform
	// binds without an explicit toolset override.
	DefaultToolset string
}

// Platforms is the ordered registry of supported platforms. The sequence
// matches the upstream OrderedDict in hermes_cli/platforms.py so TUI menus
// stay deterministic; tests enforce the full ordering.
var Platforms = []PlatformInfo{
	{Key: "cli", Label: "🖥️  CLI", DefaultToolset: "hermes-cli"},
	{Key: "telegram", Label: "📱 Telegram", DefaultToolset: "hermes-telegram"},
	{Key: "discord", Label: "💬 Discord", DefaultToolset: "hermes-discord"},
	{Key: "slack", Label: "💼 Slack", DefaultToolset: "hermes-slack"},
	{Key: "whatsapp", Label: "📱 WhatsApp", DefaultToolset: "hermes-whatsapp"},
	{Key: "signal", Label: "📡 Signal", DefaultToolset: "hermes-signal"},
	{Key: "bluebubbles", Label: "💙 BlueBubbles", DefaultToolset: "hermes-bluebubbles"},
	{Key: "email", Label: "📧 Email", DefaultToolset: "hermes-email"},
	{Key: "homeassistant", Label: "🏠 Home Assistant", DefaultToolset: "hermes-homeassistant"},
	{Key: "mattermost", Label: "💬 Mattermost", DefaultToolset: "hermes-mattermost"},
	{Key: "matrix", Label: "💬 Matrix", DefaultToolset: "hermes-matrix"},
	{Key: "dingtalk", Label: "💬 DingTalk", DefaultToolset: "hermes-dingtalk"},
	{Key: "feishu", Label: "🪽 Feishu", DefaultToolset: "hermes-feishu"},
	{Key: "wecom", Label: "💬 WeCom", DefaultToolset: "hermes-wecom"},
	{Key: "wecom_callback", Label: "💬 WeCom Callback", DefaultToolset: "hermes-wecom-callback"},
	{Key: "weixin", Label: "💬 Weixin", DefaultToolset: "hermes-weixin"},
	{Key: "qqbot", Label: "💬 QQBot", DefaultToolset: "hermes-qqbot"},
	{Key: "webhook", Label: "🔗 Webhook", DefaultToolset: "hermes-webhook"},
	{Key: "api_server", Label: "🌐 API Server", DefaultToolset: "hermes-api-server"},
}

// PlatformByKey returns the platform entry for key. The second return value
// reports whether the key was found.
func PlatformByKey(key string) (PlatformInfo, bool) {
	for _, p := range Platforms {
		if p.Key == key {
			return p, true
		}
	}
	return PlatformInfo{}, false
}

// PlatformLabel returns the TUI label for key, or def when key is not in the
// registry. Mirrors hermes_cli/platforms.py::platform_label(key, default="").
func PlatformLabel(key, def string) string {
	if p, ok := PlatformByKey(key); ok {
		return p.Label
	}
	return def
}
