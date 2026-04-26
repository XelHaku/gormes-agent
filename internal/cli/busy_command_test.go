package cli

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// TestBusyCommandGuardIdleByDefault asserts a freshly constructed guard does
// not report a busy command. This is the baseline before any /compress or
// other long-running CLI handler enters.
func TestBusyCommandGuardIdleByDefault(t *testing.T) {
	g := NewBusyCommandGuard()
	if g.IsBusy() {
		t.Errorf("new guard IsBusy() = true, want false")
	}
	if got := g.Status(); got != "" {
		t.Errorf("new guard Status() = %q, want empty string", got)
	}
	if got := g.ActiveCommand(); got != "" {
		t.Errorf("new guard ActiveCommand() = %q, want empty string", got)
	}
}

// TestBusyCommandGuardRunSetsBusyDuringFn is the core invariant: while the
// long-running fn executes, the guard reports the busy state and surfaces the
// operator-facing status label so the CLI/TUI can render it.
func TestBusyCommandGuardRunSetsBusyDuringFn(t *testing.T) {
	g := NewBusyCommandGuard()
	var sawBusy bool
	var sawStatus string
	var sawName string
	err := g.Run("compress", "Compressing context...", func() error {
		sawBusy = g.IsBusy()
		sawStatus = g.Status()
		sawName = g.ActiveCommand()
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !sawBusy {
		t.Errorf("IsBusy() during fn = false, want true")
	}
	if sawStatus != "Compressing context..." {
		t.Errorf("Status() during fn = %q, want %q", sawStatus, "Compressing context...")
	}
	if sawName != "compress" {
		t.Errorf("ActiveCommand() during fn = %q, want %q", sawName, "compress")
	}
}

// TestBusyCommandGuardRunClearsBusyOnSuccess asserts the busy flag is cleared
// after a successful command exit so the next user input is accepted.
func TestBusyCommandGuardRunClearsBusyOnSuccess(t *testing.T) {
	g := NewBusyCommandGuard()
	if err := g.Run("compress", "Compressing context...", func() error {
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if g.IsBusy() {
		t.Errorf("IsBusy() after successful Run = true, want false")
	}
	if got := g.Status(); got != "" {
		t.Errorf("Status() after successful Run = %q, want empty", got)
	}
	if got := g.ActiveCommand(); got != "" {
		t.Errorf("ActiveCommand() after successful Run = %q, want empty", got)
	}
}

// TestBusyCommandGuardRunClearsBusyOnError covers the "every exit path" half
// of the contract. An error from the wrapped function MUST still clear the
// busy flag — otherwise a transient compression failure would lock out user
// input until the process exits.
func TestBusyCommandGuardRunClearsBusyOnError(t *testing.T) {
	g := NewBusyCommandGuard()
	sentinel := errors.New("compression failed")
	err := g.Run("compress", "Compressing context...", func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run propagated err = %v, want %v", err, sentinel)
	}
	if g.IsBusy() {
		t.Errorf("IsBusy() after errored Run = true, want false")
	}
	if got := g.Status(); got != "" {
		t.Errorf("Status() after errored Run = %q, want empty", got)
	}
}

// TestBusyCommandGuardRunClearsBusyOnPanic is the third exit path. A panicking
// command handler must not leave the guard stuck busy.
func TestBusyCommandGuardRunClearsBusyOnPanic(t *testing.T) {
	g := NewBusyCommandGuard()
	defer func() {
		_ = recover()
		if g.IsBusy() {
			t.Errorf("IsBusy() after panicking Run = true, want false")
		}
		if got := g.Status(); got != "" {
			t.Errorf("Status() after panicking Run = %q, want empty", got)
		}
	}()
	_ = g.Run("compress", "Compressing context...", func() error {
		panic("boom")
	})
}

// TestBusyCommandGuardRunRejectsNestedRun asserts the guard is not re-entrant
// — a nested attempt while one command is already running returns
// ErrBusyCommandActive instead of corrupting state. This is what protects
// `/compress` from itself if a handler accidentally re-enters.
func TestBusyCommandGuardRunRejectsNestedRun(t *testing.T) {
	g := NewBusyCommandGuard()
	var nestedErr error
	err := g.Run("compress", "Compressing context...", func() error {
		nestedErr = g.Run("skills", "Loading skills...", func() error {
			return nil
		})
		return nil
	})
	if err != nil {
		t.Fatalf("outer Run returned %v, want nil", err)
	}
	if !errors.Is(nestedErr, ErrBusyCommandActive) {
		t.Errorf("nested Run returned %v, want ErrBusyCommandActive", nestedErr)
	}
}

// TestBusyCommandGuardEvaluateInputIdleAcceptsAnything asserts that, while
// idle, the guard never rejects input. Acceptance/rejection of slash text is
// the slash dispatcher's job; the guard must not interfere when no long-running
// command is active.
func TestBusyCommandGuardEvaluateInputIdleAcceptsAnything(t *testing.T) {
	g := NewBusyCommandGuard()
	cases := []string{"hello", "/help", "/no-such-command", "", "   ", "/new"}
	for _, in := range cases {
		v := g.EvaluateInput(in)
		if v.Rejected {
			t.Errorf("EvaluateInput(%q) Rejected = true while idle, want false (evidence=%q)", in, v.Evidence)
		}
	}
}

// TestBusyCommandGuardEvaluateInputBusyRejectsPlainText covers the second
// acceptance bullet — overlapping user prompt text during a busy command is
// rejected with visible busy evidence rather than entering the kernel.
func TestBusyCommandGuardEvaluateInputBusyRejectsPlainText(t *testing.T) {
	g := NewBusyCommandGuard()
	var verdict BusyInputVerdict
	if err := g.Run("compress", "Compressing context...", func() error {
		verdict = g.EvaluateInput("hello there")
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !verdict.Rejected {
		t.Fatalf("EvaluateInput plain text while busy Rejected = false, want true")
	}
	if verdict.Evidence == "" {
		t.Error("EvaluateInput while busy Evidence empty, want explanation")
	}
	if !strings.Contains(strings.ToLower(verdict.Evidence), "busy") {
		t.Errorf("EvaluateInput while busy Evidence = %q, want to mention busy", verdict.Evidence)
	}
}

// TestBusyCommandGuardEvaluateInputBusyAllowsBypassCommand covers the third
// acceptance bullet — bypass commands declared in the CLI command registry
// (/help, /stop, /restart) must still pass through while a long-running CLI
// command is active so the operator can stop it.
func TestBusyCommandGuardEvaluateInputBusyAllowsBypassCommand(t *testing.T) {
	g := NewBusyCommandGuard()
	bypassNames := []string{"/help", "/stop", "/restart", "  /STOP  "}
	for _, name := range bypassNames {
		var verdict BusyInputVerdict
		if err := g.Run("compress", "Compressing context...", func() error {
			verdict = g.EvaluateInput(name)
			return nil
		}); err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
		if verdict.Rejected {
			t.Errorf("EvaluateInput(%q) Rejected = true during busy, want false (bypass)", name)
		}
	}
}

// TestBusyCommandGuardEvaluateInputBusyRejectsNonBypassSlash asserts that a
// recognized non-bypass slash command (e.g. /new with busy_reject policy) is
// still rejected by the busy-command guard, with evidence that mentions busy
// state. The guard does not need to duplicate the upstream command-registry
// verdict — overlapping input is rejected uniformly.
func TestBusyCommandGuardEvaluateInputBusyRejectsNonBypassSlash(t *testing.T) {
	g := NewBusyCommandGuard()
	var verdict BusyInputVerdict
	if err := g.Run("compress", "Compressing context...", func() error {
		verdict = g.EvaluateInput("/new")
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !verdict.Rejected {
		t.Fatalf("EvaluateInput(/new) during busy Rejected = false, want true")
	}
	if !strings.Contains(strings.ToLower(verdict.Evidence), "busy") {
		t.Errorf("EvaluateInput(/new) during busy Evidence = %q, want to mention busy", verdict.Evidence)
	}
}

// TestBusyCommandGuardEvaluateInputBusyRejectsUnknownSlash asserts that an
// unknown slash command also gets the busy notice, not slash-leak text. The
// guard's contract is: while busy, only known bypass commands proceed.
func TestBusyCommandGuardEvaluateInputBusyRejectsUnknownSlash(t *testing.T) {
	g := NewBusyCommandGuard()
	var verdict BusyInputVerdict
	if err := g.Run("compress", "Compressing context...", func() error {
		verdict = g.EvaluateInput("/no-such-command-xyzzy")
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !verdict.Rejected {
		t.Errorf("EvaluateInput(unknown slash) during busy Rejected = false, want true")
	}
}

// TestBusyCommandGuardEvaluateInputBusyMentionsStatus asserts the rejection
// evidence includes the status label so the operator knows what is running
// (e.g. "Compressing context..."), not just a generic busy message.
func TestBusyCommandGuardEvaluateInputBusyMentionsStatus(t *testing.T) {
	g := NewBusyCommandGuard()
	var verdict BusyInputVerdict
	if err := g.Run("compress", "Compressing context...", func() error {
		verdict = g.EvaluateInput("hello")
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if !strings.Contains(verdict.Evidence, "Compressing context") {
		t.Errorf("Evidence = %q, want to include status label %q", verdict.Evidence, "Compressing context...")
	}
}

// TestBusyCommandGuardConcurrentEvaluateInput asserts the guard is safe to
// query from a separate goroutine (the input goroutine) while a long-running
// command is running on its own goroutine. Overlapping user input must be
// rejected during the entire busy window.
func TestBusyCommandGuardConcurrentEvaluateInput(t *testing.T) {
	g := NewBusyCommandGuard()
	started := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- g.Run("compress", "Compressing context...", func() error {
			close(started)
			<-finish
			return nil
		})
	}()
	<-started

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := g.EvaluateInput("hello"); !v.Rejected {
				t.Errorf("concurrent EvaluateInput Rejected = false, want true while busy")
			}
		}()
	}
	wg.Wait()
	close(finish)
	if err := <-done; err != nil {
		t.Errorf("Run returned %v, want nil", err)
	}
	if g.IsBusy() {
		t.Errorf("guard IsBusy after Run finished = true, want false")
	}
}

// TestBusyCommandGuardActiveCommandReportsName asserts the registry-aligned
// command name is reported while busy and cleared on exit. This is the link
// between the guard and the CLI command registry — the same identifier used to
// classify ActiveTurnPolicy.
func TestBusyCommandGuardActiveCommandReportsName(t *testing.T) {
	g := NewBusyCommandGuard()
	var name string
	if err := g.Run("compress", "Compressing context...", func() error {
		name = g.ActiveCommand()
		return nil
	}); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	if name != "compress" {
		t.Errorf("ActiveCommand during busy = %q, want %q", name, "compress")
	}
	if g.ActiveCommand() != "" {
		t.Errorf("ActiveCommand after busy = %q, want empty", g.ActiveCommand())
	}
}

// TestBusyCommandGuardRunRejectsEmptyName asserts callers cannot start a busy
// window without naming the command. An empty name would render an unhelpful
// status to the operator and break parity with the CLI command registry.
func TestBusyCommandGuardRunRejectsEmptyName(t *testing.T) {
	g := NewBusyCommandGuard()
	err := g.Run("", "Compressing context...", func() error {
		t.Error("fn should not run when name is empty")
		return nil
	})
	if !errors.Is(err, ErrBusyCommandInvalid) {
		t.Errorf("Run with empty name returned %v, want ErrBusyCommandInvalid", err)
	}
	if g.IsBusy() {
		t.Errorf("IsBusy() after rejected Run = true, want false")
	}
}
