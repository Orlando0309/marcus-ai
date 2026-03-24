package flow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

// Engine is the main flow execution engine
type Engine struct {
	folderEngine *folder.FolderEngine
	executor     *FlowExecutor
	cfg          *config.Config
	notify       func(string)
}

// NewEngine creates a new flow engine
func NewEngine(cfg *config.Config, notify func(string)) (*Engine, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	// Determine project .marcus path
	projectMarcusPath := ""
	if cfg.ProjectRoot != "" {
		projectMarcusPath = filepath.Join(cfg.ProjectRoot, ".marcus")
	}

	fe := folder.NewFolderEngine(
		filepath.Join(homeDir, ".marcus"),
		projectMarcusPath,
		notify,
	)

	if err := fe.Boot(); err != nil {
		return nil, fmt.Errorf("folder engine boot: %w", err)
	}

	baseDir := cfg.ProjectRoot
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	executor := NewFlowExecutor(fe, cfg, baseDir, notify)

	return &Engine{
		folderEngine: fe,
		executor:     executor,
		cfg:          cfg,
		notify:       notify,
	}, nil
}

// Run executes a flow with the given input
func (e *Engine) Run(ctx context.Context, flowName string, input map[string]interface{}) (*ExecuteResult, error) {
	data := make(TemplateData)
	for k, v := range input {
		data[k] = v
	}

	return e.executor.Execute(ctx, flowName, data)
}

// RunWithFile executes a flow for a specific file
func (e *Engine) RunWithFile(ctx context.Context, flowName, filePath, instruction string) (*ExecuteResult, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	data := TemplateData{
		"file":        filePath,
		"content":     string(content),
		"instruction": instruction,
	}

	return e.executor.Execute(ctx, flowName, data)
}

// ListFlows returns all available flows
func (e *Engine) ListFlows() []string {
	return e.folderEngine.ListFlows()
}

// GetFlow returns a flow by name
func (e *Engine) GetFlow(name string) (*folder.FlowDef, bool) {
	return e.folderEngine.GetFlow(name)
}

// ListTools returns all available tools
func (e *Engine) ListTools() []string {
	return e.folderEngine.ListTools()
}

// FolderEngine exposes the discovered folder registry/runtime.
func (e *Engine) FolderEngine() *folder.FolderEngine {
	return e.folderEngine
}

// Stream executes a flow and streams the output
func (e *Engine) Stream(ctx context.Context, flowName string, input TemplateData, onChunk func(string)) error {
	flow, ok := e.folderEngine.GetFlow(flowName)
	if !ok {
		return fmt.Errorf("flow not found: %s", flowName)
	}

	// Read and render prompt
	prompt, err := flow.ReadPrompt("")
	if err != nil {
		return fmt.Errorf("read prompt: %w", err)
	}

	renderedPrompt, err := NewTemplateEngine().Render(prompt, input)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	pname := strings.TrimSpace(flow.Model.Provider)
	if pname == "" {
		pname = e.cfg.Provider
	}
	prov, err := provider.Stack(pname, flow.Model.Model, e.cfg.ProviderFallbacks)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}

	// Stream the response
	opts := provider.CompletionOptions{
		Model:       flow.Model.Model,
		Temperature: flow.Model.Temperature,
		MaxTokens:   flow.Model.MaxTokens,
	}

	stream, err := prov.CompleteStream(ctx, renderedPrompt, opts)
	if err != nil {
		return fmt.Errorf("provider stream: %w", err)
	}

	for chunk := range stream {
		if chunk.Done {
			break
		}
		if onChunk != nil && chunk.Text != "" {
			onChunk(chunk.Text)
		}
	}

	return nil
}

// LoopEngine returns a new LoopEngine wired to this engine's dependencies.
func (e *Engine) LoopEngine(toolRunner *tool.ToolRunner, taskStore *task.Store, mem *memory.Manager, contextAsm ContextAssembler, prov *provider.Runtime) *LoopEngine {
	return NewLoopEngine(e, e.executor, taskStore, mem, contextAsm, toolRunner, prov, e.cfg, e.cfg.ProjectRoot)
}

// Reload refreshes the folder engine (hot-reload)
func (e *Engine) Reload() error {
	return e.folderEngine.Boot()
}
