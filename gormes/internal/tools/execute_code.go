package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultExecuteCodeTimeout     = 30 * time.Second
	defaultExecuteCodeStdoutLimit = 50 * 1024
	defaultExecuteCodeStderrLimit = 10 * 1024
)

// CodeExecutionRequest is the sandbox contract consumed by execute_code.
type CodeExecutionRequest struct {
	Language         string
	Code             string
	Timeout          time.Duration
	StdoutLimitBytes int
	StderrLimitBytes int
}

// CodeExecutionResult is the structured response returned by execute_code.
type CodeExecutionResult struct {
	Status           string `json:"status"`
	Language         string `json:"language,omitempty"`
	ExitCode         int    `json:"exit_code"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
	StdoutTruncated  bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated  bool   `json:"stderr_truncated,omitempty"`
	DurationMs       int64  `json:"duration_ms"`
	Error            string `json:"error,omitempty"`
	FilesystemAccess bool   `json:"filesystem_access"`
	NetworkAccess    bool   `json:"network_access"`
}

// CodeSandbox executes a code snippet under Gormes's guardrails.
type CodeSandbox interface {
	Execute(ctx context.Context, req CodeExecutionRequest) (CodeExecutionResult, error)
}

// ExecuteCodeTool ports the upstream execute_code surface to Go.
type ExecuteCodeTool struct {
	Sandbox          CodeSandbox
	DefaultTimeout   time.Duration
	DefaultStdoutCap int
	DefaultStderrCap int
}

func NewExecuteCodeTool() *ExecuteCodeTool {
	return &ExecuteCodeTool{
		Sandbox:          NewLocalCodeSandbox(),
		DefaultTimeout:   defaultExecuteCodeTimeout,
		DefaultStdoutCap: defaultExecuteCodeStdoutLimit,
		DefaultStderrCap: defaultExecuteCodeStderrLimit,
	}
}

func (*ExecuteCodeTool) Name() string { return "execute_code" }

func (*ExecuteCodeTool) Description() string {
	return "Run a small code snippet in a guarded local sandbox with language selection, output caps, timeout handling, and filesystem/network guards."
}

func (*ExecuteCodeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"language":{"type":"string","description":"runtime to use (currently sh or python)"},"code":{"type":"string","description":"code snippet to execute"},"timeout_ms":{"type":"integer","description":"optional per-run timeout in milliseconds"},"stdout_limit_bytes":{"type":"integer","description":"optional stdout capture cap in bytes"},"stderr_limit_bytes":{"type":"integer","description":"optional stderr capture cap in bytes"}},"required":["language","code"]}`)
}

func (t *ExecuteCodeTool) Timeout() time.Duration {
	if t.DefaultTimeout > 0 {
		return t.DefaultTimeout + 5*time.Second
	}
	return defaultExecuteCodeTimeout + 5*time.Second
}

