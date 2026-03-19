package cli

import (
	"fmt"
	"path/filepath"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init command
func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Initialize Marcus in a project directory",
		Long:  "Creates .marcus/ directory with initial configuration files.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}

			absRoot, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			if err := config.InitProject(absRoot); err != nil {
				return fmt.Errorf("failed to init project: %w", err)
			}

			fmt.Printf("✓ Initialized Marcus in %s\n", absRoot)
			fmt.Println("  Created .marcus/marcus.toml")
			fmt.Println("  Created .marcus/agents/coding_agent/")
			fmt.Println("  Created .marcus/agents/general_agent/")
			fmt.Println("  Created .marcus/context/")
			fmt.Println("  Created .marcus/flows/ (app_plan, app_scaffold, chat, code_edit, create_todo)")
			fmt.Println("  Created .marcus/tools/list_python_files/")
			fmt.Println("  Created .marcus/memory/")
			fmt.Println("  Created .marcus/sessions/")
			fmt.Println("  Created .marcus/tasks/")
			return nil
		},
	}
}
