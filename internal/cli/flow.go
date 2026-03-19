package cli

import (
	"fmt"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/spf13/cobra"
)

// NewFlowCmd creates the flow command
func NewFlowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Run a Marcus flow",
		Long:  "Execute a predefined flow from the .marcus/flows/ directory.",
	}

	cmd.AddCommand(
		NewFlowRunCmd(),
		NewFlowListCmd(),
	)

	return cmd
}

// NewFlowRunCmd creates the flow run subcommand
func NewFlowRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <flow-name>",
		Short: "Run a specific flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowName := args[0]
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			engine, err := flow.NewEngine(cfg, func(msg string) { fmt.Println(msg) })
			if err != nil {
				return fmt.Errorf("init flow engine: %w", err)
			}
			flowDef, ok := engine.GetFlow(flowName)
			if !ok {
				return fmt.Errorf("flow not found: %s", flowName)
			}

			fmt.Printf("Running flow: %s\n", flowDef.Name)
			fmt.Printf("Description: %s\n", flowDef.Description)
			fmt.Printf("Path: %s\n", flowDef.Path)
			fmt.Printf("Provider: %s\n", flowDef.Model.Provider)
			fmt.Printf("Model: %s\n", flowDef.Model.Model)
			fmt.Printf("Temperature: %.2f\n", flowDef.Model.Temperature)

			prompt, err := flowDef.ReadPrompt("")
			if err != nil {
				return fmt.Errorf("read prompt: %w", err)
			}

			fmt.Printf("\nPrompt preview (%d chars):\n", len(prompt))
			fmt.Println("---")
			if len(prompt) > 500 {
				fmt.Println(prompt[:500])
				fmt.Println("...")
			} else {
				fmt.Println(prompt)
			}

			return nil
		},
	}
}

// NewFlowListCmd creates the flow list subcommand
func NewFlowListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			engine, err := flow.NewEngine(cfg, nil)
			if err != nil {
				return fmt.Errorf("init flow engine: %w", err)
			}

			flows := engine.ListFlows()
			if len(flows) == 0 {
				fmt.Println("No flows found")
				return nil
			}

			fmt.Println("Available flows:")
			for _, name := range flows {
				flowDef, _ := engine.GetFlow(name)
				fmt.Printf("  %-20s %s\n", name, flowDef.Description)
			}

			return nil
		},
	}
}
