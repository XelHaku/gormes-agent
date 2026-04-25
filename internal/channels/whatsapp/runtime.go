package whatsapp

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultBridgeScript = "scripts/whatsapp-bridge/bridge.js"
	DefaultBridgePort   = 3000
	sessionLockName     = "whatsapp-session"
)

// RuntimeKind identifies which WhatsApp transport owns startup.
type RuntimeKind string

const (
	RuntimeKindBridge RuntimeKind = "bridge"
	RuntimeKindNative RuntimeKind = "native"
)

// RuntimePreference describes how Gormes chooses between bridge and native
// transports when more than one runtime is available.
type RuntimePreference string

const (
	RuntimePreferenceBridgeFirst RuntimePreference = "bridge_first"
	RuntimePreferenceNativeFirst RuntimePreference = "native_first"
	RuntimePreferenceBridgeOnly  RuntimePreference = "bridge"
	RuntimePreferenceNativeOnly  RuntimePreference = "native"
)

// AccountMode mirrors the upstream WhatsApp bridge's operator modes.
type AccountMode string

const (
	AccountModeBot      AccountMode = "bot"
	AccountModeSelfChat AccountMode = "self-chat"
)

// RuntimeConfig captures operator-controlled startup inputs before either
// bridge or native transport wiring is bound.
type RuntimeConfig struct {
	Preference  RuntimePreference
	StateRoot   string
	SessionPath string
	AccountMode AccountMode
	Bridge      BridgeRuntimeConfig
	Native      NativeRuntimeConfig
}

// BridgeRuntimeConfig contains bridge-specific startup overrides.
type BridgeRuntimeConfig struct {
	Disabled   bool
	ScriptPath string
	Port       int
}

// NativeRuntimeConfig marks the future native runtime as available to the
// decision seam without importing a concrete WhatsApp client.
type NativeRuntimeConfig struct {
	Enabled bool
}

// RuntimePlan is the transport-neutral WhatsApp startup contract.
type RuntimePlan struct {
	Startup StartupPlan
	Session SessionPlan
	Bridge  BridgePlan
	Native  NativePlan
	Account AccountPlan
}

// StartupPlan freezes the selected runtime and the candidate order.
type StartupPlan struct {
	Preference     RuntimePreference
	Selected       RuntimeKind
	CandidateOrder []RuntimeKind
}

// SessionPlan identifies which runtime owns the sensitive WhatsApp session
// directory.
type SessionPlan struct {
	Path                string
	Owner               RuntimeKind
	LockName            string
	ContainsCredentials bool
}

// BridgePlan describes the Node/Baileys bridge process contract.
type BridgePlan struct {
	ScriptPath     string
	Port           int
	SessionPath    string
	LogPath        string
	Command        []string
	RequiresNode   bool
	ManagedProcess bool
}

// NativePlan reserves the native device-store contract for a future concrete
// WhatsApp client.
type NativePlan struct {
	StorePath string
}

// AccountPlan freezes bot-vs-self-chat policy before message filtering is
// wired to either runtime.
type AccountPlan struct {
	Mode                    AccountMode
	AcceptsOwnMessages      bool
	RequiresSelfChat        bool
	PrefixesOutboundReplies bool
	SuppressAgentEcho       bool
	DropsOwnMessages        bool
}

// DecideRuntime selects the WhatsApp runtime and freezes its session/account
// policy without checking the filesystem, spawning Node, or importing a native
// WhatsApp client.
func DecideRuntime(cfg RuntimeConfig) (RuntimePlan, error) {
	preference, err := normalizeRuntimePreference(cfg.Preference)
	if err != nil {
		return RuntimePlan{}, err
	}
	sessionPath, err := resolveSessionPath(cfg)
	if err != nil {
		return RuntimePlan{}, err
	}
	account, err := decideAccount(cfg.AccountMode)
	if err != nil {
		return RuntimePlan{}, err
	}

	selected, candidates, err := selectRuntime(preference, cfg)
	if err != nil {
		return RuntimePlan{}, err
	}

	plan := RuntimePlan{
		Startup: StartupPlan{
			Preference:     preference,
			Selected:       selected,
			CandidateOrder: candidates,
		},
		Session: SessionPlan{
			Path:                sessionPath,
			Owner:               selected,
			LockName:            sessionLockName,
			ContainsCredentials: true,
		},
		Account: account,
	}
	if selected == RuntimeKindBridge {
		plan.Bridge = decideBridge(cfg.Bridge, sessionPath, account.Mode)
	}
	if selected == RuntimeKindNative {
		plan.Native = NativePlan{StorePath: sessionPath}
	}
	return plan, nil
}

