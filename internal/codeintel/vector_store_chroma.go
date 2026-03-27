package codeintel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ChromaVectorStore is a vector store using ChromaDB HTTP API
type ChromaVectorStore struct {
	client      *http.Client
	baseURL     string
	collection  string
	apiKey      string
	maxRetries  int
}

// ChromaConfig holds ChromaDB configuration
type ChromaConfig struct {
	BaseURL    string `toml:"base_url"`
	Collection string `toml:"collection"`
	APIKey     string `toml:"api_key,omitempty"`
	MaxRetries int    `toml:"max_retries,omitempty"`
}

// NewChromaVectorStore creates a new ChromaDB vector store
func NewChromaVectorStore(cfg ChromaConfig) (*ChromaVectorStore, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8000"
	}
	if cfg.Collection == "" {
		cfg.Collection = "marcus-code"
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	return &ChromaVectorStore{
		client: &http.Client{Timeout: 60 * time.Second},
		baseURL: cfg.BaseURL,
		collection: cfg.Collection,
		apiKey: cfg.APIKey,
		maxRetries: cfg.MaxRetries,
	}, nil
}

// chromaError represents a ChromaDB error response
type chromaError struct {
	Error string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

// chromaEmbedding represents an embedding record for ChromaDB
type chromaEmbedding struct {
	ID        string         `json:"id"`
	Embedding []float32      `json:"embedding"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Document  string         `json:"document,omitempty"`
}

// chromaQueryResult represents a query result from ChromaDB
type chromaQueryResult struct {
	IDs        [][]string         `json:"ids"`
	Distances  [][]float32        `json:"distances,omitempty"`
	Metadatas  [][]map[string]any `json:"metadatas,omitempty"`
	Documents  [][]string         `json:"documents,omitempty"`
}

// Store stores an embedding with metadata in ChromaDB
func (c *ChromaVectorStore) Store(id string, embedding []float32, metadata map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return c.upsertWithRetry(ctx, id, embedding, metadata)
}

// upsertWithRetry upserts an embedding with retries
func (c *ChromaVectorStore) upsertWithRetry(ctx context.Context, id string, embedding []float32, metadata map[string]any) error {
	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if err := c.upsert(ctx, id, embedding, metadata); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	return lastErr
}

// upsert upserts a single embedding
func (c *ChromaVectorStore) upsert(ctx context.Context, id string, embedding []float32, metadata map[string]any) error {
	// Prepare upsert request
	reqBody := map[string]any{
		"ids":        []string{id},
		"embeddings": [][]float32{embedding},
		"metadatas":  []map[string]any{metadata},
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal upsert request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/collections/%s/upsert", c.baseURL, c.collection),
		bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute upsert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var chromaErr chromaError
		if err := json.NewDecoder(resp.Body).Decode(&chromaErr); err == nil {
			return fmt.Errorf("upsert failed: %s - %s", chromaErr.Error, chromaErr.Detail)
		}
		return fmt.Errorf("upsert failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Query searches for similar embeddings in ChromaDB
func (c *ChromaVectorStore) Query(embedding []float32, limit int) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 10
	}

	// Prepare query request
	reqBody := map[string]any{
		"query_embeddings": [][]float32{embedding},
		"n_results":        limit,
		"include":          []string{"metadatas", "documents", "distances"},
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal query request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL, c.collection),
		bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var chromaErr chromaError
		if err := json.NewDecoder(resp.Body).Decode(&chromaErr); err == nil {
			return nil, fmt.Errorf("query failed: %s - %s", chromaErr.Error, chromaErr.Detail)
		}
		return nil, fmt.Errorf("query failed with status: %d", resp.StatusCode)
	}

	var result chromaQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert to SearchResult format
	var results []SearchResult
	if len(result.IDs) > 0 && len(result.IDs[0]) > 0 {
		for i, id := range result.IDs[0] {
			score := float32(1.0)
			if len(result.Distances) > 0 && i < len(result.Distances[0]) {
				// ChromaDB returns distance, convert to similarity
				score = 1.0 / (1.0 + result.Distances[0][i])
			}

			metadata := make(map[string]any)
			if len(result.Metadatas) > 0 && i < len(result.Metadatas[0]) {
				metadata = result.Metadatas[0][i]
			}

			results = append(results, SearchResult{
				ID:       id,
				Score:    score,
				Metadata: metadata,
			})
		}
	}

	return results, nil
}

// Delete removes an embedding from ChromaDB
func (c *ChromaVectorStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqBody := map[string]any{
		"ids": []string{id},
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal delete request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/collections/%s/delete", c.baseURL, c.collection),
		bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Size returns the number of stored embeddings
func (c *ChromaVectorStore) Size() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v1/collections/%s/count", c.baseURL, c.collection),
		nil)
	if err != nil {
		return 0
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var count int
	if err := json.NewDecoder(resp.Body).Decode(&count); err != nil {
		return 0
	}

	return count
}

// CreateCollection creates a new collection in ChromaDB
func (c *ChromaVectorStore) CreateCollection(name string, metadata map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqBody := map[string]any{
		"name":     name,
		"metadata": metadata,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/collections", c.baseURL),
		bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var chromaErr chromaError
		if err := json.NewDecoder(resp.Body).Decode(&chromaErr); err == nil {
			return fmt.Errorf("create collection failed: %s - %s", chromaErr.Error, chromaErr.Detail)
		}
		return fmt.Errorf("create collection failed with status: %d", resp.StatusCode)
	}

	return nil
}

// DeleteCollection deletes a collection from ChromaDB
func (c *ChromaVectorStore) DeleteCollection(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/v1/collections/%s", c.baseURL, name),
		nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete collection failed with status: %d", resp.StatusCode)
	}

	return nil
}

// ListCollections lists all collections in ChromaDB
func (c *ChromaVectorStore) ListCollections() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v1/collections", c.baseURL),
		nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list: %w", err)
	}
	defer resp.Body.Close()

	var collections []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&collections); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	names := make([]string, len(collections))
	for i, col := range collections {
		names[i] = col.Name
	}

	return names, nil
}

// EnsureCollection ensures a collection exists, creates it if not
func (c *ChromaVectorStore) EnsureCollection() error {
	// Try to get collection info first
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v1/collections/%s", c.baseURL, c.collection),
		nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// Connection error, try to create
		return c.CreateCollection(c.collection, map[string]any{"description": "Marcus code embeddings"})
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Collection doesn't exist, create it
		return c.CreateCollection(c.collection, map[string]any{"description": "Marcus code embeddings"})
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to check collection: %d", resp.StatusCode)
	}

	return nil
}
