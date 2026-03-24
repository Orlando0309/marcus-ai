package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// GeminiProvider uses Google AI generateContent (API key auth).
type GeminiProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewGeminiProvider reads GEMINI_API_KEY (Google AI Studio).
func NewGeminiProvider() (*GeminiProvider, error) {
	key := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}
	return &GeminiProvider{
		apiKey:     key,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error) {
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

func (p *GeminiProvider) CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error) {
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

type geminiGenRequest struct {
	SystemInstruction *struct {
		Parts []geminiPart `json:"parts"`
	} `json:"systemInstruction,omitempty"`
	Contents         []geminiContent    `json:"contents"`
	Tools            []geminiTool       `json:"tools,omitempty"`
	ToolConfig       *geminiToolConfig  `json:"toolConfig,omitempty"`
	GenerationConfig geminiGenConfig    `json:"generationConfig"`
}

type geminiGenConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string            `json:"text,omitempty"`
	FunctionCall *geminiFnCallPart `json:"functionCall,omitempty"`
}

type geminiFnCallPart struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFnDecl `json:"functionDeclarations"`
}

type geminiFnDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiToolConfig struct {
	FunctionCallingConfig struct {
		Mode string `json:"mode"`
	} `json:"functionCallingConfig"`
}

type geminiGenResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (p *GeminiProvider) CompleteRequest(ctx context.Context, req Request) (*Response, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gemini-2.0-flash"
	}
	if !strings.Contains(model, "/") {
		model = "models/" + model
	}
	maxOut := req.MaxTokens
	if maxOut == 0 {
		maxOut = 4096
	}

	var body geminiGenRequest
	body.GenerationConfig.Temperature = req.Temperature
	body.GenerationConfig.MaxOutputTokens = maxOut

	for _, msg := range req.Messages {
		c := strings.TrimSpace(msg.Content)
		if c == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system":
			if body.SystemInstruction == nil {
				body.SystemInstruction = &struct {
					Parts []geminiPart `json:"parts"`
				}{}
			}
			body.SystemInstruction.Parts = append(body.SystemInstruction.Parts, geminiPart{Text: c})
		case "assistant":
			body.Contents = append(body.Contents, geminiContent{
				Role:  "model",
				Parts: []geminiPart{{Text: c}},
			})
		default:
			body.Contents = append(body.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: c}},
			})
		}
	}
	if len(body.Contents) == 0 {
		body.Contents = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: ""}}}}
	}

	if len(req.Tools) > 0 {
		var decls []geminiFnDecl
		for _, t := range req.Tools {
			schema := t.Schema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, geminiFnDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			})
		}
		body.Tools = []geminiTool{{FunctionDeclarations: decls}}
		body.ToolConfig = &geminiToolConfig{}
		body.ToolConfig.FunctionCallingConfig.Mode = "AUTO"
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s:generateContent?key=%s",
		model, url.QueryEscape(p.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	rb, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini %d: %s", httpResp.StatusCode, string(bytes.TrimSpace(rb)))
	}
	var apiResp geminiGenResponse
	if err := json.Unmarshal(rb, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if len(apiResp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates")
	}
	var textParts []string
	var calls []ToolCall
	for _, part := range apiResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			args := part.FunctionCall.Args
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			calls = append(calls, ToolCall{
				Name:  part.FunctionCall.Name,
				Input: args,
			})
		}
	}
	return &Response{
		Text:         strings.TrimSpace(strings.Join(textParts, "")),
		Usage:        Usage{PromptTokens: apiResp.UsageMetadata.PromptTokenCount, CompletionTokens: apiResp.UsageMetadata.CandidatesTokenCount, TotalTokens: apiResp.UsageMetadata.TotalTokenCount},
		ToolCalls:    calls,
		FinishReason: apiResp.Candidates[0].FinishReason,
	}, nil
}

func (p *GeminiProvider) StreamRequest(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	resp, err := p.CompleteRequest(ctx, req)
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

var _ RequestAwareProvider = (*GeminiProvider)(nil)
