package gateway

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCoalescer_PlaceholderThenEdit(t *testing.T) {
	ch := newFakeChannel("test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newCoalescer(ch, 20*time.Millisecond, "chat1")
	go c.run(ctx)

	c.setPending("first")
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(ch.sentSnapshot()) == 1
	})

	time.Sleep(25 * time.Millisecond)
	c.setPending("second")
	waitFor(t, 200*time.Millisecond, func() bool {
		edits := ch.editsSnapshot()
		return len(edits) == 1 && edits[0].Text == "second"
	})
}

func TestCoalescer_FlushImmediateBypassesWindow(t *testing.T) {
	ch := newFakeChannel("test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newCoalescer(ch, 5*time.Second, "chat1")
	go c.run(ctx)

	c.flushImmediate(ctx, "final")
	waitFor(t, 200*time.Millisecond, func() bool {
		edits := ch.editsSnapshot()
		return len(edits) == 1 && edits[0].Text == "final"
	})
}

func TestCoalescer_FlushImmediateFinalPassesTerminalFlag(t *testing.T) {
	ch := &finalizingFakeChannel{fakeChannel: newFakeChannel("test")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newCoalescer(ch, time.Second, "chat1")

	c.flushImmediate(ctx, "partial")
	c.flushImmediateFinal(ctx, "final", true)

	if got, want := ch.finalizes, []bool{false, true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finalize flags = %v, want %v", got, want)
	}
}

func TestCoalescer_SendErrorIsSwallowed(t *testing.T) {
	ch := newFakeChannel("test")
	ch.sendErr = errors.New("transient")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newCoalescer(ch, 10*time.Millisecond, "chat1")
	go c.run(ctx)

	c.setPending("x")
	time.Sleep(50 * time.Millisecond)
}

type finalizingFakeChannel struct {
	*fakeChannel
	finalizes []bool
}

func (f *finalizingFakeChannel) EditMessageFinal(ctx context.Context, chatID, msgID, text string, finalize bool) error {
	f.finalizes = append(f.finalizes, finalize)
	return f.fakeChannel.EditMessage(ctx, chatID, msgID, text)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
