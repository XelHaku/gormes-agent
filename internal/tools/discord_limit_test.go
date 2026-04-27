package tools

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDiscordLimitCoercion_SearchMembersDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "missing", args: map[string]any{}},
		{name: "nil", args: map[string]any{"limit": nil}},
		{name: "non_numeric", args: map[string]any{"limit": "many"}},
		{name: "boolean", args: map[string]any{"limit": true}},
		{name: "nan", args: map[string]any{"limit": math.NaN()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeDiscordLimit("search_members", tc.args)
			if got.Limit != 20 {
				t.Fatalf("Limit = %d, want 20", got.Limit)
			}
			if got.Evidence != DiscordLimitEvidenceDefaulted {
				t.Fatalf("Evidence = %q, want %q", got.Evidence, DiscordLimitEvidenceDefaulted)
			}
		})
	}
}

func TestDiscordLimitCoercion_FetchMessagesDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "missing", args: map[string]any{}},
		{name: "nil", args: map[string]any{"limit": nil}},
		{name: "non_numeric", args: map[string]any{"limit": "many"}},
		{name: "boolean", args: map[string]any{"limit": false}},
		{name: "nan", args: map[string]any{"limit": math.NaN()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeDiscordLimit("fetch_messages", tc.args)
			if got.Limit != 50 {
				t.Fatalf("Limit = %d, want 50", got.Limit)
			}
			if got.Evidence != DiscordLimitEvidenceDefaulted {
				t.Fatalf("Evidence = %q, want %q", got.Evidence, DiscordLimitEvidenceDefaulted)
			}
		})
	}
}

func TestDiscordLimitCoercion_ParsesStringAndFloatIntegers(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"limit":7}`), &decoded); err != nil {
		t.Fatalf("decode JSON number: %v", err)
	}

	for _, tc := range []struct {
		name string
		raw  any
	}{
		{name: "string", raw: "7"},
		{name: "int", raw: 7},
		{name: "float_integer", raw: 7.0},
		{name: "json_decoded_float64", raw: decoded["limit"]},
		{name: "json_number", raw: json.Number("7")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, action := range []string{"search_members", "fetch_messages"} {
				got := NormalizeDiscordLimit(action, map[string]any{"limit": tc.raw})
				if got.Limit != 7 {
					t.Fatalf("%s Limit = %d, want 7", action, got.Limit)
				}
				if got.Evidence != DiscordLimitEvidenceProvided {
					t.Fatalf("%s Evidence = %q, want %q", action, got.Evidence, DiscordLimitEvidenceProvided)
				}
			}
		})
	}
}

func TestDiscordLimitCoercion_ClampsToSchemaBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  any
		want int
	}{
		{name: "zero", raw: 0, want: 1},
		{name: "fraction_below_minimum", raw: 0.5, want: 1},
		{name: "negative", raw: -8, want: 1},
		{name: "string_below_minimum", raw: "0", want: 1},
		{name: "above_maximum", raw: 101, want: 100},
		{name: "fraction_above_maximum", raw: 100.5, want: 100},
		{name: "string_above_maximum", raw: "101", want: 100},
		{name: "json_number_above_maximum", raw: json.Number("101"), want: 100},
		{name: "positive_infinity", raw: math.Inf(1), want: 100},
		{name: "negative_infinity", raw: math.Inf(-1), want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, action := range []string{"search_members", "fetch_messages"} {
				got := NormalizeDiscordLimit(action, map[string]any{"limit": tc.raw})
				if got.Limit != tc.want {
					t.Fatalf("%s Limit = %d, want %d", action, got.Limit, tc.want)
				}
				if got.Evidence != DiscordLimitEvidenceClamped {
					t.Fatalf("%s Evidence = %q, want %q", action, got.Evidence, DiscordLimitEvidenceClamped)
				}
			}
		})
	}
}
