package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/marcus-ai/marcus/internal/provider"
)

// Decomposer breaks down complex goals into manageable tasks
type Decomposer interface {
	Decompose(ctx context.Context, goal string, depth int) ([]DecomposedTask, error)
	EstimateComplexity(goal string) string
	ShouldDecompose(goal string) bool
}

// DecomposedTask is a task broken down from a larger goal
type DecomposedTask struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Complexity   string            `json:"complexity"` // simple, moderate, complex
	Dependencies []string          `json:"dependencies,omitempty"`
	Order        int               `json:"order"`
	Subtasks     []DecomposedTask  `json:"subtasks,omitempty"`
}

// LLMDecomposer uses an LLM for task decomposition
type LLMDecomposer struct {
	provider provider.Provider
	threshold int // Steps threshold for decomposition
}

// NewLLMDecomposer creates a new LLM-based decomposer
func NewLLMDecomposer(p provider.Provider) *LLMDecomposer {
	return &LLMDecomposer{
		provider:  p,
		threshold: 3, // Decompose if estimated steps > threshold
	}
}

// Decompose decomposes a goal into tasks
func (d *LLMDecomposer) Decompose(ctx context.Context, goal string, depth int) ([]DecomposedTask, error) {
	if d.provider == nil {
		return nil, fmt.Errorf("no provider configured")
	}

	// Check complexity first
	complexity := d.EstimateComplexity(goal)
	if complexity == "simple" {
		// Simple tasks don't need decomposition
		return []DecomposedTask{
			{
				ID:          "task-1",
				Title:       goal,
				Description: goal,
				Complexity:  "simple",
				Order:       1,
			},
		}, nil
	}

	// Use LLM for decomposition
	prompt := buildDecompPrompt(goal, depth)

	resp, err := d.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   1500,
		JSON:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("decomposition request failed: %w", err)
	}

	tasks, err := parseDecomposedTasks(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("parse decomposed tasks: %w", err)
	}

	// Recursively decompose complex subtasks if depth allows
	if depth > 0 {
		for i := range tasks {
			if tasks[i].Complexity == "complex" {
				subtasks, err := d.Decompose(ctx, tasks[i].Description, depth-1)
				if err == nil {
					tasks[i].Subtasks = subtasks
				}
			}
		}
	}

	// Assign orders
	for i := range tasks {
		tasks[i].Order = i + 1
	}

	return tasks, nil
}

// EstimateComplexity estimates the complexity of a goal
func (d *LLMDecomposer) EstimateComplexity(goal string) string {
	// Rule-based complexity estimation
	goalLower := strings.ToLower(goal)

	// Complex indicators
	complexIndicators := []string{
		"implement", "refactor", "architecture", "system", "framework",
		"multi", "distributed", "database", "microservice", "api design",
	}

	// Simple indicators
	simpleIndicators := []string{
		"fix typo", "update comment", "add log", "change name", "rename",
		"simple", "quick", "minor",
	}

	// Check for complex indicators
	complexScore := 0
	for _, indicator := range complexIndicators {
		if strings.Contains(goalLower, indicator) {
			complexScore++
		}
	}

	// Check for simple indicators
	simpleScore := 0
	for _, indicator := range simpleIndicators {
		if strings.Contains(goalLower, indicator) {
			simpleScore++
		}
	}

	// Classify based on scores
	if simpleScore > 0 {
		return "simple"
	}
	if complexScore >= 2 {
		return "complex"
	}
	if complexScore >= 1 {
		return "moderate"
	}

	// Check word count and structure
	words := len(strings.Fields(goal))
	if words <= 5 {
		return "simple"
	}
	if words >= 15 {
		return "complex"
	}

	return "moderate"
}

// ShouldDecompose determines if a goal should be decomposed
func (d *LLMDecomposer) ShouldDecompose(goal string) bool {
	complexity := d.EstimateComplexity(goal)
	return complexity != "simple"
}

// buildDecompPrompt creates a prompt for task decomposition
func buildDecompPrompt(goal string, depth int) string {
	depthHint := ""
	if depth > 0 {
		depthHint = fmt.Sprintf(" This can be decomposed up to %d levels deep.", depth)
	}

	return fmt.Sprintf(`Decompose the following goal into concrete, actionable tasks.

Goal: %s%s

Please provide a JSON array of tasks with this structure:
[
  {
    "id": "task-1",
    "title": "Brief task title",
    "description": "Detailed description of what to do",
    "complexity": "simple|moderate|complex",
    "dependencies": ["task-0"] // Optional: IDs of tasks that must complete first
  }
]

Guidelines:
- Each task should be independently actionable
- Keep tasks small enough to complete in one sitting
- Identify dependencies clearly
- Mark complexity based on estimated effort`,
		goal, depthHint)
}

// parseDecomposedTasks parses decomposed tasks from LLM response
func parseDecomposedTasks(text string) ([]DecomposedTask, error) {
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

	var tasks []DecomposedTask
	if err := json.Unmarshal([]byte(text[start:end]), &tasks); err != nil {
		return nil, fmt.Errorf("unmarshal tasks: %w", err)
	}

	return tasks, nil
}

