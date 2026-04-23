package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type fakeCodeSandbox struct {
	req    CodeExecutionRequest
	result CodeExecutionResult
	err    error
}

func (f *fakeCodeSandbox) Execute(_ context.Context, req CodeExecutionRequest) (CodeExecutionResult, error) {
	f.req = req
	return f.result, f.err
}

func TestExecuteCodeTool_UsesRequestedLanguage(t *testing.T) {
	sandbox := &fakeCodeSandbox{
		result: CodeExecutionResult{Status: "success", Language: "python"},
	}
	tool := &ExecuteCodeTool{Sandbox: sandbox}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"language":"python","code":"print('hi')"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sandbox.req.Language != "python" {
		t.Fatalf("sandbox language = %q, want python", sandbox.req.Language)
	}
	if !strings.Contains(string(out), `"language":"python"`) {
		t.Fatalf("output = %s, want language field", out)
	}
}

func TestLocalCodeSandbox_TruncatesStdoutAndStderr(t *testing.T) {
	sandbox := NewLocalCodeSandbox()

	result, err := sandbox.Execute(context.Background(), CodeExecutionRequest{
		Language:         "sh",
		Code:             `printf 'stdout-limit'; printf 'stderr-limit' >&2`,
		StdoutLimitBytes: 6,
		StderrLimitBytes: 5,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %q, want success", result.Status)
	}
	if !result.StdoutTruncated || !strings.Contains(result.Stdout, "[truncated at 6 bytes]") {
		t.Fatalf("stdout = %q, want truncation marker", result.Stdout)
	}
	if !result.StderrTruncated || !strings.Contains(result.Stderr, "[truncated at 5 bytes]") {
		t.Fatalf("stderr = %q, want truncation marker", result.Stderr)
	}
}

func TestLocalCodeSandbox_TimesOut(t *testing.T) {
	sandbox := NewLocalCodeSandbox()

	result, err := sandbox.Execute(context.Background(), CodeExecutionRequest{
		Language: "sh",
		Code:     `sleep 1`,
		Timeout:  20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "timeout" {
		t.Fatalf("status = %q, want timeout", result.Status)
	}
	if result.Error == "" || !strings.Contains(result.Error, "timed out") {
		t.Fatalf("error = %q, want timeout detail", result.Error)
	}
}

func TestLocalCodeSandbox_BlocksFilesystemAccess(t *testing.T) {
	sandbox := NewLocalCodeSandbox()

	result, err := sandbox.Execute(context.Background(), CodeExecutionRequest{
		Language: "sh",
		Code:     `touch blocked.txt`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !strings.Contains(strings.ToLower(result.Error), "filesystem") {
		t.Fatalf("error = %q, want filesystem guard detail", result.Error)
	}
}

func TestLocalCodeSandbox_BlocksNetworkAccess(t *testing.T) {
	sandbox := NewLocalCodeSandbox()

	result, err := sandbox.Execute(context.Background(), CodeExecutionRequest{
		Language: "sh",
		Code:     `curl https://example.com`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !strings.Contains(strings.ToLower(result.Error), "network") {
		t.Fatalf("error = %q, want network guard detail", result.Error)
	}
}
