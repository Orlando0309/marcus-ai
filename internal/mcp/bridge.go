package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/tool"
)

// MCPToolAdapter adapts an MCP tool to the Marcus tool interface
type MCPToolAdapter struct {
	client *Client
	def    ToolDefinition
}

// NewMCPToolAdapter creates a new adapter for an MCP tool
func NewMCPToolAdapter(client *Client, def ToolDefinition) *MCPToolAdapter {
	return &MCPToolAdapter{
		client: client,
		def:    def,
	}
}

// Name returns the tool name
func (a *MCPToolAdapter) Name() string {
	// Prefix with mcp: to avoid conflicts
	return fmt.Sprintf("mcp_%s_%s", a.client.Name(), a.def.Name)
}

// OriginalName returns the original MCP tool name (without prefix)
func (a *MCPToolAdapter) OriginalName() string {
	return a.def.Name
}

// Description returns the tool description
func (a *MCPToolAdapter) Description() string {
	// Include server name in description for clarity
	return fmt.Sprintf("[%s] %s", a.client.Name(), a.def.Description)
}

// Schema returns the JSON schema for the tool
func (a *MCPToolAdapter) Schema() tool.JSONSchema {
	// Parse the input schema from MCP
	var schema tool.JSONSchema
	if len(a.def.InputSchema) > 0 {
		_ = json.Unmarshal(a.def.InputSchema, &schema)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema
}

// Run executes the MCP tool
func (a *MCPToolAdapter) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	// Call the MCP tool
	result, err := a.client.CallTool(ctx, a.def.Name, input)
	if err != nil {
		return nil, err
	}

	// Convert result to JSON
	// MCP returns content items, we need to extract the text
	var output strings.Builder
	for _, item := range result.Content {
		if item.Type == "text" {
			output.WriteString(item.Text)
		}
	}

	// Return as a structured response
	response := map[string]any{
		"tool":   a.def.Name,
		"result": output.String(),
		"server": a.client.Name(),
	}

	if result.IsError {
		response["error"] = true
	}

	return json.Marshal(response)
}
