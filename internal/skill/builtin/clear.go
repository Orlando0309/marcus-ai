package builtin

import (
	"context"

	"github.com/marcus-ai/marcus/internal/skill"
)

// ClearSkill clears the conversation history
type ClearSkill struct{}

func (c *ClearSkill) Name() string { return "clear" }

func (c *ClearSkill) Pattern() string { return "/clear" }

func (c *ClearSkill) Description() string {
	return "Clear conversation history"
}

func (c *ClearSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	// Clear the session turns
	if deps.Session != nil {
		deps.Session.Turns = nil
		deps.Session.Actions = nil
		deps.Session.Events = nil
		deps.Session.ProviderMessages = nil
		deps.Session.LastContext = ""
	}

	// Persist the cleared session
	if deps.SessionStore != nil {
		_ = deps.SessionStore.Save(deps.Session)
	}

	return skill.Result{
		Message: "Conversation history cleared. Starting fresh session.",
		Done:    true,
	}, nil
}
