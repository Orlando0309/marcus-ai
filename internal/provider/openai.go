package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NewOpenAIProvider reads OPENAI_API_KEY and optional OPENAI_BASE_URL.
func NewOpenAIProvider() (Provider, error) {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return newChatCompat("openai", key, base)
}

type openAIMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []openAIToolUse `json:"tool_calls,omitempty"`
}

type openAIToolUse struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIRequest struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	Tools           []openAITool    `json:"tools,omitempty"`
	Temperature     float64         `json:"temperature,omitempty"`
	MaxTokens       int             `json:"max_tokens,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Role      string          `json:"role"`
			Content   string          `json:"content"`
			ToolCalls []openAIToolUse `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func buildOpenAIRequest(req Request, stream bool) (openAIRequest, error) {
	model := req.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 4096
	}
	out := openAIRequest{
		Model:       model,
		Temperature: req.Temperature,
		MaxTokens:   maxTok,
		Stream:      stream,
	}
	if effort := strings.TrimSpace(req.Reasoning.Effort); effort != "" {
		out.ReasoningEffort = effort
	}
	for _, msg := range req.Messages {
		c := strings.TrimSpace(msg.Content)
		if c == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" || role == "user" || role == "assistant" || role == "tool" {
			out.Messages = append(out.Messages, openAIMessage{Role: role, Content: c})
		} else {
			out.Messages = append(out.Messages, openAIMessage{Role: "user", Content: c})
		}
	}
	if len(out.Messages) == 0 {
		out.Messages = []openAIMessage{{Role: "user", Content: ""}}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var tool openAITool
		tool.Type = "function"
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = schema
		out.Tools = append(out.Tools, tool)
	}
	return out, nil
}
