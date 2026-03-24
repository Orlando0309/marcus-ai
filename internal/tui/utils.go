package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ctxpkg "github.com/marcus-ai/marcus/internal/context"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

func (m *Model) reconcileTODOTasks(paths []string) []task.Task {
	if len(paths) == 0 {
		return nil
	}
	hints := m.contextAssembler.ScanTODOs(paths)
	updated, err := m.taskStore.ReconcileTODOHints(paths, hints)
	if err != nil {
		m.addItem("error", "Task Reconcile Failed", err.Error(), "")
		return nil
	}
	return updated
}

func resultsToPaths(results []tool.ActionResult) []string {
	var paths []string
	for _, result := range results {
		if result.Proposal.Type == "write_file" && strings.TrimSpace(result.Proposal.Path) != "" {
			paths = append(paths, result.Proposal.Path)
		}
	}
	return paths
}

func (m *Model) recoveryPrompt(results []tool.ActionResult) string {
	var failures []string
	for _, result := range results {
		if result.Proposal.Type == "run_command" && !result.Success {
			failures = append(failures, fmt.Sprintf("Verification failed for %s:\n%s", result.Proposal.Command, trimText(result.Output, 1200)))
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return "The last verification step failed after applying changes.\nDiagnose the failure, inspect the relevant files, propose the smallest corrective edit, and include a follow-up verification command.\n\n" + strings.Join(failures, "\n\n")
}

func (m *Model) contextMeta(snapshot ctxpkg.Snapshot) string {
	var parts []string
	if snapshot.Branch != "" {
		parts = append(parts, "git:"+snapshot.Branch)
	}
	if len(snapshot.FileHints) > 0 {
		parts = append(parts, "@"+strings.Join(snapshot.FileHints, ", @"))
	}
	if snapshot.EstimatedTokens > 0 {
		parts = append(parts, fmt.Sprintf("~%dtok", snapshot.EstimatedTokens))
	}
	if snapshot.Truncated {
		parts = append(parts, "truncated")
	}
	return strings.Join(parts, "  ")
}

func (m *Model) persistSession() {
	if m.sessionStore == nil || !m.cfg.Session.AutoSave {
		return
	}
	m.session.LastContext = trimText(m.latestContext.Text, 2000)
	_ = m.sessionStore.Save(m.session)
}

// detectVerifyCommand looks at the project files to determine a suitable
// build/test verification command (go build, cargo build, npm build, etc.).
func (m *Model) detectVerifyCommand() string {
	baseDir := m.cfg.ProjectRoot
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	projType := detectProjectType(baseDir)
	switch projType {
	case "go":
		if fileExists(filepath.Join(baseDir, "go.mod")) {
			return "go build ./..."
		}
		if fileExists(filepath.Join(baseDir, "main.go")) {
			return "go build ."
		}
	case "rust":
		if fileExists(filepath.Join(baseDir, "Cargo.toml")) {
			return "cargo build 2>&1"
		}
	case "node":
		if fileExists(filepath.Join(baseDir, "package.json")) {
			if fileExists(filepath.Join(baseDir, "Makefile")) {
				return "make build 2>&1 || npm run build 2>&1"
			}
			return "npm run build 2>&1"
		}
	case "python":
		if fileExists(filepath.Join(baseDir, "pyproject.toml")) || fileExists(filepath.Join(baseDir, "requirements.txt")) {
			return "python -m compileall -q . 2>&1 || ruff check . 2>&1"
		}
	case "java":
		if fileExists(filepath.Join(baseDir, "pom.xml")) {
			return "mvn compile 2>&1"
		}
		if fileExists(filepath.Join(baseDir, "build.gradle")) {
			return "gradle build 2>&1"
		}
	}

	if fileExists(filepath.Join(baseDir, "Makefile")) {
		return "make 2>&1"
	}
	if fileExists(filepath.Join(baseDir, "CMakeLists.txt")) {
		return "cmake --build build 2>&1"
	}
	return ""
}

func (m *Model) detectDependencyInstallCommand(verifyOutput string) string {
	output := strings.TrimSpace(verifyOutput)
	if output == "" {
		return ""
	}

	goGetRx := regexp.MustCompile(`(?m)^\s*(go get [^\r\n]+)\s*$`)
	if matches := goGetRx.FindStringSubmatch(output); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	pyMissingRx := regexp.MustCompile(`No module named ['"]([^'"]+)['"]`)
	if matches := pyMissingRx.FindStringSubmatch(output); len(matches) == 2 && isLikelyPackageName(matches[1]) {
		return fmt.Sprintf("python -m pip install %s 2>&1", matches[1])
	}

	nodeMissingRx := regexp.MustCompile(`Cannot find module ['"]([^'"]+)['"]`)
	if matches := nodeMissingRx.FindStringSubmatch(output); len(matches) == 2 && isLikelyPackageName(matches[1]) {
		return fmt.Sprintf("npm install %s 2>&1", matches[1])
	}

	lower := strings.ToLower(output)
	if strings.Contains(lower, "ruff") &&
		(strings.Contains(lower, "not recognized") || strings.Contains(lower, "n'est pas reconnu")) {
		return "python -m pip install ruff 2>&1"
	}

	return ""
}

// selectAgent picks the best-matching agent for a user input.
// It matches against agent goals/role patterns; falls back to general_agent.
func (m *Model) selectAgent(input string) *folder.AgentDef {
	agents := m.flowEngine.FolderEngine().ListAgents()
	inputLower := strings.ToLower(input)

	// Goal pattern matching
	var best *folder.AgentDef
	bestScore := -1
	for _, name := range agents {
		agent, ok := m.flowEngine.FolderEngine().GetAgent(name)
		if !ok || agent == nil {
			continue
		}
		score := 0

		if agent.Role == "coding" && (strings.Contains(inputLower, "build") || strings.Contains(inputLower, "implement") ||
			strings.Contains(inputLower, "create") || strings.Contains(inputLower, "fix") ||
			strings.Contains(inputLower, "add") || strings.Contains(inputLower, "write") ||
			strings.Contains(inputLower, "refactor") || strings.Contains(inputLower, "develop")) {
			score = 10
		} else if agent.Role == "general" {
			score = 1
		}

		for _, goal := range agent.Goals {
			if strings.Contains(inputLower, strings.ToLower(goal)) {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			best = agent
		}
	}

	if best != nil {
		return best
	}

	if general, ok := m.flowEngine.FolderEngine().GetAgent("general_agent"); ok {
		return general
	}

	return nil
}

func detectProjectType(baseDir string) string {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case name == "go.mod" || name == "go.sum":
			return "go"
		case name == "Cargo.toml" || name == "Cargo.lock":
			return "rust"
		case name == "package.json" || name == "package-lock.json":
			return "node"
		case name == "pyproject.toml" || name == "requirements.txt" || name == "setup.py":
			return "python"
		case name == "pom.xml" || name == "build.gradle":
			return "java"
		case name == "go.sum":
			return "go"
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// formatToolCallHuman formats a tool call into a human-readable summary.
func formatToolCallHuman(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return "(no input)"
	}
	var data map[string]any
	if err := json.Unmarshal(input, &data); err == nil {
		return formatMapHuman(data)
	}
	var generic any
	if err := json.Unmarshal(input, &generic); err == nil {
		pretty, err := json.MarshalIndent(generic, "", "  ")
		if err == nil {
			return strings.TrimSpace(string(pretty))
		}
	}
	return trimText(strings.TrimSpace(string(input)), 400)
}

// formatActionHuman formats an ActionProposal into a readable action summary.
func formatActionHuman(a tool.ActionProposal) string {
	var lines []string
	switch a.Type {
	case "write_file":
		lines = append(lines, fmt.Sprintf("Write file: %s", a.Path))
		if a.Content != "" {
			preview := a.Content
			if len(preview) > 200 {
				preview = preview[:200] + "\n  ..."
			}
			lines = append(lines, "Content:\n  "+preview)
		}
	case "run_command":
		lines = append(lines, fmt.Sprintf("Command: %s", a.Command))
		if a.Dir != "" {
			lines = append(lines, fmt.Sprintf("Dir: %s", a.Dir))
		}
	case "read_file":
		lines = append(lines, fmt.Sprintf("Read: %s", a.Path))
		if a.Symbol != "" {
			lines = append(lines, fmt.Sprintf("Symbol: %s", a.Symbol))
		}
	case "search_code":
		lines = append(lines, fmt.Sprintf("Search: %s", a.Pattern))
		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("In: %s", a.Path))
		}
	case "list_files":
		lines = append(lines, fmt.Sprintf("List files: %s", valueOr(a.Path, ".")))
	case "find_symbol":
		lines = append(lines, fmt.Sprintf("Find symbol: %s", a.Symbol))
		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("In: %s", a.Path))
		}
	default:

		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("Path: %s", a.Path))
		}
		if a.Command != "" {
			lines = append(lines, fmt.Sprintf("Command: %s", a.Command))
		}
		if a.Content != "" {
			lines = append(lines, "Content:\n  "+a.Content)
		}
		if len(lines) == 0 && len(a.Input) > 0 {
			return formatMapHuman(a.Input)
		}
	}
	return strings.Join(lines, "\n")
}

