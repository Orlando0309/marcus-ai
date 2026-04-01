package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/metrics"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

type Request struct {
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	JSON        bool              `json:"json"`
	Messages    []Message         `json:"messages"`
	Tools       []ToolSpec        `json:"tools,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Reasoning   ReasoningOptions  `json:"reasoning,omitempty"`
}

type Response struct {
	Text         string     `json:"text"`
	Usage        Usage      `json:"usage"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Cached       bool       `json:"cached,omitempty"`
}

type StreamEvent struct {
	Kind     string    `json:"kind"`
	Text     string    `json:"text,omitempty"`
	ToolCall *ToolCall `json:"tool_call,omitempty"`
	Usage    *Usage    `json:"usage,omitempty"`
	Done     bool      `json:"done,omitempty"`
}

// RateLimiter controls the rate of API requests.
type RateLimiter struct {
	mu         sync.Mutex
	requests   []time.Time
	maxReqs    int
	window     time.Duration
	lastWaited time.Time
}

// NewRateLimiter creates a rate limiter with the specified max requests per window.
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		maxReqs:  maxRequests,
		window:   window,
		requests: make([]time.Time, 0, maxRequests),
	}
}

// Wait blocks until a request slot is available. Returns the wait duration (0 if no wait).
func (rl *RateLimiter) Wait() time.Duration {
	if rl == nil || rl.maxReqs <= 0 {
		return 0
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove old requests outside the window
	valid := 0
	for _, t := range rl.requests {
		if t.After(cutoff) {
			rl.requests[valid] = t
			valid++
		}
	}
	rl.requests = rl.requests[:valid]

	// If at capacity, wait until oldest request expires
	if len(rl.requests) >= rl.maxReqs {
		oldest := rl.requests[0]
		waitUntil := oldest.Add(rl.window)
		wait := time.Until(waitUntil)
		if wait > 0 {
			rl.lastWaited = now
			time.Sleep(wait)
			// Remove the expired request after waiting
			now = time.Now()
			cutoff = now.Add(-rl.window)
			valid := 0
			for _, t := range rl.requests {
				if t.After(cutoff) {
					rl.requests[valid] = t
					valid++
				}
			}
			rl.requests = rl.requests[:valid]
		}
	}

	// Record this request
	rl.requests = append(rl.requests, time.Now())
	return 0
}

// Runtime manages provider interactions with caching and rate limiting.
type Runtime struct {
	provider     Provider
	cacheDir     string
	useCache     bool
	rateLimiter  *RateLimiter
	mu           sync.Mutex
}

// RuntimeConfig holds optional runtime configuration.
type RuntimeConfig struct {
	MaxRequestsPerMinute int
}

// NewRuntimeWithConfig creates a runtime with optional rate limiting.
func NewRuntimeWithConfig(provider Provider, projectRoot string, useCache bool, cfg RuntimeConfig) *Runtime {
	runtime := NewRuntime(provider, projectRoot, useCache)
	if cfg.MaxRequestsPerMinute > 0 {
		runtime.rateLimiter = NewRateLimiter(cfg.MaxRequestsPerMinute, time.Minute)
	}
	return runtime
}

func NewRuntime(provider Provider, projectRoot string, useCache bool) *Runtime {
	cacheDir := ""
	if projectRoot != "" {
		cacheDir = filepath.Join(projectRoot, ".marcus", "cache", "provider")
		_ = os.MkdirAll(cacheDir, 0755)
	}
	return &Runtime{
		provider: provider,
		cacheDir: cacheDir,
		useCache: useCache,
	}
}

func (r *Runtime) Complete(ctx context.Context, req Request) (*Response, error) {
	// Apply rate limiting
	if r.rateLimiter != nil {
		r.rateLimiter.Wait()
	}

	if cached, ok := r.loadCache(req); ok {
		metrics.RecordProviderCacheHit()
		cached.Cached = true
		return cached, nil
	}
	if direct, ok := r.provider.(RequestAwareProvider); ok {
		resp, err := direct.CompleteRequest(ctx, req)
		if err != nil {
			metrics.RecordProviderComplete(false)
			return nil, err
		}
		metrics.RecordProviderComplete(true)
		r.saveCache(req, resp)
		return resp, nil
	}
	prompt := renderRequest(req)
	resp, err := r.provider.Complete(ctx, prompt, CompletionOptions{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		JSON:        req.JSON,
	})
	if err != nil {
		metrics.RecordProviderComplete(false)
		return nil, err
	}
	metrics.RecordProviderComplete(true)
	result := &Response{
		Text:         resp.Text,
		Usage:        resp.Usage,
		ToolCalls:    resp.ToolCalls,
		FinishReason: resp.FinishReason,
	}
	r.saveCache(req, result)
	return result, nil
}

func (r *Runtime) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	if direct, ok := r.provider.(RequestAwareProvider); ok {
		stream, err := direct.StreamRequest(ctx, req)
		if err != nil {
			return nil, err
		}
		out := make(chan StreamEvent, 32)
		go func() {
			defer close(out)
			for chunk := range stream {
				event := StreamEvent{
					Kind:     "message_delta",
					Text:     chunk.Text,
					ToolCall: chunk.ToolCall,
					Usage:    chunk.Usage,
					Done:     chunk.Done,
				}
				if chunk.Done {
					event.Kind = "done"
				} else if chunk.ToolCall != nil {
					event.Kind = "tool_call"
				}
				out <- event
			}
		}()
		return out, nil
	}
	prompt := renderRequest(req)
	stream, err := r.provider.CompleteStream(ctx, prompt, CompletionOptions{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		JSON:        req.JSON,
	})
	if err != nil {
		return nil, err
	}
	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)
		for chunk := range stream {
			event := StreamEvent{
				Kind:     "message_delta",
				Text:     chunk.Text,
				ToolCall: chunk.ToolCall,
				Usage:    chunk.Usage,
				Done:     chunk.Done,
			}
			if chunk.Done {
				event.Kind = "done"
			} else if chunk.ToolCall != nil {
				event.Kind = "tool_call"
			}
			out <- event
		}
	}()
	return out, nil
}

