package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
)

// Snapshot is the assembled prompt context.
type Snapshot struct {
	Text      string
	Branch    string
	Dirty     bool
	FileHints []string
	TODOHints []task.TODOHint
}

// Assembler builds a compact repo-aware prompt context.
type Assembler struct {
	cfg         *config.Config
	projectRoot string
	flowEngine  *flow.Engine
	taskStore   *task.Store
	memory      *memory.Manager
}

// NewAssembler creates a new context assembler.
func NewAssembler(cfg *config.Config, flowEngine *flow.Engine, taskStore *task.Store, memoryManager *memory.Manager) *Assembler {
	projectRoot := ""
	if cfg != nil {
		projectRoot = cfg.ProjectRoot
	}
	return &Assembler{
		cfg:         cfg,
		projectRoot: projectRoot,
		flowEngine:  flowEngine,
		taskStore:   taskStore,
		memory:      memoryManager,
	}
}

// Assemble builds the current prompt context.
func (a *Assembler) Assemble(input string, sess *session.Session) Snapshot {
	branch, dirty := gitState(a.projectRoot)
	snapshot := Snapshot{
		Branch: branch,
		Dirty:  dirty,
	}

	var sections []string
	if a.cfg != nil {
		projectName := a.cfg.Project.Name
		if projectName == "" && a.projectRoot != "" {
			projectName = filepath.Base(a.projectRoot)
		}
		sections = append(sections, fmt.Sprintf("Project: %s", projectName))
	}
	if a.projectRoot != "" {
		sections = append(sections, fmt.Sprintf("Root: %s", a.projectRoot))
	}
	if branch != "" {
		state := fmt.Sprintf("Git: %s", branch)
		if dirty {
			state += " (dirty)"
		}
		sections = append(sections, state)
	}
	if a.flowEngine != nil {
		flows := a.flowEngine.ListFlows()
		sort.Strings(flows)
		if len(flows) > 0 {
			sections = append(sections, "Flows:\n- "+strings.Join(flows, "\n- "))
		}
		tools := a.flowEngine.ListTools()
		sort.Strings(tools)
		if len(tools) > 0 {
			sections = append(sections, "Tools:\n- "+strings.Join(tools, "\n- "))
		}
	}
	if a.taskStore != nil {
		sections = append(sections, "Tasks:\n"+a.taskStore.Summary())
	}
	if a.memory != nil && a.cfg != nil {
		sections = append(sections, "Memory:\n"+a.memory.Summary(input, a.cfg.Memory.RecallLimit))
	}
	if docs := a.loadProjectDocs(); docs != "" {
		sections = append(sections, "Project Docs:\n"+docs)
	}
	if sess != nil {
		recent := sess.RecentTurns(6)
		if len(recent) > 0 {
			var lines []string
			for _, turn := range recent {
				lines = append(lines, fmt.Sprintf("- %s: %s", turn.Role, trim(turn.Content, 280)))
			}
			sections = append(sections, "Recent Conversation:\n"+strings.Join(lines, "\n"))
		}
	}

	files := a.extractMentionedFiles(input)
	snapshot.FileHints = files
	if len(files) > 0 {
		var fileSections []string
		var todoHints []task.TODOHint
		for _, file := range files {
			content, err := a.readFile(file)
			if err != nil {
				continue
			}
			fileSections = append(fileSections, fmt.Sprintf("File: %s\n%s", file, trim(content, a.cfg.Context.MaxFileBytes)))
			todoHints = append(todoHints, extractTODOHints(file, content)...)
		}
		if len(fileSections) > 0 {
			sections = append(sections, "Attached Files:\n"+strings.Join(fileSections, "\n\n"))
		}
		if len(todoHints) > 0 {
			snapshot.TODOHints = todoHints
			var lines []string
			for _, hint := range todoHints {
				lines = append(lines, fmt.Sprintf("- %s:%d %s", hint.Path, hint.Line, hint.Text))
			}
			sections = append(sections, "Detected TODOs:\n"+strings.Join(lines, "\n"))
		}
	}

	snapshot.Text = strings.Join(sections, "\n\n")
	return snapshot
}

func (a *Assembler) loadProjectDocs() string {
	if a.projectRoot == "" || a.cfg == nil {
		return ""
	}
	var sections []string
	for _, rel := range a.cfg.Context.AlwaysInclude {
		path := rel
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.projectRoot, rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		sections = append(sections, fmt.Sprintf("## %s\n%s", filepath.Base(path), trim(string(data), 1800)))
	}
	return strings.Join(sections, "\n\n")
}

func (a *Assembler) extractMentionedFiles(input string) []string {
	if a.projectRoot == "" || a.cfg == nil {
		return nil
	}
	parts := strings.Fields(input)
	var files []string
	for _, part := range parts {
		if !strings.HasPrefix(part, "@") {
			continue
		}
		candidate := strings.Trim(part[1:], " ,.;:()[]{}\"'")
		if candidate == "" {
			continue
		}
		path := filepath.Join(a.projectRoot, candidate)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files = append(files, candidate)
		}
		if len(files) >= a.cfg.Context.MaxFilesPerPrompt {
			break
		}
	}
	return files
}

func (a *Assembler) readFile(relPath string) (string, error) {
	if a.projectRoot == "" {
		return "", os.ErrNotExist
	}
	path := filepath.Join(a.projectRoot, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ScanTODOs scans the provided repo-relative files for TODO markers.
func (a *Assembler) ScanTODOs(relPaths []string) []task.TODOHint {
	var hints []task.TODOHint
	for _, relPath := range relPaths {
		content, err := a.readFile(relPath)
		if err != nil {
			continue
		}
		hints = append(hints, extractTODOHints(relPath, content)...)
	}
	return hints
}

func gitState(projectRoot string) (string, bool) {
	if projectRoot == "" {
		return "", false
	}
	branch := strings.TrimSpace(runGit(projectRoot, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch == "" || strings.Contains(branch, "fatal:") {
		return "", false
	}
	dirty := strings.TrimSpace(runGit(projectRoot, "status", "--short")) != ""
	return branch, dirty
}

func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func trim(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[:limit]) + "\n...[truncated]"
}

func extractTODOHints(path, content string) []task.TODOHint {
	lines := strings.Split(content, "\n")
	var hints []task.TODOHint
	for i, line := range lines {
		idx := strings.Index(strings.ToUpper(line), "TODO:")
		if idx == -1 {
			continue
		}
		text := strings.TrimSpace(line[idx+len("TODO:"):])
		if text == "" {
			continue
		}
		hints = append(hints, task.TODOHint{
			Path: path,
			Line: i + 1,
			Text: text,
		})
	}
	return hints
}
