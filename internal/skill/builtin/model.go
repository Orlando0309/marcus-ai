package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/skill"
)

// ModelSkill switches the provider/model dynamically
type ModelSkill struct{}

func (m *ModelSkill) Name() string { return "model" }

func (m *ModelSkill) Pattern() string { return "/model" }

func (m *ModelSkill) Description() string {
	return "Switch provider/model (e.g., /model anthropic:claude-sonnet-4-6)"
}

func (m *ModelSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		// Show current model
		if deps.Config != nil {
			return skill.Result{
				Message: fmt.Sprintf("Current provider: %s\nCurrent model: %s", deps.Config.Provider, deps.Config.Model),
				Done:    true,
			}, nil
		}
		return skill.Result{
			Message: "Usage: /model <provider:model>",
			Done:    true,
		}, nil
	}

	// Parse provider:model from first argument
	providerArg := args[0]
	var newProvider, newModel string

	if strings.Contains(providerArg, ":") {
		parts := strings.SplitN(providerArg, ":", 2)
		newProvider = strings.ToLower(strings.TrimSpace(parts[0]))
		newModel = strings.TrimSpace(parts[1])
	} else {
		// Just provider specified, use default model
		newProvider = strings.ToLower(strings.TrimSpace(providerArg))
	}

	// Validate provider
	validProviders := []string{"anthropic", "openai", "ollama", "groq", "gemini"}
	isValid := false
	for _, p := range validProviders {
		if p == newProvider {
			isValid = true
			break
		}
	}

	if !isValid {
		return skill.Result{
			Message: fmt.Sprintf("Invalid provider '%s'. Valid providers: %s", newProvider, strings.Join(validProviders, ", ")),
			Done:    true,
		}, nil
	}

	// Check if API key is needed
	if config.ProviderNeedsAPIKey(newProvider) {
		_, err := config.GetAPIKey(newProvider)
		if err != nil {
			return skill.Result{
				Message: fmt.Sprintf("Provider '%s' requires an API key. Set %s, or restart Marcus with `chat --model %s[:model]` to be prompted once and store it securely.",
					newProvider, config.ProviderAPIKeyEnvVar(newProvider), newProvider),
				Done: true,
			}, nil
		}
	}

	// Update config
	if deps.Config != nil {
		oldProvider := deps.Config.Provider
		oldModel := deps.Config.Model

		deps.Config.Provider = newProvider
		if newModel != "" {
			deps.Config.Model = newModel
		}

		// Save config to file
		// Note: In a full implementation, we'd update the config file
		// For now, just update in-memory

		return skill.Result{
			Message: fmt.Sprintf("Switched provider:\n  %s/%s -> %s/%s", oldProvider, oldModel, newProvider, deps.Config.Model),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: "No configuration available",
		Done:    true,
	}, nil
}
