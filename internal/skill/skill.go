package skill

import (
	"context"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/tool"
)

// Skill is a user-invocable slash command like /commit, /help, etc.
type Skill interface {
	// Name returns the skill name (e.g., "commit", "help")
	Name() string

	// Pattern returns the slash command pattern (e.g., "/commit", "/help")
	Pattern() string

	// Description returns a short description for the help text
	Description() string

	// Run executes the skill with the given arguments
	// Returns a Result indicating what to display and whether to continue to AI
	Run(ctx context.Context, args []string, deps Dependencies) (Result, error)
}

// Dependencies provides access to system resources for skill execution
type Dependencies struct {
	Config       *config.Config
	ToolRunner   *tool.ToolRunner
	SessionStore *session.Store
	Session      *session.Session
	Provider     provider.Provider
	ProjectRoot  string
}

// Result is returned by a skill after execution
type Result struct {
	// Message is displayed to the user (can be empty)
	Message string

	// Done indicates whether the skill completely handled the request
	// If true, the message is shown and no AI call is made
	// If false, execution continues to the AI with the message added to context
	Done bool

	// Error indicates the skill failed (optional, errors can also be returned)
	Error string
}
