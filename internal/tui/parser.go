package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	ctxpkg "github.com/marcus-ai/marcus/internal/context"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

func buildPromptForAgent(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, toolResults []string, agent *folder.AgentDef) string {
	var recent []string
	if sess != nil {
		for _, turn := range sess.RecentTurns(6) {
			recent = append(recent, fmt.Sprintf("%s: %s", turn.Role, turn.Content))
		}
	}
	toolContext := "No prior tool results."
	if len(toolResults) > 0 {
		toolContext = strings.Join(toolResults, "\n\n")
	}
	toolCatalog := toolCatalogForPrompt(runner)

	return strings.TrimSpace(fmt.Sprintf(`## Working Discipline
1. Read the generated project map and repo context first.
2. Identify the ONE most likely file or subsystem.
3. Start with targeted reads/searches on that area.
4. Do not broad-scan the repo unless the map is missing or the first hypothesis fails.
5. For implementation work, prefer the smallest concrete next step over more planning prose.

## CRITICAL - Actions Are Required
For ANY implementation task (build, add, fix, change), you MUST return concrete tool actions.
Describing work in "message" WITHOUT populating "actions" will FAIL.

CORRECT Response Example:
{
  "message": "Creating the main module",
  "actions": [
    {
      "type": "write_file",
      "path": "src/main.py",
      "content": "def main():\n    print('Hello')\n\nif __name__ == '__main__':\n    main()",
      "reason": "Creating entry point"
    },
    {
      "type": "run_command",
      "command": "python src/main.py",
      "reason": "Testing the module"
    }
  ]
}

INCORRECT Response (will be rejected):
{
  "message": "I will create the main module with a main function that prints hello...",
  "actions": []
}

## Output Format
Always return JSON with this shape:
{
  "message": "brief message to the user",
  "actions": [
    {
      "type": "write_file | patch_file | edit_file | run_command | read_file | search_code | find_symbol | list_files | list_directory | glob_files | get_diagnostics",
      "path": "relative/path/for files or scopes",
      "content": "full file content for write_file",
      "command": "shell command for run_command",
      "pattern": "text or regex for search_code/glob_files",
      "symbol": "symbol name for find_symbol",
      "reason": "why this action is needed"
    }
  ],
  "tasks": [
    {
      "id": "optional-slug",
      "title": "Task title",
      "description": "optional detail",
      "status": "active | done | blocked"
    }
  ]
}

If the user asked you to build, implement, add, fix, or change the codebase, you must populate "actions" with the next concrete steps while work remains. If the task is complete, mark the task status as "done" and return "actions": []. Never use no-op shell commands like "echo task done" or "printf done" to signal completion. If you need more context, emit a narrow read_file, search_code, find_symbol, or get_diagnostics action instead of a broad repository scan.

Repo Context:
%s

Recent Conversation:
%s

Prior Tool Results:
%s

Available Tool Catalog:
%s

User Request:
%s`, snapshot.Text, strings.Join(recent, "\n"), toolContext, toolCatalog, input))
}

func buildInitialMessagesForAgent(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, agent *folder.AgentDef) []provider.Message {
	return []provider.Message{
		{Role: "system", Content: buildAgentSystemPrompt(agent)},
		{Role: "user", Content: buildPromptForAgent(runner, snapshot, sess, input, nil, agent)},
	}
}

func buildLoopFollowupMessage(snapshot ctxpkg.Snapshot, goal string, toolResults []string) provider.Message {
	body := "Continue the task using the new state below."
	if strings.TrimSpace(goal) != "" {
		body += "\n\nGoal:\n" + strings.TrimSpace(goal)
	}
	if len(toolResults) > 0 {
		body += "\n\nTool Results:\n" + strings.Join(toolResults, "\n\n")
	}
	if strings.TrimSpace(snapshot.Text) != "" {
		body += "\n\nRefreshed Repo Context:\n" + snapshot.Text
	}
	return provider.Message{Role: "user", Content: strings.TrimSpace(body)}
}

func buildPrompt(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, toolResults []string) string {
	return buildPromptForAgent(runner, snapshot, sess, input, toolResults, nil)
}

