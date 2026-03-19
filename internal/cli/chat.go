package cli

import (
	"github.com/marcus-ai/marcus/internal/tui"
	"github.com/spf13/cobra"
)

// NewChatCmd creates the chat command
func NewChatCmd() *cobra.Command {
	var resumeSession string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive chat session with Marcus",
		Long:  "Opens an interactive terminal UI for conversing with Marcus.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}

	cmd.Flags().StringVarP(&resumeSession, "resume", "r", "", "Resume a previous session")
	return cmd
}