func (r *Runtime) Batch(ctx context.Context, requests []Request) ([]*Response, error) {
	responses := make([]*Response, 0, len(requests))
	for _, req := range requests {
		resp, err := r.Complete(ctx, req)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

func renderRequest(req Request) string {
	var sections []string
	for _, msg := range req.Messages {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "USER"
		}
		sections = append(sections, role+":\n"+strings.TrimSpace(msg.Content))
	}
	if len(req.Tools) > 0 {
		var lines []string
		for _, tool := range req.Tools {
			line := "- " + tool.Name + ": " + strings.TrimSpace(tool.Description)
			if len(tool.Schema) > 0 {
				line += "\n  schema: " + string(tool.Schema)
			}
			lines = append(lines, line)
		}
		sections = append(sections, "AVAILABLE TOOLS:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

// CachedResponse wraps a response with integrity metadata.
type CachedResponse struct {
	Response Response `json:"response"`
	HMAC     string   `json:"hmac"`
}

func (r *Runtime) loadCache(req Request) (*Response, bool) {
	if !r.useCache || r.cacheDir == "" {
		return nil, false
	}
	path := filepath.Join(r.cacheDir, cacheKey(req)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var cached CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}

	// Verify integrity
	respJSON, _ := json.Marshal(cached.Response)
	if !verifyCacheHMAC(respJSON, cached.HMAC) {
		// Cache tampering detected - remove corrupted file
		_ = os.Remove(path)
		return nil, false
	}

	return &cached.Response, true
}

func (r *Runtime) saveCache(req Request, resp *Response) {
	if !r.useCache || r.cacheDir == "" || resp == nil {
		return
	}

	respJSON, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return
	}

	// Compute integrity HMAC
	hmacValue := computeCacheHMAC(respJSON)

	cached := CachedResponse{
		Response: *resp,
		HMAC:     hmacValue,
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(r.cacheDir, cacheKey(req)+".json"), data, 0600)
}

// cacheKeyHash computes a SHA256 hash of the request for cache lookup.
func cacheKey(req Request) string {
	data, _ := json.Marshal(req)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// cacheHMACKey is derived at runtime from environment for cache integrity.
// It prevents cache poisoning by ensuring only processes with the same key can read/write cache.
var cacheHMACKey = []byte(os.Getenv("MARCUS_CACHE_HMAC_KEY"))

// initCacheHMACKey initializes the HMAC key from environment or generates a safe default.
func initCacheHMACKey() {
	if len(cacheHMACKey) == 0 {
		// Derive a machine-specific key if not provided
		hostname, _ := os.Hostname()
		homeDir, _ := os.UserHomeDir()
		keyData := append([]byte(hostname), []byte(homeDir)...)
		hash := sha256.Sum256(keyData)
		cacheHMACKey = hash[:]
	}
}

// computeCacheHMAC computes an HMAC-SHA256 over cache data for integrity verification.
func computeCacheHMAC(data []byte) string {
	if len(cacheHMACKey) == 0 {
		initCacheHMACKey()
	}
	mac := hmac.New(sha256.New, cacheHMACKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyCacheHMAC verifies the HMAC of cached data to detect tampering.
func verifyCacheHMAC(data []byte, expectedMAC string) bool {
	if len(cacheHMACKey) == 0 {
		initCacheHMACKey()
	}
	actualMAC := computeCacheHMAC(data)
	return hmac.Equal([]byte(actualMAC), []byte(expectedMAC))
}
