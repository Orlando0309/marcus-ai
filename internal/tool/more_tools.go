package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunBackgroundTool starts a command without waiting for completion.
type RunBackgroundTool struct {
	baseDir string
}

func NewRunBackgroundTool(baseDir string) *RunBackgroundTool {
	return &RunBackgroundTool{baseDir: baseDir}
}

func (t *RunBackgroundTool) Name() string { return "run_in_background" }

func (t *RunBackgroundTool) Description() string {
	return "Start a shell command in the background; returns PID and log file path (stdout/stderr)"
}

func (t *RunBackgroundTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"command": {Type: "string", Description: "Shell command"},
			"dir":     {Type: "string", Description: "Working directory (optional)"},
		},
		Required: []string{"command"},
	}
}

func (t *RunBackgroundTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Command string `json:"command"`
		Dir     string `json:"dir"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	params.Command = normalizeCommandForShell(params.Command)
	if params.Command == "" {
		return nil, fmt.Errorf("empty command")
	}
	logDir := filepath.Join(t.baseDir, ".marcus", "logs")
	_ = os.MkdirAll(logDir, 0755)
	logFile, err := os.CreateTemp(logDir, "bg-*.log")
	if err != nil {
		return nil, err
	}
	logPath := logFile.Name()
	_ = logFile.Close()
	out, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	if filepath.Separator == '\\' {
		cmd = exec.Command("cmd", "/c", params.Command)
	} else {
		cmd = exec.Command("sh", "-c", params.Command)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if params.Dir != "" {
		d, err := resolveToolPath(t.baseDir, params.Dir)
		if err != nil {
			out.Close()
			return nil, err
		}
		cmd.Dir = d
	} else if t.baseDir != "" {
		cmd.Dir = t.baseDir
	}
	if err := cmd.Start(); err != nil {
		out.Close()
		return nil, err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	go func() { _ = cmd.Wait(); _ = out.Close() }()
	return json.Marshal(map[string]any{
		"pid":      pid,
		"log_file": filepath.ToSlash(strings.TrimPrefix(logPath, t.baseDir+string(filepath.Separator))),
		"message":  "command started; read log_file for output",
	})
}

// GitOperationsTool runs a small set of read-only git commands.
type GitOperationsTool struct {
	baseDir string
}

func NewGitOperationsTool(baseDir string) *GitOperationsTool {
	return &GitOperationsTool{baseDir: baseDir}
}

func (t *GitOperationsTool) Name() string { return "git_operations" }

func (t *GitOperationsTool) Description() string {
	return "Run git status, diff, log, branch, or show (read-only; extra args validated)"
}

func (t *GitOperationsTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"subcommand": {Type: "string", Description: "One of: status, diff, log, branch, show"},
			"args":       {Type: "string", Description: "Extra arguments (no ; or newlines)"},
		},
		Required: []string{"subcommand"},
	}
}

var gitAllowed = map[string]bool{
	"status": true, "diff": true, "log": true, "branch": true, "show": true,
}

func (t *GitOperationsTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Subcommand string `json:"subcommand"`
		Args       string `json:"args"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	sub := strings.ToLower(strings.TrimSpace(params.Subcommand))
	if !gitAllowed[sub] {
		return nil, fmt.Errorf("subcommand not allowed: %s", params.Subcommand)
	}
	if strings.ContainsAny(params.Args, ";\n\r") {
		return nil, fmt.Errorf("invalid args")
	}
	args := []string{sub}
	if strings.TrimSpace(params.Args) != "" {
		args = append(args, strings.Fields(params.Args)...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if t.baseDir != "" {
		cmd.Dir = t.baseDir
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return json.Marshal(map[string]any{
		"output":    string(out),
		"exit_code": code,
		"failed":    err != nil && code != 0,
	})
}

// CreateFileTool writes a new file; fails if the path already exists.
type CreateFileTool struct {
	baseDir string
	list    *ListFilesTool
}

func NewCreateFileTool(baseDir string, list *ListFilesTool) *CreateFileTool {
	return &CreateFileTool{baseDir: baseDir, list: list}
}

func (t *CreateFileTool) Name() string { return "create_file" }

func (t *CreateFileTool) Description() string {
	return "Create a new file with content (fails if file already exists)"
}

func (t *CreateFileTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":    {Type: "string", Description: "Relative file path"},
			"content": {Type: "string", Description: "Full file content"},
		},
		Required: []string{"path", "content"},
	}
}

func (t *CreateFileTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("file already exists: %s", params.Path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return nil, err
	}
	if t.list != nil {
		t.list.invalidate()
	}
	return json.Marshal(map[string]any{"path": params.Path, "created": true})
}

// DeleteFileTool removes a file within the project.
type DeleteFileTool struct {
	baseDir string
	list    *ListFilesTool
}

func NewDeleteFileTool(baseDir string, list *ListFilesTool) *DeleteFileTool {
	return &DeleteFileTool{baseDir: baseDir, list: list}
}

func (t *DeleteFileTool) Name() string { return "delete_file" }

func (t *DeleteFileTool) Description() string {
	return "Delete a file (not directories)"
}

func (t *DeleteFileTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path": {Type: "string", Description: "File path"},
		},
		Required: []string{"path"},
	}
}

