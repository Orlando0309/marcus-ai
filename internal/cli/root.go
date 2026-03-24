package cli

import (
	"github.com/marcus-ai/marcus/internal/tui"
	"github.com/marcus-ai/marcus/internal/xlog"
	"github.com/spf13/cobra"
)

func init() {
	xlog.Init()
}

// NewRootCmd creates the root command for marcus
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marcus",
		Short: "MARCUS - Multi-Agent Reasoning & Coding Unified System",
		Long: `Marcus is a terminal-native AI coding assistant.

Where most AI tools are black boxes, Marcus is a glass box —
every behavior is a file, every workflow is a folder, every memory is inspectable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}

	cmd.AddCommand(
		NewChatCmd(),
		NewEditCmd(),
		NewFlowCmd(),
		NewVersionCmd(),
		NewInitCmd(),
	)

	return cmd
}
