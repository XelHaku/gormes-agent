package cli

import (
	"testing"
	"time"
)

func TestServiceRestartPoller(t *testing.T) {
	t.Run("active after delay uses hard restart timeout", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{at: 0, status: ServiceActiveStatusInactive},
			fakeServiceStatusAt{at: 1500 * time.Millisecond, status: ServiceActiveStatusActive},
		)

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service: "gormes-gateway",
			Runner:  runner,
			Clock:   clock,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollRestarted)
		if report.Timeout != 10*time.Second {
			t.Fatalf("Timeout = %v, want 10s", report.Timeout)
		}
		if report.Attempts != 4 {
			t.Fatalf("Attempts = %d, want 4", report.Attempts)
		}
		if got := clock.now.Sub(clock.start); got != 1500*time.Millisecond {
			t.Fatalf("elapsed = %v, want 1.5s", got)
		}
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollActiveEvidence)
		assertPolledServiceNames(t, runner, "gormes-gateway", 4)
	})

	t.Run("active after RestartSec polls through cooldown window", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{at: 0, status: ServiceActiveStatusInactive},
			fakeServiceStatusAt{at: 30 * time.Second, status: ServiceActiveStatusActive},
		)
		restartDelay := ParseServiceRestartDelay(ServiceRestartDelaySource{
			Manager: ServiceManagerSystemd,
			Output:  "RestartUSec=30s\n",
		})

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service:      "gormes-gateway",
			Runner:       runner,
			Clock:        clock,
			RestartDelay: restartDelay,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollRestarted)
		if report.Timeout != 40*time.Second {
			t.Fatalf("Timeout = %v, want RestartSec+10s = 40s", report.Timeout)
		}
		if report.PollInterval != 500*time.Millisecond {
			t.Fatalf("PollInterval = %v, want 500ms", report.PollInterval)
		}
		if report.Attempts != 61 {
			t.Fatalf("Attempts = %d, want 61 polls from t=0s through t=30s", report.Attempts)
		}
		if got := clock.now.Sub(clock.start); got != 30*time.Second {
			t.Fatalf("elapsed = %v, want 30s", got)
		}
		for i, sleep := range clock.sleeps {
			if sleep != 500*time.Millisecond {
				t.Fatalf("sleep[%d] = %v, want 500ms", i, sleep)
			}
		}
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollCooldownEvidence)
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollActiveEvidence)
	})

	t.Run("timeout reports operator evidence instead of restarted", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{at: 0, status: ServiceActiveStatusInactive},
		)

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service: "gormes-gateway",
			Runner:  runner,
			Clock:   clock,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollTimeout)
		if report.Attempts != 21 {
			t.Fatalf("Attempts = %d, want 21 polls from t=0s through t=10s", report.Attempts)
		}
		if got := clock.now.Sub(clock.start); got != 10*time.Second {
			t.Fatalf("elapsed = %v, want 10s", got)
		}
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollTimeoutEvidence)
		assertNoServiceRestartPollEvidence(t, report, ServiceRestartPollActiveEvidence)
	})

	t.Run("missing service manager reports unavailable evidence without sleeping", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{
				at:          0,
				status:      ServiceActiveStatusUnknown,
				unavailable: true,
				raw:         "systemctl: executable file not found",
			},
		)

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service: "gormes-gateway",
			Runner:  runner,
			Clock:   clock,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollManagerUnavailable)
		if report.Attempts != 1 {
			t.Fatalf("Attempts = %d, want one unavailable probe", report.Attempts)
		}
		if len(clock.sleeps) != 0 {
			t.Fatalf("sleeps = %#v, want none after service manager unavailable", clock.sleeps)
		}
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollManagerUnavailableEvidence)
	})

	t.Run("malformed RestartUSec falls back to hard restart timeout", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{at: 0, status: ServiceActiveStatusInactive},
			fakeServiceStatusAt{at: 9500 * time.Millisecond, status: ServiceActiveStatusActive},
		)
		restartDelay := ParseServiceRestartDelay(ServiceRestartDelaySource{
			Manager: ServiceManagerSystemd,
			Output:  "RestartUSec=soon\n",
		})

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service:      "gormes-gateway",
			Runner:       runner,
			Clock:        clock,
			RestartDelay: restartDelay,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollRestarted)
		if report.Timeout != 10*time.Second {
			t.Fatalf("Timeout = %v, want malformed RestartUSec fallback 10s", report.Timeout)
		}
		assertRestartDelayEvidence(t, report.RestartDelay, RestartDelayMalformed)
		assertRestartDelayEvidence(t, report.RestartDelay, RestartDelayDefaulted)
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollActiveEvidence)
	})

	t.Run("failed status reports crashed after restart and retry evidence", func(t *testing.T) {
		clock := newFakeServiceRestartClock()
		runner := newFakeServiceRestartRunner(clock,
			fakeServiceStatusAt{at: 0, status: ServiceActiveStatusInactive},
			fakeServiceStatusAt{at: 500 * time.Millisecond, status: ServiceActiveStatusFailed, raw: "failed"},
		)

		report := PollServiceRestartActive(ServiceRestartPollOptions{
			Service: "gormes-gateway",
			Runner:  runner,
			Clock:   clock,
		})

		assertServiceRestartPollOutcome(t, report, ServiceRestartPollCrashedAfterRestart)
		if report.Attempts != 2 {
			t.Fatalf("Attempts = %d, want failed status on second poll", report.Attempts)
		}
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollCrashedAfterRestartEvidence)
		assertServiceRestartPollEvidence(t, report, ServiceRestartPollRetryEvidence)
		assertNoServiceRestartPollEvidence(t, report, ServiceRestartPollActiveEvidence)
	})
}

