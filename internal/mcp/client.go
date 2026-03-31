package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolDefinition represents an MCP tool definition
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Result represents an MCP tool execution result
type Result struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

// ContentItem represents a content item in the result
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Client is an MCP client for communicating with MCP servers
type Client struct {
	transport Transport
	name      string
	mu        sync.RWMutex
	tools     []ToolDefinition
	ready     bool
}

// NewClient creates a new MCP client with the given transport
func NewClient(name string, transport Transport) *Client {
	return &Client{
		transport: transport,
		name:      name,
		tools:     make([]ToolDefinition, 0),
	}
}

// Initialize initializes the client and discovers available tools
func (c *Client) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Send initialize request
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "marcus",
			"version": "1.0.0",
		},
	}

	params, _ := json.Marshal(initParams)
	response, err := c.transport.Send(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Parse response
	var initResponse struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(response, &initResponse); err != nil {
		return fmt.Errorf("parse init response: %w", err)
	}

	if initResponse.Error != nil {
		return fmt.Errorf("init error %d: %s", initResponse.Error.Code, initResponse.Error.Message)
	}

	// Send initialized notification
	_, _ = c.transport.Send(ctx, "initialized", nil)

	// Fetch tools
	tools, err := c.listTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	c.tools = tools
	c.ready = true

	return nil
}

// listTools fetches the list of available tools from the server
func (c *Client) listTools(ctx context.Context) ([]ToolDefinition, error) {
	response, err := c.transport.Send(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	// Parse response
	var listResponse struct {
		Result struct {
			Tools []ToolDefinition `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(response, &listResponse); err != nil {
		return nil, fmt.Errorf("parse tools list: %w", err)
	}

	if listResponse.Error != nil {
		return nil, fmt.Errorf("tools/list error %d: %s", listResponse.Error.Code, listResponse.Error.Message)
	}

	return listResponse.Result.Tools, nil
}

// ListTools returns the cached list of available tools
func (c *Client) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.ready {
		return nil, fmt.Errorf("client not initialized")
	}

	// Return a copy to avoid external modification
	tools := make([]ToolDefinition, len(c.tools))
	copy(tools, c.tools)
	return tools, nil
}

// CallTool calls an MCP tool with the given arguments
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (Result, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.ready {
		return Result{}, fmt.Errorf("client not initialized")
	}

	// Build request
	params := map[string]interface{}{
		"name": name,
	}
	if args != nil {
		var argsMap map[string]interface{}
		if err := json.Unmarshal(args, &argsMap); err == nil {
			params["arguments"] = argsMap
		}
	}

	paramsJSON, _ := json.Marshal(params)
	response, err := c.transport.Send(ctx, "tools/call", paramsJSON)
	if err != nil {
		return Result{}, fmt.Errorf("tools/call: %w", err)
	}

	// Parse response
	var callResponse struct {
		Result Result `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(response, &callResponse); err != nil {
		return Result{}, fmt.Errorf("parse call response: %w", err)
	}

	if callResponse.Error != nil {
		return Result{}, fmt.Errorf("tools/call error %d: %s", callResponse.Error.Code, callResponse.Error.Message)
	}

	return callResponse.Result, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	c.mu.Lock()
	c.ready = false
	c.mu.Unlock()

	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// Name returns the client name
func (c *Client) Name() string {
	return c.name
}

// IsReady returns true if the client is initialized and ready
func (c *Client) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}
