package provider

import (
	"fmt"
	"os"
	"strings"
)

// NewGroqProvider uses GROQ_API_KEY against Groq's OpenAI-compatible API.
func NewGroqProvider() (Provider, error) {
	key := strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("GROQ_API_KEY not set")
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("GROQ_BASE_URL")), "/")
	if base == "" {
		base = "https://api.groq.com/openai/v1"
	}
	return newChatCompat("groq", key, base)
}
