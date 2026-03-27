package builtin

import (
	"context"
	"fmt"
	"time"

	"github.com/marcus-ai/marcus/internal/skill"
)

// NewSessionSkill creates a fresh session
type NewSessionSkill struct{}

func (n *NewSessionSkill) Name() string { return "newsession" }

func (n *NewSessionSkill) Pattern() string { return "/newsession" }

func (n *NewSessionSkill) Description() string {
	return "Create a fresh session (alias: /new)"
}

func (n *NewSessionSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if deps.Session == nil {
		return skill.Result{
			Message: "No active session to reset.",
			Done:    true,
		}, nil
	}

	// Clear the session
	deps.Session.ID = generateSessionID()
	deps.Session.CreatedAt = time.Now()
	deps.Session.UpdatedAt = time.Now()
	deps.Session.Turns = nil
	deps.Session.Actions = nil
	deps.Session.Events = nil
	deps.Session.ProviderMessages = nil
	deps.Session.LastContext = ""

	// Save the new session
	if deps.SessionStore != nil {
		_ = deps.SessionStore.Save(deps.Session)
	}

	return skill.Result{
		Message: "Created new session.",
		Done:    true,
	}, nil
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().Unix())
}

// NewSkill creates a /new alias for /newsession
func NewSkill() *NewSessionSkill {
	return &NewSessionSkill{}
}
