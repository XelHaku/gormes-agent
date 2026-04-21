package subagent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

const handleEventBufferSize = 32

// Manager owns delegated subagent runs and their live handles.
type Manager struct {
	cfg     config.DelegationCfg
	runner  Runner
	logPath string

	mu     sync.Mutex
	live   map[string]*Handle
	runSeq uint64
}

// Handle tracks one in-flight delegated run.
type Handle struct {
	RunID  string
	Events <-chan Event

	done   chan struct{}
	cancel context.CancelFunc

	mu     sync.Mutex
	result Result
	err    error

	events chan Event
}

// NewManager wires a runner to delegation defaults and optional JSONL logging.
func NewManager(cfg config.DelegationCfg, runner Runner, logPath string) *Manager {
	return &Manager{
		cfg:     cfg,
		runner:  runner,
		logPath: logPath,
		live:    make(map[string]*Handle),
	}
}

// Start launches one child run asynchronously and returns its handle.
func (m *Manager) Start(parent context.Context, spec Spec) (*Handle, error) {
	if m == nil {
		return nil, fmt.Errorf("subagent: nil manager")
	}
	if m.runner == nil {
		return nil, fmt.Errorf("subagent: nil runner")
	}
	if parent == nil {
		parent = context.Background()
	}

	spec, err := ApplyDefaults(spec, m.cfg)
	if err != nil {
		return nil, err
	}

	runID := m.nextRunID()
	runCtx, cancel := context.WithTimeout(parent, spec.Timeout)
	events := make(chan Event, handleEventBufferSize)

	handle := &Handle{
		RunID:  runID,
		Events: events,
		done:   make(chan struct{}, 1),
		cancel: cancel,
		events: events,
	}

	m.mu.Lock()
	m.live[runID] = handle
	m.mu.Unlock()

	go m.run(handle, runCtx, spec)

	return handle, nil
}

// Wait blocks until the run completes or the waiting context is canceled.
func (h *Handle) Wait(ctx context.Context) (Result, error) {
	if h == nil {
		return Result{}, fmt.Errorf("subagent: nil handle")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err
	case <-ctx.Done():
		return Result{RunID: h.RunID}, ctx.Err()
	}
}

// Cancel aborts the delegated run.
func (h *Handle) Cancel() error {
	if h == nil {
		return nil
	}
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

func (m *Manager) run(handle *Handle, runCtx context.Context, spec Spec) {
	startedAt := time.Now().UTC()
	defer m.remove(handle.RunID)
	defer close(handle.done)
	defer func() {
		if handle.events != nil {
			close(handle.events)
		}
		if handle.cancel != nil {
			handle.cancel()
		}
	}()

	result, err := m.runSafely(runCtx, spec, handle)
	finishedAt := time.Now().UTC()

	result.RunID = handle.RunID
	result = normalizeResult(runCtx, result, err)

	var orchErr error
	if err := AppendRunLog(m.logPath, RunRecord{
		RunID:        result.RunID,
		Status:       result.Status,
		Summary:      result.Summary,
		Error:        result.Error,
		FinishReason: result.FinishReason,
		ToolCalls:    append([]string(nil), result.ToolCalls...),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}); err != nil {
		orchErr = fmt.Errorf("subagent: append run log: %w", err)
		if result.Error == "" {
			result.Error = orchErr.Error()
		} else {
			result.Error = result.Error + "; bookkeeping error: " + orchErr.Error()
		}
	}

	handle.mu.Lock()
	handle.result = result
	handle.err = nil
	handle.mu.Unlock()
}

func (m *Manager) runSafely(runCtx context.Context, spec Spec, handle *Handle) (result Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("subagent: runner panicked: %v", r)
			result = Result{Status: StatusFailed, Error: err.Error()}
		}
	}()

	return m.runner.Run(runCtx, spec, func(ev Event) {
		if handle.events == nil {
			return
		}
		select {
		case handle.events <- ev:
		default:
		}
	})
}

func (m *Manager) nextRunID() string {
	seq := atomic.AddUint64(&m.runSeq, 1)
	return fmt.Sprintf("run-%d-%06d", time.Now().UTC().UnixNano(), seq)
}

func (m *Manager) remove(runID string) {
	m.mu.Lock()
	delete(m.live, runID)
	m.mu.Unlock()
}

func normalizeResult(ctx context.Context, result Result, runErr error) Result {
	switch ctx.Err() {
	case context.Canceled:
		result.Status = StatusCancelled
		if result.Error == "" {
			if runErr != nil {
				result.Error = runErr.Error()
			} else {
				result.Error = context.Canceled.Error()
			}
		}
	case context.DeadlineExceeded:
		result.Status = StatusTimedOut
		if result.Error == "" {
			if runErr != nil {
				result.Error = runErr.Error()
			} else {
				result.Error = context.DeadlineExceeded.Error()
			}
		}
	default:
		if runErr != nil {
			if result.Status == "" {
				result.Status = StatusFailed
			}
			if result.Error == "" {
				result.Error = runErr.Error()
			}
		} else if result.Status == "" {
			result.Status = StatusCompleted
		}
	}
	return result
}