func (t *ExecuteCodeTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Language         string `json:"language"`
		Code             string `json:"code"`
		TimeoutMS        int    `json:"timeout_ms"`
		StdoutLimitBytes int    `json:"stdout_limit_bytes"`
		StderrLimitBytes int    `json:"stderr_limit_bytes"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("execute_code: invalid args: %w", err)
	}
	if strings.TrimSpace(in.Language) == "" {
		return nil, fmt.Errorf("execute_code: language is required")
	}
	if strings.TrimSpace(in.Code) == "" {
		return nil, fmt.Errorf("execute_code: code is required")
	}

	req := CodeExecutionRequest{
		Language:         strings.TrimSpace(in.Language),
		Code:             in.Code,
		Timeout:          durationOrDefault(in.TimeoutMS, t.DefaultTimeout, defaultExecuteCodeTimeout),
		StdoutLimitBytes: intOrDefault(in.StdoutLimitBytes, t.DefaultStdoutCap, defaultExecuteCodeStdoutLimit),
		StderrLimitBytes: intOrDefault(in.StderrLimitBytes, t.DefaultStderrCap, defaultExecuteCodeStderrLimit),
	}

	sandbox := t.Sandbox
	if sandbox == nil {
		sandbox = NewLocalCodeSandbox()
	}
	result, err := sandbox.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func durationOrDefault(ms int, preferred, fallback time.Duration) time.Duration {
	if ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	if preferred > 0 {
		return preferred
	}
	return fallback
}

func intOrDefault(v, preferred, fallback int) int {
	if v > 0 {
		return v
	}
	if preferred > 0 {
		return preferred
	}
	return fallback
}

type LocalCodeSandbox struct {
	lookPath  func(string) (string, error)
	languages map[string]runtimeSpec
}

type runtimeSpec struct {
	Binaries  []string
	Args      []string
	Extension string
}

func NewLocalCodeSandbox() *LocalCodeSandbox {
	return &LocalCodeSandbox{
		lookPath: exec.LookPath,
		languages: map[string]runtimeSpec{
			"sh":     {Binaries: []string{"sh"}, Extension: ".sh"},
			"shell":  {Binaries: []string{"sh"}, Extension: ".sh"},
			"python": {Binaries: []string{"python3", "python"}, Extension: ".py"},
		},
	}
}

func (s *LocalCodeSandbox) Execute(ctx context.Context, req CodeExecutionRequest) (CodeExecutionResult, error) {
	req.Language = strings.ToLower(strings.TrimSpace(req.Language))
	req.Code = strings.TrimSpace(req.Code)
	req.Timeout = durationOrDefault(0, req.Timeout, defaultExecuteCodeTimeout)
	req.StdoutLimitBytes = intOrDefault(req.StdoutLimitBytes, 0, defaultExecuteCodeStdoutLimit)
	req.StderrLimitBytes = intOrDefault(req.StderrLimitBytes, 0, defaultExecuteCodeStderrLimit)

	result := CodeExecutionResult{
		Language:         req.Language,
		FilesystemAccess: false,
		NetworkAccess:    false,
	}

	if blockedReason := sandboxGuardReason(req.Language, req.Code); blockedReason != "" {
		result.Status = "blocked"
		result.Error = blockedReason
		return result, nil
	}

	spec, err := s.resolveRuntime(req.Language)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result, nil
	}

	tempDir, err := os.MkdirTemp("", "gormes-execute-code-*")
	if err != nil {
		return CodeExecutionResult{}, fmt.Errorf("execute_code: create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	scriptPath := filepath.Join(tempDir, "snippet"+spec.Extension)
	if err := os.WriteFile(scriptPath, []byte(req.Code), 0o600); err != nil {
		return CodeExecutionResult{}, fmt.Errorf("execute_code: write script: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	args := append(append([]string(nil), spec.Args...), scriptPath)
	cmd := exec.CommandContext(runCtx, spec.Binaries[0], args...)
	cmd.Dir = tempDir
	cmd.Env = safeSandboxEnv()

	stdout := newLimitedBuffer(req.StdoutLimitBytes)
	stderr := newLimitedBuffer(req.StderrLimitBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
	result.DurationMs = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()

	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		result.Status = "timeout"
		result.Error = fmt.Sprintf("execution timed out after %s", req.Timeout)
	case runErr != nil:
		result.Status = "error"
		result.ExitCode = exitCode(runErr)
		result.Error = runErr.Error()
	default:
		result.Status = "success"
	}

	return result, nil
}

func (s *LocalCodeSandbox) resolveRuntime(language string) (runtimeSpec, error) {
	spec, ok := s.languages[language]
	if !ok {
		return runtimeSpec{}, fmt.Errorf("execute_code: unsupported language %q", language)
	}
	for _, candidate := range spec.Binaries {
		if resolved, err := s.lookPath(candidate); err == nil {
			spec.Binaries = []string{resolved}
			return spec, nil
		}
	}
	return runtimeSpec{}, fmt.Errorf("execute_code: runtime for %q is unavailable", language)
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	return exitErr.ExitCode()
}

func safeSandboxEnv() []string {
	keys := []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR", "TMP", "TEMP"}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

var (
	shellFilesystemPattern  = regexp.MustCompile(`\b(cat|touch|ls|find|mkdir|rm|cp|mv)\b`)
	shellNetworkPattern     = regexp.MustCompile(`\b(curl|wget|ping|nc|ssh|scp|ftp|dig|host)\b`)
	pythonFilesystemPattern = regexp.MustCompile(`\b(open|pathlib|os\.open|os\.listdir|os\.remove|Path)\b`)
	pythonNetworkPattern    = regexp.MustCompile(`\b(socket|urllib|requests|http\.client|websocket)\b`)
)

func sandboxGuardReason(language, code string) string {
	switch language {
	case "sh", "shell":
		switch {
		case shellFilesystemPattern.MatchString(code):
			return "filesystem access is disabled in sandboxed exec"
		case shellNetworkPattern.MatchString(code):
			return "network access is disabled in sandboxed exec"
		}
	case "python":
		switch {
		case pythonFilesystemPattern.MatchString(code):
			return "filesystem access is disabled in sandboxed exec"
		case pythonNetworkPattern.MatchString(code):
			return "network access is disabled in sandboxed exec"
		}
	}
	return ""
}

type limitedBuffer struct {
	limit     int
	builder   strings.Builder
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = len(p) > 0
		return len(p), nil
	}
	remaining := b.limit - b.builder.Len()
	switch {
	case remaining <= 0:
		b.truncated = true
	case len(p) <= remaining:
		_, _ = b.builder.Write(p)
	default:
		_, _ = b.builder.Write(p[:remaining])
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if !b.truncated {
		return b.builder.String()
	}
	return fmt.Sprintf("%s\n[truncated at %d bytes]", b.builder.String(), b.limit)
}

func (b *limitedBuffer) Truncated() bool { return b.truncated }
