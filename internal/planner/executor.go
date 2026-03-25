package planner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/tool"
)

// PlanExecutor executes plans respecting dependencies
type PlanExecutor struct {
	planner    Planner
	loopEngine *flow.LoopEngine
	toolRunner *tool.ToolRunner
}

// NewPlanExecutor creates a new plan executor
func NewPlanExecutor(planner Planner, engine *flow.LoopEngine, runner *tool.ToolRunner) *PlanExecutor {
	return &PlanExecutor{
		planner:    planner,
		loopEngine: engine,
		toolRunner: runner,
	}
}

// ExecutionResult is the result of executing a plan
type ExecutionResult struct {
	PlanID     string
	Success    bool
	Error      string
	StepResults []StepExecutionResult
	Duration   time.Duration
}

// StepExecutionResult is the result of executing a step
type StepExecutionResult struct {
	StepID    string
	Success   bool
	Error     string
	Output    string
	Duration  time.Duration
}

// Execute executes a plan to completion
func (e *PlanExecutor) Execute(ctx context.Context, plan *Plan) ExecutionResult {
	result := ExecutionResult{
		PlanID:      plan.ID,
		StepResults: make([]StepExecutionResult, 0),
	}

	start := time.Now()

	// Set plan status
	plan.Status = PlanStatusActive

	// Execute until complete or failed
	for !plan.IsComplete() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			result.Error = "execution cancelled"
			return result
		default:
		}

		// Get ready steps
		readySteps := plan.GetReadySteps()
		if len(readySteps) == 0 {
			// Check if any steps are still active
			if !e.hasActiveSteps(plan) {
				// No ready steps and no active steps - we're stuck
				if e.hasFailedSteps(plan) {
					result.Error = "plan failed: some steps failed"
					return result
				}
				break
			}
			// Wait for active steps to complete
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Execute ready steps (concurrently if independent)
		if len(readySteps) == 1 {
			// Sequential execution
			stepResult := e.executeStep(ctx, plan, readySteps[0])
			result.StepResults = append(result.StepResults, stepResult)
		} else {
			// Parallel execution for independent steps
			results := e.executeStepsParallel(ctx, plan, readySteps)
			result.StepResults = append(result.StepResults, results...)
		}
	}

	result.Duration = time.Since(start)

	// Determine overall success
	if plan.IsComplete() {
		result.Success = true
		plan.Status = PlanStatusComplete
	} else {
		result.Success = false
		plan.Status = PlanStatusFailed
	}

	return result
}

// executeStep executes a single step
func (e *PlanExecutor) executeStep(ctx context.Context, plan *Plan, step PlanStep) StepExecutionResult {
	result := StepExecutionResult{StepID: step.ID}
	start := time.Now()

	// Update step status
	for i := range plan.Steps {
		if plan.Steps[i].ID == step.ID {
			plan.Steps[i].Status = StepStatusActive
			break
		}
	}

	// Execute based on tool hints or general execution
	if len(step.ToolHints) > 0 && e.toolRunner != nil {
		// Execute tool hints
		for _, toolName := range step.ToolHints {
			// This is a simplified version - real implementation would parse tool input from step description
			output, err := e.executeTool(ctx, toolName, step.Description)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				plan.MarkStepFailed(step.ID, err)
				return result
			}
			result.Output += output + "\n"
		}
	} else if e.loopEngine != nil {
		// Use loop engine for complex steps
		goal := step.Description
		if goal == "" {
			goal = fmt.Sprintf("Execute step %s", step.ID)
		}

		state, err := e.loopEngine.Run(ctx, goal, "", 10)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			plan.MarkStepFailed(step.ID, err)
			return result
		}

		result.Output = fmt.Sprintf("Loop completed: %s", state.FinishReason)
	}

	result.Duration = time.Since(start)
	result.Success = true

	// Update plan
	plan.MarkStepDone(step.ID, StepResult{
		Success:   result.Success,
		Output:    result.Output,
		Error:     result.Error,
		Duration:  result.Duration,
		Timestamp: time.Now(),
	})

	return result
}

