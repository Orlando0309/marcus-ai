package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/skill"
)

// StatusSkill shows current session status
type StatusSkill struct{}

func (s *StatusSkill) Name() string { return "status" }

func (s *StatusSkill) Pattern() string { return "/status" }

func (s *StatusSkill) Description() string {
	return "Show current session status"
}

func (s *StatusSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	var lines []string

	lines = append(lines, "Session Status")
	lines = append(lines, strings.Repeat("=", 40))

	// Provider info
	if deps.Config != nil {
		lines = append(lines, fmt.Sprintf("Provider: %s", deps.Config.Provider))
		lines = append(lines, fmt.Sprintf("Model:    %s", deps.Config.Model))
		lines = append(lines, "")
	}

	// Session info
	if deps.Session != nil {
		turnCount := len(deps.Session.Turns)
		lines = append(lines, fmt.Sprintf("Conversation turns: %d", turnCount))
		if deps.Config != nil {
			lines = append(lines, fmt.Sprintf("Max turns:          %d", deps.Config.Session.MaxTurns))
		}
		lines = append(lines, "")
	}

	// Project info
	if deps.ProjectRoot != "" {
		lines = append(lines, fmt.Sprintf("Project root: %s", deps.ProjectRoot))
	}

	// Token usage info (if available)
	// Note: This would need context assembler integration for full token count
	if deps.Config != nil {
		lines = append(lines, "")
		lines = append(lines, "Context Settings:")
		lines = append(lines, fmt.Sprintf("  Max tokens:      %d", deps.Config.Context.MaxContextTokens))
		lines = append(lines, fmt.Sprintf("  Warn at:         %d%%", deps.Config.Context.WarnAtPercent))
		lines = append(lines, fmt.Sprintf("  Compact at:      %d%%", deps.Config.Context.CompactAtPercent))
	}

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}, nil
}
