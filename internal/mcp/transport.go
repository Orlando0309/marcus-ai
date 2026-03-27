package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Transport is the interface for MCP transport layers
type Transport interface {
	// Send sends a JSON-RPC request and returns the response
	Send(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)
	// Close closes the transport connection
	Close() error
}

// StdioTransport communicates with MCP servers via stdin/stdout
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser
	mu     sync.Mutex
	closed bool
}

// NewStdioTransport creates a new stdio transport for the given command
func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// Set up environment
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	// Get stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Get stderr pipe (for logging)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	transport := &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: stderr,
	}

	// Start stderr reader for logging
	go transport.logStderr()

	return transport, nil
}

// Send sends a JSON-RPC request over stdio
func (t *StdioTransport) Send(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	// Build JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}

	// Marshal request
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Send with newline delimiter
	if _, err := t.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response with timeout
	readCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			errCh <- err
			return
		}
		readCh <- line
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, fmt.Errorf("read response: %w", err)
	case line := <-readCh:
		return line, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("request timeout")
	}
}

// Close closes the transport
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	// Close stdin to signal EOF
	if t.stdin != nil {
		t.stdin.Close()
	}

	// Wait for process to exit
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Signal(os.Interrupt)
		time.AfterFunc(5*time.Second, func() {
			t.cmd.Process.Kill()
		})
		t.cmd.Wait()
	}

	return nil
}

// logStderr logs stderr output from the MCP server
func (t *StdioTransport) logStderr() {
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		// Log to debug output if needed
		_ = scanner.Text()
	}
}

// SSETransport communicates with MCP servers via Server-Sent Events (HTTP)
type SSETransport struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
	closed     bool
}

// NewSSETransport creates a new SSE transport for the given base URL
func NewSSETransport(baseURL string) (*SSETransport, error) {
	// Normalize URL
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &SSETransport{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Send sends a JSON-RPC request over HTTP POST
func (t *SSETransport) Send(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	// Build JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}

	// Marshal request
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", t.baseURL+"/rpc", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Close closes the transport
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	return nil
}

var requestIDCounter int64
var requestIDMu sync.Mutex

func generateRequestID() int64 {
	requestIDMu.Lock()
	defer requestIDMu.Unlock()
	requestIDCounter++
	return requestIDCounter
}
