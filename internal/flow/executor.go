package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/marcus-ai/marcus/internal/codeintel"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/lsp"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/tool"
)

// FlowExecutor executes flows
type FlowExecutor struct {
	folderEngine *folder.FolderEngine
	toolRunner   *tool.ToolRunner
	templateEng  *TemplateEngine
	baseDir      string
	notify       func(string)
	codeIndex    *codeintel.Index
	lspBroker    *lsp.Broker
	cfg          *config.Config
}

// NewFlowExecutor creates a new flow executor
func NewFlowExecutor(fe *folder.FolderEngine, cfg *config.Config, baseDir string, notify func(string)) *FlowExecutor {
	index := codeintel.NewIndex(baseDir)
	_ = index.Build(context.Background())
	broker := lsp.NewBroker(cfg.LSP, baseDir)
	tr, _ := tool.BuildRunner(tool.BuildOptions{
		BaseDir:   baseDir,
		Config:    cfg,
		Folders:   fe,
		CodeIndex: index,
		LSP:       broker,
	})
	return &FlowExecutor{
		folderEngine: fe,
		toolRunner:   tr,
		templateEng:  NewTemplateEngine(),
		baseDir:      baseDir,
		notify:       notify,
		codeIndex:    index,
		lspBroker:    broker,
		cfg:          cfg,
	}
}

// ExecuteResult holds the result of a flow execution
type ExecuteResult struct {
	Success      bool
	Output       string
	Usage        provider.Usage
	ToolCalls    []provider.ToolCall
	FinishReason string
}

// Execute runs a flow with the given input data
func (fe *FlowExecutor) Execute(ctx context.Context, flowName string, data TemplateData) (*ExecuteResult, error) {
	flow, ok := fe.folderEngine.GetFlow(flowName)
	if !ok {
		return nil, fmt.Errorf("flow not found: %s", flowName)
	}

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Executing flow: %s", flow.Name))
	}

	// Read prompt template
	prompt, err := flow.ReadPrompt("")
	if err != nil {
		return nil, fmt.Errorf("read prompt: %w", err)
	}

	// Validate and render template
	if err := fe.templateEng.ValidateTemplate(prompt, flow.Input.Requires, data); err != nil {
		return nil, fmt.Errorf("validate template: %w", err)
	}

	renderedPrompt, err := fe.templateEng.Render(prompt, data)
	if err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	// Get provider
	prov, err := fe.getProvider(flow.Model.Provider, flow.Model.Model)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Execute based on streaming setting
	if flow.Behavior.Stream {
		return fe.executeStreaming(ctx, prov, renderedPrompt, flow)
	}
	return fe.executeBlocking(ctx, prov, renderedPrompt, flow)
}

// getProvider returns the provider instance based on flow config
func (fe *FlowExecutor) getProvider(providerName string, modelName string) (provider.Provider, error) {
	name := strings.TrimSpace(providerName)
	if name == "" && fe.cfg != nil {
		name = fe.cfg.Provider
	}
	var fb []string
	if fe.cfg != nil {
		fb = fe.cfg.ProviderFallbacks
	}
	return provider.Stack(name, modelName, fb)
}

// executeBlocking executes a non-streaming flow
func (fe *FlowExecutor) executeBlocking(ctx context.Context, prov provider.Provider, prompt string, flow *folder.FlowDef) (*ExecuteResult, error) {
	opts := provider.Request{
		Model:       flow.Model.Model,
		Temperature: flow.Model.Temperature,
		MaxTokens:   flow.Model.MaxTokens,
		Messages:    []provider.Message{{Role: "user", Content: prompt}},
		Tools:       toolSpecs(fe.toolRunner.Scoped(flow.Tools)),
	}

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Calling %s (%s)...", prov.Name(), opts.Model))
	}
	runtime := provider.NewRuntime(prov, fe.baseDir, fe.cfg != nil && fe.cfg.ProviderCfg.CacheEnabled)
	resp, err := runtime.Complete(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("provider complete: %w", err)
	}

	result := &ExecuteResult{
		Success:      true,
		Output:       resp.Text,
		Usage:        resp.Usage,
		ToolCalls:    resp.ToolCalls,
		FinishReason: resp.FinishReason,
	}

	// Execute tool calls if any
	runner := fe.toolRunner.Scoped(flow.Tools)
	if len(resp.ToolCalls) > 0 {
		for _, tc := range resp.ToolCalls {
			if _, err := runner.Run(ctx, tc.Name, tc.Input); err != nil {
				return nil, fmt.Errorf("execute tool %s: %w", tc.Name, err)
			}
			if fe.notify != nil {
				fe.notify(fmt.Sprintf("Tool %s executed", tc.Name))
			}
		}
	}

	return result, nil
}

