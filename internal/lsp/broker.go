package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
)

var ErrUnavailable = errors.New("lsp unavailable")

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Message  string `json:"message"`
	Severity int    `json:"severity,omitempty"`
	Source   string `json:"source,omitempty"`
	Range    Range  `json:"range"`
}

type Broker struct {
	root     string
	timeout  time.Duration
	commands map[string]string
	mu       sync.Mutex
	clients  map[string]*client
}

func NewBroker(cfg config.LSPConfig, projectRoot string) *Broker {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Broker{
		root:    projectRoot,
		timeout: timeout,
		commands: map[string]string{
			"go":         strings.TrimSpace(cfg.GoCommand),
			"python":     strings.TrimSpace(cfg.PythonCommand),
			"javascript": strings.TrimSpace(cfg.JavaScriptCommand),
			"typescript": strings.TrimSpace(cfg.TypeScriptCommand),
		},
		clients: make(map[string]*client),
	}
}

func (b *Broker) WorkspaceSymbols(ctx context.Context, language, query string) ([]Location, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return nil, err
	}
	var out []struct {
		Location Location `json:"location"`
	}
	if err := c.call(ctx, "workspace/symbol", map[string]any{"query": query}, &out); err != nil {
		return nil, err
	}
	locations := make([]Location, 0, len(out))
	for _, item := range out {
		locations = append(locations, item.Location)
	}
	return locations, nil
}

func (b *Broker) Definition(ctx context.Context, language, uri string, line, character int) ([]Location, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return nil, err
	}
	var out []Location
	err = c.call(ctx, "textDocument/definition", textDocumentPosition(uri, line, character), &out)
	if err == nil && len(out) > 0 {
		return out, nil
	}
	var single Location
	if err := c.call(ctx, "textDocument/definition", textDocumentPosition(uri, line, character), &single); err != nil {
		return nil, err
	}
	return []Location{single}, nil
}

// Hover returns plain-text hover content at a position, if any.
func (b *Broker) Hover(ctx context.Context, language, uri string, line, character int) (string, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return "", err
	}
	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/hover", textDocumentPosition(uri, line, character), &raw); err != nil {
		return "", err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var top struct {
		Contents json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(raw, &top) != nil {
		return "", nil
	}
	return strings.TrimSpace(parseHoverContentsRaw(top.Contents)), nil
}

func parseHoverContentsRaw(contents json.RawMessage) string {
	if len(contents) == 0 || string(contents) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(contents, &s) == nil {
		return s
	}
	var mc struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(contents, &mc) == nil && mc.Value != "" {
		return mc.Value
	}
	var arr []json.RawMessage
	if json.Unmarshal(contents, &arr) == nil {
		var b strings.Builder
		for _, el := range arr {
			if p := parseHoverContentsRaw(el); p != "" {
				b.WriteString(p)
				b.WriteByte('\n')
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// Rename returns a workspace edit JSON (LSP WorkspaceEdit) or null.
func (b *Broker) Rename(ctx context.Context, language, uri string, line, character int, newName string) (json.RawMessage, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return nil, err
	}
	params := textDocumentPosition(uri, line, character)
	params["newName"] = newName
	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/rename", params, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (b *Broker) References(ctx context.Context, language, uri string, line, character int) ([]Location, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return nil, err
	}
	var out []Location
	params := textDocumentPosition(uri, line, character)
	params["context"] = map[string]bool{"includeDeclaration": true}
	if err := c.call(ctx, "textDocument/references", params, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *Broker) Diagnostics(ctx context.Context, language, uri string) ([]Diagnostic, error) {
	c, err := b.clientFor(language)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []Diagnostic `json:"items"`
	}
	err = c.call(ctx, "textDocument/diagnostic", map[string]any{
		"textDocument": map[string]string{"uri": uri},
	}, &out)
	if err == nil && len(out.Items) > 0 {
		return out.Items, nil
	}
	return c.cachedDiagnostics(uri), nil
}

func (b *Broker) clientFor(language string) (*client, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	command := b.commands[language]
	if command == "" {
		return nil, ErrUnavailable
	}
	if existing, ok := b.clients[language]; ok {
		return existing, nil
	}
	c, err := startClient(command, b.root, b.timeout)
	if err != nil {
		return nil, err
	}
	b.clients[language] = c
	return c, nil
}

type client struct {
	cmd         *exec.Cmd
	in          io.WriteCloser
	out         *bufio.Reader
	timeout     time.Duration
	nextID      int64
	pendingMu   sync.Mutex
	pending     map[int64]chan responseEnvelope
	diagnostics map[string][]Diagnostic
	diagMu      sync.RWMutex
}

type responseEnvelope struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func startClient(command, root string, timeout time.Duration) (*client, error) {
	cmd := exec.Command(command, "-stdio")
	if root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &client{
		cmd:         cmd,
		in:          stdin,
		out:         bufio.NewReader(stdout),
		timeout:     timeout,
		pending:     make(map[int64]chan responseEnvelope),
		diagnostics: make(map[string][]Diagnostic),
	}
	go c.readLoop()
	initCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var initResp map[string]any
	if err := c.call(initCtx, "initialize", map[string]any{
		"processId":    nil,
		"rootUri":      pathToURI(root),
		"capabilities": map[string]any{},
	}, &initResp); err != nil {
		return nil, err
	}
	_ = c.notify("initialized", map[string]any{})
	return c, nil
}

func (c *client) call(ctx context.Context, method string, params any, out any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	respCh := make(chan responseEnvelope, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()
	if err := writeFrame(c.in, payload); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return errors.New(resp.Error.Message)
		}
		if out == nil || len(resp.Result) == 0 || string(resp.Result) == "null" {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	}
}

func (c *client) notify(method string, params any) error {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	return writeFrame(c.in, payload)
}

func (c *client) readLoop() {
	for {
		payload, err := readFrame(c.out)
		if err != nil {
			return
		}
		var resp responseEnvelope
		if err := json.Unmarshal(payload, &resp); err != nil {
			continue
		}
		if resp.Method == "textDocument/publishDiagnostics" {
			var params struct {
				URI         string       `json:"uri"`
				Diagnostics []Diagnostic `json:"diagnostics"`
			}
			if json.Unmarshal(resp.Params, &params) == nil {
				c.diagMu.Lock()
				c.diagnostics[params.URI] = params.Diagnostics
				c.diagMu.Unlock()
			}
			continue
		}
		c.pendingMu.Lock()
		ch := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.pendingMu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (c *client) cachedDiagnostics(uri string) []Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	return append([]Diagnostic(nil), c.diagnostics[uri]...)
}

func textDocumentPosition(uri string, line, character int) map[string]any {
	return map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position": map[string]int{
			"line":      line,
			"character": character,
		},
	}
}

func writeFrame(w io.Writer, payload []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	headers := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		}
	}
	length, _ := strconv.Atoi(headers["content-length"])
	if length <= 0 {
		return nil, io.EOF
	}
	buf := bytes.NewBuffer(make([]byte, 0, length))
	if _, err := io.CopyN(buf, r, int64(length)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pathToURI(path string) string {
	if path == "" {
		return ""
	}
	return "file:///" + strings.ReplaceAll(filepath.ToSlash(path), ":", "%3A")
}
