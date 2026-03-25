package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/task"
)

// Planner creates execution plans for goals
type Planner interface {
	Analyze(ctx context.Context, goal string, context map[string]any) (*Plan, error)
	Decompose(ctx context.Context, task Task) ([]Subtask, error)
	Validate(plan Plan) (ValidationResult, error)
	Revise(ctx context.Context, plan Plan, feedback Feedback) (*Plan, error)
}

// Plan represents a structured execution plan
type Plan struct {
	ID            string                 `json:"id"`
	Goal          string                 `json:"goal"`
	Description   string                 `json:"description,omitempty"`
	Steps         []PlanStep             `json:"steps"`
	DAG           *task.DAG              `json:"dag,omitempty"`
	Estimates     map[string]time.Duration `json:"estimates,omitempty"`
	Risks         []Risk                 `json:"risks,omitempty"`
	Fallbacks     []Plan                 `json:"fallbacks,omitempty"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// PlanStep is a single step in a plan
type PlanStep struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Status       string    `json:"status"` // pending, active, done, failed
	Result       *StepResult `json:"result,omitempty"`
	ToolHints    []string  `json:"tool_hints,omitempty"` // Suggested tools
	AutoExecute  bool      `json:"auto_execute"` // Can execute without user confirmation
}

// StepResult is the result of executing a step
type StepResult struct {
	Success   bool      `json:"success"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time `json:"timestamp"`
}

// Task represents a decomposable task
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Complexity  string `json:"complexity,omitempty"` // simple, moderate, complex
}

// Subtask is a subtask from decomposition
type Subtask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ParentID    string   `json:"parent_id,omitempty"`
	Order       int      `json:"order"`
}

// Risk represents a risk in the plan
type Risk struct {
	Description string `json:"description"`
	Severity    string `json:"severity"` // low, medium, high
	Mitigation  string `json:"mitigation,omitempty"`
}

// ValidationResult is the result of validating a plan
type ValidationResult struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Feedback is user/system feedback for plan revision
type Feedback struct {
	StepID      string `json:"step_id,omitempty"`
	StepIndex   int    `json:"step_index,omitempty"`
	Issue       string `json:"issue"`
	Suggestion  string `json:"suggestion,omitempty"`
	UserMessage string `json:"user_message,omitempty"`
}

// LLMPlanner uses an LLM for planning
type LLMPlanner struct {
	provider provider.Provider
}

// NewLLMPlanner creates a new LLM-based planner
func NewLLMPlanner(p provider.Provider) *LLMPlanner {
	return &LLMPlanner{provider: p}
}

// Analyze analyzes a goal and creates a plan
func (p *LLMPlanner) Analyze(ctx context.Context, goal string, context map[string]any) (*Plan, error) {
	if p.provider == nil {
		return nil, fmt.Errorf("no provider configured")
	}

	// Build planning prompt
	prompt := buildPlanningPrompt(goal, context)

	resp, err := p.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   2000,
		JSON:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("planning request failed: %w", err)
	}

	// Parse plan from response
	plan, err := parsePlanFromResponse(resp.Text, goal)
	if err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	// Generate ID and timestamps
	plan.ID = generatePlanID()
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()
	plan.Status = "ready"

	// Build DAG from dependencies
	if len(plan.Steps) > 0 {
		plan.DAG = buildPlanDAG(plan.Steps)
	}

	return plan, nil
}

// Decompose decomposes a task into subtasks
func (p *LLMPlanner) Decompose(ctx context.Context, task Task) ([]Subtask, error) {
	if p.provider == nil {
		return nil, fmt.Errorf("no provider configured")
	}

	prompt := buildTaskDecompositionPrompt(task)

	resp, err := p.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   1500,
		JSON:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("decomposition request failed: %w", err)
	}

	return parseSubtasksFromResponse(resp.Text, task.ID)
}

