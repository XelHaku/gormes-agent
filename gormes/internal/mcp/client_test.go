package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testLatestProtocol  = "2025-11-25"
	testFallbackVersion = "2025-06-18"
)

func TestDialStdioInitializesAndSendsInitializedNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "mcp.log")
	client, err := DialStdio(ctx, newTestServerConfig(t, logPath, testLatestProtocol), ClientOptions{
		ClientInfo: PeerInfo{Name: "gormes-test", Version: "0.1.0"},
		Capabilities: map[string]any{
			"elicitation": map[string]any{},
		},
		RequestedProtocolVersion:  testLatestProtocol,
		SupportedProtocolVersions: []string{testLatestProtocol, testFallbackVersion},
	})
	if err != nil {
		t.Fatalf("DialStdio() error = %v", err)
	}
	defer client.Close()

	initResult := client.InitializeResult()
	if got := initResult.ProtocolVersion; got != testLatestProtocol {
		t.Fatalf("InitializeResult().ProtocolVersion = %q, want %q", got, testLatestProtocol)
	}
	if got := initResult.ServerInfo.Name; got != "fixture-mcp" {
		t.Fatalf("InitializeResult().ServerInfo.Name = %q, want fixture-mcp", got)
	}

	messages := waitForLoggedMessages(t, logPath, 2)
	if got := messages[0].Method; got != "initialize" {
		t.Fatalf("first logged method = %q, want initialize", got)
	}
	if got := messages[1].Method; got != "notifications/initialized" {
		t.Fatalf("second logged method = %q, want notifications/initialized", got)
	}

	var params struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ClientInfo      PeerInfo       `json:"clientInfo"`
	}
	if err := json.Unmarshal(messages[0].Params, &params); err != nil {
		t.Fatalf("json.Unmarshal(initialize params): %v", err)
	}
	if params.ProtocolVersion != testLatestProtocol {
		t.Fatalf("initialize protocolVersion = %q, want %q", params.ProtocolVersion, testLatestProtocol)
	}
	if params.ClientInfo.Name != "gormes-test" || params.ClientInfo.Version != "0.1.0" {
		t.Fatalf("initialize clientInfo = %+v, want gormes-test/0.1.0", params.ClientInfo)
	}
	if _, ok := params.Capabilities["elicitation"]; !ok {
		t.Fatalf("initialize capabilities = %v, want elicitation entry", params.Capabilities)
	}
}

func TestDialStdioNegotiatesSupportedProtocolVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := DialStdio(ctx, newTestServerConfig(t, filepath.Join(t.TempDir(), "mcp.log"), testFallbackVersion), ClientOptions{
		ClientInfo:               PeerInfo{Name: "gormes-test", Version: "0.1.0"},
		RequestedProtocolVersion: testLatestProtocol,
		SupportedProtocolVersions: []string{
			testLatestProtocol,
			testFallbackVersion,
		},
	})
	if err != nil {
		t.Fatalf("DialStdio() error = %v", err)
	}
	defer client.Close()

	if got := client.InitializeResult().ProtocolVersion; got != testFallbackVersion {
		t.Fatalf("InitializeResult().ProtocolVersion = %q, want %q", got, testFallbackVersion)
	}
}

func TestDialStdioRejectsUnsupportedProtocolVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := DialStdio(ctx, newTestServerConfig(t, filepath.Join(t.TempDir(), "mcp.log"), "2024-01-01"), ClientOptions{
		ClientInfo:               PeerInfo{Name: "gormes-test", Version: "0.1.0"},
		RequestedProtocolVersion: testLatestProtocol,
		SupportedProtocolVersions: []string{
			testLatestProtocol,
			testFallbackVersion,
		},
	})
	if err == nil {
		t.Fatal("DialStdio() error = nil, want unsupported protocol version")
	}
	if !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Fatalf("DialStdio() error = %v, want unsupported protocol version detail", err)
	}
}

func TestClientListToolsAndCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "mcp.log")
	client, err := DialStdio(ctx, newTestServerConfig(t, logPath, testLatestProtocol), ClientOptions{
		ClientInfo:               PeerInfo{Name: "gormes-test", Version: "0.1.0"},
		RequestedProtocolVersion: testLatestProtocol,
		SupportedProtocolVersions: []string{
			testLatestProtocol,
			testFallbackVersion,
		},
	})
	if err != nil {
		t.Fatalf("DialStdio() error = %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(ctx, "page-1")
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 1 {
		t.Fatalf("ListTools().Tools len = %d, want 1", len(tools.Tools))
	}
	if got := tools.Tools[0].Name; got != "sum" {
		t.Fatalf("ListTools().Tools[0].Name = %q, want sum", got)
	}
	if got := tools.NextCursor; got != "page-2" {
		t.Fatalf("ListTools().NextCursor = %q, want page-2", got)
	}

	result, err := client.CallTool(ctx, "sum", map[string]any{"a": 2, "b": 5})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool().IsError = true, want false")
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != "sum=7" {
		t.Fatalf("CallTool().Content = %+v, want single text result sum=7", result.Content)
	}
	var structured struct {
		Sum float64 `json:"sum"`
	}
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("json.Unmarshal(StructuredContent): %v", err)
	}
	if structured.Sum != 7 {
		t.Fatalf("structured sum = %v, want 7", structured.Sum)
	}

	messages := waitForLoggedMessages(t, logPath, 4)
	listMsg := findLoggedMessage(t, messages, "tools/list")
	var listParams struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(listMsg.Params, &listParams); err != nil {
		t.Fatalf("json.Unmarshal(tools/list params): %v", err)
	}
	if listParams.Cursor != "page-1" {
		t.Fatalf("tools/list cursor = %q, want page-1", listParams.Cursor)
	}

	callMsg := findLoggedMessage(t, messages, "tools/call")
	var callParams struct {
		Name      string             `json:"name"`
		Arguments map[string]float64 `json:"arguments"`
	}
	if err := json.Unmarshal(callMsg.Params, &callParams); err != nil {
		t.Fatalf("json.Unmarshal(tools/call params): %v", err)
	}
	if callParams.Name != "sum" || callParams.Arguments["a"] != 2 || callParams.Arguments["b"] != 5 {
		t.Fatalf("tools/call params = %+v, want name=sum arguments a=2 b=5", callParams)
	}
}

func TestMCPTestServerProcess(t *testing.T) {
	if os.Getenv("GORMES_MCP_HELPER_PROCESS") != "1" {
		return
	}

	logPath := os.Getenv("GORMES_MCP_HELPER_LOG")
	negotiatedVersion := os.Getenv("GORMES_MCP_HELPER_NEGOTIATED_VERSION")
	if negotiatedVersion == "" {
		negotiatedVersion = testLatestProtocol
	}
	if err := runTestServer(logPath, negotiatedVersion, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

type loggedMessage struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func newTestServerConfig(t *testing.T, logPath, negotiatedVersion string) StdioConfig {
	t.Helper()
	return StdioConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPTestServerProcess", "--"},
		Env: []string{
			"GORMES_MCP_HELPER_PROCESS=1",
			"GORMES_MCP_HELPER_LOG=" + logPath,
			"GORMES_MCP_HELPER_NEGOTIATED_VERSION=" + negotiatedVersion,
		},
	}
}

func waitForLoggedMessages(t *testing.T, path string, want int) []loggedMessage {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		msgs, err := readLoggedMessages(path)
		if err == nil && len(msgs) >= want {
			return msgs
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("readLoggedMessages(%q) error = %v", path, err)
			}
			t.Fatalf("logged messages = %d, want at least %d", len(msgs), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readLoggedMessages(path string) ([]loggedMessage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	var out []loggedMessage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg loggedMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func findLoggedMessage(t *testing.T, messages []loggedMessage, method string) loggedMessage {
	t.Helper()
	for _, msg := range messages {
		if msg.Method == method {
			return msg
		}
	}
	t.Fatalf("logged method %q not found in %v", method, messages)
	return loggedMessage{}
}

func runTestServer(logPath, negotiatedVersion string, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			return err
		}
		if err := appendLogLine(logPath, line); err != nil {
			return err
		}

		switch req.Method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": negotiatedVersion,
					"capabilities": map[string]any{
						"tools": map[string]any{
							"listChanged": true,
						},
					},
					"serverInfo": map[string]any{
						"name":    "fixture-mcp",
						"version": "1.0.0",
					},
				},
			}
			if err := json.NewEncoder(out).Encode(resp); err != nil {
				return err
			}
		case "notifications/initialized":
			continue
		case "tools/list":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "sum",
							"title":       "Adder",
							"description": "Add two numbers",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"a": map[string]any{"type": "number"},
									"b": map[string]any{"type": "number"},
								},
								"required": []string{"a", "b"},
							},
						},
					},
					"nextCursor": "page-2",
				},
			}
			if err := json.NewEncoder(out).Encode(resp); err != nil {
				return err
			}
		case "tools/call":
			var params struct {
				Arguments struct {
					A float64 `json:"a"`
					B float64 `json:"b"`
				} `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return err
			}
			sum := params.Arguments.A + params.Arguments.B
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("sum=%.0f", sum)},
					},
					"structuredContent": map[string]any{
						"sum": sum,
					},
					"isError": false,
				},
			}
			if err := json.NewEncoder(out).Encode(resp); err != nil {
				return err
			}
		default:
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]any{
					"code":    -32601,
					"message": "method not found",
				},
			}
			if err := json.NewEncoder(out).Encode(resp); err != nil {
				return err
			}
		}
	}
}

func appendLogLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}
