package tuigateway

import "testing"

func TestParseToolProgressMode(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{name: "nil defaults to all", in: nil, want: "all"},
		{name: "true coerces to all", in: true, want: "all"},
		{name: "false coerces to off", in: false, want: "off"},
		{name: "empty string defaults to all", in: "", want: "all"},
		{name: "whitespace-only defaults to all", in: "   \t  ", want: "all"},
		{name: "off round-trips", in: "off", want: "off"},
		{name: "new round-trips", in: "new", want: "new"},
		{name: "all round-trips", in: "all", want: "all"},
		{name: "verbose round-trips", in: "verbose", want: "verbose"},
		{name: "uppercase normalises", in: "VERBOSE", want: "verbose"},
		{name: "mixed case with whitespace normalises", in: "  Off ", want: "off"},
		{name: "unknown string falls back to all", in: "loud", want: "all"},
		{name: "unknown string with whitespace falls back to all", in: " partial ", want: "all"},
		{name: "integer type falls back to all", in: 3, want: "all"},
		{name: "slice type falls back to all", in: []string{"off"}, want: "all"},
		{name: "map type falls back to all", in: map[string]any{"k": "off"}, want: "all"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseToolProgressMode(c.in)
			if got != c.want {
				t.Fatalf("ParseToolProgressMode(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestToolProgressEnabled(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{mode: "off", want: false},
		{mode: "all", want: true},
		{mode: "new", want: true},
		{mode: "verbose", want: true},
		{mode: "", want: true}, // non-"off" strings count as enabled (matches Python `!= "off"`)
		{mode: "unknown", want: true},
	}

	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			got := ToolProgressEnabled(c.mode)
			if got != c.want {
				t.Fatalf("ToolProgressEnabled(%q) = %v, want %v", c.mode, got, c.want)
			}
		})
	}
}
