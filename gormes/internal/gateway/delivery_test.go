package gateway

import "testing"

func TestParseDeliveryTarget_Valid(t *testing.T) {
	origin := &SessionSource{
		Platform: "telegram",
		ChatID:   "42",
		ThreadID: "99",
	}

	tests := []struct {
		name string
		raw  string
		want DeliveryTarget
	}{
		{
			name: "origin",
			raw:  "origin",
			want: DeliveryTarget{Platform: "telegram", ChatID: "42", ThreadID: "99", IsOrigin: true},
		},
		{
			name: "local",
			raw:  "local",
			want: DeliveryTarget{Platform: "local"},
		},
		{
			name: "platform home",
			raw:  "discord",
			want: DeliveryTarget{Platform: "discord"},
		},
		{
			name: "explicit chat",
			raw:  "telegram:-100123",
			want: DeliveryTarget{Platform: "telegram", ChatID: "-100123", IsExplicit: true},
		},
		{
			name: "explicit thread",
			raw:  "telegram:-100123:77",
			want: DeliveryTarget{Platform: "telegram", ChatID: "-100123", ThreadID: "77", IsExplicit: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDeliveryTarget(tt.raw, origin)
			if err != nil {
				t.Fatalf("ParseDeliveryTarget(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseDeliveryTarget(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseDeliveryTarget_Invalid(t *testing.T) {
	for _, raw := range []string{"", " ", "telegram:", ":42", "telegram::42", "telegram:42:"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseDeliveryTarget(raw, nil); err == nil {
				t.Fatalf("ParseDeliveryTarget(%q) error = nil, want non-nil", raw)
			}
		})
	}
}