type fakeServiceStatusAt struct {
	at          time.Duration
	status      ServiceActiveStatus
	unavailable bool
	raw         string
}

type fakeServiceRestartRunner struct {
	clock   *fakeServiceRestartClock
	states  []fakeServiceStatusAt
	probes  int
	service []string
}

func newFakeServiceRestartRunner(clock *fakeServiceRestartClock, states ...fakeServiceStatusAt) *fakeServiceRestartRunner {
	return &fakeServiceRestartRunner{clock: clock, states: states}
}

func (r *fakeServiceRestartRunner) ServiceActiveStatus(service string) (ServiceActiveStatusCheck, error) {
	r.probes++
	r.service = append(r.service, service)
	elapsed := r.clock.now.Sub(r.clock.start)
	current := fakeServiceStatusAt{status: ServiceActiveStatusInactive}
	for _, state := range r.states {
		if elapsed >= state.at {
			current = state
		}
	}
	return ServiceActiveStatusCheck{
		Status:      current.status,
		Unavailable: current.unavailable,
		Raw:         current.raw,
	}, nil
}

type fakeServiceRestartClock struct {
	start  time.Time
	now    time.Time
	sleeps []time.Duration
}

func newFakeServiceRestartClock() *fakeServiceRestartClock {
	start := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	return &fakeServiceRestartClock{start: start, now: start}
}

func (c *fakeServiceRestartClock) Now() time.Time {
	return c.now
}

func (c *fakeServiceRestartClock) Sleep(d time.Duration) {
	if d < 0 {
		panic("negative fake sleep")
	}
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
}

func assertServiceRestartPollOutcome(t *testing.T, report ServiceRestartPollReport, want ServiceRestartPollOutcome) {
	t.Helper()
	if report.Outcome != want {
		t.Fatalf("Outcome = %s, want %s; evidence=%#v", report.Outcome, want, report.Evidence)
	}
}

func assertServiceRestartPollEvidence(t *testing.T, report ServiceRestartPollReport, kind ServiceRestartPollEvidenceKind) {
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

func assertNoServiceRestartPollEvidence(t *testing.T, report ServiceRestartPollReport, kind ServiceRestartPollEvidenceKind) {
	t.Helper()
	for _, evidence := range report.Evidence {
		if evidence.Kind == kind {
			t.Fatalf("unexpected evidence kind=%s in %#v", kind, report.Evidence)
		}
	}
}

func assertPolledServiceNames(t *testing.T, runner *fakeServiceRestartRunner, service string, attempts int) {
	t.Helper()
	if len(runner.service) != attempts {
		t.Fatalf("probed services = %#v, want %d probes", runner.service, attempts)
	}
	for i, got := range runner.service {
		if got != service {
			t.Fatalf("service[%d] = %q, want %q", i, got, service)
		}
	}
}