// HeuristicDecomposer uses rules for task decomposition
type HeuristicDecomposer struct{}

// NewHeuristicDecomposer creates a new rule-based decomposer
func NewHeuristicDecomposer() *HeuristicDecomposer {
	return &HeuristicDecomposer{}
}

// Decompose decomposes a goal using heuristic rules
func (d *HeuristicDecomposer) Decompose(ctx context.Context, goal string, depth int) ([]DecomposedTask, error) {
	tasks := make([]DecomposedTask, 0)

	// Pattern-based decomposition
	goalLower := strings.ToLower(goal)

	// Check for "implement" patterns
	if strings.Contains(goalLower, "implement") {
		tasks = append(tasks, DecomposedTask{
			ID:          "task-1",
			Title:       "Research and design",
			Description: "Research existing solutions and design the implementation approach",
			Complexity:  "moderate",
			Order:       1,
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-2",
			Title:       "Core implementation",
			Description: "Implement the core functionality",
			Complexity:  "complex",
			Order:       2,
			Dependencies: []string{"task-1"},
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-3",
			Title:       "Testing",
			Description: "Add tests and verify the implementation",
			Complexity:  "moderate",
			Order:       3,
			Dependencies: []string{"task-2"},
		})
	} else if strings.Contains(goalLower, "fix") {
		// Bug fix pattern
		tasks = append(tasks, DecomposedTask{
			ID:          "task-1",
			Title:       "Reproduce issue",
			Description: "Reproduce the bug to understand the problem",
			Complexity:  "simple",
			Order:       1,
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-2",
			Title:       "Identify root cause",
			Description: "Find the root cause of the bug",
			Complexity:  "moderate",
			Order:       2,
			Dependencies: []string{"task-1"},
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-3",
			Title:       "Implement fix",
			Description: "Fix the bug",
			Complexity:  "simple",
			Order:       3,
			Dependencies: []string{"task-2"},
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-4",
			Title:       "Verify fix",
			Description: "Test that the fix works",
			Complexity:  "simple",
			Order:       4,
			Dependencies: []string{"task-3"},
		})
	} else {
		// Generic pattern
		tasks = append(tasks, DecomposedTask{
			ID:          "task-1",
			Title:       "Analyze requirements",
			Description: "Understand what needs to be done",
			Complexity:  "simple",
			Order:       1,
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-2",
			Title:       "Implementation",
			Description: goal,
			Complexity:  "moderate",
			Order:       2,
			Dependencies: []string{"task-1"},
		})
		tasks = append(tasks, DecomposedTask{
			ID:          "task-3",
			Title:       "Review",
			Description: "Review the changes",
			Complexity:  "simple",
			Order:       3,
			Dependencies: []string{"task-2"},
		})
	}

	return tasks, nil
}

// EstimateComplexity estimates complexity using heuristics
func (d *HeuristicDecomposer) EstimateComplexity(goal string) string {
	// Similar to LLMDecomposer but rule-based
	goalLower := strings.ToLower(goal)

	if strings.Contains(goalLower, "refactor") ||
		strings.Contains(goalLower, "implement") ||
		strings.Contains(goalLower, "architecture") {
		return "complex"
	}

	if strings.Contains(goalLower, "fix") ||
		strings.Contains(goalLower, "update") ||
		strings.Contains(goalLower, "add") {
		return "moderate"
	}

	return "simple"
}

// ShouldDecompose determines if decomposition is needed
func (d *HeuristicDecomposer) ShouldDecompose(goal string) bool {
	return d.EstimateComplexity(goal) != "simple"
}

// ComplexityEstimator estimates task complexity
type ComplexityEstimator struct {
	patterns map[string]*regexp.Regexp
}

// NewComplexityEstimator creates a new complexity estimator
func NewComplexityEstimator() *ComplexityEstimator {
	return &ComplexityEstimator{
		patterns: map[string]*regexp.Regexp{
			"simple":   regexp.MustCompile(`(?i)\b(fix|update|add|rename|change|remove)\s+(typo|comment|name|log|import)\b`),
			"moderate": regexp.MustCompile(`(?i)\b(fix|implement|update|add)\s+(function|method|test|bug)\b`),
			"complex":  regexp.MustCompile(`(?i)\b(refactor|redesign|implement|architecture|system|framework|database|api)\b`),
		},
	}
}

// Estimate estimates the complexity of a task
func (e *ComplexityEstimator) Estimate(goal string) string {
	goalLower := strings.ToLower(goal)

	if e.patterns["simple"].MatchString(goalLower) {
		return "simple"
	}
	if e.patterns["complex"].MatchString(goalLower) {
		return "complex"
	}
	if e.patterns["moderate"].MatchString(goalLower) {
		return "moderate"
	}

	// Default based on length
	words := len(strings.Fields(goal))
	if words <= 5 {
		return "simple"
	}
	if words >= 15 {
		return "complex"
	}

	return "moderate"
}
