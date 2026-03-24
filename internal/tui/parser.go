package tui

import (
	"encoding/json"
	"fmt"
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

	return strings.TrimSpace(fmt.Sprintf("## Working discipline\n1. Read the generated project map and repo context first.\n2. Identify the ONE most likely file or subsystem.\n3. Start with targeted reads/searches on that area.\n4. Do not broad-scan the repo unless the map is missing or the first hypothesis fails.\n5. For implementation work, prefer the smallest concrete next step over more planning prose.\n6. If you learn a durable repo fact that future tasks should know, update `.marcus/context/PROJECT_MAP.md` near the end of the task.\n\n## Output format\nAlways return JSON with this shape:\n{\n  \"message\": \"brief message to the user\",\n  \"actions\": [\n    {\n      \"type\": \"write_file | patch_file | edit_file | run_command | read_file | search_code | find_symbol | list_files | list_directory | glob_files | get_diagnostics\",\n      \"path\": \"relative/path/for files or scopes\",\n      \"content\": \"full file content for write_file\",\n      \"command\": \"shell command for run_command\",\n      \"pattern\": \"text or regex for search_code/glob_files\",\n      \"symbol\": \"symbol name for find_symbol\",\n      \"reason\": \"why this action is needed\"\n    }\n  ],\n  \"tasks\": [\n    {\n      \"id\": \"optional-slug\",\n      \"title\": \"Task title\",\n      \"description\": \"optional detail\",\n      \"status\": \"active | done | blocked\"\n    }\n  ]\n}\n\nIf the user asked you to build, implement, add, fix, or change the codebase, you must populate \"actions\" with the next concrete steps. Do not end a turn with only a plan in \"message\" and \"actions\": []. If you need more context, emit a narrow `read_file`, `search_code`, `find_symbol`, or `get_diagnostics` action instead of a broad repository scan.\n\nRepo Context:\n%s\n\nRecent Conversation:\n%s\n\nPrior Tool Results:\n%s\n\nAvailable Tool Catalog:\n%s\n\nUser Request:\n%s", snapshot.Text, strings.Join(recent, "\n"), toolContext, toolCatalog, input))
}

func buildPrompt(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, toolResults []string) string {
	return buildPromptForAgent(runner, snapshot, sess, input, toolResults, nil)
}

func buildAgentSystemPrompt(agent *folder.AgentDef) string {
	base := "You are Marcus, a terminal-native coding agent. Start from the project map and existing repo context, identify the single most relevant file or subsystem, and use the narrowest possible read/search before writing. When the user wants code, features, or repo changes, every reply must include a non-empty JSON \"actions\" array with concrete tool proposals (read_file, write_file, run_command, etc.) until the work is done. Describing a plan only in \"message\" without actions is not sufficient for implementation tasks. Empty \"actions\" is acceptable only for purely informational answers where no tools are needed.\n\nCRITICAL COMMAND RULES:\n- Avoid complex quoting, escaped quotes, or embedded newlines in `run_command`.\n- To run complex or multi-line shell scripts, ALWAYS use `write_file` to create a temporary script (e.g. `.py`, `.sh`, or `.bat`) and then execute that file with `run_command`."
	if agent == nil || strings.TrimSpace(agent.Autonomy.SystemPrompt) == "" {
		return base
	}
	return strings.TrimSpace(base + "\n\n" + agent.Autonomy.SystemPrompt)
}

func planOnlyNoActionsNudge(attempt, maxRetries int) string {
	return fmt.Sprintf("System notice: Your last JSON had an empty \"actions\" array (plan-only response %d/%d before stop). If the user wants implementation work, respond again with the same JSON shape and include at least one concrete action (e.g. read_file, write_file, run_command). If you truly need no tools, say so explicitly in \"message\".", attempt, maxRetries)
}

func planOnlyRetryPrompt(goalContent, modelMessage string, attempt, maxRetries int) string {
	return strings.TrimSpace(fmt.Sprintf("%s\n\nIMPORTANT: Your previous response was rejected because it contained no concrete tool actions. This is retry %d/%d. Do not restate the plan. Return the same JSON schema, but populate \"actions\" with the next executable step now. If you need more context, emit a read/search/list action. Previous plan-only response:\n%s", goalContent, attempt, maxRetries, trimText(modelMessage, 1200)))
}

func stagnationNoProgressNudge(attempt, maxRetries int) string {
	return fmt.Sprintf("System notice: Your last action plan repeated a previous plan without making visible progress (%d/%d before stop). Do not emit the same action sequence again. Choose a more specific read/search, perform the next write/command, or mark the task blocked with a concrete reason.", attempt, maxRetries)
}

func stagnationRetryPrompt(goalContent, modelMessage string, attempt, maxRetries int) string {
	return strings.TrimSpace(fmt.Sprintf("%s\n\nIMPORTANT: Your previous response repeated the same action plan and was rejected. This is retry %d/%d. Do not repeat the same actions. Pick a different concrete next step now: either a narrower file read/search, the next code edit, the verification command, or a blocked task update with a specific reason. Previous repeated response:\n%s", goalContent, attempt, maxRetries, trimText(modelMessage, 1200)))
}

func parseAssistantEnvelope(raw string) assistantEnvelope {
	text := stripMarkdownCodeFence(raw)
	if text == "" {
		return assistantEnvelope{}
	}
	var env assistantEnvelope
	if err := json.Unmarshal([]byte(text), &env); err == nil {
		return env
	}
	if start := strings.Index(text, "{"); start != -1 {
		if end := strings.LastIndex(text, "}"); end > start {
			if err := json.Unmarshal([]byte(text[start:end+1]), &env); err == nil {
				return env
			}
		}
	}
	env = recoverAssistantEnvelope(text)
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
