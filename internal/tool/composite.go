package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// CompositeTool combines multiple tools into one callable tool
type CompositeTool struct {
	name        string
	description string
	pipeline    *ToolPipeline
	schema      JSONSchema
}

// NewCompositeTool creates a new composite tool
func NewCompositeTool(name, description string, pipeline *ToolPipeline) *CompositeTool {
	ct := &CompositeTool{
		name:        name,
		description: description,
		pipeline:    pipeline,
		schema:      JSONSchema{},
	}
	ct.buildSchema()
	return ct
}

// Name returns the tool name
func (c *CompositeTool) Name() string {
	return c.name
}

// Description returns the tool description
func (c *CompositeTool) Description() string {
	return c.description + " (composite: " + c.pipeline.Name + ")"
}

// Schema returns the JSON schema for the tool
func (c *CompositeTool) Schema() JSONSchema {
	return c.schema
}

// Run executes the composite tool
func (c *CompositeTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	// Parse input to set pipeline variables
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Set pipeline variables from input
	for k, v := range params {
		c.pipeline.SetVariable(k, v)
	}

	// Get runner from context
	runner, ok := ctx.Value("toolRunner").(*ToolRunner)
	if !ok {
		return nil, fmt.Errorf("tool runner not available in context")
	}

	// Execute pipeline
	result := c.pipeline.Execute(ctx, runner)

	// Format output
	output := map[string]any{
		"success":   result.Success,
		"step_count": result.StepCount,
		"variables": result.Variables,
	}

	if len(result.Errors) > 0 {
		errors := make([]map[string]string, len(result.Errors))
		for i, e := range result.Errors {
			errors[i] = map[string]string{
				"step":    fmt.Sprintf("%d", e.Step),
				"tool":    e.Tool,
				"message": e.Error,
			}
		}
		output["errors"] = errors
	}

	return json.Marshal(output)
}

// buildSchema builds the JSON schema from pipeline parameters
func (c *CompositeTool) buildSchema() {
	// Collect required inputs from pipeline steps
	properties := make(map[string]*JSONProperty)
	required := []string{}

	for _, step := range c.pipeline.Steps {
		for _, value := range step.Input {
			if valStr, ok := value.(string); ok {
				// Check if it references a variable
				if len(valStr) > 3 && valStr[0] == '$' && valStr[1] == '{' {
					// Extract variable name
					end := len(valStr) - 1
					if valStr[end] == '}' {
						varName := valStr[2:end]
						properties[varName] = &JSONProperty{
							Type:        "string",
							Description: "Input for " + step.Tool,
						}
						required = append(required, varName)
					}
				}
			}
		}
	}

	c.schema = JSONSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// GetPipeline returns the underlying pipeline
func (c *CompositeTool) GetPipeline() *ToolPipeline {
	return c.pipeline
}

// CompositeToolRegistry manages composite tools
type CompositeToolRegistry struct {
	tools map[string]*CompositeTool
}

// NewCompositeToolRegistry creates a new registry
func NewCompositeToolRegistry() *CompositeToolRegistry {
	return &CompositeToolRegistry{
		tools: make(map[string]*CompositeTool),
	}
}

// Register registers a composite tool
func (r *CompositeToolRegistry) Register(tool *CompositeTool) error {
	if tool == nil {
		return fmt.Errorf("cannot register nil tool")
	}
	if tool.Name() == "" {
		return fmt.Errorf("composite tool must have a name")
	}
	r.tools[tool.Name()] = tool
	return nil
}

// Get retrieves a composite tool by name
func (r *CompositeToolRegistry) Get(name string) (*CompositeTool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered composite tool names
func (r *CompositeToolRegistry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Unregister removes a composite tool
func (r *CompositeToolRegistry) Unregister(name string) {
	delete(r.tools, name)
}

// CreateCompositeFromPipeline creates a composite tool from a pipeline definition
func (r *CompositeToolRegistry) CreateCompositeFromPipeline(name, description string, pipeline *ToolPipeline) *CompositeTool {
	tool := NewCompositeTool(name, description, pipeline)
	r.Register(tool)
	return tool
}

// LoadPredefined registers all predefined pipelines as composite tools
func (r *CompositeToolRegistry) LoadPredefined() {
	predefined := ListPredefinedPipelines()
	for _, name := range predefined {
		pipeline := GetPredefinedPipeline(name)
		if pipeline != nil {
			tool := NewCompositeTool(name, pipeline.Description, pipeline)
			r.Register(tool)
		}
	}
}

// CompositeToolResult represents the output of a composite tool
type CompositeToolResult struct {
	Success   bool                   `json:"success"`
	StepCount int                    `json:"step_count"`
	Variables map[string]any         `json:"variables,omitempty"`
	Errors    []CompositeToolError   `json:"errors,omitempty"`
}

// CompositeToolError represents an error in a composite tool
type CompositeToolError struct {
	Step    int    `json:"step"`
	Tool    string `json:"tool"`
	Message string `json:"message"`
}
