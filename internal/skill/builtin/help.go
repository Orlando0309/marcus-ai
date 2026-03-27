package builtin

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/marcus-ai/marcus/internal/skill"
)

// HelpSkill provides help information about available skills and commands
type HelpSkill struct {
	Registry *skill.Registry
	mu       sync.RWMutex
}

// NewHelpSkill creates a new help skill with the given registry
func NewHelpSkill(registry *skill.Registry) *HelpSkill {
	return &HelpSkill{Registry: registry}
}

func (h *HelpSkill) Name() string { return "help" }

func (h *HelpSkill) Pattern() string { return "/help" }

func (h *HelpSkill) Description() string {
	return "Show available commands and keyboard shortcuts"
}

func (h *HelpSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	var lines []string

	lines = append(lines, "Available Commands:")
	lines = append(lines, "")

	// List all registered skills
	if h.Registry != nil {
		skills := h.Registry.List()
		if len(skills) > 0 {
			lines = append(lines, "Slash Commands:")
			for _, s := range skills {
				lines = append(lines, fmt.Sprintf("  %-15s %s", s.Pattern(), s.Description()))
			}
			lines = append(lines, "")
		}
	}

	// Add keyboard shortcuts
	lines = append(lines, "Keyboard Shortcuts:")
	lines = append(lines, "  Tab          Cycle focus between panes")
	lines = append(lines, "  Ctrl+L       Clear transcript")
	lines = append(lines, "  Ctrl+Y       Apply pending actions")
	lines = append(lines, "  Ctrl+N       Discard pending actions")
	lines = append(lines, "  y/n          Approve/reject pending action (when shown)")
	lines = append(lines, "  []           Previous/next diff (when multiple)")
	lines = append(lines, "  Space        Resume step mode")
	lines = append(lines, "  Ctrl+C       Quit")
	lines = append(lines, "")

	// Add quick help
	lines = append(lines, "Quick Tips:")
	lines = append(lines, "  • Use @path to attach files to your message")
	lines = append(lines, "  • Type /clear to clear conversation history")
	lines = append(lines, "  • Type /status to see current session info")
	lines = append(lines, "  • Type /commit to generate a commit message")

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}, nil
}
