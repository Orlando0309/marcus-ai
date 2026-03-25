package codeintel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EmbeddingProvider generates embeddings for text/code
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// OpenAIEmbeddingProvider uses OpenAI's embedding API
type OpenAIEmbeddingProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
	baseURL    string
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider
func NewOpenAIEmbeddingProvider(apiKey, model string) *OpenAIEmbeddingProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    "https://api.openai.com/v1",
	}
}

// Embed generates embeddings for the given texts
func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// OpenAI embedding request
	reqBody := map[string]any{
		"input": texts,
		"model": p.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

// Dimensions returns the embedding dimensions based on model
func (p *OpenAIEmbeddingProvider) Dimensions() int {
	switch p.model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default:
		return 1536
	}
}

// OllamaEmbeddingProvider uses Ollama's embedding API
type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider
func NewOllamaEmbeddingProvider(baseURL, model string) *OllamaEmbeddingProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbeddingProvider{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Embed generates embeddings for the given texts
func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	embeddings := make([][]float32, len(texts))

	// Ollama embeddings API processes one text at a time
	for i, text := range texts {
		reqBody := map[string]any{
			"model": p.model,
			"prompt": text,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/embeddings", strings.NewReader(string(jsonBody)))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("execute request: %w", err)
		}

		var result struct {
			Embedding []float32 `json:"embedding"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		embeddings[i] = result.Embedding
	}

	return embeddings, nil
}

// Dimensions returns the embedding dimensions (typically 768 for nomic-embed-text)
func (p *OllamaEmbeddingProvider) Dimensions() int {
	return 768
}

// MockEmbeddingProvider is a mock provider for testing
type MockEmbeddingProvider struct {
	dimensions int
}

// NewMockEmbeddingProvider creates a new mock embedding provider
func NewMockEmbeddingProvider(dimensions int) *MockEmbeddingProvider {
	if dimensions == 0 {
		dimensions = 128
	}
	return &MockEmbeddingProvider{dimensions: dimensions}
}

// Embed generates fake embeddings for testing
func (p *MockEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embedding := make([]float32, p.dimensions)
		// Generate deterministic "random" values based on text hash
		hash := hashString(texts[i])
		for j := range embedding {
			embedding[j] = float32(int(hash+uint64(j)*31)%1000) / 1000.0
		}
		embeddings[i] = embedding
	}
	return embeddings, nil
}

// Dimensions returns the embedding dimensions
func (p *MockEmbeddingProvider) Dimensions() int {
	return p.dimensions
}

// hashString generates a simple hash for deterministic embeddings
func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037 // FNV offset basis
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211 // FNV prime
	}
	return h
}

// EmbeddingConfig holds configuration for embedding providers
type EmbeddingConfig struct {
	Provider   string `toml:"provider"`   // "openai", "ollama", "mock"
	Model      string `toml:"model"`
	APIKey     string `toml:"api_key,omitempty"`
	BaseURL    string `toml:"base_url,omitempty"`
	Dimensions int    `toml:"dimensions,omitempty"`
}

// NewEmbeddingProviderFromConfig creates an embedding provider from config
func NewEmbeddingProviderFromConfig(cfg EmbeddingConfig) (EmbeddingProvider, error) {
	switch cfg.Provider {
	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenAI provider requires API key")
		}
		return NewOpenAIEmbeddingProvider(cfg.APIKey, cfg.Model), nil
	case "ollama":
		return NewOllamaEmbeddingProvider(cfg.BaseURL, cfg.Model), nil
	case "mock":
		return NewMockEmbeddingProvider(cfg.Dimensions), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}

// EmbeddingCache caches embeddings to avoid recomputing
type EmbeddingCache struct {
	cache map[string][]float32
}

// NewEmbeddingCache creates a new embedding cache
func NewEmbeddingCache() *EmbeddingCache {
	return &EmbeddingCache{
		cache: make(map[string][]float32),
	}
}

// Get retrieves a cached embedding
func (c *EmbeddingCache) Get(key string) ([]float32, bool) {
	val, ok := c.cache[key]
	return val, ok
}

// Set stores an embedding in the cache
func (c *EmbeddingCache) Set(key string, embedding []float32) {
	c.cache[key] = embedding
}

// Has checks if a key exists in the cache
func (c *EmbeddingCache) Has(key string) bool {
	_, ok := c.cache[key]
	return ok
}

// Clear clears the cache
func (c *EmbeddingCache) Clear() {
	c.cache = make(map[string][]float32)
}

// Size returns the number of cached embeddings
func (c *EmbeddingCache) Size() int {
	return len(c.cache)
}