// executeStreaming executes a streaming flow
func (fe *FlowExecutor) executeStreaming(ctx context.Context, prov provider.Provider, prompt string, flow *folder.FlowDef) (*ExecuteResult, error) {
	opts := provider.Request{
		Model:       flow.Model.Model,
		Temperature: flow.Model.Temperature,
		MaxTokens:   flow.Model.MaxTokens,
		Messages:    []provider.Message{{Role: "user", Content: prompt}},
		Tools:       toolSpecs(fe.toolRunner.Scoped(flow.Tools)),
	}

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Streaming from %s (%s)...", prov.Name(), opts.Model))
	}
	runtime := provider.NewRuntime(prov, fe.baseDir, fe.cfg != nil && fe.cfg.ProviderCfg.CacheEnabled)
	stream, err := runtime.Stream(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("provider stream: %w", err)
	}

	var output strings.Builder
	var totalUsage provider.Usage
	var toolCalls []provider.ToolCall

	for chunk := range stream {
		if chunk.Done {
			if chunk.Usage != nil {
				totalUsage.PromptTokens += chunk.Usage.PromptTokens
				totalUsage.CompletionTokens += chunk.Usage.CompletionTokens
				totalUsage.TotalTokens += chunk.Usage.TotalTokens
			}
			break
		}
		output.WriteString(chunk.Text)
		if chunk.ToolCall != nil {
			toolCalls = append(toolCalls, *chunk.ToolCall)
		}
	}

	result := &ExecuteResult{
		Success:   true,
		Output:    output.String(),
		Usage:     totalUsage,
		ToolCalls: toolCalls,
	}

	// Execute tool calls if any
	runner := fe.toolRunner.Scoped(flow.Tools)
	if len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			if _, err := runner.Run(ctx, tc.Name, tc.Input); err != nil {
				return nil, fmt.Errorf("execute tool %s: %w", tc.Name, err)
			}
			if fe.notify != nil {
				fe.notify(fmt.Sprintf("Tool %s executed", tc.Name))
			}
		}
	}

	return result, nil
}

// ExecuteWithTools runs a flow with iterative tool execution and conversation history.
func (fe *FlowExecutor) ExecuteWithTools(ctx context.Context, flowName string, data TemplateData, maxIterations int) (*ExecuteResult, error) {
	flow, ok := fe.folderEngine.GetFlow(flowName)
	if !ok {
		return nil, fmt.Errorf("flow not found: %s", flowName)
	}

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Executing flow with tools: %s", flow.Name))
	}

	// Read and render prompt template
	prompt, err := flow.ReadPrompt("")
	if err != nil {
		return nil, fmt.Errorf("read prompt: %w", err)
	}
	if err := fe.templateEng.ValidateTemplate(prompt, flow.Input.Requires, data); err != nil {
		return nil, fmt.Errorf("validate template: %w", err)
	}
	renderedPrompt, err := fe.templateEng.Render(prompt, data)
	if err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	// Build system prompt
	systemPrompt := fe.buildSystemPrompt(flow)

	// Initialize conversation messages
	messages := []provider.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: renderedPrompt},
	}

	// Get provider and runtime
	prov, err := fe.getProvider(flow.Model.Provider, flow.Model.Model)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}
	runtime := provider.NewRuntime(prov, fe.baseDir, fe.cfg != nil && fe.cfg.ProviderCfg.CacheEnabled)

	runner := fe.toolRunner.Scoped(flow.Tools)
	iter := 0
	var lastText string
	var lastToolCalls []provider.ToolCall
	var totalUsage provider.Usage

	for iter < maxIterations {
		iter++

		if fe.notify != nil {
			fe.notify(fmt.Sprintf("Iteration %d: calling %s...", iter, prov.Name()))
		}

		resp, err := runtime.Complete(ctx, provider.Request{
			Model:       flow.Model.Model,
			Temperature: flow.Model.Temperature,
			MaxTokens:   flow.Model.MaxTokens,
			Messages:    messages,
			Tools:       toolSpecs(runner),
		})
		if err != nil {
			return nil, fmt.Errorf("provider complete: %w", err)
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		lastText = resp.Text
		lastToolCalls = resp.ToolCalls

		// Append assistant message to conversation history
		messages = append(messages, provider.Message{Role: "assistant", Content: resp.Text})

		if len(resp.ToolCalls) == 0 {
			// Done - no more tool calls
			if fe.notify != nil {
				fe.notify("Done - no more tool calls")
			}
			break
		}

		if fe.notify != nil {
			fe.notify(fmt.Sprintf("Executing %d tool call(s)...", len(resp.ToolCalls)))
		}

		// Execute each tool call and append result as a user message
		for _, tc := range resp.ToolCalls {
			raw, err := runner.Run(ctx, tc.Name, tc.Input)
			output := formatToolOutput(tc.Name, raw, err)
			role := "user"
			content := fmt.Sprintf("[TOOL RESULT %s]\n%s", tc.Name, output)
			if err != nil {
				content = fmt.Sprintf("[TOOL ERROR %s]: %v", tc.Name, err)
			}
			messages = append(messages, provider.Message{Role: role, Content: content})
		}
	}

	finishReason := "done"
	if iter >= maxIterations {
		finishReason = "iteration_limit"
	}

	return &ExecuteResult{
		Success:      true,
		Output:       lastText,
		Usage:        totalUsage,
		ToolCalls:    lastToolCalls,
		FinishReason: finishReason,
	}, nil
}

