// Package mcp provides the Go-native Model Context Protocol client surface
// that Phase 5.G uses to talk to external tool servers over stdio.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultProtocolVersion = "2025-11-25"

var ErrClosed = errors.New("mcp: client closed")

type PeerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientOptions struct {
	ClientInfo                PeerInfo
	Capabilities              map[string]any
	RequestedProtocolVersion  string
	SupportedProtocolVersions []string
	Stderr                    io.Writer
}

type StdioConfig struct {
	Command string
	Args    []string
	Env     []string
	Dir     string
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ServerInfo      PeerInfo       `json:"serverInfo"`
}

type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

type CallToolResult struct {
	Content           []Content       `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

type Client struct {
	cmd *exec.Cmd

	stdin io.WriteCloser

	initializeResult InitializeResult
	supported        map[string]struct{}

	writeMu sync.Mutex
	stateMu sync.Mutex
	pending map[int64]chan rpcOutcome
	readErr error

	nextID    int64
	closed    chan struct{}
	closeOnce sync.Once
}

func DialStdio(ctx context.Context, cfg StdioConfig, opts ClientOptions) (*Client, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, errors.New("mcp: stdio command is required")
	}
	if strings.TrimSpace(opts.ClientInfo.Name) == "" || strings.TrimSpace(opts.ClientInfo.Version) == "" {
		return nil, errors.New("mcp: client info name and version are required")
	}

	requested := strings.TrimSpace(opts.RequestedProtocolVersion)
	if requested == "" {
		requested = DefaultProtocolVersion
	}
	supported := normalizeSupportedVersions(requested, opts.SupportedProtocolVersions)

	cmd := exec.CommandContext(ctx, command, cfg.Args...)
	cmd.Dir = cfg.Dir
	cmd.Env = append(os.Environ(), cfg.Env...)
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = io.Discard
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start stdio server: %w", err)
	}

	c := &Client{
		cmd:       cmd,
		stdin:     stdin,
		supported: supported,
		pending:   make(map[int64]chan rpcOutcome),
		closed:    make(chan struct{}),
	}
	go c.readLoop(bufio.NewReader(stdout))

	initResult, err := c.initialize(ctx, requested, opts.ClientInfo, opts.Capabilities)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	c.initializeResult = initResult

	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

func (c *Client) InitializeResult() InitializeResult {
	return c.initializeResult
}

func (c *Client) ListTools(ctx context.Context, cursor string) (ListToolsResult, error) {
	var result ListToolsResult
	var params any
	if trimmed := strings.TrimSpace(cursor); trimmed != "" {
		params = struct {
			Cursor string `json:"cursor"`
		}{Cursor: trimmed}
	}
	if err := c.request(ctx, "tools/list", params, &result); err != nil {
		return ListToolsResult{}, err
	}
	return result, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallToolResult, error) {
	var result CallToolResult
	params := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}{
		Name:      strings.TrimSpace(name),
		Arguments: arguments,
	}
	if params.Name == "" {
		return CallToolResult{}, errors.New("mcp: tool name is required")
	}
	if err := c.request(ctx, "tools/call", params, &result); err != nil {
		return CallToolResult{}, err
	}
	return result, nil
}

func (c *Client) Close() error {
	var waitErr error
	c.closeOnce.Do(func() {
		close(c.closed)

		c.writeMu.Lock()
		stdin := c.stdin
		c.stdin = nil
		c.writeMu.Unlock()
		if stdin != nil {
			_ = stdin.Close()
		}

		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case waitErr = <-done:
		case <-time.After(2 * time.Second):
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			waitErr = <-done
		}
	})
	return waitErr
}

func (c *Client) initialize(ctx context.Context, requested string, info PeerInfo, capabilities map[string]any) (InitializeResult, error) {
	params := struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities,omitempty"`
		ClientInfo      PeerInfo       `json:"clientInfo"`
	}{
		ProtocolVersion: requested,
		Capabilities:    capabilities,
		ClientInfo:      info,
	}
	var result InitializeResult
	if err := c.request(ctx, "initialize", params, &result); err != nil {
		return InitializeResult{}, err
	}
	if strings.TrimSpace(result.ProtocolVersion) == "" {
		return InitializeResult{}, errors.New("mcp: initialize returned empty protocol version")
	}
	if _, ok := c.supported[result.ProtocolVersion]; !ok {
		return InitializeResult{}, fmt.Errorf("mcp: unsupported protocol version %q", result.ProtocolVersion)
	}
	return result, nil
}

func (c *Client) request(ctx context.Context, method string, params any, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan rpcOutcome, 1)

	c.stateMu.Lock()
	if c.readErr != nil {
		err := c.readErr
		c.stateMu.Unlock()
		return fmt.Errorf("mcp: transport closed: %w", err)
	}
	select {
	case <-c.closed:
		c.stateMu.Unlock()
		return ErrClosed
	default:
	}
	c.pending[id] = ch
	c.stateMu.Unlock()

	msg := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.writeMessage(msg); err != nil {
		c.removePending(id)
		return err
	}

	select {
	case outcome := <-ch:
		if outcome.err != nil {
			return fmt.Errorf("mcp: %s transport error: %w", method, outcome.err)
		}
		if outcome.response.Error != nil {
			return outcome.response.Error
		}
		if out == nil || len(outcome.response.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(outcome.response.Result, out); err != nil {
			return fmt.Errorf("mcp: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.closed:
		c.removePending(id)
		return ErrClosed
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	msg := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(msg)
}

func (c *Client) writeMessage(msg any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.stdin == nil {
		return ErrClosed
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: encode message: %w", err)
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("mcp: write message: %w", err)
	}
	return nil
}

func (c *Client) readLoop(reader *bufio.Reader) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			c.failPending(err)
			return
		}

		var msg rpcEnvelope
		if err := json.Unmarshal(line, &msg); err != nil {
			c.failPending(fmt.Errorf("decode message: %w", err))
			return
		}
		if msg.ID == nil {
			continue
		}

		c.stateMu.Lock()
		ch := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.stateMu.Unlock()
		if ch == nil {
			continue
		}
		ch <- rpcOutcome{response: rpcResponse{Result: msg.Result, Error: msg.Error}}
		close(ch)
	}
}

func (c *Client) failPending(err error) {
	c.stateMu.Lock()
	if c.readErr == nil {
		c.readErr = err
	}
	pending := c.pending
	c.pending = make(map[int64]chan rpcOutcome)
	c.stateMu.Unlock()

	for _, ch := range pending {
		ch <- rpcOutcome{err: err}
		close(ch)
	}
}

func (c *Client) removePending(id int64) {
	c.stateMu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.stateMu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func normalizeSupportedVersions(requested string, versions []string) map[string]struct{} {
	out := make(map[string]struct{})
	addVersion := func(version string) {
		version = strings.TrimSpace(version)
		if version != "" {
			out[version] = struct{}{}
		}
	}
	addVersion(requested)
	for _, version := range versions {
		addVersion(version)
	}
	return out
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcEnvelope struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage
	Error  *RPCError
}

type rpcOutcome struct {
	response rpcResponse
	err      error
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("mcp: rpc error %d: %s", e.Code, e.Message)
}
