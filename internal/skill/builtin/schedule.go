package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/scheduler"
	"github.com/marcus-ai/marcus/internal/skill"
)

// ScheduleSkill manages scheduled triggers
type ScheduleSkill struct {
	Scheduler *scheduler.Scheduler
}

func (s *ScheduleSkill) Name() string { return "schedule" }

func (s *ScheduleSkill) Pattern() string { return "/schedule" }

func (s *ScheduleSkill) Description() string {
	return "Create and manage scheduled triggers"
}

func (s *ScheduleSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if s.Scheduler == nil {
		return skill.Result{
			Message: "Scheduler not available.",
			Done:    true,
		}, nil
	}

	if len(args) == 0 {
		return s.showUsage(), nil
	}

	subcommand := strings.ToLower(args[0])

	switch subcommand {
	case "create":
		return s.createTrigger(args[1:], deps), nil
	case "list":
		return s.listTriggers(), nil
	case "enable":
		return s.enableTrigger(args[1:]), nil
	case "disable":
		return s.disableTrigger(args[1:]), nil
	case "delete":
		return s.deleteTrigger(args[1:]), nil
	case "run":
		return s.runTrigger(ctx, args[1:]), nil
	case "help":
		return s.showUsage(), nil
	default:
		return skill.Result{
			Message: fmt.Sprintf("Unknown subcommand: %s\n\n%s", subcommand, s.usageText()),
			Done:    true,
		}, nil
	}
}

func (s *ScheduleSkill) showUsage() skill.Result {
	return skill.Result{
		Message: s.usageText(),
		Done:    true,
	}
}

func (s *ScheduleSkill) usageText() string {
	return `Schedule Management

Usage: /schedule <subcommand> [args]

Subcommands:
  create --cron "expr" --action type:target    Create a new scheduled trigger
  list                                         List all triggers
  enable <id>                                  Enable a trigger
  disable <id>                                 Disable a trigger
  delete <id>                                  Delete a trigger
  run <id>                                     Manually run a trigger

Examples:
  /schedule create --cron "0 9 * * *" --action flow:daily_report
  /schedule create --cron "*/5 * * * *" --action skill:/status
  /schedule list
  /schedule disable trig_1234567890_1`
}

func (s *ScheduleSkill) createTrigger(args []string, deps skill.Dependencies) skill.Result {
	var cronExpr, actionType, actionTarget string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cron":
			if i+1 < len(args) {
				cronExpr = args[i+1]
				i++
			}
		case "--action":
			if i+1 < len(args) {
				action := args[i+1]
				parts := strings.SplitN(action, ":", 2)
				if len(parts) == 2 {
					actionType = parts[0]
					actionTarget = parts[1]
				}
				i++
			}
		}
	}

	if cronExpr == "" {
		return skill.Result{
			Message: "Error: --cron is required\n\n" + s.usageText(),
			Done:    true,
		}
	}

	if actionType == "" || actionTarget == "" {
		return skill.Result{
			Message: "Error: --action is required (format: type:target)\n\n" + s.usageText(),
			Done:    true,
		}
	}

	// Validate cron expression
	schedule, err := scheduler.ParseCron(cronExpr)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Invalid cron expression: %v", err),
			Done:    true,
		}
	}

	// Create trigger
	trigger := &scheduler.Trigger{
		Name:    fmt.Sprintf("Scheduled %s:%s", actionType, actionTarget),
		Type:    scheduler.TriggerCron,
		Enabled: true,
		Config: scheduler.TriggerConfig{
			CronExpression: cronExpr,
		},
		Action: scheduler.ActionConfig{
			Type:   actionType,
			Target: actionTarget,
		},
		CreatedAt: time.Now(),
	}

	next := schedule.Next(time.Now())
	trigger.NextRun = &next

	if err := s.Scheduler.Register(trigger); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Created trigger %s\nCron: %s\nNext run: %s", trigger.ID, cronExpr, next.Format(time.RFC3339)),
		Done:    true,
	}
}

func (s *ScheduleSkill) listTriggers() skill.Result {
	triggers := s.Scheduler.List()

	if len(triggers) == 0 {
		return skill.Result{
			Message: "No triggers configured.\n\nUse '/schedule create' to add one.",
			Done:    true,
		}
	}

	var lines []string
	lines = append(lines, "Scheduled Triggers:")
	lines = append(lines, "")

	for _, t := range triggers {
		status := t.Status()
		statusIcon := "●"
		if status == "disabled" {
			statusIcon = "○"
		} else if status == "error" {
			statusIcon = "✗"
		}

		lines = append(lines, fmt.Sprintf("%s %s", statusIcon, t.ID))
		lines = append(lines, fmt.Sprintf("   Name: %s", t.Name))
		lines = append(lines, fmt.Sprintf("   Type: %s", t.Type))
		lines = append(lines, fmt.Sprintf("   Status: %s", status))

		if t.Type == scheduler.TriggerCron && t.Config.CronExpression != "" {
			lines = append(lines, fmt.Sprintf("   Cron: %s", t.Config.CronExpression))
		}

		if t.NextRun != nil {
			lines = append(lines, fmt.Sprintf("   Next run: %s", t.NextRun.Format("2006-01-02 15:04")))
		}

		if t.LastRun.IsZero() {
			lines = append(lines, "   Last run: never")
		} else {
			lines = append(lines, fmt.Sprintf("   Last run: %s", t.LastRun.Format("2006-01-02 15:04")))
		}

		lines = append(lines, fmt.Sprintf("   Action: %s:%s", t.Action.Type, t.Action.Target))
		lines = append(lines, "")
	}

	stats := s.Scheduler.Stats()
	lines = append(lines, fmt.Sprintf("Total: %d triggers", stats["triggers"]))

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}

func (s *ScheduleSkill) enableTrigger(args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /schedule enable <id>",
			Done:    true,
		}
	}

	id := args[0]
	if err := s.Scheduler.Enable(id); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to enable trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Enabled trigger %s", id),
		Done:    true,
	}
}

func (s *ScheduleSkill) disableTrigger(args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /schedule disable <id>",
			Done:    true,
		}
	}

	id := args[0]
	if err := s.Scheduler.Disable(id); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to disable trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Disabled trigger %s", id),
		Done:    true,
	}
}

func (s *ScheduleSkill) deleteTrigger(args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /schedule delete <id>",
			Done:    true,
		}
	}

	id := args[0]
	if err := s.Scheduler.Unregister(id); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to delete trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Deleted trigger %s", id),
		Done:    true,
	}
}

func (s *ScheduleSkill) runTrigger(ctx context.Context, args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /schedule run <id>",
			Done:    true,
		}
	}

	id := args[0]
	if err := s.Scheduler.TriggerNow(ctx, id); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to run trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Running trigger %s", id),
		Done:    true,
	}
}
