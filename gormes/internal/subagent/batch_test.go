package subagent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type concurrencyProbeRunner struct {
	mu      sync.Mutex
	current int
	maxSeen *atomic.Int64
	hold    time.Duration
}

func (c *concurrencyProbeRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	c.mu.Lock()
	c.current++
	if int64(c.current) > c.maxSeen.Load() {
		c.maxSeen.Store(int64(c.current))
	}
	c.mu.Unlock()

	select {
	case <-time.After(c.hold):
	case <-ctx.Done():
	}

	c.mu.Lock()
	c.current--
	c.mu.Unlock()

	return &SubagentResult{Status: StatusCompleted, Summary: cfg.Goal, ExitReason: "probe_done"}
}

func TestSpawnBatchEnforcesMaxConcurrent(t *testing.T) {
	maxSeen := &atomic.Int64{}
	probe := &concurrencyProbeRunner{maxSeen: maxSeen, hold: 30 * time.Millisecond}

	mgr := NewManager(ManagerOpts{
		ParentCtx: context.Background(),
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return probe },
	})

	cfgs := make([]SubagentConfig, 10)
	for i := range cfgs {
		cfgs[i] = SubagentConfig{Goal: "p"}
	}

	results, err := mgr.SpawnBatch(context.Background(), cfgs, 2)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	if len(results) != len(cfgs) {
		t.Fatalf("results len: want %d, got %d", len(cfgs), len(results))
	}
	if maxSeen.Load() > 2 {
		t.Errorf("maxSeen: want <= 2, got %d", maxSeen.Load())
	}
	for i, r := range results {
		if r == nil {
			t.Errorf("results[%d] nil", i)
		}
	}
}

func TestSpawnBatchPreservesInputOrder(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	cfgs := []SubagentConfig{
		{Goal: "alpha"},
		{Goal: "bravo"},
		{Goal: "charlie"},
		{Goal: "delta"},
	}
	results, err := mgr.SpawnBatch(context.Background(), cfgs, 0)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	for i, want := range cfgs {
		if results[i] == nil {
			t.Fatalf("results[%d] nil", i)
		}
		if results[i].Summary != want.Goal {
			t.Errorf("results[%d].Summary: want %q, got %q", i, want.Goal, results[i].Summary)
		}
	}
}

func TestSpawnBatchEmptyInput(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	results, err := mgr.SpawnBatch(context.Background(), nil, 3)
	if err != nil {
		t.Fatalf("SpawnBatch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results len: want 0, got %d", len(results))
	}
}

func TestSpawnBatchContextCancellationCancelsChildren(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	reg := NewRegistry()
	mgr := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  reg,
		NewRunner: func() Runner { return blockingRunner{} },
	})
	defer mgr.Close()

	batchCtx, cancelBatch := context.WithCancel(context.Background())
	done := make(chan []*SubagentResult, 1)
	go func() {
		results, _ := mgr.SpawnBatch(batchCtx, []SubagentConfig{
			{Goal: "first"},
			{Goal: "second"},
		}, 1)
		done <- results
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(reg.List()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(reg.List()) == 0 {
		t.Fatal("expected at least one live subagent before batch cancellation")
	}

	cancelBatch()

	select {
	case results := <-done:
		if len(results) != 2 {
			t.Fatalf("results len: want 2, got %d", len(results))
		}
		if results[0] == nil {
			t.Fatal("results[0] nil after batch cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SpawnBatch did not return after batch context cancellation")
	}

	waitForRegistryEmpty(t, reg, 2*time.Second)
}
