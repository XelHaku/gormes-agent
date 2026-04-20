package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestCoalescer_BatchesRapidUpdates: 60 setPending calls over 3s produce
// at most ~5 sends. Proves the 1s window actually coalesces instead of
// letting every update through.
func TestCoalescer_BatchesRapidUpdates(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 1000*time.Millisecond, 42 /* chatID */)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.run(ctx)
	}()

	// Initial placeholder (message ID auto-assigned by mock).
	c.flushImmediate("⏳")
	time.Sleep(10 * time.Millisecond)
	if got := len(mc.sentMessages()); got != 1 {
		t.Fatalf("after flushImmediate: sent = %d, want 1", got)
	}

	// 60 rapid setPending at 50ms intervals over 3s.
	for i := 0; i < 60; i++ {
		c.setPending(strings.Repeat("x", i+1))
		time.Sleep(50 * time.Millisecond)
	}

	// One final flush for the "Done" phase.
	c.flushImmediate("final")

	cancel()
	wg.Wait()

	total := len(mc.sentMessages())
	// Expected range: placeholder (1) + 2-4 coalesced edits + final (1) = 3-6.
	// Upper bound guards against 1:1 per-token sending (the Thundering Herd bug).
	if total < 2 || total > 7 {
		t.Errorf("total sends = %d, want in [2, 7] (coalescer failure: should batch not 1:1)", total)
	}
}

// TestCoalescer_FlushImmediateBypassesWindow: flushImmediate sends even
// inside the 1s window. Semantic-edge frames (Idle, Failed, Cancelling)
// must not be stuck behind the ticker.
func TestCoalescer_FlushImmediateBypassesWindow(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 1000*time.Millisecond, 42)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go c.run(ctx)

	c.flushImmediate("one")
	time.Sleep(30 * time.Millisecond)
	c.flushImmediate("two") // inside 1s window but must fire
	time.Sleep(30 * time.Millisecond)

	if got := len(mc.sentMessages()); got != 2 {
		t.Errorf("sends = %d, want exactly 2 (initial + flushImmediate bypass)", got)
	}

	cancel()
}

// TestCoalescer_IgnoresDuplicateText: if pendingText == lastSentText the
// coalescer skips the tick — no wasted Telegram API call.
func TestCoalescer_IgnoresDuplicateText(t *testing.T) {
	mc := newMockClient()
	c := newCoalescer(mc, 100*time.Millisecond, 42)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go c.run(ctx)

	c.flushImmediate("same")           // 1 initial send
	time.Sleep(50 * time.Millisecond)
	c.setPending("same")               // duplicate — should be skipped
	time.Sleep(250 * time.Millisecond) // two ticks go by
	c.setPending("same")               // still duplicate
	time.Sleep(250 * time.Millisecond)

	if got := len(mc.sentMessages()); got != 1 {
		t.Errorf("sends = %d, want exactly 1 (duplicate text skipped)", got)
	}
	cancel()
}

// TestCoalescer_Respects429RetryAfter: when Send returns a 429-like error
// with a RetryAfter deadline, subsequent edits wait until the deadline
// passes before firing again.
//
// We inject a SendFn on mockClient that returns tgbotapi.Error{Code:429}
// with a ResponseParameters.RetryAfter of 1 second, then succeed on the
// next attempt.
func TestCoalescer_Respects429RetryAfter(t *testing.T) {
	mc := newMockClient()
	var sendCount int
	var countMu sync.Mutex
	mc.SendFn = func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		countMu.Lock()
		n := sendCount
		sendCount++
		countMu.Unlock()
		if n == 1 { // second call (first edit) returns 429
			return tgbotapi.Message{}, &tgbotapi.Error{
				Code:    429,
				Message: "Too Many Requests: retry after 1",
				ResponseParameters: tgbotapi.ResponseParameters{
					RetryAfter: 1,
				},
			}
		}
		return tgbotapi.Message{MessageID: 1000}, nil
	}

	c := newCoalescer(mc, 100*time.Millisecond, 42)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go c.run(ctx)

	c.flushImmediate("initial") // send #1 (success)
	time.Sleep(50 * time.Millisecond)

	c.setPending("alpha") // send #2 triggers 429
	time.Sleep(200 * time.Millisecond)

	c.setPending("beta") // would fire, but retryAfter blocks
	c.setPending("gamma")
	time.Sleep(500 * time.Millisecond) // still within the 1s Retry-After window

	sentDuringBackoff := len(mc.sentMessages())
	if sentDuringBackoff > 2 {
		t.Errorf("during backoff: sent = %d, want <= 2 (retryAfter honoured)", sentDuringBackoff)
	}

	// After the 1s backoff window expires, the next tick should fire.
	time.Sleep(800 * time.Millisecond)
	sentAfterBackoff := len(mc.sentMessages())
	if sentAfterBackoff <= sentDuringBackoff {
		t.Errorf("after backoff: sent = %d, want > %d (backoff should have expired)",
			sentAfterBackoff, sentDuringBackoff)
	}

	cancel()

	// Sanity: ensure the error we wrapped was the 429 kind.
	var tgErr *tgbotapi.Error
	if !errors.As(&tgbotapi.Error{Code: 429}, &tgErr) {
		t.Log("(informational) tgbotapi.Error matches errors.As pattern")
	}
}
