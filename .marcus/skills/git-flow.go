package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillResult is returned by the skill after execution
type SkillResult struct {
	Message string `json:"message"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

// ChangeGroup represents a logical group of changes
type ChangeGroup struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // feat, fix, docs, test, refactor, config, chore
	Description string   `json:"description"`
	Files       []string `json:"files"`
	Additions   int      `json:"additions"`
	Deletions   int      `json:"deletions"`
	Reasoning   string   `json:"reasoning"`
}

func main() {
	argsStr := os.Getenv("MARCUS_SKILL_ARGS")
	var args []string
	if argsStr != "" {
		args = strings.Split(argsStr, "\x00")
	}

	projectRoot, _ := os.Getwd()
	result := run(args, projectRoot)
	fmt.Println(result.Message)

	if result.Error != "" {
		os.Exit(1)
	}
}

func run(args []string, projectRoot string) SkillResult {
	// Parse flags
	var (
		dryRun    = false
		commitAll = false
	)

	for _, arg := range args {
		switch arg {
		case "--commit-all":
			commitAll = true
		case "--dry-run", "-d":
			dryRun = true
		}
	}

	// Check git repo
	if !isGitRepo(projectRoot) {
		return SkillResult{Message: "Error: Not in a git repository", Done: true, Error: "not a git repository"}
	}

	// Get all changes (staged + unstaged + untracked)
	allChanges, err := getAllChanges(projectRoot)
	if err != nil {
		return SkillResult{Message: fmt.Sprintf("Failed to get changes: %v", err), Done: true, Error: err.Error()}
	}

	if len(allChanges) == 0 {
		return SkillResult{Message: "Nothing to commit - working tree clean", Done: true}
	}

	// Analyze and group changes intelligently
	groups := analyzeAndGroupChanges(allChanges, projectRoot)

	// Build the output
	var msg strings.Builder
	msg.WriteString("=== Git Flow - Change Analysis ===\n\n")
	msg.WriteString(fmt.Sprintf("Found %d changed file(s)\n\n", len(allChanges)))

	// Show current status
	msg.WriteString("Current Status:\n")
	staged, unstaged, untracked := categorizeFiles(allChanges)
	if len(staged) > 0 {
		msg.WriteString(fmt.Sprintf("  Staged:     %d file(s)\n", len(staged)))
	}
	if len(unstaged) > 0 {
		msg.WriteString(fmt.Sprintf("  Unstaged:   %d file(s)\n", len(unstaged)))
	}
	if len(untracked) > 0 {
		msg.WriteString(fmt.Sprintf("  Untracked:  %d file(s)\n", len(untracked)))
	}
	msg.WriteString("\n")

	// Show suggested commit groups
	msg.WriteString("=== Suggested Commit Groups ===\n\n")
	msg.WriteString("Based on file analysis, here are logical groupings:\n\n")

	for i, group := range groups {
		msg.WriteString(fmt.Sprintf("[%d] %s(%s): %s\n", i+1, group.Type, group.Name, group.Description))
		msg.WriteString(fmt.Sprintf("    Files: %d | +%d/-%d\n", len(group.Files), group.Additions, group.Deletions))
		msg.WriteString(fmt.Sprintf("    Reasoning: %s\n", group.Reasoning))
		msg.WriteString("    Files:\n")
		for _, f := range group.Files {
			msg.WriteString(fmt.Sprintf("      - %s\n", f))
		}
		msg.WriteString("\n")
	}

	// Show recommendations
	msg.WriteString("=== Recommendations ===\n\n")

	if len(groups) == 1 {
		msg.WriteString("All changes fit well in a single commit.\n")
		msg.WriteString(fmt.Sprintf("Suggested: %s(%s): %s\n\n", groups[0].Type, groups[0].Name, groups[0].Description))
	} else {
		msg.WriteString(fmt.Sprintf("Consider splitting into %d separate commits for clarity.\n\n", len(groups)))

		// Identify dependencies
		msg.WriteString("Commit Order (based on dependencies):\n")
		for i, group := range groups {
			msg.WriteString(fmt.Sprintf("  %d. %s(%s): %s\n", i+1, group.Type, group.Name, group.Description))
		}
		msg.WriteString("\n")
	}

	// Show usage options
	msg.WriteString("=== Usage ===\n\n")
	msg.WriteString("To commit a specific group:\n")
	for i, group := range groups {
		msg.WriteString(fmt.Sprintf("  /git-flow --group %d  # Commit: %s(%s): %s\n", i+1, group.Type, group.Name, group.Description))
	}
	msg.WriteString("\n")
	msg.WriteString("Other options:\n")
	msg.WriteString("  /git-flow --commit-all    # Commit all changes as one\n")
	msg.WriteString("  /git-flow --dry-run       # Preview without committing\n")
	msg.WriteString("  /git-flow --status        # Show detailed file status\n")

	// Handle --commit-all
	if commitAll {
		return commitAllChanges(groups, projectRoot, dryRun)
	}

	return SkillResult{Message: msg.String(), Done: true}
}

// FileChange represents a single file change
type FileChange struct {
	Path      string
	Status    string // staged, unstaged, untracked, modified, added, deleted
	Diff      string
	Additions int
	Deletions int
}

func getAllChanges(projectRoot string) ([]FileChange, error) {
	// Get status with porclean
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = projectRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var changes []FileChange
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		filePath := strings.TrimSpace(line[3:])

		change := FileChange{Path: filePath}

		// Parse status codes
		switch {
		case statusCode == "??":
			change.Status = "untracked"
		case statusCode[0] != ' ' && statusCode[0] != '?':
			change.Status = "staged"
		case statusCode[1] != ' ':
			change.Status = "unstaged"
		}

		// Get diff for this file
		change.Diff = getFileDiff(projectRoot, filePath)
		change.Additions, change.Deletions = countDiffStats(change.Diff)

		changes = append(changes, change)
	}

	return changes, nil
}

func categorizeFiles(changes []FileChange) (staged, unstaged, untracked []string) {
	for _, c := range changes {
		switch c.Status {
		case "staged":
			staged = append(staged, c.Path)
		case "unstaged":
			unstaged = append(unstaged, c.Path)
		case "untracked":
			untracked = append(untracked, c.Path)
		}
	}
	return
}

func analyzeAndGroupChanges(changes []FileChange, projectRoot string) []ChangeGroup {
	var groups []ChangeGroup

	// Group by file type/purpose
	testFiles := extractByPattern(changes, `_(test|spec)\.(go|py|js|ts|java)|\.(test|spec)\.(js|ts|py)|test.*\.go`)
	docFiles := extractByPattern(changes, `README|\.md$|CHANGELOG|LICENSE|docs/|\.txt$`)
	configFiles := extractByPattern(changes, `\.yaml$|\.yml$|\.json$|\.toml$|\.ini$|\.conf$|config`)
	buildFiles := extractByPattern(changes, `Makefile|Dockerfile|\.sh$|\.bash$|\.ps1$|build\.go|go\.mod|go\.sum`)

	// Source code (non-test, non-config)
	sourceFiles := excludeFiles(changes, append(append(append(testFiles, docFiles...), configFiles...), buildFiles...))

	// Create groups based on what we found

	// Group 1: Tests (if any)
	if len(testFiles) > 0 {
		adds, dels := sumDiffStats(testFiles)
		groups = append(groups, ChangeGroup{
			Name:        "tests",
			Type:        "test",
			Description: fmt.Sprintf("add/update tests for %s", describeFiles(testFiles)),
			Files:       filePaths(testFiles),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   "Test files should be committed separately to keep test changes isolated",
		})
	}

	// Group 2: Documentation (if any)
	if len(docFiles) > 0 {
		adds, dels := sumDiffStats(docFiles)
		desc := "documentation"
		if len(docFiles) == 1 && strings.Contains(docFiles[0].Path, "README") {
			desc = "README"
		}
		groups = append(groups, ChangeGroup{
			Name:        "docs",
			Type:        "docs",
			Description: fmt.Sprintf("update %s", desc),
			Files:       filePaths(docFiles),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   "Documentation changes are usually independent of code changes",
		})
	}

	// Group 3: Configuration (if any)
	if len(configFiles) > 0 {
		adds, dels := sumDiffStats(configFiles)
		groups = append(groups, ChangeGroup{
			Name:        "config",
			Type:        "config",
			Description: fmt.Sprintf("update %s", describeFiles(configFiles)),
			Files:       filePaths(configFiles),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   "Configuration changes should be separate from business logic",
		})
	}

	// Group 4: Build scripts (if any)
	if len(buildFiles) > 0 {
		adds, dels := sumDiffStats(buildFiles)
		groups = append(groups, ChangeGroup{
			Name:        "build",
			Type:        "chore",
			Description: fmt.Sprintf("update %s", describeFiles(buildFiles)),
			Files:       filePaths(buildFiles),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   "Build/tooling changes are infrastructure, not features",
		})
	}

	// Group 5: Source code - split by purpose if large
	if len(sourceFiles) > 0 {
		// Try to split by directory/purpose
		sourceGroups := groupByPurpose(sourceFiles)
		for _, sg := range sourceGroups {
			groups = append(groups, sg)
		}
	}

	// If we couldn't group anything, create a default group
	if len(groups) == 0 && len(changes) > 0 {
		adds, dels := sumDiffStats(changes)
		groups = append(groups, ChangeGroup{
			Name:        "changes",
			Type:        determineChangeType(changes),
			Description: fmt.Sprintf("update %d files", len(changes)),
			Files:       filePaths(changes),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   "General update",
		})
	}

	// Sort groups by priority (config first, then source, then tests, then docs)
	return sortGroupsByPriority(groups)
}

func groupByPurpose(files []FileChange) []ChangeGroup {
	// Group by directory or common purpose
	byDir := make(map[string][]FileChange)

	for _, f := range files {
		dir := filepath.Dir(f.Path)
		if dir == "." {
			dir = "root"
		}
		// Simplify to top-level directory
		parts := strings.Split(dir, string(filepath.Separator))
		if len(parts) > 0 && parts[0] != "." {
			dir = parts[0]
		}
		byDir[dir] = append(byDir[dir], f)
	}

	var groups []ChangeGroup
	for dir, merged := range byDir {

		adds, dels := sumDiffStats(merged)
		changeType := determineChangeType(merged)

		// Determine scope and description
		scope := dir
		if scope == "root" {
			scope = ""
		}

		desc := fmt.Sprintf("update %s", scope)
		if len(merged) == 1 {
			desc = fmt.Sprintf("update %s", filepath.Base(merged[0].Path))
		}

		group := ChangeGroup{
			Name:        scope,
			Type:        changeType,
			Description: desc,
			Files:       filePaths(merged),
			Additions:   adds,
			Deletions:   dels,
			Reasoning:   fmt.Sprintf("Changes in %s/ directory", dir),
		}

		if scope == "" {
			group.Name = "main"
			group.Reasoning = "Root level changes"
		}

		groups = append(groups, group)
	}

	return groups
}

func sortGroupsByPriority(groups []ChangeGroup) []ChangeGroup {
	// Priority: config < chore < refactor < feat/fix < test < docs
	priority := map[string]int{
		"config": 1,
		"chore":  2,
		"refactor": 3,
		"feat":   4,
		"fix":    4,
		"test":   5,
		"docs":   6,
	}

	// Simple bubble sort
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			if priority[groups[i].Type] > priority[groups[j].Type] {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}

	return groups
}

func commitAllChanges(groups []ChangeGroup, projectRoot string, dryRun bool) SkillResult {
	// Stage all files
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = projectRoot
	if err := cmd.Run(); err != nil {
		return SkillResult{Message: fmt.Sprintf("Failed to stage: %v", err), Done: true, Error: err.Error()}
	}

	// Generate commit message
	var msgType string
	var description string

	if len(groups) == 1 {
		msgType = groups[0].Type
		description = groups[0].Description
	} else {
		msgType = "update"
		descs := make([]string, 0, len(groups))
		for _, g := range groups {
			descs = append(descs, g.Name)
		}
		description = strings.Join(descs, ", ")
	}

	commitMsg := fmt.Sprintf("%s: %s", msgType, description)
	if len(commitMsg) > 72 {
		commitMsg = commitMsg[:69] + "..."
	}

	if dryRun {
		var msg strings.Builder
		msg.WriteString("=== Dry Run - Would Commit ===\n\n")
		msg.WriteString(fmt.Sprintf("Message: %s\n\n", commitMsg))
		for _, g := range groups {
			msg.WriteString(fmt.Sprintf("  - %s: %d files\n", g.Name, len(g.Files)))
		}
		return SkillResult{Message: msg.String(), Done: true}
	}

	// Execute commit
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return SkillResult{Message: fmt.Sprintf("Failed to commit:\n%s", string(output)), Done: true, Error: err.Error()}
	}

	return SkillResult{
		Message: fmt.Sprintf("Committed:\n%s\n\n%s", commitMsg, strings.TrimSpace(string(output))),
		Done:    true,
	}
}

// Helper functions

func extractByPattern(files []FileChange, pattern string) []FileChange {
	re := regexp.MustCompile(pattern)
	var result []FileChange
	for _, f := range files {
		if re.MatchString(f.Path) {
			result = append(result, f)
		}
	}
	return result
}

func excludeFiles(files, exclude []FileChange) []FileChange {
	excludeMap := make(map[string]bool)
	for _, e := range exclude {
		excludeMap[e.Path] = true
	}

	var result []FileChange
	for _, f := range files {
		if !excludeMap[f.Path] {
			result = append(result, f)
		}
	}
	return result
}

func filePaths(files []FileChange) []string {
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}

func sumDiffStats(files []FileChange) (adds, dels int) {
	for _, f := range files {
		adds += f.Additions
		dels += f.Deletions
	}
	return
}

func describeFiles(files []FileChange) string {
	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		return filepath.Base(files[0].Path)
	}
	if len(files) <= 3 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = filepath.Base(f.Path)
		}
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%d files", len(files))
}

func determineChangeType(files []FileChange) string {
	var totalAdd, totalDel int
	hasNewFiles := false

	for _, f := range files {
		totalAdd += f.Additions
		totalDel += f.Deletions
		if f.Status == "untracked" || f.Additions > 0 && f.Deletions == 0 {
			hasNewFiles = true
		}
	}

	// Determine by change ratio
	if hasNewFiles && totalDel == 0 {
		return "feat"
	}
	if totalAdd == 0 && totalDel > 0 {
		return "remove"
	}
	if totalDel > totalAdd {
		return "refactor"
	}

	// Check file content for hints
	for _, f := range files {
		if strings.Contains(f.Diff, "fix") || strings.Contains(f.Diff, "bug") {
			return "fix"
		}
	}

	return "update"
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil
}

func getFileDiff(dir, file string) string {
	// Try staged first
	cmd := exec.Command("git", "diff", "--cached", "--no-color", "--", file)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		// Try unstaged
		cmd = exec.Command("git", "diff", "--no-color", "--", file)
		cmd.Dir = dir
		output, _ = cmd.Output()
	}
	return string(output)
}

func getStagedDiff(dir string) string {
	cmd := exec.Command("git", "diff", "--cached", "--no-color")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	return out.String()
}

func countDiffStats(diff string) (adds, dels int) {
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			adds++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			dels++
		}
	}
	return
}
