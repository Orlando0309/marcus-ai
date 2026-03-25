package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ToolChainer manages passing results between tool calls
type ToolChainer struct {
	variables map[string]any
	results   []ChainedResult
}

// ChainedResult stores the result of a chained tool execution
type ChainedResult struct {
	Tool     string
	Input    map[string]any
	Output   any
	Error    error
	Variable string // Name of the variable this result is stored as
}

// NewToolChainer creates a new tool chainer
func NewToolChainer() *ToolChainer {
	return &ToolChainer{
		variables: make(map[string]any),
		results:   make([]ChainedResult, 0),
	}
}

// SetVariable sets a variable value
func (c *ToolChainer) SetVariable(name string, value any) {
	c.variables[name] = value
}

// GetVariable gets a variable value
func (c *ToolChainer) GetVariable(name string) (any, bool) {
	val, ok := c.variables[name]
	return val, ok
}

// GetVariableString gets a variable as a string
func (c *ToolChainer) GetVariableString(name string) string {
	val, ok := c.variables[name]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// ResolveInput resolves variable references in input
func (c *ToolChainer) ResolveInput(input map[string]any) map[string]any {
	resolved := make(map[string]any)
	for k, v := range input {
		resolved[k] = c.resolveValue(v)
	}
	return resolved
}

// resolveValue recursively resolves variable references in a value
func (c *ToolChainer) resolveValue(value any) any {
	switch v := value.(type) {
	case string:
		return c.resolveVariableReference(v)
	case map[string]any:
		return c.ResolveInput(v)
	case []any:
		resolved := make([]any, len(v))
		for i, item := range v {
			resolved[i] = c.resolveValue(item)
		}
		return resolved
	default:
		return v
	}
}

// resolveVariableReference resolves ${varname} or $varname patterns
func (c *ToolChainer) resolveVariableReference(s string) string {
	// Handle ${varname} pattern
	if len(s) >= 4 && s[0:2] == "${" && s[len(s)-1] == '}' {
		varName := s[2 : len(s)-1]
		if val, ok := c.variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return s // Keep original if not found
	}

	// Handle $varname pattern (simple substitution)
	if len(s) > 1 && s[0] == '$' {
		varName := s[1:]
		if val, ok := c.variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return s // Keep original if not found
	}

	return s
}

// StoreResult stores a tool result as a variable
func (c *ToolChainer) StoreResult(result ChainedResult) {
	c.results = append(c.results, result)
	if result.Variable != "" {
		c.variables[result.Variable] = result.Output
	}
}

// GetLastResult returns the most recent result
func (c *ToolChainer) GetLastResult() *ChainedResult {
	if len(c.results) == 0 {
		return nil
	}
	return &c.results[len(c.results)-1]
}

// GetResults returns all results
func (c *ToolChainer) GetResults() []ChainedResult {
	return c.results
}

// Clear clears all results and variables
func (c *ToolChainer) Clear() {
	c.variables = make(map[string]any)
	c.results = make([]ChainedResult, 0)
}

// ExtractPath extracts a nested value from the last result using path notation
// e.g., "output.content", "result.0.name"
func (c *ToolChainer) ExtractPath(path string) (any, bool) {
	last := c.GetLastResult()
	if last == nil {
		return nil, false
	}

	return extractPath(last.Output, path)
}

// extractPath extracts a value from an object using dot notation
func extractPath(obj any, path string) (any, bool) {
	parts := splitPath(path)
	current := obj

	for _, part := range parts {
		if current == nil {
			return nil, false
		}

		switch v := current.(type) {
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val

		case []any:
			// Try to index into array
			idx := 0
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				return nil, false
			}

		default:
			// Use reflection for structs
			rv := reflect.ValueOf(current)
			if rv.Kind() == reflect.Ptr {
				rv = rv.Elem()
			}
			if rv.Kind() == reflect.Struct {
				field := rv.FieldByName(part)
				if !field.IsValid() {
					return nil, false
				}
				current = field.Interface()
			} else {
				return nil, false
			}
		}
	}

	return current, true
}

// splitPath splits a path by dots, respecting escaped dots
func splitPath(path string) []string {
	var parts []string
	var current strings.Builder

	for i, r := range path {
		if r == '.' {
			if i > 0 && path[i-1] == '\\' {
				// Escaped dot, remove backslash and keep dot
				str := current.String()
				current.Reset()
				current.WriteString(str[:len(str)-1])
				current.WriteRune(r)
			} else {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// ChainLink represents a link in the tool chain
type ChainLink struct {
	Tool      string
	Input     map[string]any
	OutputTo  string // Variable name to store output as
	Condition string // Condition for execution
}

// ToolChain is a linear chain of tool executions
type ToolChain struct {
	Name  string
	Links []ChainLink
}

// NewToolChain creates a new tool chain
func NewToolChain(name string) *ToolChain {
	return &ToolChain{
		Name:  name,
		Links: make([]ChainLink, 0),
	}
}

// AddLink adds a link to the chain
func (tc *ToolChain) AddLink(link ChainLink) {
	tc.Links = append(tc.Links, link)
}

// Execute runs the tool chain
func (tc *ToolChain) Execute(ctx context.Context, runner *ToolRunner, chainer *ToolChainer) ([]ChainedResult, error) {
	results := make([]ChainedResult, 0)

	for _, link := range tc.Links {
		// Check condition
		if !chainer.evaluateChainCondition(link.Condition) {
			continue
		}

		// Resolve input variables
		resolved := chainer.ResolveInput(link.Input)

		// Execute tool
		inputBytes, _ := json.Marshal(resolved)
		raw, err := runner.Run(ctx, link.Tool, inputBytes)

		var output any
		_ = json.Unmarshal(raw, &output)

		result := ChainedResult{
			Tool:     link.Tool,
			Input:    resolved,
			Output:   output,
			Error:    err,
			Variable: link.OutputTo,
		}

		chainer.StoreResult(result)
		results = append(results, result)

		if err != nil {
			return results, fmt.Errorf("chain failed at %s: %w", link.Tool, err)
		}
	}

	return results, nil
}

// evaluateChainCondition evaluates a simple condition
func (c *ToolChainer) evaluateChainCondition(condition string) bool {
	if condition == "" {
		return true
	}

	// Check variable existence
	if val, ok := c.variables[condition]; ok {
		// Check truthiness
		switch v := val.(type) {
		case bool:
			return v
		case string:
			return v != ""
		case int, int64, float64:
			return v != 0
		default:
			return val != nil
		}
	}

	// Check for comparison operators
	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			left = strings.Trim(left, "\"'")
			right = strings.Trim(right, "\"'")
			if val, ok := c.variables[left]; ok {
				return fmt.Sprintf("%v", val) == right
			}
		}
	}

	return true
}

