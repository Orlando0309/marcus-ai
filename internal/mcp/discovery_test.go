package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.json")

	// Test loading non-existent config returns empty config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed for non-existent file: %v", err)
	}
	if len(config.Servers) != 0 {
		t.Errorf("expected empty servers, got %d", len(config.Servers))
	}

	// Test loading valid config
	configJSON := `{
		"servers": [
			{
				"name": "filesystem",
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			},
			{
				"name": "remote",
				"url": "http://localhost:3000"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err = LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(config.Servers))
	}

	// Check first server
	if config.Servers[0].Name != "filesystem" {
		t.Errorf("expected server name 'filesystem', got %s", config.Servers[0].Name)
	}
	if config.Servers[0].Command != "npx" {
		t.Errorf("expected command 'npx', got %s", config.Servers[0].Command)
	}
	if len(config.Servers[0].Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(config.Servers[0].Args))
	}

	// Check second server
	if config.Servers[1].Name != "remote" {
		t.Errorf("expected server name 'remote', got %s", config.Servers[1].Name)
	}
	if config.Servers[1].URL != "http://localhost:3000" {
		t.Errorf("expected URL 'http://localhost:3000', got %s", config.Servers[1].URL)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.json")

	config := &Config{
		Servers: []ServerConfig{
			{
				Name:    "test",
				Command: "echo",
				Args:    []string{"hello"},
			},
		},
	}

	if err := SaveConfig(configPath, config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("Config file not created: %v", err)
	}

	// Load and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(loaded.Servers))
	}

	if loaded.Servers[0].Name != "test" {
		t.Errorf("expected name 'test', got %s", loaded.Servers[0].Name)
	}
}

func TestConfig_AddServer(t *testing.T) {
	config := &Config{
		Servers: []ServerConfig{},
	}

	// Add first server
	server1 := ServerConfig{
		Name:    "server1",
		Command: "cmd1",
	}

	if err := config.AddServer(server1); err != nil {
		t.Errorf("AddServer failed: %v", err)
	}

	if len(config.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(config.Servers))
	}

	// Add duplicate should fail
	if err := config.AddServer(server1); err == nil {
		t.Error("expected error for duplicate server name")
	}

	// Add server without name should fail
	server2 := ServerConfig{
		Command: "cmd2",
	}
	if err := config.AddServer(server2); err == nil {
		t.Error("expected error for server without name")
	}

	// Add server without command or URL should fail
	server3 := ServerConfig{
		Name: "server3",
	}
	if err := config.AddServer(server3); err == nil {
		t.Error("expected error for server without command or URL")
	}
}

func TestConfig_RemoveServer(t *testing.T) {
	config := &Config{
		Servers: []ServerConfig{
			{Name: "server1", Command: "cmd1"},
			{Name: "server2", Command: "cmd2"},
			{Name: "server3", Command: "cmd3"},
		},
	}

	// Remove existing server
	if !config.RemoveServer("server2") {
		t.Error("expected RemoveServer to return true for existing server")
	}

	if len(config.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(config.Servers))
	}

	// Remove non-existent server
	if config.RemoveServer("server2") {
		t.Error("expected RemoveServer to return false for non-existent server")
	}
}

func TestConfig_GetServer(t *testing.T) {
	config := &Config{
		Servers: []ServerConfig{
			{Name: "server1", Command: "cmd1"},
			{Name: "server2", Command: "cmd2"},
		},
	}

	server, ok := config.GetServer("server1")
	if !ok {
		t.Error("expected GetServer to return true for existing server")
	}
	if server.Name != "server1" {
		t.Errorf("expected name 'server1', got %s", server.Name)
	}

	_, ok = config.GetServer("nonexistent")
	if ok {
		t.Error("expected GetServer to return false for non-existent server")
	}
}
