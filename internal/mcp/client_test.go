package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewClient(t *testing.T) {
	transport := &MockTransport{}
	client := NewClient("test", transport)

	if client.Name() != "test" {
		t.Errorf("expected name 'test', got %s", client.Name())
	}

	if client.IsReady() {
		t.Error("client should not be ready before Initialize")
	}
}

func TestMCPToolAdapter_Name(t *testing.T) {
	transport := &MockTransport{}
	client := NewClient("filesystem", transport)
	def := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
	}

	adapter := NewMCPToolAdapter(client, def)

	expected := "mcp_filesystem_read_file"
	if adapter.Name() != expected {
		t.Errorf("expected name %q, got %q", expected, adapter.Name())
	}
}

func TestMCPToolAdapter_OriginalName(t *testing.T) {
	transport := &MockTransport{}
	client := NewClient("fs", transport)
	def := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
	}

	adapter := NewMCPToolAdapter(client, def)

	if adapter.OriginalName() != "read_file" {
		t.Errorf("expected original name 'read_file', got %s", adapter.OriginalName())
	}
}

func TestMCPToolAdapter_Description(t *testing.T) {
	transport := &MockTransport{}
	client := NewClient("fs", transport)
	def := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
	}

	adapter := NewMCPToolAdapter(client, def)

	expected := "[fs] Read a file"
	if adapter.Description() != expected {
		t.Errorf("expected description %q, got %q", expected, adapter.Description())
	}
}

// MockTransport is a mock implementation of Transport for testing
type MockTransport struct {
	SendFunc func(method string, params json.RawMessage) (json.RawMessage, error)
	Closed   bool
}

func (m *MockTransport) Send(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if m.SendFunc != nil {
		return m.SendFunc(method, params)
	}
	return json.RawMessage(`{"result": {}}`), nil
}

func (m *MockTransport) Close() error {
	m.Closed = true
	return nil
}
