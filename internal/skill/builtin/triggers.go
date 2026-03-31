package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/scheduler"
	"github.com/marcus-ai/marcus/internal/skill"
)

// TriggersSkill provides an alternative interface for trigger management
type TriggersSkill struct {
	Scheduler *scheduler.Scheduler
}

func (t *TriggersSkill) Name() string { return "triggers" }

func (t *TriggersSkill) Pattern() string { return "/triggers" }

func (t *TriggersSkill) Description() string {
	return "List and manage triggers (alias for /schedule)"
}

func (t *TriggersSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if t.Scheduler == nil {
		return skill.Result{
			Message: "Scheduler not available.",
			Done:    true,
		}, nil
	}

	if len(args) == 0 {
		return t.listTriggers(), nil
	}

	subcommand := strings.ToLower(args[0])

	switch subcommand {
	case "list":
		return t.listTriggers(), nil
	case "status":
		return t.triggerStatus(args[1:]), nil
	case "run":
		return t.runTrigger(ctx, args[1:]), nil
	case "stats":
		return t.showStats(), nil
	case "help":
		return t.showUsage(), nil
	default:
		return skill.Result{
			Message: fmt.Sprintf("Unknown subcommand: %s\n\n%s", subcommand, t.usageText()),
			Done:    true,
		}, nil
	}
}

func (t *TriggersSkill) showUsage() skill.Result {
	return skill.Result{
		Message: t.usageText(),
		Done:    true,
	}
}

func (t *TriggersSkill) usageText() string {
	return `Triggers Management

Usage: /triggers <subcommand> [args]

Subcommands:
  list                    List all triggers (default)
  status <id>             Show detailed status of a trigger
  run <id>                Manually run a trigger
  stats                   Show scheduler statistics

For creating triggers, use /schedule create`
}

func (t *TriggersSkill) listTriggers() skill.Result {
	triggers := t.Scheduler.List()

	if len(triggers) == 0 {
		return skill.Result{
			Message: "No triggers configured.\n\nUse '/schedule create' to add one.",
			Done:    true,
		}
	}

	var lines []string
	lines = append(lines, "Triggers:")
	lines = append(lines, "")

	for _, trigger := range triggers {
		statusIcon := "●"
		if trigger.Status() == "disabled" {
			statusIcon = "○"
		} else if trigger.Status() == "error" {
			statusIcon = "✗"
		}

		nextRun := "-"
		if trigger.NextRun != nil {
			nextRun = trigger.NextRun.Format("15:04")
		}

		lines = append(lines, fmt.Sprintf("%s %s - %s:%s (next: %s)",
			statusIcon,
			trigger.ID[:min(20, len(trigger.ID))],
			trigger.Action.Type,
			trigger.Action.Target,
			nextRun,
		))
	}

	lines = append(lines, "")
	stats := t.Scheduler.Stats()
	lines = append(lines, fmt.Sprintf("Total: %d triggers | Running: %d | Max concurrent: %d",
		stats["triggers"],
		stats["running"],
		stats["max_concurrent"],
	))

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}

func (t *TriggersSkill) triggerStatus(args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /triggers status <id>",
			Done:    true,
		}
	}

	id := args[0]
	trigger, ok := t.Scheduler.Get(id)
	if !ok {
		return skill.Result{
			Message: fmt.Sprintf("Trigger not found: %s", id),
			Done:    true,
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Trigger: %s", trigger.ID))
	lines = append(lines, fmt.Sprintf("Name: %s", trigger.Name))
	lines = append(lines, fmt.Sprintf("Type: %s", trigger.Type))
	lines = append(lines, fmt.Sprintf("Status: %s", trigger.Status()))
	lines = append(lines, fmt.Sprintf("Enabled: %v", trigger.Enabled))

	if trigger.Type == scheduler.TriggerCron {
		lines = append(lines, fmt.Sprintf("Cron Expression: %s", trigger.Config.CronExpression))
	}

	if trigger.NextRun != nil {
		lines = append(lines, fmt.Sprintf("Next Run: %s", trigger.NextRun.Format("2006-01-02 15:04:05")))
	}

	if !trigger.LastRun.IsZero() {
		lines = append(lines, fmt.Sprintf("Last Run: %s", trigger.LastRun.Format("2006-01-02 15:04:05")))
	}

	lines = append(lines, fmt.Sprintf("Run Count: %d", trigger.RunCount))
	lines = append(lines, fmt.Sprintf("Error Count: %d", trigger.ErrorCount))

	if trigger.LastError != "" {
		lines = append(lines, fmt.Sprintf("Last Error: %s", trigger.LastError))
	}

	lines = append(lines, fmt.Sprintf("Action: %s:%s", trigger.Action.Type, trigger.Action.Target))

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}

func (t *TriggersSkill) runTrigger(ctx context.Context, args []string) skill.Result {
	if len(args) == 0 {
		return skill.Result{
			Message: "Usage: /triggers run <id>",
			Done:    true,
		}
	}

	id := args[0]
	trigger, ok := t.Scheduler.Get(id)
	if !ok {
		return skill.Result{
			Message: fmt.Sprintf("Trigger not found: %s", id),
			Done:    true,
		}
	}

	if err := t.Scheduler.TriggerNow(ctx, id); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to run trigger: %v", err),
			Done:    true,
		}
	}

	return skill.Result{
		Message: fmt.Sprintf("Manually running trigger %s (%s:%s)",
			trigger.ID,
			trigger.Action.Type,
			trigger.Action.Target,
		),
		Done:    true,
	}
}

func (t *TriggersSkill) showStats() skill.Result {
	stats := t.Scheduler.Stats()

	var lines []string
	lines = append(lines, "Scheduler Statistics:")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Total Triggers: %d", stats["triggers"]))
	lines = append(lines, fmt.Sprintf("Currently Running: %d", stats["running"]))
	lines = append(lines, fmt.Sprintf("Max Concurrent: %d", stats["max_concurrent"]))

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}
