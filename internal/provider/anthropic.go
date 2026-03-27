package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AnthropicProvider implements the Provider interface for Anthropic
type AnthropicProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider() (*AnthropicProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	return &AnthropicProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		baseURL:    "https://api.anthropic.com/v1",
	}, nil
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// anthropicRequest represents the Anthropic API request body
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []messageParam     `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Thinking  *anthropicThinking `json:"thinking,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type messageParam struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// anthropicResponse represents the Anthropic API response body
type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Complete sends a completion request to Anthropic
func (p *AnthropicProvider) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error) {
	resp, err := p.CompleteRequest(ctx, promptOnlyRequest(prompt, opts))
	if err != nil {
		return nil, err
	}
	return &CompletionResponse{
		Text:         resp.Text,
		Usage:        resp.Usage,
		ToolCalls:    resp.ToolCalls,
		FinishReason: resp.FinishReason,
	}, nil
}

func (p *AnthropicProvider) CompleteRequest(ctx context.Context, req Request) (*Response, error) {
	reqBody, err := buildAnthropicRequest(req, false)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	headers := map[string]string{
		"x-api-key":         p.apiKey,
		"anthropic-version": "2023-06-01",
	}
	var apiResp anthropicResponse
	if err := postJSON(ctx, p.httpClient, p.baseURL+"/messages", headers, body, &apiResp); err != nil {
		return nil, err
	}

	var textParts []string
	var toolCalls []ToolCall
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			toolCalls = append(toolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return &Response{
		Text: strings.Join(textParts, ""),
		Usage: Usage{
			PromptTokens:     apiResp.Usage.InputTokens,
			CompletionTokens: apiResp.Usage.OutputTokens,
			TotalTokens:      apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
		},
		ToolCalls:    toolCalls,
		FinishReason: apiResp.StopReason,
	}, nil
}

func (p *AnthropicProvider) StreamRequest(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	reqBody, err := buildAnthropicRequest(req, true)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("anthropic API error %d: %s", httpResp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 32)

	go func() {
		defer close(ch)
		defer httpResp.Body.Close()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		var currentEvent string
		toolBuilders := map[int]*toolCallBuilder{}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if data == "" || data == "[DONE]" {
				continue
			}
			var event struct {
				Type    string             `json:"type"`
				Index   int                `json:"index,omitempty"`
				Message *anthropicResponse `json:"message,omitempty"`
				Delta   *struct {
					Type        string `json:"type"`
					Text        string `json:"text,omitempty"`
					PartialJSON string `json:"partial_json,omitempty"`
				} `json:"delta,omitempty"`
				ContentBlock *struct {
					Type string `json:"type"`
					ID   string `json:"id,omitempty"`
					Name string `json:"name,omitempty"`
				} `json:"content_block,omitempty"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage,omitempty"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			switch event.Type {
			case "message_start":
				if event.Message != nil {
					ch <- StreamChunk{Usage: &Usage{PromptTokens: event.Message.Usage.InputTokens}}
				}
			case "content_block_start":
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					toolBuilders[event.Index] = &toolCallBuilder{
						id:   event.ContentBlock.ID,
						name: event.ContentBlock.Name,
					}
				}
			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "text_delta":
					ch <- StreamChunk{Text: event.Delta.Text}
				case "input_json_delta":
					if builder := toolBuilders[event.Index]; builder != nil {
						builder.input.WriteString(event.Delta.PartialJSON)
					}
				}
			case "content_block_stop":
				if builder := toolBuilders[event.Index]; builder != nil {
					input := []byte(strings.TrimSpace(builder.input.String()))
					if len(input) == 0 {
						input = []byte("{}")
					}
					ch <- StreamChunk{
						ToolCall: &ToolCall{
							ID:    builder.id,
							Name:  builder.name,
							Input: json.RawMessage(input),
						},
					}
					delete(toolBuilders, event.Index)
				}
			case "message_delta":
				if event.Usage != nil {
					ch <- StreamChunk{Usage: &Usage{CompletionTokens: event.Usage.OutputTokens}}
				}
			case "message_stop":
				ch <- StreamChunk{Done: true}
				return
			default:
				if currentEvent == "message_stop" {
					ch <- StreamChunk{Done: true}
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Text: fmt.Sprintf("stream error: %v", err), Done: true}
			return
		}
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}

type toolCallBuilder struct {
	id    string
	name  string
	input strings.Builder
}

func promptOnlyRequest(prompt string, opts CompletionOptions) Request {
	return Request{
		Model:       opts.Model,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		JSON:        opts.JSON,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
	}
}

func buildAnthropicRequest(req Request, stream bool) (anthropicRequest, error) {
	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-6-20251106"
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Stream:    stream,
	}
	if req.Reasoning.BudgetTokens > 0 {
		reqBody.Thinking = &anthropicThinking{
			Type:         "enabled",
			BudgetTokens: req.Reasoning.BudgetTokens,
		}
	}
	for _, msg := range req.Messages {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		if strings.EqualFold(msg.Role, "system") {
			if reqBody.System == "" {
				reqBody.System = strings.TrimSpace(msg.Content)
			} else {
				reqBody.System += "\n\n" + strings.TrimSpace(msg.Content)
			}
			continue
		}
		reqBody.Messages = append(reqBody.Messages, messageParam{
			Role: msg.Role,
			Content: []contentBlock{
				{Type: "text", Text: msg.Content},
			},
		})
	}
	if len(reqBody.Messages) == 0 {
		reqBody.Messages = []messageParam{
			{
				Role: "user",
				Content: []contentBlock{
					{Type: "text", Text: ""},
				},
			},
		}
	}
	for _, tool := range req.Tools {
		schema := tool.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		reqBody.Tools = append(reqBody.Tools, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schema,
		})
	}
	return reqBody, nil
}

// CompleteStream sends a streaming completion request to Anthropic
func (p *AnthropicProvider) CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error) {
	return p.StreamRequest(ctx, promptOnlyRequest(prompt, opts))
}

// SSEEvent represents a server-sent event line
type SSEEvent struct {
	Event string
	Data  string
}

// parseSSELine parses a single SSE line
func parseSSELine(line string) *SSEEvent {
	if strings.HasPrefix(line, "event: ") {
		return &SSEEvent{Event: strings.TrimPrefix(line, "event: ")}
	}
	if strings.HasPrefix(line, "data: ") {
		return &SSEEvent{Data: strings.TrimPrefix(line, "data: ")}
	}
	return nil
}
