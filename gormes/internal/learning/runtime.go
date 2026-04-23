package learning

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultThreshold               = 2
	defaultTokenThreshold          = 400
	defaultTranscriptCharThreshold = 280
	defaultDurationThreshold       = 30 * time.Second
)

type Config struct {
	Threshold               int
	TokenThreshold          int
	TranscriptCharThreshold int
	DurationThreshold       time.Duration
}

type Turn struct {
	SessionID        string
	UserMessage      string
	AssistantMessage string
	ToolNames        []string
	TokensIn         int
	TokensOut        int
	Duration         time.Duration
	FinishedAt       time.Time
}

type Metrics struct {
	ToolCallCount   int   `json:"tool_call_count"`
	TokenTotal      int   `json:"token_total"`
	TokensIn        int   `json:"tokens_in"`
	TokensOut       int   `json:"tokens_out"`
	TranscriptChars int   `json:"transcript_chars"`
	DurationMs      int64 `json:"duration_ms"`
}

type Signal struct {
	Timestamp     time.Time `json:"timestamp"`
	SessionID     string    `json:"session_id,omitempty"`
	WorthLearning bool      `json:"worth_learning"`
	Score         int       `json:"score"`
	Threshold     int       `json:"threshold"`
	Reasons       []string  `json:"reasons,omitempty"`
	ToolNames     []string  `json:"tool_names,omitempty"`
	Metrics       Metrics   `json:"metrics"`
}

type Recorder interface {
	RecordTurn(ctx context.Context, turn Turn) (Signal, error)
}

type Detector struct {
	cfg Config
}

type Runtime struct {
	detector Detector
	path     string
	mu       sync.Mutex
}

func NewDetector(cfg Config) Detector {
	return Detector{cfg: cfg.withDefaults()}
}

func NewRuntime(path string, cfg Config) *Runtime {
	return &Runtime{
		detector: NewDetector(cfg),
		path:     strings.TrimSpace(path),
	}
}

func (d Detector) Evaluate(turn Turn) Signal {
	cfg := d.cfg.withDefaults()
	toolNames := normalizeToolNames(turn.ToolNames)
	metrics := Metrics{
		ToolCallCount:   len(toolNames),
		TokenTotal:      maxInt(turn.TokensIn, 0) + maxInt(turn.TokensOut, 0),
		TokensIn:        maxInt(turn.TokensIn, 0),
		TokensOut:       maxInt(turn.TokensOut, 0),
		TranscriptChars: len(strings.TrimSpace(turn.UserMessage)) + len(strings.TrimSpace(turn.AssistantMessage)),
		DurationMs:      durationMillis(turn.Duration),
	}

	reasons := make([]string, 0, 5)
	score := 0
	if metrics.ToolCallCount > 0 {
		reasons = append(reasons, "tool_calls")
		score++
	}
	if metrics.ToolCallCount >= 2 {
		reasons = append(reasons, "multi_tool_calls")
		score++
	}
	if metrics.TokenTotal >= cfg.TokenThreshold {
		reasons = append(reasons, "token_total")
		score++
	}
	if metrics.TranscriptChars >= cfg.TranscriptCharThreshold {
		reasons = append(reasons, "transcript_chars")
		score++
	}
	if turn.Duration >= cfg.DurationThreshold {
		reasons = append(reasons, "duration")
		score++
	}

	finishedAt := turn.FinishedAt.UTC()
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}

	return Signal{
		Timestamp:     finishedAt,
		SessionID:     strings.TrimSpace(turn.SessionID),
		WorthLearning: score >= cfg.Threshold,
		Score:         score,
		Threshold:     cfg.Threshold,
		Reasons:       reasons,
		ToolNames:     toolNames,
		Metrics:       metrics,
	}
}

func (r *Runtime) RecordTurn(ctx context.Context, turn Turn) (Signal, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return Signal{}, ctx.Err()
	default:
	}

	if r == nil {
		return Signal{}, nil
	}

	signal := r.detector.Evaluate(turn)
	if r.path == "" {
		return signal, nil
	}

	raw, err := json.Marshal(signal)
	if err != nil {
		return Signal{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return Signal{}, err
	}

	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Signal{}, err
	}
	defer f.Close()

	if _, err := f.Write(append(raw, '\n')); err != nil {
		return Signal{}, err
	}
	return signal, nil
}

func (c Config) withDefaults() Config {
	if c.Threshold <= 0 {
		c.Threshold = defaultThreshold
	}
	if c.TokenThreshold <= 0 {
		c.TokenThreshold = defaultTokenThreshold
	}
	if c.TranscriptCharThreshold <= 0 {
		c.TranscriptCharThreshold = defaultTranscriptCharThreshold
	}
	if c.DurationThreshold <= 0 {
		c.DurationThreshold = defaultDurationThreshold
	}
	return c
}

func normalizeToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func durationMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

func maxInt(v, floor int) int {
	if v < floor {
		return floor
	}
	return v
}
