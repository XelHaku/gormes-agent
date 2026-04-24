package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ToolEvent is one tool invocation observed inside the turn that produced a
// worth-learning signal. The trace is deterministic input to the extractor
// prompt so operators can replay exactly what the LLM saw.
type ToolEvent struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
}

// Source bundles the Phase 6.A signal and the turn trace that feeds the
// skill extractor. All string fields are preserved as-is; the extractor
// never mutates them so repeated runs distil the same input identically.
type Source struct {
	Signal           Signal
	SessionID        string
	UserMessage      string
	AssistantMessage string
	ToolEvents       []ToolEvent
}

// DistillResponse is the minimal structured reply the extractor requires
// from the LLM seam. Name, Description, and Body mirror the SKILL.md
// triple that Phase 6.C will serialise.
type DistillResponse struct {
	Name        string
	Description string
	Body        string
}

// LLM is the narrow seam the extractor needs. Production wiring plugs a
// provider-backed client behind it; tests plug a deterministic fake so the
// extractor stays auditable.
type LLM interface {
	Distill(ctx context.Context, prompt string) (DistillResponse, error)
}

// Candidate is the distilled skill proposal the extractor appends as JSONL
// for downstream promotion. It carries both the distilled artefact and the
// learning signal metadata so 6.C can decide what to persist as SKILL.md.
type Candidate struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Body        string    `json:"body"`
	SessionID   string    `json:"session_id,omitempty"`
	ToolNames   []string  `json:"tool_names,omitempty"`
	Score       int       `json:"score"`
	Threshold   int       `json:"threshold"`
	Reasons     []string  `json:"reasons,omitempty"`
	DistilledAt time.Time `json:"distilled_at"`
}

// Extractor orchestrates LLM-assisted pattern distillation. It gates on
// the 6.A worth-learning signal, builds a deterministic prompt from the
// source trace, invokes the LLM seam, validates the reply, and appends
// one JSONL line per accepted candidate.
type Extractor struct {
	llm  LLM
	path string
	mu   sync.Mutex
	now  func() time.Time
}

// NewExtractor constructs an Extractor that logs accepted candidates to
// path. An empty path makes acceptance a no-op on disk; the in-memory
// candidate is still returned so callers can hand it to downstream storage.
func NewExtractor(llm LLM, path string) *Extractor {
	return &Extractor{
		llm:  llm,
		path: strings.TrimSpace(path),
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// SetClock overrides the time source used for Candidate.DistilledAt. It
// exists so tests can pin timestamps; production callers leave it alone.
func (e *Extractor) SetClock(clock func() time.Time) {
	if e == nil || clock == nil {
		return
	}
	e.now = clock
}

// Extract distils one skill candidate from the source when the signal
// crossed the learning threshold. It returns (zero, false, nil) when the
// signal is skippable, and (candidate, true, nil) once the LLM reply has
// been validated and appended. A nil receiver is a defensive no-op so
// callers can wire the extractor unconditionally.
func (e *Extractor) Extract(ctx context.Context, src Source) (Candidate, bool, error) {
	if e == nil {
		return Candidate{}, false, nil
	}
	if !src.Signal.WorthLearning {
		return Candidate{}, false, nil
	}
	if e.llm == nil {
		return Candidate{}, false, errors.New("learning: extractor requires a non-nil LLM seam")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return Candidate{}, false, ctx.Err()
	default:
	}

	prompt := buildDistillPrompt(src)
	resp, err := e.llm.Distill(ctx, prompt)
	if err != nil {
		return Candidate{}, false, fmt.Errorf("learning: distill: %w", err)
	}
	if err := validateDistillResponse(resp); err != nil {
		return Candidate{}, false, err
	}

	cand := Candidate{
		Name:        strings.TrimSpace(resp.Name),
		Description: strings.TrimSpace(resp.Description),
		Body:        strings.TrimSpace(resp.Body),
		SessionID:   strings.TrimSpace(src.SessionID),
		ToolNames:   cloneStrings(src.Signal.ToolNames),
		Score:       src.Signal.Score,
		Threshold:   src.Signal.Threshold,
		Reasons:     cloneStrings(src.Signal.Reasons),
		DistilledAt: e.stamp(),
	}

	if err := e.appendCandidate(cand); err != nil {
		return Candidate{}, false, err
	}
	return cand, true, nil
}

func (e *Extractor) appendCandidate(cand Candidate) error {
	if e.path == "" {
		return nil
	}
	raw, err := json.Marshal(cand)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

// buildDistillPrompt renders a deterministic prompt from the source trace.
// The layout is stable so a fake LLM in tests can assert required fragments
// and operators can replay historical distillations byte-for-byte.
func buildDistillPrompt(src Source) string {
	var b strings.Builder
	b.WriteString("You are distilling a reusable skill from one successful agent turn.\n")
	b.WriteString("Return a concise SKILL.md-ready triple: Name, Description, Body.\n\n")

	b.WriteString("Session: ")
	b.WriteString(strings.TrimSpace(src.SessionID))
	b.WriteString("\n")
	if len(src.Signal.Reasons) > 0 {
		b.WriteString("Signal reasons: ")
		b.WriteString(strings.Join(src.Signal.Reasons, ", "))
		b.WriteString("\n")
	}
	if len(src.Signal.ToolNames) > 0 {
		b.WriteString("Tools used: ")
		b.WriteString(strings.Join(src.Signal.ToolNames, ", "))
		b.WriteString("\n")
	}
	b.WriteString("\nUser message:\n")
	b.WriteString(strings.TrimSpace(src.UserMessage))
	b.WriteString("\n\nAssistant message:\n")
	b.WriteString(strings.TrimSpace(src.AssistantMessage))

	if len(src.ToolEvents) > 0 {
		b.WriteString("\n\nTool trace:\n")
		for i, ev := range src.ToolEvents {
			fmt.Fprintf(&b, "  [%d] %s\n", i+1, strings.TrimSpace(ev.Name))
			if args := strings.TrimSpace(ev.Arguments); args != "" {
				fmt.Fprintf(&b, "       args: %s\n", args)
			}
			if result := strings.TrimSpace(ev.Result); result != "" {
				fmt.Fprintf(&b, "       result: %s\n", result)
			}
		}
	}
	return b.String()
}

func validateDistillResponse(resp DistillResponse) error {
	if strings.TrimSpace(resp.Name) == "" {
		return errors.New("learning: distilled skill missing Name")
	}
	if strings.TrimSpace(resp.Description) == "" {
		return errors.New("learning: distilled skill missing Description")
	}
	if strings.TrimSpace(resp.Body) == "" {
		return errors.New("learning: distilled skill missing Body")
	}
	return nil
}

func (e *Extractor) stamp() time.Time {
	if e == nil || e.now == nil {
		return time.Now().UTC()
	}
	t := e.now()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
