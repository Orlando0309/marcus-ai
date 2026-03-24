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

			// Build prompt - ask for unified diff format
			prompt := fmt.Sprintf(`You are an AI coding assistant. Edit the following file based on the instruction.

File: %s
Instruction: %s

Current content:
%s

Provide ONLY a unified diff showing the changes. Use this exact format:
@@ -original_start,original_count +new_start,new_count @@
-line to remove
+line to add

Do not include any explanation text - just the diff.`, filePath, instruction, string(content))

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

			// Try to parse as unified diff
			patches, err := diff.ParseUnifiedDiff(resp.Text)
			if err != nil {
				fmt.Printf("Could not parse as unified diff: %v\n", err)
				fmt.Println("Raw response:")
				fmt.Println(resp.Text)
				fmt.Println("\nFalling back to simple diff generation...")

				// Ask for the full new content
				prompt2 := fmt.Sprintf(`Given the instruction "%s", provide the complete new content for the file.

Current content:
%s

Return ONLY the new file content, no markdown, no explanations.`, instruction, string(content))

				resp2, err := prov.Complete(ctx, prompt2, provider.CompletionOptions{
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
				})
				if err != nil {
					return fmt.Errorf("provider complete v2: %w", err)
				}

				// Generate diff
				generatedDiff, err := diff.GenerateDiff(string(content), resp2.Text)
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
					if err := os.WriteFile(filePath, []byte(resp2.Text), 0644); err != nil {
						return fmt.Errorf("write file: %w", err)
					}
					fmt.Println("✓ File updated")
				} else {
					fmt.Println("Edit cancelled")
				}
				return nil
			}

			// Render the diff
			fmt.Println("\nProposed changes:")
			fmt.Println(diff.RenderDiff(resp.Text))

			// Ask for confirmation
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("\nApply this diff? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(answer)

			if answer == "y" || answer == "Y" {
				if err := diff.ApplyPatchToFile(filePath, patches); err != nil {
					return fmt.Errorf("apply patch: %w", err)
				}
				fmt.Println("✓ Diff applied successfully")
			} else {
				fmt.Println("Edit cancelled")
			}

			return nil
		},
	}

	return cmd
}
