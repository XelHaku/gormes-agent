// Package kernel is the single-owner state machine for Gormes. It owns the
// turn phase, the assistant draft buffer, the conversation history (in
// memory only in Phase 1), and the render snapshot. TUI, hermes, and store
// are edge adapters that communicate with the kernel through bounded mailboxes.
package kernel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// ErrEventMailboxFull is returned by Submit when the platform-event mailbox
// is saturated. The TUI should react by re-enabling input briefly; in
// practice this is rare with a 16-slot buffer.
var ErrEventMailboxFull = errors.New("kernel: event mailbox full")

type Config struct {
	Model             string
	Endpoint          string
	Admission         Admission
	Tools             *tools.Registry // nil → tool_calls are treated as fatal
	MaxToolIterations int             // default 10 when zero
	MaxToolDuration   time.Duration   // default 30s when zero
}

type Kernel struct {
	cfg    Config
	client hermes.Client
	store  store.Store
	tm     telemetry.Telemetry
	log    *slog.Logger

	render chan RenderFrame
	events chan PlatformEvent

	// Atomic — shared-read, kernel-write. Monotonically increasing per process.
	seq atomic.Uint64

	// All fields below this line are OWNED EXCLUSIVELY by the Run goroutine.
	// No other goroutine may read or write them without a channel-based
	// handshake. Violating this invariant is a race.
	phase     Phase
	draft     string
	history   []hermes.Message
	soul      []SoulEntry
	sessionID string
	lastError string
}

func New(cfg Config, c hermes.Client, s store.Store, tm telemetry.Telemetry, log *slog.Logger) *Kernel {
	if log == nil {
		log = slog.Default()
	}
	tm.SetModel(cfg.Model)
	return &Kernel{
		cfg:    cfg,
		client: c,
		store:  s,
		tm:     tm,
		log:    log,
		render: make(chan RenderFrame, RenderMailboxCap),
		events: make(chan PlatformEvent, PlatformEventMailboxCap),
	}
}

// Render returns the receive side of the render mailbox. The channel is
// closed when Run exits.
func (k *Kernel) Render() <-chan RenderFrame { return k.render }

// Submit enqueues a platform event. Returns ErrEventMailboxFull if the
// mailbox is saturated; the caller decides whether to retry or drop.
// Safe to call from any goroutine.
func (k *Kernel) Submit(e PlatformEvent) error {
	select {
	case k.events <- e:
		return nil
	default:
		return ErrEventMailboxFull
	}
}

// ResetSession clears the conversation history, server-assigned session id,
// and last error. Valid only from PhaseIdle or PhaseFailed; returns
// ErrResetDuringTurn if called during an in-flight turn. The Zero-Leak
// Invariant: never truncates streaming; callers must /stop first if they
// want to abandon an active turn.
//
// Implementation: enqueues a PlatformEventResetSession with a synchronous
// ack channel; the Run loop performs the mutation on its own goroutine,
// preserving the single-owner invariant. 500 ms ack timeout.
func (k *Kernel) ResetSession() error {
	ack := make(chan error, 1)
	select {
	case k.events <- PlatformEvent{Kind: PlatformEventResetSession, ack: ack}:
	default:
		return ErrEventMailboxFull
	}
	select {
	case err := <-ack:
		return err
	case <-time.After(500 * time.Millisecond):
		return errors.New("kernel: ResetSession ack timeout")
	}
}

// Run is the kernel loop. MUST be called from exactly one goroutine. Exits
// when ctx is cancelled or a PlatformEventQuit is received. Closes the
// render channel on exit.
func (k *Kernel) Run(ctx context.Context) error {
	defer close(k.render)
	k.emitFrame("idle")
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-k.events:
			switch e.Kind {
			case PlatformEventSubmit:
				if k.phase != PhaseIdle {
					k.lastError = ErrTurnInFlight.Error()
					k.emitFrame("still processing previous turn")
					continue
				}
				k.runTurn(ctx, e.Text)
			case PlatformEventCancel:
				// No active turn; ignore (cancel during a turn is handled
				// inside runTurn's select on k.events).
			case PlatformEventResetSession:
				if k.phase != PhaseIdle && k.phase != PhaseFailed {
					if e.ack != nil {
						e.ack <- ErrResetDuringTurn
					}
					continue
				}
				k.history = nil
				k.sessionID = ""
				k.lastError = ""
				k.phase = PhaseIdle
				k.emitFrame("session reset")
				if e.ack != nil {
					e.ack <- nil
				}
			case PlatformEventQuit:
				return nil
			}
		}
	}
}

