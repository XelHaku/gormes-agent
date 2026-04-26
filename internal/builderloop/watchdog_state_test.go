package builderloop

import (
	"testing"
	"time"
)

func TestDiagnose(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	deadAfter := 120 * time.Second
	slowAfter := 600 * time.Second

	cases := []struct {
		name string
		v    WorkerVitals
		want Verdict
	}{
		{
			name: "zero_pid_dead",
			v:    WorkerVitals{PID: 0, LastCommitAt: now.Add(-1 * time.Second), PIDIsLive: true},
			want: VerdictDead,
		},
		{
			name: "pid_not_live_after_dead_threshold",
			v:    WorkerVitals{PID: 1234, LastCommitAt: now.Add(-200 * time.Second), PIDIsLive: false},
			want: VerdictDead,
		},
		{
			name: "live_after_slow_threshold",
			v:    WorkerVitals{PID: 1234, LastCommitAt: now.Add(-700 * time.Second), PIDIsLive: true},
			want: VerdictSlow,
		},
		{
			name: "dead_wins_when_both_thresholds_fire",
			v:    WorkerVitals{PID: 1234, LastCommitAt: now.Add(-700 * time.Second), PIDIsLive: false},
			want: VerdictDead,
		},
		{
			name: "healthy_recent_live",
			v:    WorkerVitals{PID: 1234, LastCommitAt: now.Add(-5 * time.Second), PIDIsLive: true},
			want: VerdictHealthy,
		},
		{
			name: "pid_live_silent_for_a_year_is_slow_not_dead",
			v:    WorkerVitals{PID: 1234, LastCommitAt: now.Add(-99999 * time.Second), PIDIsLive: true},
			want: VerdictSlow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diagnose(now, tc.v, deadAfter, slowAfter)
			if got != tc.want {
				t.Fatalf("Diagnose(%+v) = %q; want %q", tc.v, got, tc.want)
			}
		})
	}
}
