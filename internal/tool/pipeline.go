package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolPipeline chains multiple tools together
type ToolPipeline struct {
	Name        string
	Description string
	Steps       []PipelineStep
	variables   map[string]any // Stores intermediate results
}

// PipelineStep is a single step in a pipeline
type PipelineStep struct {
	Name       string                 `json:"name"`
	Tool       string                 `json:"tool"`
	Input      map[string]any         `json:"input,omitempty"`
	OutputTo   string                 `json:"output_to,omitempty"`   // Variable to store output
	Condition  string                 `json:"condition,omitempty"`   // Condition for execution
	OnError    string                 `json:"on_error,omitempty"`    // "stop", "continue", "retry"
	MaxRetries int                    `json:"max_retries,omitempty"`
}

// PipelineResult contains the result of executing a pipeline
type PipelineResult struct {
	Success    bool
	Variables  map[string]any
	Errors     []PipelineError
	StepCount  int
	DurationMs int64
}

// PipelineError records an error from a pipeline step
type PipelineError struct {
	Step    int
	Tool    string
	Error   string
	Retryable bool
}

// NewPipeline creates a new empty pipeline
func NewPipeline(name, description string) *ToolPipeline {
	return &ToolPipeline{
		Name:        name,
		Description: description,
		Steps:       make([]PipelineStep, 0),
		variables:   make(map[string]any),
	}
}

// AddStep adds a step to the pipeline
func (p *ToolPipeline) AddStep(step PipelineStep) {
	p.Steps = append(p.Steps, step)
}

// SetVariable sets a pipeline variable
func (p *ToolPipeline) SetVariable(name string, value any) {
	if p.variables == nil {
		p.variables = make(map[string]any)
	}
	p.variables[name] = value
}

// GetVariable gets a pipeline variable
func (p *ToolPipeline) GetVariable(name string) (any, bool) {
	val, ok := p.variables[name]
	return val, ok
}

// Execute runs the pipeline with the given ToolRunner
func (p *ToolPipeline) Execute(ctx context.Context, runner *ToolRunner) PipelineResult {
	result := PipelineResult{
		Success:   true,
		Variables: make(map[string]any),
		Errors:    make([]PipelineError, 0),
	}

	// Copy initial variables
	for k, v := range p.variables {
		result.Variables[k] = v
	}

	for i, step := range p.Steps {
		// Check condition
		if !p.evaluateCondition(step.Condition, result.Variables) {
			continue
		}

		// Resolve input with variable substitution
		resolvedInput := p.resolveVariables(step.Input, result.Variables)

		// Execute tool
		var stepErr error
		var raw json.RawMessage
		attempts := 0
		maxRetries := step.MaxRetries
		if maxRetries == 0 {
			maxRetries = 1
		}

		for attempts < maxRetries {
			attempts++
			inputBytes, _ := json.Marshal(resolvedInput)
			raw, stepErr = runner.Run(ctx, step.Tool, inputBytes)
			if stepErr == nil {
				break
			}
			if step.OnError == "stop" || attempts >= maxRetries {
				break
			}
			// Retry
		}

		if stepErr != nil {
			errInfo := PipelineError{
				Step:    i,
				Tool:    step.Tool,
				Error:   stepErr.Error(),
				Retryable: step.OnError == "retry",
			}
			result.Errors = append(result.Errors, errInfo)

			if step.OnError == "stop" {
				result.Success = false
				return result
			}
			continue
		}

		// Store output in variable if specified
		if step.OutputTo != "" {
			var output any
			_ = json.Unmarshal(raw, &output)
			result.Variables[step.OutputTo] = output
			// Also store raw for string access
			result.Variables[step.OutputTo+"_raw"] = string(raw)
		}

		result.StepCount++
	}

	// Copy final variables to pipeline
	p.variables = result.Variables

	return result
}

// evaluateCondition checks if a condition is met
func (p *ToolPipeline) evaluateCondition(condition string, vars map[string]any) bool {
	if condition == "" {
		return true
	}

	// Simple condition evaluation
	// Supports: "var exists", "var == value", "var != value", "var contains value"
	condition = strings.TrimSpace(condition)

	// Check "exists" condition
	if strings.HasSuffix(condition, " exists") {
		varName := strings.TrimSuffix(condition, " exists")
		varName = strings.TrimSpace(varName)
		_, exists := vars[varName]
		return exists
	}

	// Check equality conditions
	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			expected = strings.Trim(expected, "\"'")
			if val, ok := vars[varName]; ok {
				return fmt.Sprintf("%v", val) == expected
			}
			return false
		}
	}

	// Check inequality
	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			expected = strings.Trim(expected, "\"'")
			if val, ok := vars[varName]; ok {
				return fmt.Sprintf("%v", val) != expected
			}
			return true
		}
	}

	// Check contains
	if strings.Contains(condition, " contains ") {
		parts := strings.SplitN(condition, " contains ", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			search := strings.TrimSpace(parts[1])
			search = strings.Trim(search, "\"'")
			if val, ok := vars[varName]; ok {
				return strings.Contains(fmt.Sprintf("%v", val), search)
			}
		}
	}

	return true
}

