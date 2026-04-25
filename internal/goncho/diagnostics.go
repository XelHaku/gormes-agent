package goncho

import (
	"context"
	"database/sql"
	"errors"
)

// QueueTaskTypes are the only Honcho-style reasoning work units that Goncho
// reports. Delivery, deletion, and vector reconciliation counters are
// deliberately excluded because queue status is observability, not sync.
var QueueTaskTypes = []string{"representation", "summary", "dream"}

// QueueWorkUnitStatus mirrors Honcho's queue status count shape.
type QueueWorkUnitStatus struct {
	CompletedWorkUnits  int `json:"completed_work_units"`
	InProgressWorkUnits int `json:"in_progress_work_units"`
	PendingWorkUnits    int `json:"pending_work_units"`
	TotalWorkUnits      int `json:"total_work_units"`
}

// QueueStatus is the local Goncho queue status read model. Until a dedicated
// Goncho task queue exists, it reports deterministic zero-state counts with
// degraded evidence.
type QueueStatus struct {
	Status            string                         `json:"status"`
	ObservabilityOnly bool                           `json:"observability_only"`
	WorkUnits         map[string]QueueWorkUnitStatus `json:"work_units"`
	Degraded          bool                           `json:"degraded"`
	Message           string                         `json:"message"`
}

// ReadQueueStatus currently returns a deterministic zero-state read model. It
// never waits for the queue to drain.
func ReadQueueStatus(ctx context.Context, db *sql.DB) (QueueStatus, error) {
	if db == nil {
		return QueueStatus{}, errors.New("goncho: nil db")
	}
	if err := ctx.Err(); err != nil {
		return QueueStatus{}, err
	}
	return ZeroQueueStatus(), nil
}

// ZeroQueueStatus reports that no dedicated Goncho task queue exists yet while
// preserving Honcho-compatible work-unit fields.
func ZeroQueueStatus() QueueStatus {
	workUnits := make(map[string]QueueWorkUnitStatus, len(QueueTaskTypes))
	for _, taskType := range QueueTaskTypes {
		workUnits[taskType] = QueueWorkUnitStatus{}
	}
	return QueueStatus{
		Status:            "degraded",
		ObservabilityOnly: true,
		WorkUnits:         workUnits,
		Degraded:          true,
		Message:           "no dedicated Goncho task queue exists yet; zero tracked work units",
	}
}
