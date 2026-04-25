package progress

// RowHealth is execution-history metadata about one progress.json item.
// Owned by autoloop. The planner READS it to prioritize repairs and MUST
// preserve any unknown fields verbatim across regenerations.
type RowHealth struct {
	AttemptCount        int             `json:"attempt_count,omitempty"`
	ConsecutiveFailures int             `json:"consecutive_failures,omitempty"`
	LastAttempt         string          `json:"last_attempt,omitempty"`
	LastSuccess         string          `json:"last_success,omitempty"`
	LastFailure         *FailureSummary `json:"last_failure,omitempty"`
	BackendsTried       []string        `json:"backends_tried,omitempty"`
	Quarantine          *Quarantine     `json:"quarantine,omitempty"`
}

// FailureSummary is autoloop's classification of a worker outcome.
type FailureSummary struct {
	RunID      string          `json:"run_id"`
	Category   FailureCategory `json:"category"`
	Backend    string          `json:"backend,omitempty"`
	StderrTail string          `json:"stderr_tail,omitempty"`
}

// FailureCategory is the closed set of failure classifications autoloop emits.
type FailureCategory string

const (
	FailureWorkerError      FailureCategory = "worker_error"
	FailureReportValidation FailureCategory = "report_validation_failed"
	FailureProgressSummary  FailureCategory = "progress_summary_failed"
	FailureTimeout          FailureCategory = "timeout"
	FailureBackendDegraded  FailureCategory = "backend_degraded"
)

// Quarantine is set when ConsecutiveFailures crosses QUARANTINE_THRESHOLD.
// Cleared when (a) a future run succeeds on the row, (b) the row's spec hash
// changes (planner reshape detected), or (c) a human deletes the block.
type Quarantine struct {
	Reason       string          `json:"reason"`
	Since        string          `json:"since"`
	AfterRunID   string          `json:"after_run_id"`
	Threshold    int             `json:"threshold"`
	SpecHash     string          `json:"spec_hash"`
	LastCategory FailureCategory `json:"last_category"`
}