// Validate validates a plan
func (p *LLMPlanner) Validate(plan Plan) (ValidationResult, error) {
	result := ValidationResult{
		Valid:    true,
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Check for empty steps
	if len(plan.Steps) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "Plan has no steps")
		return result, nil
	}

	// Check for duplicate step IDs
	stepIDs := make(map[string]bool)
	for _, step := range plan.Steps {
		if stepIDs[step.ID] {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Duplicate step ID: %s", step.ID))
		}
		stepIDs[step.ID] = true
	}

	// Check for missing dependencies
	for _, step := range plan.Steps {
		for _, dep := range step.Dependencies {
			if !stepIDs[dep] {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("Step %s has unknown dependency: %s", step.ID, dep))
			}
		}
	}

	// Check for circular dependencies via DAG
	if plan.DAG != nil {
		_, err := plan.DAG.TopologicalOrder()
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "Plan has circular dependencies")
		}
	}

	// Warn about steps without descriptions
	for _, step := range plan.Steps {
		if step.Description == "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Step %s has no description", step.ID))
		}
	}

	return result, nil
}

// Revise revises a plan based on feedback
func (p *LLMPlanner) Revise(ctx context.Context, plan Plan, feedback Feedback) (*Plan, error) {
	if p.provider == nil {
		return nil, fmt.Errorf("no provider configured")
	}

	prompt := buildRevisionPrompt(plan, feedback)

	resp, err := p.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   2000,
		JSON:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("revision request failed: %w", err)
	}

	revisedPlan, err := parsePlanFromResponse(resp.Text, plan.Goal)
	if err != nil {
		return nil, fmt.Errorf("parse revised plan: %w", err)
	}

	// Preserve ID and update timestamps
	revisedPlan.ID = plan.ID
	revisedPlan.CreatedAt = plan.CreatedAt
	revisedPlan.UpdatedAt = time.Now()

	return revisedPlan, nil
}

// buildPlanningPrompt creates a prompt for the planning request
func buildPlanningPrompt(goal string, context map[string]any) string {
	ctxStr := ""
	if len(context) > 0 {
		ctxBytes, _ := json.MarshalIndent(context, "", "  ")
		ctxStr = string(ctxBytes)
	}

	return fmt.Sprintf(`Create a detailed execution plan for the following goal.

Goal: %s

Context: %s

Please provide a JSON response with the following structure:
{
  "description": "Brief description of the approach",
  "steps": [
    {
      "id": "step-1",
      "description": "What to do in this step",
      "dependencies": ["step-0"], // Optional: IDs of steps that must complete first
      "tool_hints": ["tool_name"], // Optional: Suggested tools
      "auto_execute": true/false // Whether this step can auto-execute
    }
  ],
  "risks": [
    {
      "description": "Potential risk",
      "severity": "low/medium/high",
      "mitigation": "How to mitigate"
    }
  ]
}

Make steps specific and actionable. Include estimated dependencies between steps.`,
		goal, ctxStr)
}

// buildTaskDecompositionPrompt creates a prompt for task decomposition
func buildTaskDecompositionPrompt(task Task) string {
	return fmt.Sprintf(`Decompose the following task into smaller subtasks.

Task: %s
Description: %s
Complexity: %s

Please provide a JSON response with an array of subtasks:
[
  {
    "id": "subtask-1",
    "title": "Brief title",
    "description": "Detailed description",
    "order": 1
  }
]

Each subtask should be independently actionable and ordered by dependency.`,
		task.Title, task.Description, task.Complexity)
}

// buildRevisionPrompt creates a prompt for plan revision
func buildRevisionPrompt(plan Plan, feedback Feedback) string {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")

	return fmt.Sprintf(`Revise the following plan based on the feedback.

Current Plan:
%s

Feedback:
- Issue: %s
- Suggestion: %s

Please provide a revised JSON plan that addresses the feedback.`,
		string(planJSON), feedback.Issue, feedback.Suggestion)
}

// parsePlanFromResponse parses a plan from LLM response
func parsePlanFromResponse(text, goal string) (*Plan, error) {
	// Extract JSON from response
	start := 0
	end := len(text)
	for i := 0; i < len(text); i++ {
		if text[i] == '{' {
			start = i
			break
		}
	}
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == '}' {
			end = i + 1
			break
		}
	}

	if start >= end {
		// Try to parse the whole text
		start = 0
		end = len(text)
	}

	var parsed struct {
		Description string    `json:"description"`
		Steps       []PlanStep `json:"steps"`
		Risks       []Risk     `json:"risks"`
	}

	if err := json.Unmarshal([]byte(text[start:end]), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal plan: %w", err)
	}

	// Assign IDs to steps if missing
	for i := range parsed.Steps {
		if parsed.Steps[i].ID == "" {
			parsed.Steps[i].ID = fmt.Sprintf("step-%d", i+1)
		}
		if parsed.Steps[i].Status == "" {
			parsed.Steps[i].Status = "pending"
		}
	}

	return &Plan{
		Goal:        goal,
		Description: parsed.Description,
		Steps:       parsed.Steps,
		Risks:       parsed.Risks,
		Estimates:   make(map[string]time.Duration),
	}, nil
}

