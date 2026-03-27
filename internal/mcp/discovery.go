package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerConfig represents the configuration for a single MCP server
type ServerConfig struct {
	Name    string   `json:"name"`    // Display name for the server
	Command string   `json:"command"` // Command to run (for stdio transport)
	Args    []string `json:"args"`    // Arguments for the command
	URL     string   `json:"url"`     // URL for SSE transport (alternative to command)
	Env     []string `json:"env"`     // Environment variables (KEY=VALUE format)
}

// Config represents the MCP configuration file
type Config struct {
	Servers []ServerConfig `json:"servers"`
}

// LoadConfig loads MCP configuration from the given path
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Servers: []ServerConfig{}}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves MCP configuration to the given path
func SaveConfig(path string, config *Config) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// DiscoverServers creates MCP clients for all configured servers
func (c *Config) DiscoverServers(ctx context.Context) ([]*Client, error) {
	var clients []*Client

	for _, serverConfig := range c.Servers {
		// Skip disabled servers (can be extended with a Disabled field)
		if serverConfig.Name == "" {
			continue
		}

		client, err := connectServer(ctx, serverConfig)
		if err != nil {
			// Log error but continue with other servers
			// In a real implementation, we might want to collect these errors
			_ = err
			continue
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// connectServer connects to a single MCP server based on its configuration
func connectServer(ctx context.Context, config ServerConfig) (*Client, error) {
	var transport Transport
	var err error

	// Determine transport type
	if config.URL != "" {
		// SSE transport
		transport, err = NewSSETransport(config.URL)
		if err != nil {
			return nil, fmt.Errorf("create SSE transport: %w", err)
		}
	} else if config.Command != "" {
		// Stdio transport
		transport, err = NewStdioTransport(config.Command, config.Args, config.Env)
		if err != nil {
			return nil, fmt.Errorf("create stdio transport: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no transport configured for server %s", config.Name)
	}

	// Create client
	client := NewClient(config.Name, transport)

	// Initialize the client
	if err := client.Initialize(ctx); err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize client: %w", err)
	}

	return client, nil
}

// AddServer adds a server to the configuration
func (c *Config) AddServer(server ServerConfig) error {
	// Validate server config
	if server.Name == "" {
		return fmt.Errorf("server name is required")
	}
	if server.Command == "" && server.URL == "" {
		return fmt.Errorf("either command or URL is required")
	}

	// Check for duplicate names
	for _, existing := range c.Servers {
		if existing.Name == server.Name {
			return fmt.Errorf("server with name %q already exists", server.Name)
		}
	}

	c.Servers = append(c.Servers, server)
	return nil
}

// RemoveServer removes a server from the configuration by name
func (c *Config) RemoveServer(name string) bool {
	for i, server := range c.Servers {
		if server.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			return true
		}
	}
	return false
}

// GetServer returns a server configuration by name
func (c *Config) GetServer(name string) (ServerConfig, bool) {
	for _, server := range c.Servers {
		if server.Name == name {
			return server, true
		}
	}
	return ServerConfig{}, false
}

// DefaultConfigPath returns the default path for MCP configuration
func DefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".marcus", "mcp.json")
}