func normalizeRuntimePreference(preference RuntimePreference) (RuntimePreference, error) {
	raw := strings.TrimSpace(strings.ToLower(string(preference)))
	raw = strings.ReplaceAll(raw, "-", "_")
	if raw == "" {
		return RuntimePreferenceBridgeFirst, nil
	}
	switch RuntimePreference(raw) {
	case RuntimePreferenceBridgeFirst, RuntimePreferenceNativeFirst, RuntimePreferenceBridgeOnly, RuntimePreferenceNativeOnly:
		return RuntimePreference(raw), nil
	default:
		return "", fmt.Errorf("whatsapp: unsupported runtime preference %q", preference)
	}
}

func resolveSessionPath(cfg RuntimeConfig) (string, error) {
	if path := strings.TrimSpace(cfg.SessionPath); path != "" {
		return filepath.Clean(path), nil
	}
	stateRoot := strings.TrimSpace(cfg.StateRoot)
	if stateRoot == "" {
		return "", fmt.Errorf("whatsapp: state root or session path is required")
	}
	return filepath.Join(filepath.Clean(stateRoot), "whatsapp", "session"), nil
}

func selectRuntime(preference RuntimePreference, cfg RuntimeConfig) (RuntimeKind, []RuntimeKind, error) {
	bridgeEnabled := !cfg.Bridge.Disabled
	nativeEnabled := cfg.Native.Enabled

	switch preference {
	case RuntimePreferenceBridgeFirst:
		candidates := runtimeCandidates([]RuntimeKind{RuntimeKindBridge, RuntimeKindNative}, bridgeEnabled, nativeEnabled)
		if bridgeEnabled {
			return RuntimeKindBridge, candidates, nil
		}
		if nativeEnabled {
			return RuntimeKindNative, candidates, nil
		}
	case RuntimePreferenceNativeFirst:
		candidates := runtimeCandidates([]RuntimeKind{RuntimeKindNative, RuntimeKindBridge}, bridgeEnabled, nativeEnabled)
		if nativeEnabled {
			return RuntimeKindNative, candidates, nil
		}
		if bridgeEnabled {
			return RuntimeKindBridge, candidates, nil
		}
	case RuntimePreferenceBridgeOnly:
		if bridgeEnabled {
			return RuntimeKindBridge, []RuntimeKind{RuntimeKindBridge}, nil
		}
		return "", nil, fmt.Errorf("whatsapp: bridge runtime requested but disabled")
	case RuntimePreferenceNativeOnly:
		if nativeEnabled {
			return RuntimeKindNative, []RuntimeKind{RuntimeKindNative}, nil
		}
		return "", nil, fmt.Errorf("whatsapp: native runtime requested but not enabled")
	}

	return "", nil, fmt.Errorf("whatsapp: no runtime enabled")
}

func runtimeCandidates(order []RuntimeKind, bridgeEnabled, nativeEnabled bool) []RuntimeKind {
	out := make([]RuntimeKind, 0, len(order))
	for _, candidate := range order {
		if candidate == RuntimeKindBridge && bridgeEnabled {
			out = append(out, candidate)
		}
		if candidate == RuntimeKindNative && nativeEnabled {
			out = append(out, candidate)
		}
	}
	return out
}

func decideBridge(cfg BridgeRuntimeConfig, sessionPath string, mode AccountMode) BridgePlan {
	port := cfg.Port
	if port <= 0 {
		port = DefaultBridgePort
	}
	scriptPath := strings.TrimSpace(cfg.ScriptPath)
	if scriptPath == "" {
		scriptPath = DefaultBridgeScript
	} else {
		scriptPath = filepath.Clean(scriptPath)
	}
	portArg := strconv.Itoa(port)
	return BridgePlan{
		ScriptPath:     scriptPath,
		Port:           port,
		SessionPath:    sessionPath,
		LogPath:        filepath.Join(filepath.Dir(sessionPath), "bridge.log"),
		RequiresNode:   true,
		ManagedProcess: true,
		Command: []string{
			"node",
			scriptPath,
			"--port", portArg,
			"--session", sessionPath,
			"--mode", string(mode),
		},
	}
}

func decideAccount(mode AccountMode) (AccountPlan, error) {
	normalized := strings.TrimSpace(strings.ToLower(string(mode)))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" {
		normalized = string(AccountModeSelfChat)
	}
	switch AccountMode(normalized) {
	case AccountModeSelfChat:
		return AccountPlan{
			Mode:                    AccountModeSelfChat,
			AcceptsOwnMessages:      true,
			RequiresSelfChat:        true,
			PrefixesOutboundReplies: true,
			SuppressAgentEcho:       true,
		}, nil
	case AccountModeBot:
		return AccountPlan{
			Mode:             AccountModeBot,
			DropsOwnMessages: true,
		}, nil
	default:
		return AccountPlan{}, fmt.Errorf("whatsapp: unsupported account mode %q", mode)
	}
}
