package autoloop

import "testing"

func TestBackendDegrader_NoChainNoSwitch(t *testing.T) {
	d := newBackendDegrader(nil, 3)
	for i := 0; i < 5; i++ {
		d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	}
	if d.degraded {
		t.Fatal("empty chain should never degrade")
	}
}

func TestBackendDegrader_SingleElementChainNoSwitch(t *testing.T) {
	d := newBackendDegrader([]string{"codexu"}, 3)
	for i := 0; i < 5; i++ {
		d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	}
	// Should mark degraded (no further fallback) but not actually switch.
	if d.Current() != "codexu" {
		t.Fatalf("Current() = %q, want codexu", d.Current())
	}
}

func TestBackendDegrader_DegradesAfterThreshold(t *testing.T) {
	d := newBackendDegrader([]string{"codexu", "claudeu", "opencode"}, 3)
	var switched bool
	var to string
	for i := 0; i < 3; i++ {
		switched, _, to = d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	}
	if !switched {
		t.Fatal("should have switched after 3 backend errors")
	}
	if to != "claudeu" {
		t.Fatalf("to = %q, want claudeu", to)
	}
	if d.Current() != "claudeu" {
		t.Fatalf("Current() = %q, want claudeu", d.Current())
	}
}

func TestBackendDegrader_SuccessResetsCounter(t *testing.T) {
	d := newBackendDegrader([]string{"codexu", "claudeu"}, 3)
	d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	d.ObserveOutcome(workerOutcome{IsSuccessFlag: true})
	d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	if d.degraded {
		t.Fatal("success should have reset the counter")
	}
}

func TestBackendDegrader_RowFailureWithCommitDoesNotDegrade(t *testing.T) {
	d := newBackendDegrader([]string{"codexu", "claudeu"}, 3)
	for i := 0; i < 5; i++ {
		// IsBackendErrorFlag is false because the row failed, not the backend
		// (the worker produced a commit but the report didn't validate, etc).
		d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: false})
	}
	if d.degraded {
		t.Fatal("row failures should not degrade backend")
	}
}

func TestBackendDegrader_DoesNotDegradePastEndOfChain(t *testing.T) {
	d := newBackendDegrader([]string{"codexu", "claudeu"}, 2)
	// First 2 errors → switch from codexu to claudeu
	d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	switched, _, _ := d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	if !switched || d.Current() != "claudeu" {
		t.Fatalf("expected switch to claudeu, got switched=%v current=%q", switched, d.Current())
	}
	// Next errors should NOT switch further (no fallback past the last entry).
	switched, _, _ = d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	switched2, _, _ := d.ObserveOutcome(workerOutcome{IsBackendErrorFlag: true})
	if switched || switched2 {
		t.Fatal("should not switch past the last chain entry")
	}
	if d.Current() != "claudeu" {
		t.Fatalf("Current() = %q, want claudeu (terminal)", d.Current())
	}
}
