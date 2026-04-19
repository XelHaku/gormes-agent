package kernel

import (
	"context"
	"testing"
	"time"
)

func TestRetryBudget_NextDelay_ExponentialWithJitter(t *testing.T) {
	b := NewRetryBudget()
	base := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
	for i, want := range base {
		got := b.NextDelay()
		low := time.Duration(float64(want) * 0.8)
		high := time.Duration(float64(want) * 1.2)
		if got < low || got > high {
			t.Errorf("attempt %d: delay = %v, want within +/-20%% of %v", i+1, got, want)
		}
	}
	if got := b.NextDelay(); got != -1 {
		t.Errorf("attempt 6: delay = %v, want -1 (budget exhausted)", got)
	}
}

func TestRetryBudget_Exhausted(t *testing.T) {
	b := NewRetryBudget()
	for i := 0; i < 5; i++ {
		_ = b.NextDelay()
	}
	if !b.Exhausted() {
		t.Error("Exhausted should be true after 5 attempts")
	}
}

func TestRetryBudget_WaitRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := Wait(ctx, 1*time.Hour)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Wait blocked %v on cancelled ctx; must return immediately", d)
	}
}
