package cli

import (
	"testing"
	"time"
)

func TestServiceRestartParserParsesBoundedSystemdDurations(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   time.Duration
	}{
		{name: "systemd value seconds", output: "30s\n", want: 30 * time.Second},
		{name: "systemd value milliseconds", output: "100ms\n", want: 100 * time.Millisecond},
		{name: "systemd compound minutes and seconds", output: "1min 30s\n", want: 90 * time.Second},
		{name: "systemd RestartUSec property", output: "RestartUSec=30s\n", want: 30 * time.Second},
		{name: "systemd RestartSec property", output: "RestartSec=1min 30s\n", want: 90 * time.Second},
		{name: "systemd zero microseconds", output: "RestartUSec=0\n", want: 0},
		{
			name:   "systemd delay is bounded",
			output: "RestartUSec=10min\n",
			want:   2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ParseServiceRestartDelay(ServiceRestartDelaySource{
				Manager:  ServiceManagerSystemd,
				Output:   tt.output,
				MaxDelay: 2 * time.Minute,
			})

			if report.Delay != tt.want {
				t.Fatalf("delay = %v, want %v", report.Delay, tt.want)
			}
			assertNoRestartDelayEvidence(t, report, RestartDelayDefaulted)
			assertNoRestartDelayEvidence(t, report, RestartDelayMalformed)
			assertNoRestartDelayEvidence(t, report, RestartDelayInfinite)
			assertNoRestartDelayEvidence(t, report, ServiceManagerUnavailable)
		})
	}
}

func TestServiceRestartParserReportsDefaultedEvidence(t *testing.T) {
	tests := []struct {
		name         string
		source       ServiceRestartDelaySource
		wantEvidence []ServiceRestartDelayEvidenceKind
	}{
		{
			name: "blank output is missing",
			source: ServiceRestartDelaySource{
				Manager: ServiceManagerSystemd,
				Output:  "\n",
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				RestartDelayMissing,
				RestartDelayDefaulted,
			},
		},
		{
			name: "missing RestartUSec property is missing",
			source: ServiceRestartDelaySource{
				Manager: ServiceManagerSystemd,
				Output:  "MainPID=123\nActiveState=active\n",
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				RestartDelayMissing,
				RestartDelayDefaulted,
			},
		},
		{
			name: "malformed output is defaulted",
			source: ServiceRestartDelaySource{
				Manager: ServiceManagerSystemd,
				Output:  "RestartUSec=soon\n",
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				RestartDelayMalformed,
				RestartDelayDefaulted,
			},
		},
		{
			name: "infinite output is defaulted",
			source: ServiceRestartDelaySource{
				Manager: ServiceManagerSystemd,
				Output:  "RestartUSec=infinity\n",
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				RestartDelayInfinite,
				RestartDelayDefaulted,
			},
		},
		{
			name: "unsupported manager is defaulted",
			source: ServiceRestartDelaySource{
				Manager: ServiceManagerUnsupported,
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				RestartDelayUnsupported,
				RestartDelayDefaulted,
			},
		},
		{
			name: "unavailable service manager is defaulted",
			source: ServiceRestartDelaySource{
				Manager:     ServiceManagerSystemd,
				Unavailable: true,
			},
			wantEvidence: []ServiceRestartDelayEvidenceKind{
				ServiceManagerUnavailable,
				RestartDelayDefaulted,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ParseServiceRestartDelay(tt.source)
			if report.Delay != DefaultServiceRestartDelay {
				t.Fatalf("delay = %v, want default %v", report.Delay, DefaultServiceRestartDelay)
			}

			for _, kind := range tt.wantEvidence {
				assertRestartDelayEvidence(t, report, kind)
			}
		})
	}
}

func assertRestartDelayEvidence(t *testing.T, report ServiceRestartDelayReport, kind ServiceRestartDelayEvidenceKind) {
	t.Helper()
	for _, evidence := range report.Evidence {
		if evidence.Kind == kind {
			if evidence.Detail == "" {
				t.Fatalf("evidence kind=%s has blank operator detail: %#v", kind, report.Evidence)
			}
			return
		}
	}
	t.Fatalf("missing evidence kind=%s in %#v", kind, report.Evidence)
}

func assertNoRestartDelayEvidence(t *testing.T, report ServiceRestartDelayReport, kind ServiceRestartDelayEvidenceKind) {
	t.Helper()
	for _, evidence := range report.Evidence {
		if evidence.Kind == kind {
			t.Fatalf("unexpected evidence kind=%s in %#v", kind, report.Evidence)
		}
	}
}
