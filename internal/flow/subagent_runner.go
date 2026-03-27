package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/isolation"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

type subagentRunner struct {
	folders *folder.FolderEngine
	cfg     *config.Config
	baseDir string
}

func NewSubagentRunner(folders *folder.FolderEngine, cfg *config.Config, baseDir string) tool.SubagentRunner {
	return &subagentRunner{folders: folders, cfg: cfg, baseDir: baseDir}
}

func (r *subagentRunner) RunSubagent(ctx context.Context, req tool.SubagentRequest) (*tool.SubagentResult, error) {
	cfg := r.cfg
	if cfg == nil {
		cfg = config.DefaultConfig()
		cfg.ProjectRoot = r.baseDir
	}
	root := r.baseDir
	iso := isolation.NewManager(root, cfg.Isolation)
	isoSession := &isolation.Session{Mode: isolation.ModeInPlace, Root: root, Name: "main"}
	if req.UseIsolation {
		prepared, err := iso.Prepare(ctx, []tool.ActionProposal{{Type: "write_file", Path: ".marcus/subagent.tmp"}})
		if err != nil {
			return nil, err
		}
		isoSession = prepared
	}
	mem := memory.NewManager(isoSession.Root, cfg.Memory.RecallLimit)
	_ = mem.EnsureStructure()
	taskStore := task.NewStore(isoSession.Root)
	_ = taskStore.EnsureStructure()
	prov, err := provider.Stack(cfg.Provider, cfg.Model, cfg.ProviderFallbacks)
	if err != nil {
		return nil, err
	}
	runner, err := tool.BuildRunner(tool.BuildOptions{
		BaseDir:        isoSession.Root,
		Config:         cfg,
		Folders:        r.folders,
		SubagentRunner: nil,
	})
	if err != nil {
		return nil, err
	}
	if len(req.AllowedTools) > 0 {
		runner = runner.Scoped(req.AllowedTools)
	} else if agent, ok := r.folders.GetAgent(req.Agent); ok && len(agent.Tools) > 0 {
		runner = runner.Scoped(agent.Tools)
	}

	maxIterations := req.MaxIterations
	if maxIterations <= 0 {
		maxIterations = cfg.Autonomy.MaxIterations / 4
		if maxIterations < 1 {
			maxIterations = 1
		}
		if maxIterations == 0 {
			maxIterations = 8
		}
	}
	toolResults := []string{}
	finish := "done"
	taskStatus := ""
	iterations := 0
	var actionLabels []string
	var lastMessage string
	for i := 0; i < maxIterations; i++ {
		iterations = i + 1
		prompt := buildSubagentPrompt(req.Goal, mem.Summary(req.Goal, cfg.Memory.RecallLimit), taskStore.Summary(), toolResults)
		resp, err := provider.NewRuntime(prov, isoSession.Root, cfg.ProviderCfg.CacheEnabled).Complete(ctx, provider.Request{
			Model:       cfg.Model,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
			JSON:        true,
			Messages: []provider.Message{
				{Role: "system", Content: buildSubagentSystemPrompt(req.Agent)},
				{Role: "user", Content: prompt},
			},
			Tools:     toolSpecs(runner),
			Reasoning: provider.ReasoningOptions{Effort: cfg.Reasoning.Effort, BudgetTokens: cfg.Reasoning.BudgetTokens},
		})
		if err != nil {
			return nil, err
		}
		env := parseSubagentEnvelope(resp.Text)
		lastMessage = strings.TrimSpace(env.Message)
		if len(env.Tasks) > 0 {
			applied, err := taskStore.ApplyUpdates(env.Tasks)
			if err == nil && len(applied) > 0 {
				taskStatus = applied[len(applied)-1].Status
			}
		}
		if len(env.Actions) == 0 {
			if lastMessage == "" {
				lastMessage = "Subagent completed without further actions."
			}
			break
		}
		actionLabels = actionLabels[:0]
		for _, action := range env.Actions {
			actionLabels = append(actionLabels, action.Label())
			result, err := runner.ApplyAction(ctx, action)
			if err != nil {
				toolResults = append(toolResults, fmt.Sprintf("Tool %s failed: %v", action.Label(), err))
				finish = "error"
				lastMessage = fmt.Sprintf("Subagent failed while running %s: %v", action.Label(), err)
				return &tool.SubagentResult{
					Summary:      lastMessage,
					Iterations:   iterations,
					FinishReason: finish,
					TaskStatus:   taskStatus,
					Actions:      append([]string(nil), actionLabels...),
					Workspace:    isoSession.Root,
				}, nil
			}
			out := strings.TrimSpace(result.Output)
			if out == "" {
				out = strings.TrimSpace(result.Summary)
			}
			toolResults = append(toolResults, fmt.Sprintf("Tool %s\n%s", action.Label(), out))
		}
	}
	if lastMessage == "" {
		lastMessage = "Subagent completed."
	}
	return &tool.SubagentResult{
		Summary:      lastMessage,
		Iterations:   iterations,
		FinishReason: finish,
		TaskStatus:   taskStatus,
		Actions:      append([]string(nil), actionLabels...),
		Workspace:    isoSession.Root,
	}, nil
}

type subagentEnvelope struct {
	Message string                `json:"message"`
	Actions []tool.ActionProposal `json:"actions"`
	Tasks   []task.Update         `json:"tasks"`
}

func parseSubagentEnvelope(raw string) subagentEnvelope {
	var env subagentEnvelope
	text := strings.TrimSpace(raw)
	if start := strings.Index(text, "{"); start != -1 {
		if end := strings.LastIndex(text, "}"); end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &env)
		}
	}
	if env.Message == "" && len(env.Actions) == 0 && len(env.Tasks) == 0 {
		env.Message = text
	}
	return env
}

func buildSubagentSystemPrompt(agent string) string {
	base := `You are Marcus running as an isolated subagent. Return JSON with "message", "actions", and "tasks". Keep "message" concise. Use actions to inspect or modify the workspace until the delegated goal is complete. Mark tasks active, done, or blocked.`
	if strings.TrimSpace(agent) != "" {
		base += "\nAgent profile: " + strings.TrimSpace(agent)
	}
	return base
}

func buildSubagentPrompt(goal, memorySummary, taskSummary string, toolResults []string) string {
	var parts []string
	parts = append(parts, "Delegated Goal:\n"+strings.TrimSpace(goal))
	if strings.TrimSpace(memorySummary) != "" {
		parts = append(parts, "Memory:\n"+memorySummary)
	}
	if strings.TrimSpace(taskSummary) != "" {
		parts = append(parts, "Tasks:\n"+taskSummary)
	}
	if len(toolResults) > 0 {
		parts = append(parts, "Tool Results:\n"+strings.Join(toolResults, "\n\n"))
	}
	return strings.Join(parts, "\n\n")
}
