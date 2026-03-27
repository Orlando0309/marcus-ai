package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

type SubagentRequest struct {
	Goal          string   `json:"goal"`
	Agent         string   `json:"agent,omitempty"`
	MaxIterations int      `json:"max_iterations,omitempty"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`
	UseIsolation  bool     `json:"use_isolation,omitempty"`
}

type SubagentResult struct {
	Summary      string   `json:"summary"`
	Iterations   int      `json:"iterations"`
	FinishReason string   `json:"finish_reason"`
	TaskStatus   string   `json:"task_status,omitempty"`
	Actions      []string `json:"actions,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
}

type SubagentRunner interface {
	RunSubagent(ctx context.Context, req SubagentRequest) (*SubagentResult, error)
}

type TaskTool struct {
	runner SubagentRunner
}

func NewTaskTool(runner SubagentRunner) *TaskTool {
	return &TaskTool{runner: runner}
}

func (t *TaskTool) Name() string { return "task_tool" }

func (t *TaskTool) Description() string {
	return "Run an isolated subagent loop and return a concise summary"
}

func (t *TaskTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"goal":           {Type: "string", Description: "The delegated subtask goal"},
			"agent":          {Type: "string", Description: "Optional agent profile name"},
			"max_iterations": {Type: "number", Description: "Optional iteration cap"},
			"use_isolation":  {Type: "boolean", Description: "Whether to prefer isolated execution"},
		},
		Required: []string{"goal"},
	}
}

func (t *TaskTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.runner == nil {
		return nil, fmt.Errorf("task_tool is unavailable: no subagent runner configured")
	}
	var req SubagentRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	result, err := t.runner.RunSubagent(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