// buildSystemPrompt constructs the system prompt for a flow.
func (fe *FlowExecutor) buildSystemPrompt(flow *folder.FlowDef) string {
	var parts []string
	parts = append(parts, "You are Marcus, a terminal-native coding agent. Read the project map and relevant context first, follow existing patterns, prefer small reviewable diffs, and verify changes before declaring success.")
	if flow.Description != "" {
		parts = append(parts, flow.Description)
	}
	return strings.Join(parts, "\n\n")
}

// formatToolOutput formats tool output for inclusion in conversation history.
func formatToolOutput(toolName string, raw json.RawMessage, err error) string {
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if len(raw) == 0 {
		return "(no output)"
	}
	varpretty, merr := json.MarshalIndent(json.RawMessage(raw), "", "  ")
	if merr != nil {
		return string(raw)
	}
	return string(varpretty)
}

// ApplyDiff applies a unified diff to a file
func ApplyDiff(filePath, diff string, baseDir string) error {
	// Parse the diff and apply changes
	// This is a simplified implementation - production would use a proper diff parser

	lines := strings.Split(diff, "\n")
	var originalLines, newLines []string

	inHunk := false
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case strings.HasPrefix(line, "-"):
			if inHunk {
				originalLines = append(originalLines, line[1:])
			}
		case strings.HasPrefix(line, "+"):
			if inHunk {
				newLines = append(newLines, line[1:])
			}
		case strings.HasPrefix(line, " "):
			// Context line - keep as is
		}
	}

	// Read original file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Simple replacement strategy - replace first occurrence of each original line
	originalContent := string(content)
	for i, orig := range originalLines {
		if i < len(newLines) {
			originalContent = strings.Replace(originalContent, orig, newLines[i], 1)
		}
	}

	// Write back
	path := filePath
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, filePath)
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(path, []byte(originalContent), 0644)
}

// RunShellTool executes a shell-based tool from a folder
func RunShellTool(ctx context.Context, toolPath, scriptName string, input json.RawMessage) (json.RawMessage, error) {
	var shellCmd string
	if runtime.GOOS == "windows" {
		shellCmd = "cmd"
	} else {
		shellCmd = "sh"
	}

	scriptPath := filepath.Join(toolPath, scriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("script not found: %s", scriptPath)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		os.Chmod(scriptPath, 0755)
	}

	// Write input to stdin
	cmd := exec.CommandContext(ctx, shellCmd, scriptPath)
	cmd.Stdin = strings.NewReader(string(input))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run shell tool: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"output": string(output),
	})
}

func toolSpecs(runner *tool.ToolRunner) []provider.ToolSpec {
	if runner == nil {
		return nil
	}
	defs := runner.Definitions()
	specs := make([]provider.ToolSpec, 0, len(defs))
	for _, def := range defs {
		raw, _ := json.Marshal(def.Schema)
		specs = append(specs, provider.ToolSpec{
			Name:        def.Name,
			Description: def.Description,
			Schema:      raw,
		})
	}
	return specs
}
