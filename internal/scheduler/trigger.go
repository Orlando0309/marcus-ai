package scheduler

import (
	"time"
)

// TriggerType represents the type of trigger
type TriggerType string

const (
	TriggerCron    TriggerType = "cron"
	TriggerFile    TriggerType = "file"
	TriggerWebhook TriggerType = "webhook"
	TriggerEvent   TriggerType = "event"
)

// Trigger represents a scheduled or event-based trigger
type Trigger struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Type       TriggerType   `json:"type"`
	Enabled    bool          `json:"enabled"`
	Config     TriggerConfig `json:"config"`
	Action     ActionConfig  `json:"action"`
	LastRun    time.Time     `json:"last_run"`
	NextRun    *time.Time    `json:"next_run,omitempty"`
	RunCount   int           `json:"run_count"`
	ErrorCount int           `json:"error_count"`
	LastError  string        `json:"last_error,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

// TriggerConfig contains type-specific configuration
type TriggerConfig struct {
	// For Cron triggers
	CronExpression string `json:"cron_expression,omitempty"`

	// For File triggers
	WatchPath   string   `json:"watch_path,omitempty"`
	WatchEvents []string `json:"watch_events,omitempty"` // "create", "modify", "delete"

	// For Webhook triggers
	WebhookPath string `json:"webhook_path,omitempty"` // e.g., "/webhook/github"
	Secret      string `json:"secret,omitempty"`       // for signature verification

	// For Event triggers
	EventTypes []string `json:"event_types,omitempty"` // e.g., ["git.commit", "build.failed"]
}

// ActionConfig defines what action to take when trigger fires
type ActionConfig struct {
	Type   string            `json:"type"`   // "flow", "agent", "command", "skill"
	Target string            `json:"target"` // flow name, agent name, command, or skill pattern
	Input  map[string]string `json:"input,omitempty"`
}

// Event represents an event that can trigger actions
type Event struct {
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Source    string                 `json:"source"`
}

// Status returns a human-readable status string
func (t *Trigger) Status() string {
	if !t.Enabled {
		return "disabled"
	}
	if t.LastError != "" {
		return "error"
	}
	if t.NextRun != nil && time.Now().After(*t.NextRun) {
		return "pending"
	}
	return "active"
}

// ShouldRun returns true if the trigger should run now
func (t *Trigger) ShouldRun(now time.Time) bool {
	if !t.Enabled {
		return false
	}
	if t.NextRun == nil {
		return false
	}
	return now.After(*t.NextRun) || now.Equal(*t.NextRun)
}
