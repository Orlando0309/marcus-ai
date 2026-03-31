package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/mcp"
	"github.com/marcus-ai/marcus/internal/skill"
)

// MCPSkill manages MCP server configuration
type MCPSkill struct{}

func (m *MCPSkill) Name() string { return "mcp" }

func (m *MCPSkill) Pattern() string { return "/mcp" }

func (m *MCPSkill) Description() string {
	return "Manage MCP servers (list, add, remove)"
}

func (m *MCPSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return m.showUsage(), nil
	}

	subcommand := strings.ToLower(args[0])

	switch subcommand {
	case "list":
		return m.listServers(), nil
	case "add":
		return m.addServer(args[1:]), nil
	case "remove":
		return m.removeServer(args[1:]), nil
	case "help":
		return m.showUsage(), nil
	default:
		return skill.Result{
			Message: fmt.Sprintf("Unknown subcommand: %s\n\n%s", subcommand, m.usageText()),
			Done:    true,
		}, nil
	}
}

func (m *MCPSkill) showUsage() skill.Result {
	return skill.Result{
		Message: m.usageText(),
		Done:    true,
	}
}

func (m *MCPSkill) usageText() string {
	return `MCP Server Management

Usage: /mcp <subcommand> [args]

Subcommands:
  list                     List configured MCP servers
  add <name> <command>     Add a new MCP server (stdio)
  add <name> <url>         Add a new MCP server (SSE)
  remove <name>            Remove an MCP server

Examples:
  /mcp list
  /mcp add filesystem npx -y @modelcontextprotocol/server-filesystem /path/to/allow
  /mcp add myserver http://localhost:3000
  /mcp remove filesystem`
}

func (m *MCPSkill) listServers() skill.Result {
	configPath := mcp.DefaultConfigPath()
	if configPath == "" {
		return skill.Result{
			Message: "Failed to determine MCP config path.",
			Done:    true,
		}
	}

	config, err := mcp.LoadConfig(configPath)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to load MCP config: %v", err),
			Done:    true,
		}
	}

	if len(config.Servers) == 0 {
		return skill.Result{
			Message: "No MCP servers configured.\n\nUse '/mcp add' to add a server.",
			Done:    true,
		}
	}

	var lines []string
	lines = append(lines, "Configured MCP Servers:")
	lines = append(lines, "")

	for _, server := range config.Servers {
		lines = append(lines, fmt.Sprintf("  %s:", server.Name))
		if server.Command != "" {
			lines = append(lines, fmt.Sprintf("    Type:    stdio", ))
			lines = append(lines, fmt.Sprintf("    Command: %s", server.Command))
			if len(server.Args) > 0 {
				lines = append(lines, fmt.Sprintf("    Args:    %s", strings.Join(server.Args, " ")))
			}
		} else if server.URL != "" {
			lines = append(lines, fmt.Sprintf("    Type: SSE"))
			lines = append(lines, fmt.Sprintf("    URL:  %s", server.URL))
		}
		if len(server.Env) > 0 {
			lines = append(lines, fmt.Sprintf("    Env:     %d vars", len(server.Env)))
		}
		lines = append(lines, "")
	}

	lines = append(lines, fmt.Sprintf("Config file: %s", configPath))

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}

func (m *MCPSkill) addServer(args []string) skill.Result {
	if len(args) < 2 {
		return skill.Result{
			Message: "Usage: /mcp add <name> <command> [args...]\n   or: /mcp add <name> <url>",
			Done:    true,
		}
	}

	name := args[0]
	arg1 := args[1]

	configPath := mcp.DefaultConfigPath()
	if configPath == "" {
		return skill.Result{
			Message: "Failed to determine MCP config path.",
			Done:    true,
		}
	}

	// Load existing config
	config, err := mcp.LoadConfig(configPath)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to load MCP config: %v", err),
			Done:    true,
		}
	}

	// Check for duplicate names
	for _, server := range config.Servers {
		if server.Name == name {
			return skill.Result{
				Message: fmt.Sprintf("Server '%s' already exists. Remove it first with '/mcp remove %s'.", name, name),
				Done:    true,
			}
		}
	}

	// Determine if it's a URL or command
	var serverConfig mcp.ServerConfig
	serverConfig.Name = name

	if strings.HasPrefix(arg1, "http://") || strings.HasPrefix(arg1, "https://") {
		// SSE transport
		serverConfig.URL = arg1
	} else {
		// Stdio transport
		serverConfig.Command = arg1
		if len(args) > 2 {
			serverConfig.Args = args[2:]
		}
	}

	// Add server
	config.Servers = append(config.Servers, serverConfig)

	// Save config
	if err := mcp.SaveConfig(configPath, config); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to save MCP config: %v", err),
			Done:    true,
		}
	}

	var msg string
	if serverConfig.URL != "" {
		msg = fmt.Sprintf("Added MCP server '%s' with SSE transport.\nURL: %s", name, serverConfig.URL)
	} else {
		msg = fmt.Sprintf("Added MCP server '%s' with stdio transport.\nCommand: %s", name, serverConfig.Command)
		if len(serverConfig.Args) > 0 {
			msg += fmt.Sprintf(" %s", strings.Join(serverConfig.Args, " "))
		}
	}

	return skill.Result{
		Message: msg + "\n\nRestart Marcus to load the new server.",
		Done:    true,
	}
}

func (m *MCPSkill) removeServer(args []string) skill.Result {
	if len(args) < 1 {
		return skill.Result{
			Message: "Usage: /mcp remove <name>",
			Done:    true,
		}
	}

	name := args[0]
	configPath := mcp.DefaultConfigPath()
	if configPath == "" {
		return skill.Result{
			Message: "Failed to determine MCP config path.",
			Done:    true,
		}
	}

	// Load existing config
	config, err := mcp.LoadConfig(configPath)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to load MCP config: %v", err),
			Done:    true,
		}
	}

	// Find and remove server
	found := false
	for i, server := range config.Servers {
		if server.Name == name {
			config.Servers = append(config.Servers[:i], config.Servers[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return skill.Result{
			Message: fmt.Sprintf("Server '%s' not found.\n\nUse '/mcp list' to see configured servers.", name),
			Done:    true,
		}
	}

	// Save config
	if err := mcp.SaveConfig(configPath, config); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to save MCP config: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Removed MCP server '%s'.\n\nRestart Marcus for changes to take effect.", name),
		Done:    true,
	}
}
