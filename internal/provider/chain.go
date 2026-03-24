package provider

import (
	"context"
	"fmt"
	"strings"
)

type chain struct {
	provs []Provider
}

// BuildChain tries each provider name in order; collects those that construct successfully.
// If more than one, returns a fallback chain for CompleteRequest / CompleteStream.
// Stack builds a chain from a primary provider name and optional fallbacks (deduplicated).
func Stack(primary, model string, fallbacks []string) (Provider, error) {
	names := []string{strings.TrimSpace(primary)}
	if names[0] == "" {
		return nil, fmt.Errorf("empty primary provider")
	}
	for _, fb := range fallbacks {
		fb = strings.TrimSpace(fb)
		if fb == "" {
			continue
		}
		dup := false
		for _, ex := range names {
			if strings.EqualFold(ex, fb) {
				dup = true
				break
			}
		}
		if !dup {
			names = append(names, fb)
		}
	}
	return BuildChain(names, model)
}

// BuildChain tries each provider name in order; collects those that construct successfully.
func BuildChain(names []string, model string) (Provider, error) {
	var provs []Provider
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		p, err := singleNamedProvider(n, model)
		if err != nil {
			continue
		}
		provs = append(provs, p)
	}
	if len(provs) == 0 {
		return nil, fmt.Errorf("no working provider (tried: %v)", names)
	}
	if len(provs) == 1 {
		return provs[0], nil
	}
	return &chain{provs: provs}, nil
}

func singleNamedProvider(name, model string) (Provider, error) {
	switch strings.ToLower(name) {
	case "anthropic":
		return NewAnthropicProvider()
	case "openai":
		return NewOpenAIProvider()
	case "groq":
		return NewGroqProvider()
	case "gemini":
		return NewGeminiProvider()
	case "ollama":
		return NewOllamaProvider(model)
	default:
		return NewOllamaProvider(model)
	}
}

func (c *chain) Name() string {
	var parts []string
	for _, p := range c.provs {
		parts = append(parts, p.Name())
	}
	return "chain:" + strings.Join(parts, ",")
}

func (c *chain) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error) {
	var last error
	for _, p := range c.provs {
		out, err := p.Complete(ctx, prompt, opts)
		if err == nil {
			return out, nil
		}
		last = err
	}
	return nil, last
}

func (c *chain) CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error) {
	var last error
	for _, p := range c.provs {
		ch, err := p.CompleteStream(ctx, prompt, opts)
		if err == nil {
			return ch, nil
		}
		last = err
	}
	return nil, last
}

func (c *chain) CompleteRequest(ctx context.Context, req Request) (*Response, error) {
	var last error
	for _, p := range c.provs {
		ra, ok := p.(RequestAwareProvider)
		if !ok {
			continue
		}
		out, err := ra.CompleteRequest(ctx, req)
		if err == nil {
			return out, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("no request-aware provider in chain")
	}
	return nil, last
}

func (c *chain) StreamRequest(ctx context.Context, req Request) (<-chan StreamChunk, error) {
	var last error
	for _, p := range c.provs {
		ra, ok := p.(RequestAwareProvider)
		if !ok {
			continue
		}
		ch, err := ra.StreamRequest(ctx, req)
		if err == nil {
			return ch, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("no streaming provider in chain")
	}
	return nil, last
}

var _ RequestAwareProvider = (*chain)(nil)