// runTurn handles exactly one user turn end-to-end. On entry k.phase must be
// PhaseIdle; on exit it is PhaseIdle (or PhaseFailed on a fatal error).
// All state mutations happen on the calling goroutine, which is the Run
// goroutine — this is part of the single-owner invariant.
func (k *Kernel) runTurn(ctx context.Context, text string) {
	prov := newProvenance(k.cfg.Endpoint)

	// 1. Admission. Reject locally before any HTTP.
	if err := k.cfg.Admission.Validate(text); err != nil {
		k.lastError = err.Error()
		k.emitFrame(err.Error())
		return
	}
	prov.LogAdmitted(k.log)

	// 2. Persist user turn with hard 250ms ack deadline (spec §7.8 store row).
	storeCtx, storeCancel := context.WithTimeout(ctx, StoreAckDeadline)
	payload := []byte(fmt.Sprintf(`{"text":%q}`, text))
	_, err := k.store.Exec(storeCtx, store.Command{Kind: store.AppendUserTurn, Payload: payload})
	storeCancel()
	if err != nil {
		k.phase = PhaseFailed
		k.lastError = fmt.Sprintf("store ack timeout: %v", err)
		k.emitFrame(k.lastError)
		return
	}

	// 3. Update state for the new turn. These mutations are safe because we
	// are on the Run goroutine.
	k.history = append(k.history, hermes.Message{Role: "user", Content: text})
	k.draft = ""
	k.lastError = ""
	k.phase = PhaseConnecting
	k.emitFrame("connecting")
	prov.LogPOSTSent(k.log)

	// 4. Tool loop — wraps the Route-B retry loop. On finish_reason=="tool_calls"
	// we execute the tools in-process and issue a follow-up stream with the
	// tool results appended to the message history. Capped at MaxToolIterations
	// to prevent runaway agent loops.
	request := hermes.ChatRequest{
		Model:     k.cfg.Model,
		SessionID: k.sessionID,
		Stream:    true,
		Messages:  []hermes.Message{{Role: "user", Content: text}},
	}
	if k.cfg.Tools != nil {
		descs := k.cfg.Tools.Descriptors()
		wireDescs := make([]hermes.ToolDescriptor, len(descs))
		for i, d := range descs {
			wireDescs[i] = hermes.ToolDescriptor{Name: d.Name, Description: d.Description, Schema: d.Schema}
		}
		request.Tools = wireDescs
	}
	maxIter := k.cfg.MaxToolIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	var (
		cancelled       bool
		fatalErr        error
		finalDelta      hermes.Event
		gotFinal        bool
		latestSessionID string
		toolIteration   = 0
	)

	start := time.Now()
	k.tm.StartTurn()

toolLoop:
	for {
		// Fresh retry budget each tool iteration — reconnect retries are for
		// network drops, not for multi-round agent reasoning.
		retryBudget := NewRetryBudget()
		var replaceOnNextToken bool

	retryLoop:
		for {
			runCtx, cancelRun := context.WithCancel(ctx)

			stream, err := k.client.OpenStream(runCtx, request)
			if err != nil {
				cancelRun()
				if hermes.Classify(err) == hermes.ClassRetryable && !retryBudget.Exhausted() {
					k.phase = PhaseReconnecting
					k.lastError = "reconnecting: " + err.Error()
					k.emitFrame("reconnecting")
					delay := retryBudget.NextDelay()
					if werr := Wait(ctx, delay); werr != nil {
						cancelled = true
						break toolLoop
					}
					replaceOnNextToken = true
					continue retryLoop
				}
				prov.ErrorClass = hermes.Classify(err).String()
				prov.ErrorText = err.Error()
				prov.LogError(k.log)
				k.phase = PhaseFailed
				k.lastError = err.Error()
				k.emitFrame("open stream failed")
				return
			}

			k.phase = PhaseStreaming
			k.emitFrame("streaming")

			outcome := k.streamInner(ctx, runCtx, cancelRun, stream, &finalDelta, &gotFinal, &fatalErr, &cancelled, &replaceOnNextToken)
			_ = stream.Close()
			if sid := stream.SessionID(); sid != "" {
				latestSessionID = sid
			}
			cancelRun()

			switch outcome {
			case streamOutcomeDone:
				break retryLoop
			case streamOutcomeCancelled:
				break toolLoop
			case streamOutcomeFatal:
				break toolLoop
			case streamOutcomeRetryable:
				if retryBudget.Exhausted() {
					k.phase = PhaseFailed
					k.lastError = "reconnect budget exhausted"
					k.emitFrame("reconnect budget exhausted")
					return
				}
				k.phase = PhaseReconnecting
				k.emitFrame("reconnecting")
				delay := retryBudget.NextDelay()
				if werr := Wait(ctx, delay); werr != nil {
					cancelled = true
					break toolLoop
				}
				replaceOnNextToken = true
				continue retryLoop
			}
		}

		// retryLoop exited cleanly (EventDone received). Inspect finish_reason.
		if !gotFinal {
			fatalErr = fmt.Errorf("stream closed without finish_reason")
			break toolLoop
		}

		if finalDelta.FinishReason != "tool_calls" {
			// Normal end of turn. Exit the tool loop to finalise.
			break toolLoop
		}

		// tool_calls round. Execute tools and append results to the request.
		toolIteration++
		if toolIteration > maxIter {
			k.phase = PhaseFailed
			k.lastError = fmt.Sprintf("tool iteration limit exceeded (%d)", maxIter)
			k.emitFrame(k.lastError)
			return
		}

		runCtx, cancelRun := context.WithCancel(ctx)
		results := k.executeToolCalls(runCtx, finalDelta.ToolCalls)
		cancelRun()

		// Append the assistant's tool-requesting message plus one tool-result
		// message per call. The draft so far is captured in the assistant
		// message.
		assistantMsg := hermes.Message{
			Role:      "assistant",
			Content:   k.draft,
			ToolCalls: finalDelta.ToolCalls,
		}
		request.Messages = append(request.Messages, assistantMsg)
		for _, r := range results {
			request.Messages = append(request.Messages, hermes.Message{
				Role:       "tool",
				ToolCallID: r.ID,
				Name:       r.Name,
				Content:    r.Content,
			})
		}

		// Clear draft between tool iterations — the next LLM response is a
		// fresh continuation; the assistant message we appended captures
		// what we had so far.
		k.draft = ""
		gotFinal = false
		finalDelta = hermes.Event{}
		k.emitFrame("executing tools")
	}

	// 5. Finalisation (unchanged shape from Route-B).
	latency := time.Since(start)
	k.tm.FinishTurn(latency)
	prov.LatencyMs = int(latency / time.Millisecond)

	if fatalErr != nil {
		prov.ErrorClass = hermes.Classify(fatalErr).String()
		prov.ErrorText = fatalErr.Error()
		prov.LogError(k.log)
		k.phase = PhaseFailed
		k.lastError = fatalErr.Error()
		k.emitFrame("stream error")
		return
	}

	if gotFinal {
		prov.FinishReason = finalDelta.FinishReason
		prov.TokensIn = finalDelta.TokensIn
		prov.TokensOut = finalDelta.TokensOut
		if finalDelta.TokensIn > 0 {
			k.tm.SetTokensIn(finalDelta.TokensIn)
		}
	}

	if latestSessionID != "" {
		k.sessionID = latestSessionID
		prov.ServerSessionID = latestSessionID
		prov.LogSSEStart(k.log)
	}

	if cancelled {
		k.phase = PhaseCancelling
		k.emitFrame("cancelled")
	} else if k.draft != "" {
		k.history = append(k.history, hermes.Message{Role: "assistant", Content: k.draft})
	}

	prov.LogDone(k.log)
	k.phase = PhaseIdle
	k.emitFrame("idle")
}