func buildAgentSystemPrompt(agent *folder.AgentDef) string {
	base := `You are Marcus, a terminal-native coding agent. Start from the project map and existing repo context, identify the single most relevant file or subsystem, and use the narrowest possible read/search before writing.

CRITICAL RULE - ACTIONS ARE REQUIRED:
When the user wants code, features, or repo changes, EVERY reply must include a non-empty JSON "actions" array with concrete tool proposals (read_file, write_file, run_command, etc.) until the work is done.

DO NOT just describe what you will do in the "message" field - you must actually populate the "actions" array with executable steps.

GOOD EXAMPLE:
{
  "message": "Creating the main_window.py file",
  "actions": [
    {
      "type": "write_file",
      "path": "main_window.py",
      "content": "...",
      "reason": "Creating the main window module"
    }
  ]
}

BAD EXAMPLE (will be rejected):
{
  "message": "I will write the main_window.py file with dark theme...",
  "actions": []
}

Once work is complete, mark the task "done" and return "actions": []. Empty "actions" is only acceptable for informational answers or completed tasks.

CRITICAL COMMAND RULES:
- Avoid complex quoting, escaped quotes, or embedded newlines in run_command.
- To run complex or multi-line shell scripts, ALWAYS use write_file to create a temporary script, then execute that file.
- Never use echo, printf, Write-Host, or similar no-op commands to announce completion.`

	if agent == nil || strings.TrimSpace(agent.Autonomy.SystemPrompt) == "" {
		return base
	}
	return strings.TrimSpace(base + "\n\n" + agent.Autonomy.SystemPrompt)
}

func planOnlyNoActionsNudge(attempt, maxRetries int) string {
	return fmt.Sprintf("System notice: Your last JSON had an empty \"actions\" array (plan-only response %d/%d before stop). If the user wants implementation work, respond again with the same JSON shape and include at least one concrete action (e.g. read_file, write_file, run_command). If you truly need no tools, say so explicitly in \"message\".", attempt, maxRetries)
}

func planOnlyRetryPrompt(goalContent, modelMessage string, attempt, maxRetries int) string {
	return strings.TrimSpace(fmt.Sprintf(`%s

CRITICAL ERROR - PREVIOUS RESPONSE REJECTED (retry %d/%d):
Your last response described what you would do but had empty "actions": [] in the JSON.

YOU MUST FIX THIS NOW:
1. Do NOT describe your plan in the "message" field
2. DO populate "actions" with the actual tool calls to execute
3. Each action must have: type, path (for file ops), command (for run_command), and reason

EXAMPLE OF CORRECT RESPONSE:
{
  "message": "Creating the file",
  "actions": [
    {
      "type": "write_file",
      "path": "src/main.py",
      "content": "print('hello')",
      "reason": "Creating main module"
    }
  ]
}

Previous incorrect response (had empty actions):
%s

NOW: Return the JSON with populated "actions" array.`, goalContent, attempt, maxRetries, trimText(modelMessage, 800)))
}

func stagnationNoProgressNudge(attempt, maxRetries int) string {
	return fmt.Sprintf("System notice: Your last action plan repeated a previous plan without making visible progress (%d/%d before stop). Do not emit the same action sequence again. Choose a more specific read/search, perform the next write/command, or mark the task blocked with a concrete reason.", attempt, maxRetries)
}

func stagnationRetryPrompt(goalContent, modelMessage string, attempt, maxRetries int) string {
	return strings.TrimSpace(fmt.Sprintf("%s\n\nIMPORTANT: Your previous response repeated the same action plan and was rejected. This is retry %d/%d. Do not repeat the same actions. Pick a different concrete next step now: either a narrower file read/search, the next code edit, the verification command, or a blocked task update with a specific reason. Previous repeated response:\n%s", goalContent, attempt, maxRetries, trimText(modelMessage, 1200)))
}

func currentTaskStatus(updates []task.Update) string {
	for _, u := range updates {
		switch strings.TrimSpace(u.Status) {
		case task.StatusDone, task.StatusActive, task.StatusBlocked:
			return strings.TrimSpace(u.Status)
		}
	}
	return ""
}

