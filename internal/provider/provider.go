package provider

import (
	"context"
	"encoding/json"
)

// Provider defines the interface for LLM providers
type Provider interface {
	Name() string
	Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error)
	CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error)
}

// RequestAwareProvider can consume structured requests directly, including
// separate system/user messages and native tool definitions.
type RequestAwareProvider interface {
	CompleteRequest(ctx context.Context, req Request) (*Response, error)
	StreamRequest(ctx context.Context, req Request) (<-chan StreamChunk, error)
}

// CompletionOptions holds options for a completion request
type CompletionOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	JSON        bool
}

// CompletionResponse holds the response from an LLM
type CompletionResponse struct {
	Text         string
	Usage        Usage
	ToolCalls    []ToolCall
	FinishReason string
}

// Usage holds token usage information
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ToolCall represents a tool call from the LLM
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type ReasoningOptions struct {
	Effort       string `json:"effort,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// StreamChunk represents a chunk of streamed response
type StreamChunk struct {
	Text     string
	Done     bool
	ToolCall *ToolCall
	Usage    *Usage
}
