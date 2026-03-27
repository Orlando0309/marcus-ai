package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/diff"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/spf13/cobra"
)

// NewEditCmd creates the edit command
func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <file> <instruction>",
		Short: "Edit a file using AI",
		Long:  "Opens a file, sends the instruction to the AI, shows a diff, and applies the change with confirmation.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			instruction := args[1]

			// Load config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Check file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", filePath)
			}

			// Read file
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			// Initialize folder engine
			cwd, _ := os.Getwd()
			fe := folder.NewFolderEngine(
				"",
				filepath.Join(cwd, ".marcus"),
				func(msg string) { fmt.Println(msg) },
			)
			if err := fe.Boot(); err != nil {
				fmt.Printf("Warning: folder engine boot failed: %v\n", err)
			}

			// Get flow - use flow's provider config
			flow, ok := fe.GetFlow("code_edit")
			if !ok {
				fmt.Println("Using default code_edit flow")
			} else {
				fmt.Printf("Using flow: %s\n", flow.Name)
			}

			pname := cfg.Provider
			model := cfg.Model
			if flow != nil {
				if strings.TrimSpace(flow.Model.Provider) != "" {
					pname = flow.Model.Provider
				}
				if strings.TrimSpace(flow.Model.Model) != "" {
					model = flow.Model.Model
				}
			}
			prov, err := provider.Stack(pname, model, cfg.ProviderFallbacks)
			if err != nil {
				return fmt.Errorf("provider: %w", err)
			}

			temperature := cfg.Temperature
			maxTokens := cfg.MaxTokens
			if flow != nil {
				temperature = flow.Model.Temperature
				maxTokens = flow.Model.MaxTokens
			}
			// Increase max_tokens for file editing to ensure complete response
			if maxTokens < 8192 {
				maxTokens = 8192
			}

			// Build prompt - ask for complete new file content (more reliable than diff format)
			prompt := fmt.Sprintf(`You are an AI coding assistant. Edit the following file based on the instruction.

File: %s
Instruction: %s

Current content:
%s

Return the COMPLETE new file content with all fixes applied.
Do not include markdown code blocks, no explanations - just the raw code.`, filePath, instruction, string(content))

			fmt.Printf("Sending request to %s (%s)...\n", prov.Name(), model)

			// Call provider
			ctx := context.Background()
			resp, err := prov.Complete(ctx, prompt, provider.CompletionOptions{
				Model:       model,
				Temperature: temperature,
				MaxTokens:   maxTokens,
			})
			if err != nil {
				return fmt.Errorf("provider complete: %w", err)
			}

			fmt.Printf("\nReceived response (%d tokens):\n", resp.Usage.TotalTokens)

			// Debug: save raw response to file
			os.WriteFile("debug_response.txt", []byte(resp.Text), 0644)

			// Clean up the response - strip markdown code blocks if present
			newContent := strings.TrimSpace(resp.Text)
			// Handle ```python or ``` at start
			if strings.HasPrefix(newContent, "```") {
				lines := strings.Split(newContent, "\n")
				if len(lines) > 1 {
					lines = lines[1:] // Remove first line (```python or ```)
				}
				if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
					lines = lines[:len(lines)-1] // Remove last line (```)
				}
				newContent = strings.Join(lines, "\n")
			}
			newContent = strings.TrimSpace(newContent)

			// Debug: show first few chars of response
			fmt.Printf("New content length: %d bytes\n", len(newContent))
			if len(newContent) < 100 {
				fmt.Printf("WARNING: Response seems too short, raw response saved to debug_response.txt\n")
			}

			// Generate diff from original to new content
			generatedDiff, err := diff.GenerateDiff(string(content), newContent)
			if err != nil {
				return fmt.Errorf("generate diff: %w", err)
			}

			// Render and confirm
			fmt.Println("\nProposed changes:")
			fmt.Println(diff.RenderDiff(generatedDiff))

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("\nApply this edit? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(answer)

			if answer == "y" || answer == "Y" {
				if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				fmt.Println("✓ File updated")
			} else {
				fmt.Println("Edit cancelled")
			}

			return nil
		},
	}

	return cmd
}
