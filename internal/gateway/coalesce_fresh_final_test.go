package gateway

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCoalescerFreshFinal_DisabledThresholdEditsInPlace(t *testing.T) {
	ch := &freshFinalFakeChannel{fakeChannel: newFakeChannel("test")}
	now := time.Date(2026, 4, 27, 1, 0, 0, 0, time.UTC)
	c := newCoalescer(ch, time.Second, "chat1",
		coalescerFreshFinalAfter(0),
		coalescerNow(func() time.Time { return now }),
	)

	c.flushImmediate(context.Background(), "preview")
	now = now.Add(10 * time.Minute)
	c.flushImmediateFinal(context.Background(), "final", true)

	if got := ch.sentSnapshot(); len(got) != 1 {
		t.Fatalf("Send calls = %d, want only initial placeholder send; sent=%#v", len(got), got)
	}
	edits := ch.editsSnapshot()
	if len(edits) != 2 {
		t.Fatalf("EditMessageFinal calls = %d, want preview edit and final edit; edits=%#v", len(edits), edits)
	}
	if edits[1].Text != "final" {
		t.Fatalf("final edit text = %q, want %q", edits[1].Text, "final")
	}
	if got, want := ch.finalizes, []bool{false, true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finalize flags = %v, want %v", got, want)
	}
	if len(ch.deletesSnapshot()) != 0 {
		t.Fatalf("DeleteMessage calls = %v, want none", ch.deletesSnapshot())
	}
}

func TestCoalescerFreshFinal_YoungPreviewEditsInPlace(t *testing.T) {
	ch := &freshFinalFakeChannel{fakeChannel: newFakeChannel("test")}
	now := time.Date(2026, 4, 27, 1, 0, 0, 0, time.UTC)
	c := newCoalescer(ch, time.Second, "chat1",
		coalescerFreshFinalAfter(time.Minute),
		coalescerNow(func() time.Time { return now }),
	)

	c.flushImmediate(context.Background(), "preview")
	now = now.Add(30 * time.Second)
	c.flushImmediateFinal(context.Background(), "final", true)

	if got := ch.sentSnapshot(); len(got) != 1 {
		t.Fatalf("Send calls = %d, want only initial placeholder send; sent=%#v", len(got), got)
	}
	edits := ch.editsSnapshot()
	if len(edits) != 2 {
		t.Fatalf("EditMessageFinal calls = %d, want preview edit and final edit; edits=%#v", len(edits), edits)
	}
	if edits[1].Text != "final" {
		t.Fatalf("final edit text = %q, want %q", edits[1].Text, "final")
	}
	if len(ch.deletesSnapshot()) != 0 {
		t.Fatalf("DeleteMessage calls = %v, want none", ch.deletesSnapshot())
	}
}

func TestCoalescerFreshFinal_OldPreviewSendsFreshAndDeletesOld(t *testing.T) {
	ch := &freshFinalFakeChannel{fakeChannel: newFakeChannel("test")}
	now := time.Date(2026, 4, 27, 1, 0, 0, 0, time.UTC)
	c := newCoalescer(ch, time.Second, "chat1",
		coalescerFreshFinalAfter(time.Minute),
		coalescerNow(func() time.Time { return now }),
	)

	c.flushImmediate(context.Background(), "preview")
	oldID := c.currentMessageID()
	now = now.Add(time.Minute)
	c.flushImmediateFinal(context.Background(), "final", true)

	sent := ch.sentSnapshot()
	if len(sent) != 2 {
		t.Fatalf("Send calls = %d, want placeholder plus fresh final; sent=%#v", len(sent), sent)
	}
	if sent[1].Text != "final" {
		t.Fatalf("fresh final text = %q, want %q", sent[1].Text, "final")
	}
	edits := ch.editsSnapshot()
	if len(edits) != 1 {
		t.Fatalf("EditMessageFinal calls = %d, want only initial preview edit; edits=%#v", len(edits), edits)
	}
	if got, want := c.currentMessageID(), sent[1].MsgID; got != want {
		t.Fatalf("currentMessageID = %q, want fresh id %q", got, want)
	}
	if got, want := ch.deletesSnapshot(), []fakeDelete{{ChatID: "chat1", MsgID: oldID}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DeleteMessage calls = %#v, want %#v", got, want)
	}
}

