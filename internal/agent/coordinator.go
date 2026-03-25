package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Coordinator manages multi-agent workflows
type Coordinator struct {
	registry *InMemoryAgentRegistry
	pool     *AgentPool
	config   CoordinatorConfig
}

// CoordinatorConfig configures the coordinator
type CoordinatorConfig struct {
	MaxParallel    int           `json:"max_parallel"`
	ResultTimeout  time.Duration `json:"result_timeout"`
	AutoSynthesize bool        `json:"auto_synthesize"`
}

// DefaultCoordinatorConfig returns default config
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		MaxParallel:    4,
		ResultTimeout:  5 * time.Minute,
		AutoSynthesize: true,
	}
}

// NewCoordinator creates a new coordinator
func NewCoordinator(registry *InMemoryAgentRegistry, config CoordinatorConfig) *Coordinator {
	return &Coordinator{
		registry: registry,
		pool:     NewAgentPool(config.MaxParallel),
		config:   config,
	}
}

// ExecuteParallel executes multiple agents in parallel
func (c *Coordinator) ExecuteParallel(ctx context.Context, tasks []AgentTask) ParallelExecutionResult {
	result := ParallelExecutionResult{
		Results: make([]AgentResult, 0, len(tasks)),
	}

	start := time.Now()

	// Spawn agents for each task
	var wg sync.WaitGroup
	resultsCh := make(chan agentTaskResult, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(t AgentTask) {
			defer wg.Done()

			// Submit to pool for execution
			poolResult := c.pool.Submit(ctx, t, c.registry)
			resultsCh <- poolResult
		}(task)
	}

	// Wait for all to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	var agents []string
	for r := range resultsCh {
		agents = append(agents, r.AgentID)
		if r.Result != nil {
			result.Results = append(result.Results, *r.Result)
		}
		if r.Error != "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Agent %s: %s", r.AgentID, r.Error))
		}
	}

	result.Duration = time.Since(start)
	result.AgentIDs = agents

	// Synthesize results if enabled
	if c.config.AutoSynthesize {
		synthesis := c.synthesizeResults(result.Results)
		result.Synthesis = synthesis
	}

	return result
}

