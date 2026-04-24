package gateway

import "context"

// TakeoverMarker records the inbound event that triggered the most recent
// gateway restart so a redelivered /restart update (e.g. a Telegram update that
// reappears post-restart because the old process never acked it) cannot loop a
// fresh process.
type TakeoverMarker struct {
	Platform string
	ChatID   string
	MsgID    string
}

// RestartMarkerStore persists the takeover marker across process boundaries.
// Implementations must tolerate a missing marker on first run by returning
// (zero, false, nil) from LoadTakeoverMarker.
type RestartMarkerStore interface {
	LoadTakeoverMarker(ctx context.Context) (TakeoverMarker, bool, error)
	SaveTakeoverMarker(ctx context.Context, m TakeoverMarker) error
}

// RestartFunc is the service-manager-facing hook the gateway invokes after the
// active turn has drained. Implementations typically log, call sync/flush, and
// then exit with a code the service manager treats as "restart me" (or swap
// execv in, etc.). The manager intentionally stays agnostic about exit codes.
type RestartFunc func(ctx context.Context, ev InboundEvent) error

const restartUnsupportedNotice = "/restart is unsupported for this gateway build."

const restartNotice = "Restarting Gormes — you can send your next message once I come back up."

// markerMatches reports whether the stored takeover marker already captured
// the inbound event, i.e. this delivery is a replay of the update that caused
// the restart we just came back from.
func markerMatches(stored TakeoverMarker, ev InboundEvent) bool {
	if stored.Platform == "" && stored.ChatID == "" && stored.MsgID == "" {
		return false
	}
	return stored.Platform == ev.Platform &&
		stored.ChatID == ev.ChatID &&
		stored.MsgID == ev.MsgID
}
