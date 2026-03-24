package context

import (
	"fmt"
	"io"
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
	Text              string
	Branch            string
	Dirty             bool
	FileHints         []string
	TODOHints         []task.TODOHint
	EstimatedTokens   int
	Truncated         bool
	DroppedSections   []string
}

// Assembler builds a compact repo-aware prompt context.
type Assembler struct {
	cfg         *config.Config
	projectRoot string
	flowEngine  *flow.Engine
	taskStore   *task.Store
	memory      *memory.Manager
	projectMap  *ProjectMapManager
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
		projectMap:  NewProjectMapManager(projectRoot),
	}
}

// Assemble builds the current prompt context.
func (a *Assembler) Assemble(input string, sess *session.Session) Snapshot {
	branch, dirty := gitState(a.projectRoot)
	snapshot := Snapshot{
		Branch: branch,
		Dirty:  dirty,
	}

	maxTok := 100000
	if a.cfg != nil && a.cfg.Context.MaxContextTokens > 0 {
		maxTok = a.cfg.Context.MaxContextTokens
	}

	order := 0
	nextOrder := func() int { o := order; order++; return o }

	var blocks []contextBlock

	// --- Core (high keep) ---
	var headerLines []string
	if a.cfg != nil {
		projectName := a.cfg.Project.Name
		if projectName == "" && a.projectRoot != "" {
			projectName = filepath.Base(a.projectRoot)
		}
		headerLines = append(headerLines, fmt.Sprintf("Project: %s", projectName))
	}
	if a.projectRoot != "" {
		headerLines = append(headerLines, fmt.Sprintf("Root: %s", a.projectRoot))
	}
	if branch != "" {
		state := fmt.Sprintf("Git: %s", branch)
		if dirty {
			state += " (dirty)"
		}
		headerLines = append(headerLines, state)
	}
	if len(headerLines) > 0 {
		blocks = append(blocks, contextBlock{
			name:  "header",
			text:  strings.Join(headerLines, "\n"),
			keep:  1000,
			order: nextOrder(),
		})
	}

	if a.flowEngine != nil {
		flows := a.flowEngine.ListFlows()
		sort.Strings(flows)
		if len(flows) > 0 {
			blocks = append(blocks, contextBlock{
				name:  "flows",
				text:  "Flows:\n- " + strings.Join(flows, "\n- "),
				keep:  900,
				order: nextOrder(),
			})
		}
		tools := a.flowEngine.ListTools()
		sort.Strings(tools)
		if len(tools) > 0 {
			blocks = append(blocks, contextBlock{
				name:  "tools",
				text:  "Tools:\n- " + strings.Join(tools, "\n- "),
				keep:  880,
				order: nextOrder(),
			})
		}
	}

	if a.taskStore != nil {
		blocks = append(blocks, contextBlock{
			name:  "tasks",
			text:  "Tasks:\n" + a.taskStore.Summary(),
			keep:  850,
			order: nextOrder(),
		})
	}

	if projectMap := a.loadProjectMap(); projectMap != "" {
		blocks = append(blocks, contextBlock{
			name:  "project_map",
			text:  "Project Map:\n" + projectMap,
			keep:  940,
			order: nextOrder(),
		})
	}

	// --- Files (@mentions): relevance-ranked + same-dir neighbors ---
	files := a.extractMentionedFiles(input)
	maxFiles := 8
	if a.cfg != nil && a.cfg.Context.MaxFilesPerPrompt > 0 {
		maxFiles = a.cfg.Context.MaxFilesPerPrompt
	}
	files = a.augmentWithNeighbors(files, maxFiles, 4)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	snapshot.FileHints = files
	type scoredFile struct {
		path  string
		score int
		text  string
	}
	var sf []scoredFile
	maxBytes := 8192
	if a.cfg != nil && a.cfg.Context.MaxFileBytes > 0 {
		maxBytes = a.cfg.Context.MaxFileBytes
	}
	var todoHints []task.TODOHint
	for i, file := range files {
		peek, err := a.peekFilePrefix(file, 4096)
		if err != nil {
			continue
		}
		if ProbablyBinary(peek) {
			sc := fileRelevanceScore(input, file, "") + (len(files)-i)*3
			sf = append(sf, scoredFile{
				path:  file,
				score: sc,
				text:  fmt.Sprintf("File: %s\n[binary or non-text file omitted from context]", file),
			})
			continue
		}
		content, err := a.readFile(file)
		if err != nil {
			continue
		}
		todoHints = append(todoHints, extractTODOHints(file, content)...)
		sc := fileRelevanceScore(input, file, content) + (len(files)-i)*3
		body := trim(content, maxBytes)
		sf = append(sf, scoredFile{
			path:  file,
			score: sc,
			text:  fmt.Sprintf("File: %s\n%s", file, body),
		})
	}
	sort.Slice(sf, func(i, j int) bool { return sf[i].score > sf[j].score })
	for _, f := range sf {
		blocks = append(blocks, contextBlock{
			name:  "file:" + f.path,
			text:  f.text,
			keep:  400 + min(f.score, 200),
			order: nextOrder(),
		})
	}
	if len(todoHints) > 0 {
		snapshot.TODOHints = todoHints
		var lines []string
		for _, hint := range todoHints {
			lines = append(lines, fmt.Sprintf("- %s:%d %s", hint.Path, hint.Line, hint.Text))
		}
		blocks = append(blocks, contextBlock{
			name:  "todos",
			text:  "Detected TODOs:\n" + strings.Join(lines, "\n"),
			keep:  500,
			order: nextOrder(),
		})
	}

	if a.memory != nil && a.cfg != nil {
		mem := a.memory.Summary(input, a.cfg.Memory.RecallLimit)
		if mem != "" {
			blocks = append(blocks, contextBlock{
				name:  "memory",
				text:  "Memory:\n" + mem,
				keep:  520,
				order: nextOrder(),
			})
		}
		epi := a.memory.EpisodicSummary(6)
		if epi != "" {
			blocks = append(blocks, contextBlock{
				name:  "episodic",
				text:  "Recent turns:\n" + epi,
				keep:  510,
				order: nextOrder(),
			})
		}
	}

	if docs := a.loadProjectDocs(); docs != "" {
		blocks = append(blocks, contextBlock{
			name:  "docs",
			text:  "Project Docs:\n" + docs,
			keep:  480,
			order: nextOrder(),
		})
	}

	if sess != nil {
		recent := sess.RecentTurns(6)
		if len(recent) > 0 {
			var lines []string
			for _, turn := range recent {
				lines = append(lines, fmt.Sprintf("- %s: %s", turn.Role, trim(turn.Content, 280)))
			}
			blocks = append(blocks, contextBlock{
				name:  "conversation",
				text:  "Recent Conversation:\n" + strings.Join(lines, "\n"),
				keep:  300,
				order: nextOrder(),
			})
		}
	}

	text, est, trunc, dropped := applyTokenBudget(blocks, maxTok)
	snapshot.Text = text
	snapshot.EstimatedTokens = est
	snapshot.Truncated = trunc
	snapshot.DroppedSections = dropped
	return snapshot
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *Assembler) loadProjectDocs() string {
	if a.projectRoot == "" || a.cfg == nil {
		return ""
	}
	var sections []string
	for _, rel := range a.cfg.Context.AlwaysInclude {
		if filepath.ToSlash(strings.TrimSpace(rel)) == ".marcus/context/PROJECT_MAP.md" {
			continue
		}
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

func (a *Assembler) loadProjectMap() string {
	if a.projectMap == nil {
		return ""
	}
	return a.projectMap.Ensure()
}

func (a *Assembler) RefreshProjectMap() string {
	if a.projectMap == nil {
		return ""
	}
	return a.projectMap.Refresh()
}

// augmentWithNeighbors adds up to maxExtra same-extension files from the same directory as already-mentioned paths.
func (a *Assembler) augmentWithNeighbors(files []string, maxTotal, maxExtra int) []string {
	if a.projectRoot == "" || maxExtra <= 0 {
		return files
	}
	seen := make(map[string]bool, len(files)+maxExtra)
	for _, f := range files {
		seen[f] = true
	}
	out := append([]string(nil), files...)
	added := 0
	for _, f := range files {
		if added >= maxExtra || len(out) >= maxTotal {
			break
		}
		dir := filepath.Dir(f)
		ext := filepath.Ext(f)
		if ext == "" {
			continue
		}
		fullDir := filepath.Join(a.projectRoot, dir)
		entries, err := os.ReadDir(fullDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if filepath.Ext(e.Name()) != ext {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(dir, e.Name()))
			if seen[rel] {
				continue
			}
			seen[rel] = true
			out = append(out, rel)
			added++
			if added >= maxExtra || len(out) >= maxTotal {
				return out
			}
		}
	}
	return out
}

func (a *Assembler) peekFilePrefix(relPath string, n int) ([]byte, error) {
	if a.projectRoot == "" {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(a.projectRoot, relPath)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	nr, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return buf[:nr], nil
	}
	if err != nil {
		return nil, err
	}
	return buf[:nr], nil
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
