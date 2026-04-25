package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
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

	// MaxDepth overrides the package default depth limit when > 0.
	MaxDepth int

	// Registry tracks every live subagent process-wide.
	Registry SubagentRegistry

	// NewRunner mints a Runner for each spawned subagent. Phase 2.E closeout
	// defaults this to StubRunner; later slices may supply alternate runners.
	NewRunner func() Runner

	// DefaultMaxIterations overrides the package default iteration budget when
	// cfg.MaxIterations <= 0.
	DefaultMaxIterations int

	// DefaultMaxConcurrent overrides SpawnBatch's package default semaphore
	// size when the caller passes maxConcurrent <= 0.
	DefaultMaxConcurrent int

	// DefaultTimeout applies when cfg.Timeout <= 0.
	DefaultTimeout time.Duration

	// RunLogPath enables append-only JSONL run logging when non-empty.
	RunLogPath string

	// DurableLedger enables SQLite-backed restart/replay state when non-nil.
	// It is additive to RunLogPath; delegate_task output remains unchanged.
	DurableLedger *DurableLedger

	// ToolExecutor is injected into every spawned child config so runners can
	// enforce allowlists at tool-execution time.
	ToolExecutor tools.ToolExecutor

	// ToolAudit records append-only JSONL child-tool execution events when
	// delegate_task spawns a runner that invokes tools.
	ToolAudit audit.Recorder
}

type manager struct {
	opts ManagerOpts

	mu        sync.RWMutex
	children  map[string]*Subagent
	runLogger *runLogger

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
		opts:      opts,
		children:  make(map[string]*Subagent),
		runLogger: newRunLogger(opts.RunLogPath),
		closed:    make(chan struct{}, 0),
	}
}

func (m *manager) Spawn(ctx context.Context, cfg SubagentConfig) (*Subagent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.opts.Depth >= m.maxDepth() {
		return nil, fmt.Errorf("%w (depth=%d)", ErrMaxDepth, m.opts.Depth)
	}

	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = m.defaultMaxIterations()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = m.opts.DefaultTimeout
	}
	if cfg.toolExecutor == nil {
		cfg.toolExecutor = m.opts.ToolExecutor
	}
	if cfg.toolAudit == nil {
		cfg.toolAudit = m.opts.ToolAudit
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
		callerStop:    func() bool { return true },
		publicEvents:  make(chan SubagentEvent, 16),
		done:          make(chan struct{}, 0),
	}
	if ctx != nil {
		sa.callerStop = context.AfterFunc(ctx, func() {
			sa.setCancelReason(classifyContextErr(ctx.Err()))
			cancel()
		})
		if err := ctx.Err(); err != nil {
			sa.setCancelReason(classifyContextErr(err))
			cancel()
		}
	}
	sa.cfg.agentID = sa.ID

	m.children[sa.ID] = sa
	m.opts.Registry.Register(sa)
	if err := m.startDurableLedger(sa); err != nil {
		slog.Warn("subagent: durable ledger start failed", "subagent_id", sa.ID, "err", err)
	}

	go m.run(sa)
	return sa, nil
}

// run is the per-subagent lifecycle goroutine.
func (m *manager) run(sa *Subagent) {
	start := time.Now()
	runner := m.opts.NewRunner()

	internalEvents := make(chan SubagentEvent, 16)
	resultCh := make(chan *SubagentResult, 1)
	runnerDone := make(chan struct{}, 0)

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

	forwarderDone := make(chan struct{}, 0)
	var capturedToolCalls []ToolCallInfo
	go func() {
		defer close(forwarderDone)
		defer close(sa.publicEvents)
		for ev := range internalEvents {
			if ev.Type == EventToolCall && ev.ToolCall != nil {
				capturedToolCalls = append(capturedToolCalls, *ev.ToolCall)
			}
			sa.publicEvents <- ev
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
	if sa.callerStop != nil {
		sa.callerStop()
	}
	result = normalizeResult(sa, result)
	result.ID = sa.ID
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}
	if err := m.appendRunLog(sa, result); err != nil {
		slog.Warn("subagent: append run log failed", "subagent_id", sa.ID, "path", m.opts.RunLogPath, "err", err)
	}

	<-runnerDone
	if sa.ctx.Err() != nil {
		msg, _ := sa.interruptMsg.Load().(string)
		internalEvents <- SubagentEvent{Type: EventInterrupted, Message: msg}
	}
	close(internalEvents)
	<-forwarderDone
	result.ToolCalls = mergeToolCallAudit(result.ToolCalls, capturedToolCalls)
	if err := m.finishDurableLedger(sa, result); err != nil {
		slog.Warn("subagent: durable ledger finish failed", "subagent_id", sa.ID, "err", err)
	}

	if sa.timeoutCancel != nil {
		sa.timeoutCancel()
	}
	sa.cancel()

	sa.setResult(result)
	close(sa.done)

	m.removeChild(sa.ID)
	m.opts.Registry.Unregister(sa.ID)
}

func mergeToolCallAudit(existing, captured []ToolCallInfo) []ToolCallInfo {
	if len(captured) == 0 {
		return existing
	}
	out := append([]ToolCallInfo(nil), existing...)
	seen := make(map[ToolCallInfo]struct{}, len(existing))
	for _, info := range existing {
		seen[info] = struct{}{}
	}
	for _, info := range captured {
		if _, ok := seen[info]; ok {
			continue
		}
		seen[info] = struct{}{}
		out = append(out, info)
	}
	return out
}

func (m *manager) removeChild(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.children, id)
}

