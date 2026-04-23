package threadtext

import "sync"

// ReplyTracker remembers the latest reply target per chat for threaded-text
// adapters that can use native thread metadata on outbound sends.
type ReplyTracker struct {
	mu      sync.RWMutex
	targets map[string]ReplyTarget
}

func NewReplyTracker() *ReplyTracker {
	return &ReplyTracker{targets: map[string]ReplyTarget{}}
}

func (t *ReplyTracker) Record(msg InboundMessage, mode ReplyMode) {
	if t == nil {
		return
	}
	target, ok := ResolveReplyTarget(msg, mode)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets[target.ChatID] = target
}

func (t *ReplyTracker) Lookup(chatID string) (ReplyTarget, bool) {
	if t == nil {
		return ReplyTarget{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.targets[trim(chatID)]
	return target, ok
}
