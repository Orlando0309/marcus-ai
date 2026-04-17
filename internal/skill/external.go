package skill

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/safety"
)

// ExternalSkill wraps a SkillDef for runtime execution
type ExternalSkill struct {
	def       SkillDef
	projectRoot string
}

// NewExternalSkill creates a skill from a YAML definition
func NewExternalSkill(def SkillDef, projectRoot string) *ExternalSkill {
	return &ExternalSkill{
		def:       def,
		projectRoot: projectRoot,
	}
}

func (e *ExternalSkill) Name() string { return e.def.Name }

func (e *ExternalSkill) Pattern() string { return e.def.Pattern }

func (e *ExternalSkill) Description() string { return e.def.Description }

func (e *ExternalSkill) Run(ctx context.Context, args []string, deps Dependencies) (Result, error) {
	if e.def.IsShellCommand() {
		return e.runShellCommand(ctx, args)
	}
	if e.def.IsExternalHandler() {
		return e.runExternalHandler(ctx, args)
	}
	return Result{
		Message: fmt.Sprintf("Skill %s has no handler defined", e.def.Name),
		Done:    true,
	}, nil
}

func (e *ExternalSkill) runShellCommand(ctx context.Context, args []string) (Result, error) {
	// Replace placeholders in command
	cmdStr := e.def.Run
	for i, arg := range args {
		placeholder := fmt.Sprintf("${args[%d]}", i)
		// Sanitize arg before substitution
		sanitizedArg := sanitizeShellArg(arg)
		cmdStr = strings.ReplaceAll(cmdStr, placeholder, sanitizedArg)
	}
	// Replace ${args} with space-separated args
	sanitizedArgs := make([]string, len(args))
	for i, arg := range args {
		sanitizedArgs[i] = sanitizeShellArg(arg)
	}
	cmdStr = strings.ReplaceAll(cmdStr, "${args}", strings.Join(sanitizedArgs, " "))

	// Validate command before execution
	validator := safety.DefaultShellValidator()
	if err := validator.ValidateCommand(cmdStr); err != nil {
		return Result{
			Message: fmt.Sprintf("Command blocked by security policy: %v", err),
			Done:    true,
			Error:   "command_blocked",
		}, nil
	}

	// Parse timeout
	timeout := 30 * time.Second
	if e.def.Timeout != "" {
		if d, err := time.ParseDuration(e.def.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = e.projectRoot

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range e.def.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Result{
				Message: fmt.Sprintf("Command timed out after %v", timeout),
				Done:    true,
				Error:   "timeout",
			}, nil
		}
		return Result{
			Message: fmt.Sprintf("Command failed:\n%s", string(output)),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	return Result{
		Message: strings.TrimSpace(string(output)),
		Done:    true,
	}, nil
}

// sanitizeShellArg escapes special shell characters in an argument.
func sanitizeShellArg(arg string) string {
	return safety.SanitizeShellArg(arg)
}

func (e *ExternalSkill) runExternalHandler(ctx context.Context, args []string) (Result, error) {
	// Resolve handler path
	handlerPath := e.def.Handler
	if !filepath.IsAbs(handlerPath) {
		handlerPath = filepath.Join(e.projectRoot, ".marcus", handlerPath)
	}

	// Check if file exists
	if _, err := os.Stat(handlerPath); err != nil {
		return Result{
			Message: fmt.Sprintf("Handler not found: %s", handlerPath),
			Done:    true,
			Error:   "handler not found",
		}, nil
	}

	// Validate handler path for security
	if err := safety.ValidateFilePath(handlerPath, e.projectRoot); err != nil {
		return Result{
			Message: fmt.Sprintf("Handler path blocked: %v", err),
			Done:    true,
			Error:   "handler_path_blocked",
		}, nil
	}

	// Parse timeout
	timeout := 30 * time.Second
	if e.def.Timeout != "" {
		if d, err := time.ParseDuration(e.def.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine how to run based on extension
	ext := strings.ToLower(filepath.Ext(handlerPath))
	var cmd *exec.Cmd

	switch ext {
	case ".go":
		cmd = exec.CommandContext(ctx, "go", "run", handlerPath)
	case ".py":
		cmd = exec.CommandContext(ctx, "python", handlerPath)
	case ".js", ".mjs":
		cmd = exec.CommandContext(ctx, "node", handlerPath)
	case ".sh":
		cmd = exec.CommandContext(ctx, "sh", handlerPath)
	case ".exe", ".bat", ".cmd":
		cmd = exec.CommandContext(ctx, handlerPath, args...)
	default:
		// Try to execute directly with args as interpreter
		cmd = exec.CommandContext(ctx, handlerPath, args...)
	}

	if ext != ".exe" && ext != ".bat" && ext != ".cmd" {
		cmd.Args = append(cmd.Args, args...)
	}

	cmd.Dir = e.projectRoot

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range e.def.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	// Pass args via environment for complex handlers
	cmd.Env = append(cmd.Env, fmt.Sprintf("MARCUS_SKILL_ARGS=%s", strings.Join(args, "\x00")))
	cmd.Env = append(cmd.Env, fmt.Sprintf("MARCUS_SKILL_NAME=%s", e.def.Name))

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Result{
				Message: fmt.Sprintf("Handler timed out after %v", timeout),
				Done:    true,
				Error:   "timeout",
			}, nil
		}
		return Result{
			Message: fmt.Sprintf("Handler failed:\n%s", string(output)),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	return Result{
		Message: strings.TrimSpace(string(output)),
		Done:    true,
	}, nil
}
