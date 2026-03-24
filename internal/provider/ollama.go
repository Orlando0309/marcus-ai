package provider

import (
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

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(model string) (*OllamaProvider, error) {
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	if model == "" {
		model = "qwen3.5:397b-cloud"
	}

	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// ollamaRequest represents the Ollama API request body
type ollamaRequest struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Stream   bool   `json:"stream"`
	Options  ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaResponse represents the Ollama API response body
type ollamaResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount     int `json:"eval_count"`
}

// Complete sends a completion request to Ollama
func (p *OllamaProvider) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		Options: ollamaOptions{
			Temperature: opts.Temperature,
			NumPredict:  opts.MaxTokens,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &CompletionResponse{
		Text: apiResp.Response,
		Usage: Usage{
			PromptTokens:     apiResp.PromptEvalCount,
			CompletionTokens: apiResp.EvalCount,
			TotalTokens:      apiResp.PromptEvalCount + apiResp.EvalCount,
		},
		FinishReason: func() string {
			if apiResp.Done {
				return "stop"
			}
			return "length"
		}(),
	}, nil
}

// CompleteStream sends a streaming completion request to Ollama
func (p *OllamaProvider) CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
		Options: ollamaOptions{
			Temperature: opts.Temperature,
			NumPredict:  opts.MaxTokens,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	ch := make(chan StreamChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var event ollamaResponse
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					ch <- StreamChunk{Done: true}
					return
				}
				ch <- StreamChunk{
					Text: fmt.Sprintf("stream error: %v", err),
					Done: true,
				}
				return
			}

			if event.Response != "" {
				ch <- StreamChunk{
					Text: event.Response,
				}
			}

			if event.Done {
				ch <- StreamChunk{
					Done: true,
					Usage: &Usage{
						PromptTokens:     event.PromptEvalCount,
						CompletionTokens: event.EvalCount,
						TotalTokens:      event.PromptEvalCount + event.EvalCount,
					},
				}
				return
			}
		}
	}()

	return ch, nil
}

// ListModels returns available models from Ollama
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// IsAvailable checks if Ollama is running
func (p *OllamaProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// FormatModel formats a model name for Ollama
func (p *OllamaProvider) FormatModel(model string) string {
	// Ollama uses colon separator: model:tag
	if !strings.Contains(model, ":") {
		return model + ":latest"
	}
	return model
}
