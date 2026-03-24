package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// chatCompatProvider implements OpenAI-compatible /v1/chat/completions (OpenAI, Groq, etc.).
type chatCompatProvider struct {
	name       string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newChatCompat(name, apiKey, baseURL string) (*chatCompatProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s: API key not set", name)
	}
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("%s: base URL empty", name)
	}
	return &chatCompatProvider{
		name:       name,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (p *chatCompatProvider) Name() string { return p.name }

func (p *chatCompatProvider) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error) {
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

func (p *chatCompatProvider) CompleteRequest(ctx context.Context, req Request) (*Response, error) {
	body, err := buildOpenAIRequest(req, false)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Authorization": "Bearer " + p.apiKey,
	}
	var apiResp openAIResponse
	if err := postJSON(ctx, p.httpClient, p.baseURL+"/chat/completions", headers, raw, &apiResp); err != nil {
		return nil, err
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("%s: empty choices", p.name)
	}
	msg := apiResp.Choices[0].Message
	var calls []ToolCall
	for _, tc := range msg.ToolCalls {
		if tc.Type != "" && tc.Type != "function" {
			continue
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}
	return &Response{
		Text:         strings.TrimSpace(msg.Content),
		Usage:        Usage{PromptTokens: apiResp.Usage.PromptTokens, CompletionTokens: apiResp.Usage.CompletionTokens, TotalTokens: apiResp.Usage.TotalTokens},
		ToolCalls:    calls,
		FinishReason: apiResp.Choices[0].FinishReason,
	}, nil
}

func (p *chatCompatProvider) CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error) {
	resp, err := p.Complete(ctx, prompt, opts)
	ch := make(chan StreamChunk, 2)
	go func() {
		defer close(ch)
		if err != nil {
			ch <- StreamChunk{Text: fmt.Sprintf("error: %v", err), Done: true}
			return
		}
		if resp.Text != "" {
			ch <- StreamChunk{Text: resp.Text}
		}
		u := resp.Usage
		ch <- StreamChunk{Done: true, Usage: &u}
	}()
	return ch, nil
}

func (p *chatCompatProvider) StreamRequest(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	body, err := buildOpenAIRequest(req, true)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		return nil, fmt.Errorf("%s stream %d: %s", p.name, httpResp.StatusCode, string(b))
	}

	ch := make(chan StreamChunk, 32)
	go func() {
		defer httpResp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(httpResp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}
			var ev struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			if len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
				ch <- StreamChunk{Text: ev.Choices[0].Delta.Content}
			}
		}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}

var _ RequestAwareProvider = (*chatCompatProvider)(nil)