// ExecuteSequential executes agents sequentially
func (c *Coordinator) ExecuteSequential(ctx context.Context, tasks []AgentTask) SequentialExecutionResult {
	result := SequentialExecutionResult{
		Results: make([]AgentResult, 0, len(tasks)),
	}

	start := time.Now()

	for _, task := range tasks {
		// Execute each task in sequence
		poolResult := c.pool.Submit(ctx, task, c.registry)

		if poolResult.Error != "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Agent %s: %s", poolResult.AgentID, poolResult.Error))
			break // Stop on error
		}

		if poolResult.Result != nil {
			result.Results = append(result.Results, *poolResult.Result)

			// Pass previous result to next task if applicable
			if task.PassPreviousResult && len(result.Results) > 1 {
				task.Context["previous_result"] = result.Results[len(result.Results)-1]
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}

// ExecuteWorkflow executes a multi-agent workflow
func (c *Coordinator) ExecuteWorkflow(ctx context.Context, workflow Workflow) WorkflowResult {
	result := WorkflowResult{
		StepResults: make(map[string]AgentResult),
	}

	start := time.Now()

	// Build execution graph
	executed := make(map[string]bool)
	inProgress := make(map[string]bool)

	for !c.isWorkflowComplete(workflow, executed) {
		// Find ready steps
		readySteps := c.getReadySteps(workflow, executed, inProgress)

		if len(readySteps) == 0 && len(inProgress) == 0 {
			// Deadlock - no ready steps and nothing in progress
			result.Error = "workflow deadlock: no steps can proceed"
			break
		}

		// Execute ready steps
		var wg sync.WaitGroup
		stepResults := make(chan stepExecution, len(readySteps))

		for _, step := range readySteps {
			inProgress[step.ID] = true
			wg.Add(1)

			go func(s WorkflowStep) {
				defer wg.Done()

				// Create task for this step
				task := AgentTask{
					Role:    s.Role,
					Goal:    s.Goal,
					Context: s.Context,
				}

				// Add context from previous steps
				for _, depID := range s.Dependencies {
					if depResult, ok := result.StepResults[depID]; ok {
						task.Context["dep_"+depID] = depResult
					}
				}

				poolResult := c.pool.Submit(ctx, task, c.registry)
				stepResults <- stepExecution{StepID: s.ID, Result: poolResult}
			}(step)
		}

		// Wait for steps to complete
		go func() {
			wg.Wait()
			close(stepResults)
		}()

		// Collect results
		for sr := range stepResults {
			delete(inProgress, sr.StepID)
			executed[sr.StepID] = true

			if sr.Result.Error != "" {
				result.StepResults[sr.StepID] = AgentResult{
					Success: false,
					Error:   sr.Result.Error,
				}
			} else if sr.Result.Result != nil {
				result.StepResults[sr.StepID] = *sr.Result.Result
			}
		}
	}

	result.Duration = time.Since(start)
	result.Success = len(result.Errors) == 0 && c.isWorkflowComplete(workflow, executed)

	return result
}

// stepExecution represents a workflow step execution result
type stepExecution struct {
	StepID string
	Result agentTaskResult
}

// isWorkflowComplete checks if all workflow steps are complete
func (c *Coordinator) isWorkflowComplete(workflow Workflow, executed map[string]bool) bool {
	for _, step := range workflow.Steps {
		if !executed[step.ID] {
			return false
		}
	}
	return true
}

// getReadySteps returns steps that can be executed
func (c *Coordinator) getReadySteps(workflow Workflow, executed, inProgress map[string]bool) []WorkflowStep {
	var ready []WorkflowStep

	for _, step := range workflow.Steps {
		if executed[step.ID] || inProgress[step.ID] {
			continue
		}

		// Check if dependencies are satisfied
		depsSatisfied := true
		for _, dep := range step.Dependencies {
			if !executed[dep] {
				depsSatisfied = false
				break
			}
		}

		if depsSatisfied {
			ready = append(ready, step)
		}
	}

	return ready
}

// synthesizeResults synthesizes multiple agent results
func (c *Coordinator) synthesizeResults(results []AgentResult) string {
	if len(results) == 0 {
		return "No results to synthesize"
	}

	if len(results) == 1 {
		return results[0].Summary
	}

	// Simple synthesis - combine summaries
	var summaries []string
	successCount := 0

	for _, r := range results {
		if r.Success {
			successCount++
			summaries = append(summaries, r.Summary)
		}
	}

	synthesis := fmt.Sprintf("Synthesized %d agent results (%d successful):\n\n", len(results), successCount)
	for i, summary := range summaries {
		synthesis += fmt.Sprintf("[%d] %s\n", i+1, summary)
	}

	return synthesis
}

// AgentTask represents a task for an agent
type AgentTask struct {
	Role               string
	Goal               string
	Context            map[string]any
	PassPreviousResult bool
}

// ParallelExecutionResult is the result of parallel execution
type ParallelExecutionResult struct {
	AgentIDs    []string      `json:"agent_ids"`
	Results     []AgentResult `json:"results"`
	Synthesis   string        `json:"synthesis,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
	Duration    time.Duration `json:"duration"`
}

// SequentialExecutionResult is the result of sequential execution
type SequentialExecutionResult struct {
	Results  []AgentResult `json:"results"`
	Errors   []string      `json:"errors,omitempty"`
	Duration time.Duration `json:"duration"`
}

// Workflow is a multi-step workflow
type Workflow struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Steps []WorkflowStep `json:"steps"`
}

// WorkflowStep is a single step in a workflow
type WorkflowStep struct {
	ID          string         `json:"id"`
	Role        string         `json:"role"`
	Goal        string         `json:"goal"`
	Dependencies []string      `json:"dependencies,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
}

// WorkflowResult is the result of executing a workflow
type WorkflowResult struct {
	StepResults map[string]AgentResult `json:"step_results"`
	Success     bool                     `json:"success"`
	Errors      []string                 `json:"errors,omitempty"`
	Error       string                   `json:"error,omitempty"`
	Duration    time.Duration            `json:"duration"`
}

// AgentPool manages a pool of agent execution slots
type AgentPool struct {
	maxParallel int
	semaphore   chan struct{}
}

// agentTaskResult is the result of executing an agent task
type agentTaskResult struct {
	AgentID string
	Result  *AgentResult
	Error   string
}

// NewAgentPool creates a new agent pool
func NewAgentPool(maxParallel int) *AgentPool {
	if maxParallel <= 0 {
		maxParallel = 4
	}
	return &AgentPool{
		maxParallel: maxParallel,
		semaphore:   make(chan struct{}, maxParallel),
	}
}

// Submit submits a task for execution
func (p *AgentPool) Submit(ctx context.Context, task AgentTask, registry *InMemoryAgentRegistry) agentTaskResult {
	// Acquire semaphore
	select {
	case p.semaphore <- struct{}{}:
	case <-ctx.Done():
		return agentTaskResult{Error: ctx.Err().Error()}
	}
	defer func() { <-p.semaphore }()

	// Spawn agent
	instance, err := registry.Spawn(task.Role, task.Goal, "", SpawnOptions{
		Context: task.Context,
	})
	if err != nil {
		return agentTaskResult{Error: err.Error()}
	}

	// Wait for completion
	timeout := 5 * time.Minute
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			registry.Kill(instance.ID)
			return agentTaskResult{
				AgentID: instance.ID,
				Error:   "timeout",
			}
		case <-ticker.C:
			if agent, ok := registry.Get(instance.ID); ok {
				if agent.Status == "completed" || agent.Status == "failed" {
					if agent.Result != nil {
						return agentTaskResult{
							AgentID: instance.ID,
							Result:  agent.Result,
						}
					}
					return agentTaskResult{
						AgentID: instance.ID,
						Error:   agent.Status,
					}
				}
			}
		case <-ctx.Done():
			registry.Kill(instance.ID)
			return agentTaskResult{Error: ctx.Err().Error()}
		}
	}
}