type streamOutcome int

const (
	streamOutcomeDone streamOutcome = iota
	streamOutcomeCancelled
	streamOutcomeRetryable
	streamOutcomeFatal
)

// streamInner runs one stream attempt. Pumps events from hermes.Stream.Recv
// into a bounded channel, multiplexes over the kernel's platform events and
// a 16ms flush ticker, and returns a classified outcome so the retry-loop
// caller knows what to do next.
//
// The outer ctx (from runTurn) is used for ambient cancellation checks.
// The runCtx (per-attempt) is what the pump goroutine uses for Recv; when
// this stream ends (normal, cancel, or retryable error), runCtx is cancelled
// by the caller.
func (k *Kernel) streamInner(
	ctx, runCtx context.Context,
	cancelRun context.CancelFunc,
	stream hermes.Stream,
	finalDelta *hermes.Event,
	gotFinal *bool,
	fatalErr *error,
	cancelled *bool,
	replaceOnNextToken *bool,
) streamOutcome {
	type streamResult struct {
		event hermes.Event
		err   error
	}
	deltaCh := make(chan streamResult, 8)
	go func() {
		defer close(deltaCh)
		for {
			ev, err := stream.Recv(runCtx)
			select {
			case deltaCh <- streamResult{event: ev, err: err}:
			case <-runCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	var (
		dirty   bool
		outcome streamOutcome
	)
	outcome = streamOutcomeFatal // default if something truly unexpected happens

streamLoop:
	for {
		select {
		case <-ctx.Done():
			*cancelled = true
			cancelRun()
			outcome = streamOutcomeCancelled
			break streamLoop

		case e := <-k.events:
			switch e.Kind {
			case PlatformEventCancel:
				*cancelled = true
				cancelRun()
				outcome = streamOutcomeCancelled
				break streamLoop
			case PlatformEventSubmit:
				k.lastError = ErrTurnInFlight.Error()
				k.emitFrame("still processing previous turn")
			case PlatformEventResetSession:
				// Zero-Leak Invariant: never truncate an active turn. Reject
				// the reset without mutating state; the caller must /stop
				// first if they want to abandon this stream.
				if e.ack != nil {
					e.ack <- ErrResetDuringTurn
				}
			case PlatformEventQuit:
				*cancelled = true
				cancelRun()
				outcome = streamOutcomeCancelled
				break streamLoop
			}

		case res, ok := <-deltaCh:
			if !ok {
				// Pump exited on its own — treat as retryable (unexpected EOF).
				// Only treat as Done if EventDone was already consumed (*gotFinal).
				if *gotFinal {
					outcome = streamOutcomeDone
				} else {
					outcome = streamOutcomeRetryable
				}
				break streamLoop
			}
			if res.err != nil {
				if res.err == io.EOF {
					if *gotFinal {
						outcome = streamOutcomeDone
					} else {
						// Stream ended without EventDone — treat as retryable.
						outcome = streamOutcomeRetryable
					}
					break streamLoop
				}
				if runCtx.Err() != nil {
					*cancelled = true
					outcome = streamOutcomeCancelled
					break streamLoop
				}
				// Classify the error: Retryable → caller retries; otherwise fatal.
				if hermes.Classify(res.err) == hermes.ClassRetryable {
					outcome = streamOutcomeRetryable
				} else {
					*fatalErr = res.err
					outcome = streamOutcomeFatal
				}
				break streamLoop
			}
			ev := res.event
			switch ev.Kind {
			case hermes.EventToken:
				if *replaceOnNextToken {
					k.draft = ""
					*replaceOnNextToken = false
				}
				k.draft += ev.Token
				k.tm.Tick(ev.TokensOut)
				dirty = true
			case hermes.EventReasoning:
				if *replaceOnNextToken {
					// Reasoning doesn't count as visible content; the NEXT EventToken
					// still clears the draft. Do NOT flip replaceOnNextToken here.
				}
				k.addSoul("reasoning: " + truncate(ev.Reasoning, 60))
				dirty = true
			case hermes.EventDone:
				*finalDelta = ev
				*gotFinal = true
				outcome = streamOutcomeDone
				break streamLoop
			}

		case <-ticker.C:
			if dirty {
				k.emitFrame("streaming")
				dirty = false
			}
		}
	}

	// Drain deltaCh so the pump goroutine exits before we return.
	cancelRun()
	for range deltaCh {
	}
	return outcome
}

// addSoul appends a Soul Monitor entry with a ring-buffer cap.
func (k *Kernel) addSoul(text string) {
	k.soul = append(k.soul, SoulEntry{At: time.Now(), Text: text})
	if len(k.soul) > SoulBufferSize {
		k.soul = k.soul[len(k.soul)-SoulBufferSize:]
	}
}

// emitFrame builds a RenderFrame snapshot and publishes it to the render
// mailbox with replace-latest semantics: if an unread frame already sits
// in the capacity-1 buffer, drain it and drop it before enqueueing the new
// one. This is what keeps a slow TUI from backpressuring the kernel.
func (k *Kernel) emitFrame(status string) {
	frame := RenderFrame{
		Seq:        k.seq.Add(1),
		Phase:      k.phase,
		DraftText:  k.draft,
		History:    append([]hermes.Message(nil), k.history...),
		Telemetry:  k.tm.Snapshot(),
		StatusText: status,
		SessionID:  k.sessionID,
		Model:      k.cfg.Model,
		LastError:  k.lastError,
		SoulEvents: append([]SoulEntry(nil), k.soul...),
	}
	// Drain old frame if present, then enqueue new.
	select {
	case <-k.render:
	default:
	}
	select {
	case k.render <- frame:
	default:
		// Should be unreachable after the drain above.
	}
}

// truncate returns s clamped to n runes with an ellipsis suffix. Safe on
// non-ASCII input.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