func TestCoalescerFreshFinal_DeleteUnsupportedStillSucceeds(t *testing.T) {
	ch := newFakeChannel("test")
	now := time.Date(2026, 4, 27, 1, 0, 0, 0, time.UTC)
	c := newCoalescer(ch, time.Second, "chat1",
		coalescerFreshFinalAfter(time.Minute),
		coalescerNow(func() time.Time { return now }),
	)

	c.flushImmediate(context.Background(), "preview")
	now = now.Add(2 * time.Minute)
	c.flushImmediateFinal(context.Background(), "final", true)

	sent := ch.sentSnapshot()
	if len(sent) != 2 {
		t.Fatalf("Send calls = %d, want placeholder plus fresh final; sent=%#v", len(sent), sent)
	}
	if sent[1].Text != "final" {
		t.Fatalf("fresh final text = %q, want %q", sent[1].Text, "final")
	}
	if got, want := c.currentMessageID(), sent[1].MsgID; got != want {
		t.Fatalf("currentMessageID = %q, want fresh id %q", got, want)
	}
}

func TestCoalescerFreshFinal_FreshSendFailureFallsBackToEdit(t *testing.T) {
	ch := &freshFinalFakeChannel{fakeChannel: newFakeChannel("test")}
	now := time.Date(2026, 4, 27, 1, 0, 0, 0, time.UTC)
	c := newCoalescer(ch, time.Second, "chat1",
		coalescerFreshFinalAfter(time.Minute),
		coalescerNow(func() time.Time { return now }),
	)

	c.flushImmediate(context.Background(), "preview")
	oldID := c.currentMessageID()
	ch.failNextSend(errors.New("network"))
	now = now.Add(2 * time.Minute)
	c.flushImmediateFinal(context.Background(), "final", true)

	if got := ch.sentSnapshot(); len(got) != 1 {
		t.Fatalf("successful Send calls = %d, want only initial placeholder send; sent=%#v", len(got), got)
	}
	edits := ch.editsSnapshot()
	if len(edits) != 2 {
		t.Fatalf("EditMessageFinal calls = %d, want preview edit and fallback final edit; edits=%#v", len(edits), edits)
	}
	if edits[1].MsgID != oldID || edits[1].Text != "final" {
		t.Fatalf("fallback edit = %#v, want msgID %q text %q", edits[1], oldID, "final")
	}
	if got := c.currentMessageID(); got != oldID {
		t.Fatalf("currentMessageID = %q, want original id %q", got, oldID)
	}
	if len(ch.deletesSnapshot()) != 0 {
		t.Fatalf("DeleteMessage calls = %v, want none after fresh-send failure", ch.deletesSnapshot())
	}
}

type freshFinalFakeChannel struct {
	*fakeChannel

	finalizes []bool
	deletes   []fakeDelete
}

type fakeDelete struct{ ChatID, MsgID string }

func (f *freshFinalFakeChannel) EditMessageFinal(ctx context.Context, chatID, msgID, text string, finalize bool) error {
	f.finalizes = append(f.finalizes, finalize)
	return f.fakeChannel.EditMessage(ctx, chatID, msgID, text)
}

func (f *freshFinalFakeChannel) DeleteMessage(_ context.Context, chatID, msgID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, fakeDelete{ChatID: chatID, MsgID: msgID})
	return nil
}

func (f *freshFinalFakeChannel) failNextSend(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendErr = err
}

func (f *freshFinalFakeChannel) Send(ctx context.Context, chatID, text string) (string, error) {
	msgID, err := f.fakeChannel.Send(ctx, chatID, text)
	if err != nil {
		f.mu.Lock()
		f.sendErr = nil
		f.mu.Unlock()
	}
	return msgID, err
}

func (f *freshFinalFakeChannel) deletesSnapshot() []fakeDelete {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeDelete, len(f.deletes))
	copy(out, f.deletes)
	return out
}