// Interrupt records the message for the final interrupted event and cancels the
// tracked subagent context.
func (m *manager) Interrupt(sa *Subagent, message string) error {
	m.mu.RLock()
	tracked, ok := m.children[sa.ID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrSubagentNotFound, sa.ID)
	}
	tracked.interruptMsg.Store(message)
	tracked.setCancelReason("interrupted")
	tracked.cancel()
	return nil
}

// Collect returns the terminal result once the subagent is done, otherwise nil.
func (m *manager) Collect(sa *Subagent) *SubagentResult {
	select {
	case <-sa.done:
		sa.mu.RLock()
		defer sa.mu.RUnlock()
		return sa.result
	default:
		return nil
	}
}

// Close cancels every live child, waits for them to finish, and closes the
// manager exactly once.
func (m *manager) Close() error {
	m.closeOnce.Do(func() {
		m.mu.RLock()
		snap := make([]*Subagent, 0, len(m.children))
		for _, sa := range m.children {
			snap = append(snap, sa)
		}
		m.mu.RUnlock()

		for _, sa := range snap {
			sa.cancel()
		}
		for _, sa := range snap {
			<-sa.done
		}
		close(m.closed)
	})
	return nil
}

// SpawnBatch executes multiple subagent specs with bounded concurrency and
// returns one result per input config in order.
func (m *manager) SpawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error) {
	return m.spawnBatch(ctx, cfgs, maxConcurrent)
}

func (m *manager) maxDepth() int {
	if m.opts.MaxDepth > 0 {
		return m.opts.MaxDepth
	}
	return MaxDepth
}

func (m *manager) defaultMaxIterations() int {
	if m.opts.DefaultMaxIterations > 0 {
		return m.opts.DefaultMaxIterations
	}
	return DefaultMaxIterations
}

func (m *manager) defaultMaxConcurrent() int {
	if m.opts.DefaultMaxConcurrent > 0 {
		return m.opts.DefaultMaxConcurrent
	}
	return DefaultMaxConcurrent
}

func (m *manager) appendRunLog(sa *Subagent, result *SubagentResult) error {
	if m.runLogger == nil {
		return nil
	}
	return m.runLogger.append(sa, result)
}

func (m *manager) startDurableLedger(sa *Subagent) error {
	if m.opts.DurableLedger == nil {
		return nil
	}
	progress, err := json.Marshal(map[string]any{
		"goal":  sa.cfg.Goal,
		"phase": "started",
	})
	if err != nil {
		return err
	}
	if _, err := m.opts.DurableLedger.Submit(context.Background(), DurableJobSubmission{
		ID:       sa.ID,
		Kind:     WorkKindLLMSubagent,
		ParentID: sa.ParentID,
		Depth:    sa.Depth,
		Progress: progress,
	}); err != nil {
		return err
	}
	lockUntil := time.Now().UTC().Add(24 * time.Hour)
	if sa.cfg.Timeout > 0 {
		lockUntil = time.Now().UTC().Add(sa.cfg.Timeout + time.Minute)
	}
	_, ok, err := m.opts.DurableLedger.ClaimJob(context.Background(), sa.ID, DurableClaim{
		WorkerID:  sa.ID,
		LockUntil: lockUntil,
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("durable job %s was not claimable", sa.ID)
	}
	return nil
}

func (m *manager) finishDurableLedger(sa *Subagent, result *SubagentResult) error {
	if m.opts.DurableLedger == nil {
		return nil
	}
	resultJSON, err := json.Marshal(map[string]any{
		"exit_reason": result.ExitReason,
		"iterations":  result.Iterations,
		"status":      string(result.Status),
		"summary":     result.Summary,
	})
	if err != nil {
		return err
	}
	switch result.Status {
	case StatusCompleted:
		_, _, err = m.opts.DurableLedger.Complete(context.Background(), sa.ID, sa.ID, resultJSON)
		return err
	case StatusFailed, StatusError:
		errorText := strings.TrimSpace(result.Error)
		if errorText == "" {
			errorText = result.ExitReason
		}
		_, _, err = m.opts.DurableLedger.Fail(context.Background(), sa.ID, sa.ID, errorText)
		return err
	case StatusInterrupted:
		reason := strings.TrimSpace(result.ExitReason)
		if reason == "" {
			reason = result.Error
		}
		_, _, err = m.opts.DurableLedger.Cancel(context.Background(), sa.ID, reason)
		return err
	default:
		return nil
	}
}

func normalizeResult(sa *Subagent, result *SubagentResult) *SubagentResult {
	if result == nil {
		return nil
	}
	if result.Status == StatusCompleted || result.Status == StatusFailed {
		return result
	}

	reason := sa.getCancelReason()
	if reason == "" {
		reason = classifyContextErr(sa.ctx.Err())
	}
	if reason == "" {
		return result
	}

	result.Status = StatusInterrupted
	result.ExitReason = reason
	if result.Error == "" && sa.ctx.Err() != nil {
		result.Error = sa.ctx.Err().Error()
	}
	return result
}

func classifyContextErr(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return ""
	}
}