// parseSubtasksFromResponse parses subtasks from LLM response
func parseSubtasksFromResponse(text, parentID string) ([]Subtask, error) {
	// Extract JSON array
	start := 0
	end := len(text)
	for i := 0; i < len(text); i++ {
		if text[i] == '[' {
			start = i
			break
		}
	}
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == ']' {
			end = i + 1
			break
		}
	}

	if start >= end {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var subtasks []Subtask
	if err := json.Unmarshal([]byte(text[start:end]), &subtasks); err != nil {
		return nil, fmt.Errorf("unmarshal subtasks: %w", err)
	}

	// Set parent ID
	for i := range subtasks {
		subtasks[i].ParentID = parentID
	}

	return subtasks, nil
}

// buildPlanDAG builds a DAG from plan steps
func buildPlanDAG(steps []PlanStep) *task.DAG {
	nodes := make([]string, 0, len(steps))
	edges := make([]task.Edge, 0)

	for _, step := range steps {
		nodes = append(nodes, step.ID)
		for _, dep := range step.Dependencies {
			edges = append(edges, task.Edge{From: dep, To: step.ID})
		}
	}

	return &task.DAG{Nodes: nodes, Edges: edges}
}

// generatePlanID generates a unique plan ID
func generatePlanID() string {
	return fmt.Sprintf("plan-%d", time.Now().UnixNano())
}

// PlanStatus constants
const (
	PlanStatusReady    = "ready"
	PlanStatusActive   = "active"
	PlanStatusPaused   = "paused"
	PlanStatusComplete = "complete"
	PlanStatusFailed   = "failed"
)

// StepStatus constants
const (
	StepStatusPending  = "pending"
	StepStatusActive   = "active"
	StepStatusDone     = "done"
	StepStatusFailed   = "failed"
	StepStatusSkipped  = "skipped"
	StepStatusBlocked  = "blocked"
)

// GetReadySteps returns steps that can be executed (dependencies satisfied)
func (p *Plan) GetReadySteps() []PlanStep {
	if p.DAG == nil {
		return nil
	}

	done := make(map[string]bool)
	for _, step := range p.Steps {
		if step.Status == StepStatusDone {
			done[step.ID] = true
		}
	}

	readyIDs := p.DAG.Ready(done)
	readySet := make(map[string]bool)
	for _, id := range readyIDs {
		readySet[id] = true
	}

	var ready []PlanStep
	for _, step := range p.Steps {
		if readySet[step.ID] && step.Status == StepStatusPending {
			ready = append(ready, step)
		}
	}

	return ready
}

// MarkStepDone marks a step as completed
func (p *Plan) MarkStepDone(stepID string, result StepResult) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepStatusDone
			p.Steps[i].Result = &result
			break
		}
	}
}

// MarkStepFailed marks a step as failed
func (p *Plan) MarkStepFailed(stepID string, err error) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepStatusFailed
			p.Steps[i].Result = &StepResult{
				Success:   false,
				Error:     err.Error(),
				Timestamp: time.Now(),
			}
			break
		}
	}
}

// IsComplete checks if all steps are done
func (p *Plan) IsComplete() bool {
	for _, step := range p.Steps {
		if step.Status != StepStatusDone && step.Status != StepStatusSkipped {
			return false
		}
	}
	return true
}

// GetProgress returns completion percentage
func (p *Plan) GetProgress() float64 {
	if len(p.Steps) == 0 {
		return 0
	}

	done := 0
	for _, step := range p.Steps {
		if step.Status == StepStatusDone || step.Status == StepStatusSkipped {
			done++
		}
	}

	return float64(done) / float64(len(p.Steps)) * 100
}