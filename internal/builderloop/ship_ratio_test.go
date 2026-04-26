package builderloop

import (
	"testing"
	"time"
)

func TestClassifySubphase(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  RowKind
	}{
		{name: "bare_1c", input: "1.C", want: RowKindSelfImprovement},
		{name: "path_1c", input: "1/1.C/Watchdog dead-process vs slow-progress separation", want: RowKindSelfImprovement},
		{name: "bare_5o", input: "5.O", want: RowKindSelfImprovement},
		{name: "path_5o", input: "5/5.O/CLI log snapshot reader", want: RowKindSelfImprovement},
		{name: "control_plane_prefix", input: "control-plane/backend", want: RowKindSelfImprovement},
		{name: "user_feature_4a", input: "4.A", want: RowKindUserFeature},
		{name: "user_feature_4h", input: "4.H", want: RowKindUserFeature},
		{name: "user_feature_6a", input: "6.A", want: RowKindUserFeature},
		{name: "user_feature_7b", input: "7.B", want: RowKindUserFeature},
		{name: "blank_input", input: "", want: RowKindUnclassified},
		{name: "whitespace_only", input: "   ", want: RowKindUnclassified},
		{name: "unknown_99x", input: "99.X", want: RowKindUnclassified},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifySubphase(tc.input)
			if got != tc.want {
				t.Fatalf("ClassifySubphase(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestComputeShipRatio(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	window := 24 * time.Hour

	t.Run("inclusive_window_start", func(t *testing.T) {
		events := []ShippedRowEvent{
			{SubphaseID: "1.C", ShippedAt: now.Add(-window)},
		}
		got := ComputeShipRatio(events, window, now)
		if got.Total != 1 {
			t.Fatalf("event at exact window boundary should be included; got Total=%d", got.Total)
		}
		if got.SelfImprovement != 1 {
			t.Fatalf("expected SelfImprovement=1, got %d", got.SelfImprovement)
		}
	})

	t.Run("future_event_excluded", func(t *testing.T) {
		events := []ShippedRowEvent{
			{SubphaseID: "1.C", ShippedAt: now.Add(1 * time.Hour)},
		}
		got := ComputeShipRatio(events, window, now)
		if got.Total != 0 {
			t.Fatalf("future event should be excluded; got Total=%d", got.Total)
		}
		if got.SelfImprovement != 0 {
			t.Fatalf("future event should not count toward SelfImprovement; got %d", got.SelfImprovement)
		}
	})

	t.Run("per_kind_counts", func(t *testing.T) {
		events := []ShippedRowEvent{
			{SubphaseID: "1.C", ShippedAt: now.Add(-1 * time.Hour)},
			{SubphaseID: "5.O", ShippedAt: now.Add(-2 * time.Hour)},
			{SubphaseID: "control-plane/backend", ShippedAt: now.Add(-3 * time.Hour)},
			{SubphaseID: "4.A", ShippedAt: now.Add(-4 * time.Hour)},
			{SubphaseID: "6.A", ShippedAt: now.Add(-5 * time.Hour)},
			{SubphaseID: "99.X", ShippedAt: now.Add(-6 * time.Hour)},
			{SubphaseID: "", ShippedAt: now.Add(-7 * time.Hour)},
		}
		got := ComputeShipRatio(events, window, now)
		if got.SelfImprovement != 3 {
			t.Errorf("SelfImprovement = %d, want 3", got.SelfImprovement)
		}
		if got.UserFeature != 2 {
			t.Errorf("UserFeature = %d, want 2", got.UserFeature)
		}
		if got.Unclassified != 2 {
			t.Errorf("Unclassified = %d, want 2", got.Unclassified)
		}
		want := got.SelfImprovement + got.UserFeature + got.Unclassified
		if got.Total != want {
			t.Errorf("Total = %d, want %d (sum of kinds)", got.Total, want)
		}
	})

	t.Run("zero_events_returns_zero_total", func(t *testing.T) {
		got := ComputeShipRatio(nil, window, now)
		if got != (ShipRatio{}) {
			t.Fatalf("ComputeShipRatio(nil) = %+v, want zero-valued ShipRatio", got)
		}
	})
}