func filterCompletionNoopActions(actions []tool.ActionProposal, taskStatus string) []tool.ActionProposal {
	if taskStatus != task.StatusDone && taskStatus != task.StatusBlocked {
		return actions
	}
	filtered := make([]tool.ActionProposal, 0, len(actions))
	for _, action := range actions {
		if !isCompletionNoopAction(action) {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func isCompletionNoopAction(action tool.ActionProposal) bool {
	if action.Type != "run_command" {
		return false
	}
	cmd := strings.ToLower(strings.TrimSpace(action.Command))
	if cmd == "" {
		return false
	}
	phrases := []string{"task done", "done", "completed", "complete", "success"}
	prefixes := []string{"echo ", "printf ", "write-host ", "write-output "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(cmd, prefix) {
			for _, phrase := range phrases {
				if strings.Contains(cmd, phrase) {
					return true
				}
			}
		}
	}
	return false
}

func parseAssistantEnvelope(raw string) assistantEnvelope {
	text := stripMarkdownCodeFence(raw)
	if text == "" {
		return assistantEnvelope{}
	}
	var env assistantEnvelope
	if err := json.Unmarshal([]byte(text), &env); err == nil {
		// If JSON parsed but actions is empty, try to extract from message text
		if len(env.Actions) == 0 && strings.TrimSpace(env.Message) != "" {
			extracted := extractActionsFromMessage(env.Message)
			if len(extracted) > 0 {
				env.Actions = deduplicateActions(extracted)
			}
		}
		return env
	}
	if start := strings.Index(text, "{"); start != -1 {
		if end := strings.LastIndex(text, "}"); end > start {
			if err := json.Unmarshal([]byte(text[start:end+1]), &env); err == nil {
				// If JSON parsed but actions is empty, try to extract from message text
				if len(env.Actions) == 0 && strings.TrimSpace(env.Message) != "" {
					extracted := extractActionsFromMessage(env.Message)
					if len(extracted) > 0 {
						env.Actions = deduplicateActions(extracted)
					}
				}
				return env
			}
		}
	}
	env = recoverAssistantEnvelope(text)
	// Try extraction from recovered message too
	if len(env.Actions) == 0 && strings.TrimSpace(env.Message) != "" {
		extracted := extractActionsFromMessage(env.Message)
		if len(extracted) > 0 {
			env.Actions = deduplicateActions(extracted)
		}
	}
	if strings.TrimSpace(env.Message) != "" || len(env.Actions) > 0 || len(env.Tasks) > 0 {
		return env
	}
	return assistantEnvelope{Message: text}
}

func recoverAssistantEnvelope(text string) assistantEnvelope {
	env := assistantEnvelope{
		Message: extractJSONStringField(text, "message"),
		Actions: extractJSONArrayObjects[tool.ActionProposal](text, "actions"),
		Tasks:   extractJSONArrayObjects[task.Update](text, "tasks"),
	}
	if strings.TrimSpace(env.Message) == "" && (len(env.Actions) > 0 || len(env.Tasks) > 0) {
		env.Message = "Recovered structured actions from a partial provider response."
	}
	return env
}

func visibleAssistantMessage(env assistantEnvelope, raw string) string {
	if message := strings.TrimSpace(env.Message); message != "" {
		return message
	}
	if len(env.Actions) > 0 || len(env.Tasks) > 0 {
		return summarizeStructuredResponse(env)
	}
	text := strings.TrimSpace(stripMarkdownCodeFence(raw))
	if text == "" || looksLikeStructuredPayload(text) {
		return "Prepared a structured response."
	}
	return trimText(text, 800)
}

func summarizeStructuredResponse(env assistantEnvelope) string {
	var parts []string
	if len(env.Actions) > 0 {
		parts = append(parts, fmt.Sprintf("Prepared %d action(s)", len(env.Actions)))
	}
	if len(env.Tasks) > 0 {
		parts = append(parts, fmt.Sprintf("%d task update(s)", len(env.Tasks)))
	}
	if len(parts) == 0 {
		return "Prepared a structured response."
	}
	return strings.Join(parts, " and ") + "."
}

func stripMarkdownCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	lines = lines[1:]
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func looksLikeStructuredPayload(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.Contains(trimmed, `"actions"`) ||
		strings.Contains(trimmed, `"tasks"`) ||
		strings.Contains(trimmed, `"message"`)
}

func extractJSONStringField(text, field string) string {
	key := `"` + field + `"`
	start := strings.Index(text, key)
	if start == -1 {
		return ""
	}
	colon := strings.Index(text[start+len(key):], ":")
	if colon == -1 {
		return ""
	}
	rest := strings.TrimSpace(text[start+len(key)+colon+1:])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	var out string
	if err := json.Unmarshal([]byte(readJSONObjectPrefix(rest)), &out); err != nil {
		return ""
	}
	return out
}

func extractJSONArrayObjects[T any](text, field string) []T {
	key := `"` + field + `"`
	start := strings.Index(text, key)
	if start == -1 {
		return nil
	}
	open := strings.Index(text[start:], "[")
	if open == -1 {
		return nil
	}
	arrayText := text[start+open:]
	var results []T
	for _, objectText := range splitJSONObjectCandidates(arrayText) {
		var item T
		if err := json.Unmarshal([]byte(objectText), &item); err == nil {
			results = append(results, item)
		}
	}
	return results
}

func splitJSONObjectCandidates(input string) []string {
	var objects []string
	inString := false
	escaped := false
	depth := 0
	start := -1
	for i, r := range input {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case !inString && r == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case !inString && r == '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objects = append(objects, input[start:i+1])
					start = -1
				}
			}
		}
	}
	return objects
}

