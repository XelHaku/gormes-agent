package gateway

import "testing"

func TestInboundEvent_ChatKey(t *testing.T) {
	tests := []struct {
		name string
		e    InboundEvent
		want string
	}{
		{"telegram", InboundEvent{Platform: "telegram", ChatID: "42"}, "telegram:42"},
		{"discord", InboundEvent{Platform: "discord", ChatID: "987654321"}, "discord:987654321"},
		{"empty chat id", InboundEvent{Platform: "telegram", ChatID: ""}, "telegram:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.ChatKey(); got != tt.want {
				t.Errorf("ChatKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEventKind_String(t *testing.T) {
	tests := []struct {
		k    EventKind
		want string
	}{
		{EventUnknown, "unknown"},
		{EventSubmit, "submit"},
		{EventCancel, "cancel"},
		{EventReset, "reset"},
		{EventStart, "start"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}
