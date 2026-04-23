package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/learning"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

type KernelSessionFactoryOptions struct {
	Model             string
	Endpoint          string
	ClientFactory     func() hermes.Client
	RegistryFactory   func(hermes.Client) *tools.Registry
	ModelRouting      kernel.SmartModelRouting
	Skills            kernel.SkillProvider
	SkillUsage        kernel.SkillUsageRecorder
	Learning          learning.Recorder
	MaxToolIterations int
	MaxToolDuration   time.Duration
	Logger            *slog.Logger
}

type KernelSessionFactory struct {
	opts KernelSessionFactoryOptions
}

func NewKernelSessionFactory(opts KernelSessionFactoryOptions) *KernelSessionFactory {
	return &KernelSessionFactory{opts: opts}
}

func (f *KernelSessionFactory) NewSession(parent context.Context, cwd string) (Session, error) {
	if f == nil || f.opts.ClientFactory == nil {
		return nil, errors.New("acp: ClientFactory is required")
	}
	client := f.opts.ClientFactory()
	reg := tools.NewRegistry()
	if f.opts.RegistryFactory != nil {
		reg = f.opts.RegistryFactory(client)
	}
	log := f.opts.Logger
	if log == nil {
		log = slog.Default()
	}
	model := strings.TrimSpace(f.opts.Model)
	if model == "" {
		model = "gormes"
	}

	sessionCtx, cancel := context.WithCancel(parent)
	k := kernel.New(kernel.Config{
		Model:             model,
		Endpoint:          f.opts.Endpoint,
		ModelRouting:      f.opts.ModelRouting,
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		Skills:            f.opts.Skills,
		SkillUsage:        f.opts.SkillUsage,
		Learning:          f.opts.Learning,
		MaxToolIterations: f.opts.MaxToolIterations,
		MaxToolDuration:   f.opts.MaxToolDuration,
	}, client, store.NewNoop(), telemetry.New(), log)

	frames := k.Render()
	go k.Run(sessionCtx)

	select {
	case <-sessionCtx.Done():
		cancel()
		return nil, sessionCtx.Err()
	case _, ok := <-frames:
		if !ok {
			cancel()
			return nil, io.EOF
		}
	}

	return &kernelSession{
		cwd:        cwd,
		kernel:     k,
		frames:     frames,
		cancelRoot: cancel,
		promptMu:   make(chan struct{}, 1),
	}, nil
}

type kernelSession struct {
	cwd        string
	kernel     *kernel.Kernel
	frames     <-chan kernel.RenderFrame
	cancelRoot context.CancelFunc
	promptMu   chan struct{}
}

func (s *kernelSession) Prompt(ctx context.Context, prompt []ContentBlock, send func(SessionUpdate)) (PromptResult, error) {
	select {
	case s.promptMu <- struct{}{}:
		defer func() { <-s.promptMu }()
	case <-ctx.Done():
		return PromptResult{}, ctx.Err()
	}

	text := joinPromptText(prompt)
	if strings.TrimSpace(text) == "" {
		return PromptResult{}, errors.New("acp: prompt must include at least one text block")
	}
	if err := s.kernel.Submit(kernel.PlatformEvent{
		Kind:           kernel.PlatformEventSubmit,
		Text:           text,
		SessionContext: workspaceContext(s.cwd),
	}); err != nil {
		return PromptResult{}, err
	}

	var (
		lastText  string
		cancelled bool
	)
	for {
		select {
		case <-ctx.Done():
			return PromptResult{}, ctx.Err()
		case frame, ok := <-s.frames:
			if !ok {
				return PromptResult{}, io.EOF
			}
			current := frame.DraftText
			if assistant := lastAssistant(frame.History); len(assistant) > len(current) {
				current = assistant
			}
			if delta := streamDelta(lastText, current); delta != "" {
				send(SessionUpdate{
					SessionUpdate: "agent_message_chunk",
					Content:       TextContentBlock{Type: "text", Text: delta},
				})
				lastText = current
			}
			if frame.Phase == kernel.PhaseCancelling {
				cancelled = true
				continue
			}
			if frame.Phase == kernel.PhaseFailed {
				if strings.TrimSpace(frame.LastError) == "" {
					return PromptResult{}, errors.New("acp: kernel prompt failed")
				}
				return PromptResult{}, errors.New(frame.LastError)
			}
			if frame.Phase == kernel.PhaseIdle {
				if cancelled {
					return PromptResult{StopReason: StopReasonCancelled}, nil
				}
				return PromptResult{StopReason: StopReasonEndTurn}, nil
			}
		}
	}
}

func (s *kernelSession) Cancel() {
	if s == nil || s.kernel == nil {
		return
	}
	_ = s.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
}

func (s *kernelSession) Close() error {
	if s == nil || s.cancelRoot == nil {
		return nil
	}
	s.cancelRoot()
	return nil
}

func joinPromptText(prompt []ContentBlock) string {
	parts := make([]string, 0, len(prompt))
	for _, block := range prompt {
		if block.Type != "text" {
			continue
		}
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n")
}

func workspaceContext(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	return fmt.Sprintf("ACP workspace root: %s", cwd)
}

func lastAssistant(history []hermes.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" {
			return history[i].Content
		}
	}
	return ""
}

func streamDelta(previous, current string) string {
	if current == previous {
		return ""
	}
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	return current
}