func readJSONObjectPrefix(input string) string {
	inString := false
	escaped := false
	for i, r := range input {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			if inString {
				return input[:i+1]
			}
			inString = true
		}
	}
	return input
}

func trimText(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[:limit]) + "\n...[truncated]"
}

func thinkingCardBody(charCount, toolCallsSeen int) string {
	if toolCallsSeen > 0 {
		return fmt.Sprintf("Streaming a structured response.\nCaptured %d chars and %d provider tool call(s).", charCount, toolCallsSeen)
	}
	return fmt.Sprintf("Streaming a structured response.\nCaptured %d chars so far.", charCount)
}

func actionPlanSignature(actions []tool.ActionProposal) string {
	if len(actions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, fmt.Sprintf("%s|%s|%s|%s|%s", action.Type, action.Path, action.Command, action.Pattern, action.Symbol))
	}
	return strings.Join(parts, ";")
}

func providerToolSpecs(runner *tool.ToolRunner) []provider.ToolSpec {
	if runner == nil {
		return nil
	}
	defs := runner.Definitions()
	specs := make([]provider.ToolSpec, 0, len(defs))
	for _, def := range defs {
		raw, _ := json.Marshal(def.Schema)
		specs = append(specs, provider.ToolSpec{
			Name:        def.Name,
			Description: def.Description,
			Schema:      raw,
		})
	}
	return specs
}

func toolCatalogForPrompt(runner *tool.ToolRunner) string {
	if runner == nil {
		return "No tools available."
	}
	defs := runner.Definitions()
	if len(defs) == 0 {
		return "No tools available."
	}
	var lines []string
	for _, def := range defs {
		safety := "approval-required"
		if def.Safe {
			safety = "safe"
		}
		raw, _ := json.Marshal(def.Schema)
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s\n  schema: %s", def.Name, safety, def.Description, string(raw)))
	}
	return strings.Join(lines, "\n")
}

// extractActionsFromMessage attempts to extract concrete actions from message text
// when the model describes actions but doesn't put them in the JSON actions array
func extractActionsFromMessage(message string) []tool.ActionProposal {
	var actions []tool.ActionProposal
	message = strings.ToLower(message)

	// Pattern: "writing the file X" or "creating file X" or "updating X"
	writePatterns := []string{
		`writing\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
		`creating\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
		`updating\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
		`will\s+(?:write|create|update)\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
	}
	for _, pattern := range writePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(message, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" {
				path := match[1]
				// Ensure path has an extension
				if strings.Contains(path, ".") {
					actions = append(actions, tool.ActionProposal{
						Type:   "write_file",
						Path:   path,
						Reason: fmt.Sprintf("Extracted from message: writing %s", path),
					})
				}
			}
		}
	}

	// Pattern: "reading file X" or "checking X"
	readPatterns := []string{
		`reading\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
		`checking\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
		`looking\s+at\s+(?:the\s+)?(?:file\s+)?([\w\./]+)`,
	}
	for _, pattern := range readPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(message, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" {
				path := match[1]
				if strings.Contains(path, ".") {
					actions = append(actions, tool.ActionProposal{
						Type:   "read_file",
						Path:   path,
						Reason: fmt.Sprintf("Extracted from message: reading %s", path),
					})
				}
			}
		}
	}

	// Pattern: "running X" or "executing X"
	runPatterns := []string{
		`running\s+['"]?([^'"\n]+)['"]?`,
		`executing\s+['"]?([^'"\n]+)['"]?`,
		`will\s+run\s+['"]?([^'"\n]+)['"]?`,
	}
	for _, pattern := range runPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(message, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" {
				cmd := strings.TrimSpace(match[1])
				// Filter out vague commands
				if len(cmd) > 3 && !strings.Contains(cmd, " ") {
					// Likely a command name, add more context
					cmd = cmd + " ."
				}
				if len(cmd) > 5 {
					actions = append(actions, tool.ActionProposal{
						Type:    "run_command",
						Command: cmd,
						Reason:  fmt.Sprintf("Extracted from message: running command"),
					})
				}
			}
		}
	}

	return actions
}

// deduplicateActions removes duplicate actions from a slice
func deduplicateActions(actions []tool.ActionProposal) []tool.ActionProposal {
	seen := make(map[string]bool)
	result := make([]tool.ActionProposal, 0, len(actions))
	for _, a := range actions {
		key := fmt.Sprintf("%s:%s:%s", a.Type, a.Path, a.Command)
		if !seen[key] {
			seen[key] = true
			result = append(result, a)
		}
	}
	return result
}