// executeStepsParallel executes multiple steps in parallel
func (e *PlanExecutor) executeStepsParallel(ctx context.Context, plan *Plan, steps []PlanStep) []StepExecutionResult {
	results := make([]StepExecutionResult, len(steps))
	var wg sync.WaitGroup

	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s PlanStep) {
			defer wg.Done()
			results[idx] = e.executeStep(ctx, plan, s)
		}(i, step)
	}

	wg.Wait()
	return results
}

// executeTool executes a tool with inferred input from description
func (e *PlanExecutor) executeTool(ctx context.Context, toolName, description string) (string, error) {
	if e.toolRunner == nil {
		return "", fmt.Errorf("no tool runner configured")
	}

	// This is simplified - real implementation would parse tool-specific input from description
	input := []byte(`{}`) // Empty input
	output, err := e.toolRunner.Run(ctx, toolName, input)
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// hasActiveSteps checks if any steps are currently active
func (e *PlanExecutor) hasActiveSteps(plan *Plan) bool {
	for _, step := range plan.Steps {
		if step.Status == StepStatusActive {
			return true
		}
	}
	return false
}

// hasFailedSteps checks if any steps have failed
func (e *PlanExecutor) hasFailedSteps(plan *Plan) bool {
	for _, step := range plan.Steps {
		if step.Status == StepStatusFailed {
			return true
		}
	}
	return false
}

// ExecuteWithRevisions executes a plan with automatic revision on failure
func (e *PlanExecutor) ExecuteWithRevisions(ctx context.Context, plan *Plan, maxRevisions int) ExecutionResult {
	result := e.Execute(ctx, plan)

	if result.Success {
		return result
	}

	// Try to revise on failure
	for revision := 0; revision < maxRevisions; revision++ {
		// Find failed step
		var failedStep *PlanStep
		for i := range plan.Steps {
			if plan.Steps[i].Status == StepStatusFailed {
				failedStep = &plan.Steps[i]
				break
			}
		}

		if failedStep == nil {
			break
		}

		// Create feedback
		feedback := Feedback{
			StepID:    failedStep.ID,
			Issue:     "Step failed: " + failedStep.Description,
			Suggestion: "Revise approach for this step",
		}

		// Revise plan
		revised, err := e.planner.Revise(ctx, *plan, feedback)
		if err != nil {
			result.Error = fmt.Sprintf("execution failed and revision failed: %v", err)
			return result
		}

		// Execute revised plan
		plan = revised
		result = e.Execute(ctx, plan)

		if result.Success {
			return result
		}
	}

	result.Error = fmt.Sprintf("execution failed after %d revision attempts", maxRevisions)
	return result
}

// Resume resumes execution of a partially completed plan
func (e *PlanExecutor) Resume(ctx context.Context, plan *Plan) ExecutionResult {
	// Mark failed steps as pending so they can be retried
	for i := range plan.Steps {
		if plan.Steps[i].Status == StepStatusFailed {
			plan.Steps[i].Status = StepStatusPending
			plan.Steps[i].Result = nil
		}
	}

	return e.Execute(ctx, plan)
}

// ValidatePlan validates a plan before execution
func (e *PlanExecutor) ValidatePlan(plan *Plan) (ValidationResult, error) {
	return e.planner.Validate(*plan)
}

// PlanStore stores and retrieves plans
type PlanStore interface {
	Save(plan *Plan) error
	Load(id string) (*Plan, error)
	List() ([]string, error)
	Delete(id string) error
}

// InMemoryPlanStore is an in-memory plan store
type InMemoryPlanStore struct {
	mu     sync.RWMutex
	plans  map[string]*Plan
}

// NewInMemoryPlanStore creates a new in-memory plan store
func NewInMemoryPlanStore() PlanStore {
	return &InMemoryPlanStore{
		plans: make(map[string]*Plan),
	}
}

// Save saves a plan
func (s *InMemoryPlanStore) Save(plan *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.ID] = plan
	return nil
}

// Load loads a plan
func (s *InMemoryPlanStore) Load(id string) (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[id]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", id)
	}
	return plan, nil
}

// List lists all plan IDs
func (s *InMemoryPlanStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.plans))
	for id := range s.plans {
		ids = append(ids, id)
	}
	return ids, nil
}

// Delete deletes a plan
func (s *InMemoryPlanStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, id)
	return nil
}
