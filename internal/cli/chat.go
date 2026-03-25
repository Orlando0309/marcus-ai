package cli

import (
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/tui"
	"github.com/spf13/cobra"
)

// NewChatCmd creates the chat command
func NewChatCmd() *cobra.Command {
	var resumeSession string
	var modelFlag string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive chat session with Marcus",
		Long: `Opens an interactive terminal UI for conversing with Marcus.

Use --model to specify a provider and model in the format "provider:model".
Examples:
  marcus chat --model anthropic:claude-sonnet-4-6
  marcus chat --model openai:gpt-4o
  marcus chat --model ollama:qwen3.5:397b-cloud

For providers that require API keys (anthropic, openai, groq, gemini), you will
be prompted for your API key on first use. The key is stored securely using
your OS's native credential store or encrypted file storage.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChat(resumeSession, modelFlag)
		},
	}

	cmd.Flags().StringVarP(&resumeSession, "resume", "r", "", "Resume a previous session")
	cmd.Flags().StringVarP(&modelFlag, "model", "m", "", "Provider and model to use (format: provider:model)")
	return cmd
}

// parseModelFlag parses the --model flag in format "provider:model".
// Returns provider and model. Model may be empty if not specified.
func parseModelFlag(flag string) (provider, model string, err error) {
	if flag == "" {
		return "", "", nil
	}

	parts := strings.SplitN(flag, ":", 2)
	if len(parts) < 1 {
		return "", "", fmt.Errorf("invalid --model format: %q", flag)
	}

	provider = strings.ToLower(strings.TrimSpace(parts[0]))
	if provider == "" {
		return "", "", fmt.Errorf("provider cannot be empty in --model flag")
	}

	if len(parts) > 1 {
		model = strings.TrimSpace(parts[1])
	}

	return provider, model, nil
}

// runChat handles the chat command execution with model override support.
func runChat(resumeSession, modelFlag string) error {
	// Load config first
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Parse and apply --model flag if provided
	if modelFlag != "" {
		provider, model, err := parseModelFlag(modelFlag)
		if err != nil {
			return err
		}

		// Update config with provider
		cfg.Provider = provider

		// Update model if specified
		if model != "" {
			cfg.Model = model
		}

		// Check if provider needs API key
		if config.ProviderNeedsAPIKey(provider) {
			// Try to get API key - this will prompt if not found
			_, err := config.GetAPIKey(provider)
			if err != nil {
				// API key not found, prompt user
				fmt.Printf("API key required for provider %s.\n", provider)
				_, err = config.PromptAndStoreAPIKey(provider)
				if err != nil {
					return fmt.Errorf("failed to get API key: %w", err)
				}
			}
		}
	}

	// Run TUI with the potentially modified config
	return tui.RunWithConfig(cfg, resumeSession)
}
