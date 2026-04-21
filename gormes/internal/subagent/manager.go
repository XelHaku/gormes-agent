package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SubagentManager owns the goroutine lifecycle for every subagent it spawns.
type SubagentManager interface {
	Spawn(ctx context.Context, cfg SubagentConfig) (*Subagent, error)
	SpawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error)
	Interrupt(sa *Subagent, message string) error
	Collect(sa *Subagent) *SubagentResult
	Close() error
}

// ManagerOpts configures NewManager.
type ManagerOpts struct {
	// ParentCtx is the context every spawned subagent's ctx will derive from
	// via WithCancel. Cancelling ParentCtx cancels every child.
	ParentCtx context.Context

	// ParentID is recorded on every spawned Subagent.ParentID. Informational.
	ParentID string

	// Depth is the manager's depth in the subagent tree. Children of a
	// manager at depth D are spawned at depth D+1. Spawn returns ErrMaxDepth
	// when Depth >= MaxDepth.
	Depth int

	// Registry tracks every live subagent process-wide.
	Registry SubagentRegistry

	// NewRunner mints a Runner for each spawned subagent. This slice always
	// passes a func returning StubRunner{}; later tasks will pass different
	// runner factories.
	NewRunner func() Runner
}

type manager struct {
	opts ManagerOpts

	mu       sync.RWMutex
	children map[string]*Subagent

	closeOnce sync.Once
	closed    chan struct{}
}

// NewManager constructs a SubagentManager.
func NewManager(opts ManagerOpts) SubagentManager {
	if opts.NewRunner == nil {
		opts.NewRunner = func() Runner { return StubRunner{} }
	}
	if opts.Registry == nil {
		opts.Registry = NewRegistry()
	}
	if opts.ParentCtx == nil {
		opts.ParentCtx = context.Background()
	}
	return &manager{
		opts:     opts,
		children: make(map[string]*Subagent),
		closed:   make(chan struct{}),
	}
}

func (m *manager) Spawn(_ context.Context, cfg SubagentConfig) (*Subagent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.opts.Depth >= MaxDepth {
		return nil, fmt.Errorf("%w (depth=%d)", ErrMaxDepth, m.opts.Depth)
	}

	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}

	childCtx, cancel := context.WithCancel(m.opts.ParentCtx)
	var timeoutCancel context.CancelFunc
	if cfg.Timeout > 0 {
		childCtx, timeoutCancel = context.WithTimeout(childCtx, cfg.Timeout)
	}

	sa := &Subagent{
		ID:            newSubagentID(),
		ParentID:      m.opts.ParentID,
		Depth:         m.opts.Depth + 1,
		cfg:           cfg,
		ctx:           childCtx,
		cancel:        cancel,
		timeoutCancel: timeoutCancel,
		publicEvents:  make(chan SubagentEvent, 16),
		done:          make(chan struct{}),
	}

	m.children[sa.ID] = sa
	m.opts.Registry.Register(sa)

	go m.run(sa)
	return sa, nil
}

// run is the per-subagent lifecycle goroutine.
func (m *manager) run(sa *Subagent) {
	start := time.Now()
	runner := m.opts.NewRunner()

	internalEvents := make(chan SubagentEvent, 16)
	resultCh := make(chan *SubagentResult, 1)
	runnerDone := make(chan struct{})

	go func() {
		defer close(runnerDone)
		defer func() {
			if r := recover(); r != nil {
				resultCh <- &SubagentResult{
					Status:     StatusError,
					ExitReason: "panic",
					Error:      fmt.Sprintf("%v", r),
				}
			}
		}()
		resultCh <- runner.Run(sa.ctx, sa.cfg, internalEvents)
	}()

	forwarderDone := make(chan struct{})
	go func() {
		defer close(forwarderDone)
		defer close(sa.publicEvents)
		for ev := range internalEvents {
			sa.publicEvents <- ev
		}
	}()

	interruptDone := make(chan struct{})
	go func() {
		defer close(interruptDone)
		select {
		case <-sa.ctx.Done():
			msg, _ := sa.interruptMsg.Load().(string)
			internalEvents <- SubagentEvent{Type: EventInterrupted, Message: msg}
		case <-runnerDone:
		}
	}()

	result := <-resultCh
	if result == nil {
		result = &SubagentResult{
			Status:     StatusError,
			ExitReason: "nil_result",
			Error:      "subagent: runner returned nil result",
		}
	}
	result.ID = sa.ID
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}

	<-runnerDone
	<-interruptDone
	close(internalEvents)
	<-forwarderDone

	if sa.timeoutCancel != nil {
		sa.timeoutCancel()
	}

	sa.setResult(result)
	close(sa.done)

	m.removeChild(sa.ID)
	m.opts.Registry.Unregister(sa.ID)
}

func (m *manager) removeChild(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.children, id)
}

// Interrupt is implemented in a later task.
func (m *manager) Interrupt(sa *Subagent, message string) error {
	m.mu.RLock()
	tracked, ok := m.children[sa.ID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrSubagentNotFound, sa.ID)
	}
	tracked.interruptMsg.Store(message)
	tracked.cancel()
	return nil
}

// Collect is implemented in a later task.
func (m *manager) Collect(_ *Subagent) *SubagentResult {
	return nil
}

// Close is implemented in a later task.
func (m *manager) Close() error {
	return nil
}

// SpawnBatch is implemented in a later task.
func (m *manager) SpawnBatch(_ context.Context, _ []SubagentConfig, _ int) ([]*SubagentResult, error) {
	return nil, fmt.Errorf("subagent: SpawnBatch not implemented")
}