// resolveVariables substitutes variables in input values
func (p *ToolPipeline) resolveVariables(input map[string]any, vars map[string]any) map[string]any {
	resolved := make(map[string]any)
	for k, v := range input {
		resolved[k] = p.resolveValue(v, vars)
	}
	return resolved
}

// resolveValue substitutes variables in a value
func (p *ToolPipeline) resolveValue(value any, vars map[string]any) any {
	switch v := value.(type) {
	case string:
		return p.substituteVariables(v, vars)
	case map[string]any:
		return p.resolveVariables(v, vars)
	case []any:
		resolved := make([]any, len(v))
		for i, item := range v {
			resolved[i] = p.resolveValue(item, vars)
		}
		return resolved
	default:
		return v
	}
}

// substituteVariables replaces ${varname} with actual values
func (p *ToolPipeline) substituteVariables(s string, vars map[string]any) string {
	result := s
	for name, value := range vars {
		placeholder := "${" + name + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// ToJSON serializes the pipeline to JSON
func (p *ToolPipeline) ToJSON() ([]byte, error) {
	return json.MarshalIndent(struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Steps       []PipelineStep `json:"steps"`
	}{
		Name:        p.Name,
		Description: p.Description,
		Steps:       p.Steps,
	}, "", "  ")
}

// FromJSON deserializes a pipeline from JSON
func (p *ToolPipeline) FromJSON(data []byte) error {
	var temp struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Steps       []PipelineStep `json:"steps"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	p.Name = temp.Name
	p.Description = temp.Description
	p.Steps = temp.Steps
	return nil
}

// Predefined pipelines
var (
	// PipelineSearchAndRead searches for files and reads their contents
	PipelineSearchAndRead = &ToolPipeline{
		Name:        "search_and_read",
		Description: "Search for files matching a pattern and read their contents",
		Steps: []PipelineStep{
			{
				Name:     "search",
				Tool:     "search_code",
				Input:    map[string]any{"pattern": "${search_pattern}", "max_results": 10},
				OutputTo: "search_results",
				OnError:  "stop",
			},
			{
				Name:      "read",
				Tool:      "read_file",
				Input:     map[string]any{"path": "${search_results.0.path}"},
				OutputTo:  "file_content",
				Condition: "search_results exists",
				OnError:   "continue",
			},
		},
	}

	// PipelineAnalyzeAndFix analyzes code and applies fixes
	PipelineAnalyzeAndFix = &ToolPipeline{
		Name:        "analyze_and_fix",
		Description: "Run tests, analyze failures, and apply fixes",
		Steps: []PipelineStep{
			{
				Name:     "test",
				Tool:     "test_runner",
				Input:    map[string]any{"command": "go test ./..."},
				OutputTo: "test_results",
				OnError:  "continue",
			},
			{
				Name:      "find_failure",
				Tool:      "search_code",
				Input:     map[string]any{"pattern": "${failure_pattern}"},
				OutputTo:  "failure_location",
				Condition: "test_results contains FAIL",
				OnError:   "stop",
			},
			{
				Name:      "apply_fix",
				Tool:      "edit_file",
				Input:     map[string]any{"path": "${failure_location.path}"},
				Condition: "failure_location exists",
				OnError:   "stop",
			},
		},
	}

	// PipelineExploreAndPlan explores codebase and creates a plan
	PipelineExploreAndPlan = &ToolPipeline{
		Name:        "explore_and_plan",
		Description: "Explore project structure and create an implementation plan",
		Steps: []PipelineStep{
			{
				Name:     "list_structure",
				Tool:     "list_directory",
				Input:    map[string]any{"path": ".", "recursive": false},
				OutputTo: "structure",
				OnError:  "stop",
			},
			{
				Name:      "read_readme",
				Tool:      "read_file",
				Input:     map[string]any{"path": "README.md"},
				OutputTo:  "readme",
				Condition: "structure contains README.md",
				OnError:   "continue",
			},
			{
				Name:      "find_go_mod",
				Tool:      "read_file",
				Input:     map[string]any{"path": "go.mod"},
				OutputTo:  "go_mod",
				Condition: "structure contains go.mod",
				OnError:   "continue",
			},
		},
	}
)

// GetPredefinedPipeline returns a predefined pipeline by name
func GetPredefinedPipeline(name string) *ToolPipeline {
	switch name {
	case "search_and_read":
		p := *PipelineSearchAndRead
		return &p
	case "analyze_and_fix":
		p := *PipelineAnalyzeAndFix
		return &p
	case "explore_and_plan":
		p := *PipelineExploreAndPlan
		return &p
	default:
		return nil
	}
}

// ListPredefinedPipelines returns names of all predefined pipelines
func ListPredefinedPipelines() []string {
	return []string{"search_and_read", "analyze_and_fix", "explore_and_plan"}
}
