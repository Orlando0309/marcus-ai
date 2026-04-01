package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/safety"
)

func (tr *ToolRunner) runHooks(ctx context.Context, rules []config.HookRule, name string, input, output json.RawMessage, runErr error, source string) error {
	for _, rule := range rules {
		if !hookMatches(rule.Matcher, name) {
			continue
		}
		for _, command := range rule.Commands {
			command = strings.TrimSpace(command)
			if command == "" {
				continue
			}
			if err := tr.runHookCommand(ctx, command, name, input, output, runErr, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func hookMatches(matcher, name string) bool {
	matcher = strings.TrimSpace(strings.ToLower(matcher))
	if matcher == "" || matcher == "*" {
		return true
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, part := range strings.Split(matcher, "|") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if part == name {
			return true
		}
	}
	return false
}

// redactSensitiveData removes potentially sensitive patterns from strings.
func redactSensitiveData(s string) string {
	// Redact common sensitive patterns
	redacted := s
	// API keys (common patterns)
	apiKeyPatterns := []string{
		"sk-ant-[a-zA-Z0-9-]+",
		"sk-[a-zA-Z0-9]{20,}",
		"gsk_[a-zA-Z0-9]+",
		"ghp_[a-zA-Z0-9]+",
		"glpat-[a-zA-Z0-9-]+",
	}
	for _, pattern := range apiKeyPatterns {
		re := regexp.MustCompile(pattern)
		redacted = re.ReplaceAllString(redacted, "[REDACTED_API_KEY]")
	}
	// Passwords in URLs (user:pass@host)
	urlPassRe := regexp.MustCompile(`://[^:]+:[^@]+@`)
	redacted = urlPassRe.ReplaceAllString(redacted, "://[REDACTED]:[REDACTED]@")
	// Bearer tokens
	bearerRe := regexp.MustCompile(`Bearer [a-zA-Z0-9._-]+`)
	redacted = bearerRe.ReplaceAllString(redacted, "Bearer [REDACTED_TOKEN]")
	return redacted
}

func (tr *ToolRunner) runHookCommand(ctx context.Context, command, name string, input, output json.RawMessage, runErr error, source string) error {
	// Validate command before execution
	validator := safety.DefaultShellValidator()
	validator.StrictMode = true // Hooks require strict allowlisting
	if err := validator.ValidateCommand(command); err != nil {
		return fmt.Errorf("hook command blocked: %w", err)
	}

	var cmd *exec.Cmd
	if filepath.Separator == '\\' {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if tr.baseDir != "" {
		cmd.Dir = tr.baseDir
	}

	// Redact sensitive data from tool input/output before passing to hooks
	safeInput := redactSensitiveData(string(input))
	safeOutput := redactSensitiveData(string(output))
	safeSource := redactSensitiveData(source)
	safeError := ""
	if runErr != nil {
		safeError = redactSensitiveData(runErr.Error())
	}

	cmd.Env = append(os.Environ(),
		"MARCUS_HOOK_SOURCE="+safeSource,
		"MARCUS_TOOL_NAME="+name,
		"MARCUS_TOOL_INPUT="+safeInput,
		"MARCUS_TOOL_OUTPUT="+safeOutput,
		"MARCUS_PROJECT_ROOT="+tr.baseDir,
	)
	if runErr != nil {
		cmd.Env = append(cmd.Env, "MARCUS_TOOL_ERROR="+safeError)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hook %q for %s failed: %s", command, name, strings.TrimSpace(string(out)))
	}
	return nil
}
