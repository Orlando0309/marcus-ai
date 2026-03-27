package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
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

func (tr *ToolRunner) runHookCommand(ctx context.Context, command, name string, input, output json.RawMessage, runErr error, source string) error {
	var cmd *exec.Cmd
	if filepath.Separator == '\\' {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if tr.baseDir != "" {
		cmd.Dir = tr.baseDir
	}
	cmd.Env = append(os.Environ(),
		"MARCUS_HOOK_SOURCE="+source,
		"MARCUS_TOOL_NAME="+name,
		"MARCUS_TOOL_INPUT="+string(input),
		"MARCUS_TOOL_OUTPUT="+string(output),
		"MARCUS_PROJECT_ROOT="+tr.baseDir,
	)
	if runErr != nil {
		cmd.Env = append(cmd.Env, "MARCUS_TOOL_ERROR="+runErr.Error())
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hook %q for %s failed: %s", command, name, strings.TrimSpace(string(out)))
	}
	return nil
}