// formatMapHuman formats a flat-ish map into readable key: value lines.
func formatMapHuman(data map[string]any) string {
	var lines []string

	priority := []string{"path", "file", "command", "pattern", "content", "dir", "symbol", "regex"}
	for _, key := range priority {
		if v, ok := data[key]; ok {
			lines = append(lines, formatKeyValue(key, v))
		}
	}

	for key, v := range data {
		if isPriorityKey(key, priority) {
			continue
		}
		lines = append(lines, formatKeyValue(key, v))
	}
	if len(lines) == 0 {
		return "(empty)"
	}
	return strings.Join(lines, "\n")
}

func formatKeyValue(key string, v any) string {
	switch val := v.(type) {
	case string:
		if len(val) > 150 {
			return fmt.Sprintf("%s: %s\n  ...", key, val[:150])
		}
		if strings.Contains(val, "\n") {
			return fmt.Sprintf("%s:\n  %s", key, strings.Join(strings.Split(val, "\n"), "\n  "))
		}
		return fmt.Sprintf("%s: %s", key, val)
	case float64:
		return fmt.Sprintf("%s: %.0f", key, val)
	case bool:
		return fmt.Sprintf("%s: %v", key, val)
	case []any:
		if len(val) == 0 {
			return fmt.Sprintf("%s: (empty)", key)
		}
		items := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				items = append(items, s)
			} else {
				items = append(items, fmt.Sprintf("%v", item))
			}
		}
		joined := strings.Join(items, ", ")
		if len(joined) > 120 {
			joined = joined[:120] + " ..."
		}
		return fmt.Sprintf("%s: [%s]", key, joined)
	default:
		return fmt.Sprintf("%s: %v", key, val)
	}
}

func isPriorityKey(key string, priority []string) bool {
	for _, p := range priority {
		if key == p {
			return true
		}
	}
	return false
}

func isLikelyPackageName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "node:") {
		return false
	}
	return true
}

func hasAssistantTranscriptItem(items []transcriptItem, message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	for _, item := range items {
		if item.Kind == "assistant" && strings.TrimSpace(item.Body) == message {
			return true
		}
	}
	return false
}
