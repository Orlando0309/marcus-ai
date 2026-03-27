package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

type Runtime struct {
	provider Provider
	cacheDir string
	useCache bool
	mu       sync.Mutex
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

func (r *Runtime) loadCache(req Request) (*Response, bool) {
	if !r.useCache || r.cacheDir == "" {
		return nil, false
	}
	path := filepath.Join(r.cacheDir, cacheKey(req)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

func (r *Runtime) saveCache(req Request, resp *Response) {
	if !r.useCache || r.cacheDir == "" || resp == nil {
		return
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(r.cacheDir, cacheKey(req)+".json"), data, 0644)
}

func cacheKey(req Request) string {
	data, _ := json.Marshal(req)
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}