func (t *DeleteFileTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}
	if t.list != nil {
		t.list.invalidate()
	}
	return json.Marshal(map[string]any{"path": params.Path, "deleted": true})
}

// FetchURLTool fetches HTTP/HTTPS URLs with a size cap.
type FetchURLTool struct{}

func NewFetchURLTool() *FetchURLTool { return &FetchURLTool{} }

func (t *FetchURLTool) Name() string { return "fetch_url" }

func (t *FetchURLTool) Description() string {
	return "Fetch a public http(s) URL (text only, max ~512KiB)"
}

func (t *FetchURLTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"url": {Type: "string", Description: "http or https URL"},
		},
		Required: []string{"url"},
	}
}

func (t *FetchURLTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	u, err := url.Parse(params.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("only http(s) URLs allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	const maxBody = 512 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("response larger than %d bytes", maxBody)
	}
	return json.Marshal(map[string]any{
		"url":         params.URL,
		"status_code": resp.StatusCode,
		"body":        string(body),
	})
}

// TestRunnerTool runs a test command (default: go test ./...).
type TestRunnerTool struct {
	baseDir string
}

func NewTestRunnerTool(baseDir string) *TestRunnerTool {
	return &TestRunnerTool{baseDir: baseDir}
}

func (t *TestRunnerTool) Name() string { return "test_runner" }

func (t *TestRunnerTool) Description() string {
	return "Run tests (default: go test ./... from project root)"
}

func (t *TestRunnerTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"command": {Type: "string", Description: "Override command (optional)"},
			"dir":     {Type: "string", Description: "Working directory (optional)"},
			"timeout_seconds": {Type: "number", Description: "Timeout (default 120)"},
		},
	}
}

func (t *TestRunnerTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Command        string `json:"command"`
		Dir            string `json:"dir"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.TimeoutSeconds <= 0 {
		params.TimeoutSeconds = 120
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(params.TimeoutSeconds)*time.Second)
	defer cancel()
	cmdStr := strings.TrimSpace(params.Command)
	if cmdStr == "" {
		cmdStr = "go test ./..."
	}
	var cmd *exec.Cmd
	if filepath.Separator == '\\' {
		cmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	wd := t.baseDir
	if strings.TrimSpace(params.Dir) != "" {
		var err error
		wd, err = resolveToolPath(t.baseDir, params.Dir)
		if err != nil {
			return nil, err
		}
	}
	cmd.Dir = wd
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return json.Marshal(map[string]any{
		"command":   cmdStr,
		"exit_code": code,
		"output":    string(out),
		"passed":    err == nil || code == 0,
	})
}
