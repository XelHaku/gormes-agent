package whatsapp

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecideRuntime_BridgeFirstDefaultOwnsBridgeSession(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")

	plan, err := DecideRuntime(RuntimeConfig{
		StateRoot: stateRoot,
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Startup.Preference != RuntimePreferenceBridgeFirst {
		t.Fatalf("Startup.Preference = %q, want %q", plan.Startup.Preference, RuntimePreferenceBridgeFirst)
	}
	if plan.Startup.Selected != RuntimeKindBridge {
		t.Fatalf("Startup.Selected = %q, want %q", plan.Startup.Selected, RuntimeKindBridge)
	}
	wantSessionPath := filepath.Join(stateRoot, "whatsapp", "session")
	if plan.Session.Path != wantSessionPath {
		t.Fatalf("Session.Path = %q, want %q", plan.Session.Path, wantSessionPath)
	}
	if plan.Session.Owner != RuntimeKindBridge {
		t.Fatalf("Session.Owner = %q, want %q", plan.Session.Owner, RuntimeKindBridge)
	}
	if plan.Session.LockName != "whatsapp-session" {
		t.Fatalf("Session.LockName = %q, want whatsapp-session", plan.Session.LockName)
	}
	if !plan.Session.ContainsCredentials {
		t.Fatal("Session.ContainsCredentials = false, want true")
	}
	if plan.Bridge.Port != 3000 {
		t.Fatalf("Bridge.Port = %d, want 3000", plan.Bridge.Port)
	}
	if !plan.Bridge.RequiresNode {
		t.Fatal("Bridge.RequiresNode = false, want true")
	}
	if !plan.Bridge.ManagedProcess {
		t.Fatal("Bridge.ManagedProcess = false, want true")
	}
	if plan.Bridge.SessionPath != wantSessionPath {
		t.Fatalf("Bridge.SessionPath = %q, want %q", plan.Bridge.SessionPath, wantSessionPath)
	}
	wantLogPath := filepath.Join(stateRoot, "whatsapp", "bridge.log")
	if plan.Bridge.LogPath != wantLogPath {
		t.Fatalf("Bridge.LogPath = %q, want %q", plan.Bridge.LogPath, wantLogPath)
	}
	wantCommand := []string{
		"node",
		DefaultBridgeScript,
		"--port", "3000",
		"--session", wantSessionPath,
		"--mode", "self-chat",
	}
	if !reflect.DeepEqual(plan.Bridge.Command, wantCommand) {
		t.Fatalf("Bridge.Command = %#v, want %#v", plan.Bridge.Command, wantCommand)
	}
	if plan.Account.Mode != AccountModeSelfChat {
		t.Fatalf("Account.Mode = %q, want %q", plan.Account.Mode, AccountModeSelfChat)
	}
	if !plan.Account.AcceptsOwnMessages {
		t.Fatal("Account.AcceptsOwnMessages = false, want true")
	}
	if !plan.Account.RequiresSelfChat {
		t.Fatal("Account.RequiresSelfChat = false, want true")
	}
	if !plan.Account.PrefixesOutboundReplies {
		t.Fatal("Account.PrefixesOutboundReplies = false, want true")
	}
	if !plan.Account.SuppressAgentEcho {
		t.Fatal("Account.SuppressAgentEcho = false, want true")
	}
	if plan.Account.DropsOwnMessages {
		t.Fatal("Account.DropsOwnMessages = true, want false")
	}
}

func TestDecideRuntime_NativeFirstSelectsNativeWhenEnabled(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")

	plan, err := DecideRuntime(RuntimeConfig{
		Preference:  RuntimePreferenceNativeFirst,
		StateRoot:   stateRoot,
		AccountMode: AccountModeBot,
		Native: NativeRuntimeConfig{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Startup.Selected != RuntimeKindNative {
		t.Fatalf("Startup.Selected = %q, want %q", plan.Startup.Selected, RuntimeKindNative)
	}
	wantCandidates := []RuntimeKind{RuntimeKindNative, RuntimeKindBridge}
	if !reflect.DeepEqual(plan.Startup.CandidateOrder, wantCandidates) {
		t.Fatalf("Startup.CandidateOrder = %#v, want %#v", plan.Startup.CandidateOrder, wantCandidates)
	}
	wantSessionPath := filepath.Join(stateRoot, "whatsapp", "session")
	if plan.Session.Path != wantSessionPath {
		t.Fatalf("Session.Path = %q, want %q", plan.Session.Path, wantSessionPath)
	}
	if plan.Session.Owner != RuntimeKindNative {
		t.Fatalf("Session.Owner = %q, want %q", plan.Session.Owner, RuntimeKindNative)
	}
	if plan.Native.StorePath != wantSessionPath {
		t.Fatalf("Native.StorePath = %q, want %q", plan.Native.StorePath, wantSessionPath)
	}
	if plan.Bridge.Command != nil {
		t.Fatalf("Bridge.Command = %#v, want nil for native runtime", plan.Bridge.Command)
	}
	if plan.Account.Mode != AccountModeBot {
		t.Fatalf("Account.Mode = %q, want %q", plan.Account.Mode, AccountModeBot)
	}
	if plan.Account.AcceptsOwnMessages {
		t.Fatal("Account.AcceptsOwnMessages = true, want false")
	}
	if !plan.Account.DropsOwnMessages {
		t.Fatal("Account.DropsOwnMessages = false, want true")
	}
	if plan.Account.PrefixesOutboundReplies {
		t.Fatal("Account.PrefixesOutboundReplies = true, want false")
	}
}

func TestDecideRuntime_ExplicitBridgePathAndPortPreserveSessionParentOwnership(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "wa-state", "session")
	scriptPath := filepath.Join("bin", "whatsapp-bridge.js")

	plan, err := DecideRuntime(RuntimeConfig{
		SessionPath: " " + sessionPath + " ",
		Bridge: BridgeRuntimeConfig{
			ScriptPath: " " + scriptPath + " ",
			Port:       4040,
		},
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Startup.Selected != RuntimeKindBridge {
		t.Fatalf("Startup.Selected = %q, want %q", plan.Startup.Selected, RuntimeKindBridge)
	}
	if plan.Session.Path != sessionPath {
		t.Fatalf("Session.Path = %q, want %q", plan.Session.Path, sessionPath)
	}
	wantLogPath := filepath.Join(filepath.Dir(sessionPath), "bridge.log")
	if plan.Bridge.LogPath != wantLogPath {
		t.Fatalf("Bridge.LogPath = %q, want %q", plan.Bridge.LogPath, wantLogPath)
	}
	wantCommand := []string{
		"node",
		scriptPath,
		"--port", "4040",
		"--session", sessionPath,
		"--mode", "self-chat",
	}
	if !reflect.DeepEqual(plan.Bridge.Command, wantCommand) {
		t.Fatalf("Bridge.Command = %#v, want %#v", plan.Bridge.Command, wantCommand)
	}
}

func TestDecideRuntime_RejectsUnavailableOrInvalidStartupInputs(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")

	tests := []struct {
		name    string
		cfg     RuntimeConfig
		wantErr string
	}{
		{
			name:    "missing session root",
			cfg:     RuntimeConfig{},
			wantErr: "whatsapp: state root or session path is required",
		},
		{
			name: "strict native disabled",
			cfg: RuntimeConfig{
				Preference: RuntimePreferenceNativeOnly,
				StateRoot:  stateRoot,
			},
			wantErr: "whatsapp: native runtime requested but not enabled",
		},
		{
			name: "strict bridge disabled",
			cfg: RuntimeConfig{
				Preference: RuntimePreferenceBridgeOnly,
				StateRoot:  stateRoot,
				Bridge: BridgeRuntimeConfig{
					Disabled: true,
				},
			},
			wantErr: "whatsapp: bridge runtime requested but disabled",
		},
		{
			name: "unsupported account mode",
			cfg: RuntimeConfig{
				StateRoot:   stateRoot,
				AccountMode: AccountMode("business-api"),
			},
			wantErr: `whatsapp: unsupported account mode "business-api"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecideRuntime(tt.cfg)
			if err == nil {
				t.Fatal("DecideRuntime() error = nil, want failure")
			}
			if got := err.Error(); got != tt.wantErr {
				t.Fatalf("DecideRuntime() error = %q, want %q", got, tt.wantErr)
			}
		})
	}
}
