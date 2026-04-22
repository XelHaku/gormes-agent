package gateway

import (
	"context"
	"sync"
)

type HookPoint string

const (
	HookBeforeReceive HookPoint = "before_receive"
	HookAfterReceive  HookPoint = "after_receive"
	HookBeforeSend    HookPoint = "before_send"
	HookAfterSend     HookPoint = "after_send"
	HookOnError       HookPoint = "on_error"
)

// HookEvent is the normalized event payload emitted around gateway manager
// receive/send/error boundaries.
type HookEvent struct {
	Point    HookPoint
	Platform string
	ChatID   string
	MsgID    string
	Kind     EventKind
	Text     string
	Inbound  *InboundEvent
	Err      error
}

type HookFunc func(context.Context, HookEvent)

// Hooks is a small in-process hook registry for gateway lifecycle events.
type Hooks struct {
	mu       sync.RWMutex
	handlers map[HookPoint][]HookFunc
}

func NewHooks() *Hooks {
	return &Hooks{handlers: make(map[HookPoint][]HookFunc)}
}

func (h *Hooks) Add(point HookPoint, fn HookFunc) {
	if h == nil || fn == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[point] = append(h.handlers[point], fn)
}

func (h *Hooks) Fire(ctx context.Context, ev HookEvent) {
	if h == nil {
		return
	}
	h.mu.RLock()
	handlers := append([]HookFunc(nil), h.handlers[ev.Point]...)
	h.mu.RUnlock()
	for _, fn := range handlers {
		fn(ctx, ev)
	}
}
