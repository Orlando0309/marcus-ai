package skills

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/marcus-ai/marcus/internal/skill"
)

// GitFlowSkill handles intelligent git commit workflow
type GitFlowSkill struct{}

// Run executes the git-flow skill
func (s *GitFlowSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	// Parse flags
	var (
		stageAll = false
		dryRun   = false
		amend    = false
	)

	for _, arg := range args {
		switch arg {
		case "--all", "-a":
			stageAll = true
		case "--dry-run", "-d":
			dryRun = true
		case "--amend":
			amend = true
		}
	}

	// Check if we're in a git repository
	if !s.isGitRepo(ctx, deps.ProjectRoot) {
		return skill.Result{
			Message: "Error: Not in a git repository",
			Done:    true,
			Error:   "not a git repository",
		}, nil
	}

	// Check for merge conflicts
	if conflicts, err := s.hasConflicts(ctx, deps.ProjectRoot); err == nil && conflicts {
		return skill.Result{
			Message: "Error: Merge conflicts detected. Resolve them before committing.",
			Done:    true,
			Error:   "merge conflicts",
		}, nil
	}

	// Handle --amend
	if amend {
		return s.handleAmend(ctx, deps, dryRun)
	}

	// Get current status
	status, err := s.getGitStatus(ctx, deps.ProjectRoot)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to get git status: %v", err),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	// Parse status
	staged, unstaged, untracked := s.parseStatus(status)

	// If nothing to commit
	if len(staged) == 0 && len(unstaged) == 0 && len(untracked) == 0 {
		return skill.Result{
			Message: "Nothing to commit - working tree clean",
			Done:    true,
		}, nil
	}

	// Stage all if requested
	if stageAll {
		if err := s.stageAll(ctx, deps.ProjectRoot); err != nil {
			return skill.Result{
				Message: fmt.Sprintf("Failed to stage changes: %v", err),
				Done:    true,
				Error:   err.Error(),
			}, nil
		}
		// Refresh staged list
		status, _ = s.getGitStatus(ctx, deps.ProjectRoot)
		staged, _, _ = s.parseStatus(status)
	}

	// Check if we have staged changes
	if len(staged) == 0 {
		var msg strings.Builder
		msg.WriteString("No staged changes to commit.\n\n")
		msg.WriteString(fmt.Sprintf("Unstaged changes: %d files\n", len(unstaged)))
		msg.WriteString(fmt.Sprintf("Untracked files: %d files\n", len(untracked)))
		msg.WriteString("\nUse /git-flow --all to stage and commit all changes")
		return skill.Result{
			Message: msg.String(),
			Done:    true,
		}, nil
	}

	// Get the diff of staged changes
	diff, err := s.getStagedDiff(ctx, deps.ProjectRoot)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to get diff: %v", err),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	// Generate commit message
	commitMsg, summary := s.generateCommitMessage(diff, staged)

	// Handle dry run
	if dryRun {
		var msg strings.Builder
		msg.WriteString("=== Dry Run - Commit Preview ===\n\n")
		msg.WriteString(fmt.Sprintf("Message:\n  %s\n\n", commitMsg))
		msg.WriteString(fmt.Sprintf("Summary:\n  %s\n\n", summary))
		msg.WriteString(fmt.Sprintf("Files to commit (%d):\n", len(staged)))
		for _, f := range staged {
			msg.WriteString(fmt.Sprintf("  + %s\n", f))
		}
		return skill.Result{
			Message: msg.String(),
			Done:    true,
		}, nil
	}

	// Execute the commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	cmd.Dir = deps.ProjectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to commit:\n%s\nError: %v", string(output), err),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	// Success message
	var msg strings.Builder
	msg.WriteString("=== Commit Successful ===\n\n")
	msg.WriteString(fmt.Sprintf("%s\n\n", commitMsg))
	msg.WriteString(fmt.Sprintf("Summary: %s\n", summary))
	msg.WriteString(fmt.Sprintf("Files: %d\n\n", len(staged)))
	msg.WriteString(strings.TrimSpace(string(output)))

	return skill.Result{
		Message: msg.String(),
		Done:    true,
	}, nil
}

// handleAmend handles commit amendment
func (s *GitFlowSkill) handleAmend(ctx context.Context, deps skill.Dependencies, dryRun bool) (skill.Result, error) {
	// Get the last commit diff
	diff, err := s.getLastCommitDiff(ctx, deps.ProjectRoot)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to get last commit diff: %v", err),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	if dryRun {
		return skill.Result{
			Message: fmt.Sprintf("=== Dry Run - Amend Preview ===\n\nWould amend previous commit with:\n%s", diff),
			Done:    true,
		}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "commit", "--amend", "--no-edit")
	cmd.Dir = deps.ProjectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to amend commit:\n%s", string(output)),
			Done:    true,
			Error:   err.Error(),
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Amended previous commit:\n%s", strings.TrimSpace(string(output))),
		Done:    true,
	}, nil
}

// isGitRepo checks if the directory is a git repository
func (s *GitFlowSkill) isGitRepo(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	err := cmd.Run()
	return err == nil
}

// hasConflicts checks for merge conflicts
func (s *GitFlowSkill) hasConflicts(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// getGitStatus returns the git status porcelain output
func (s *GitFlowSkill) getGitStatus(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	return string(output), err
}

// parseStatus parses git status output into staged, unstaged, and untracked files
func (s *GitFlowSkill) parseStatus(status string) (staged, unstaged, untracked []string) {
	lines := strings.Split(status, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		file := strings.TrimSpace(line[3:])

		// Staged: first char is not space or ?
		if statusCode[0] != ' ' && statusCode[0] != '?' {
			staged = append(staged, file)
		}

		// Unstaged: second char is not space
		if statusCode[1] != ' ' && statusCode[1] != '?' {
			unstaged = append(unstaged, file)
		}

		// Untracked: ??
		if statusCode == "??" {
			untracked = append(untracked, file)
		}
	}
	return
}

// stageAll stages all changes
func (s *GitFlowSkill) stageAll(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	cmd.Dir = dir
	return cmd.Run()
}

// getStagedDiff returns the diff of staged changes
func (s *GitFlowSkill) getStagedDiff(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--no-color")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// getLastCommitDiff returns diff from the last commit
func (s *GitFlowSkill) getLastCommitDiff(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--stat", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	return string(output), err
}

// generateCommitMessage creates a clear, conventional commit message
func (s *GitFlowSkill) generateCommitMessage(diff string, stagedFiles []string) (message, summary string) {
	// Analyze the diff
	additions, deletions, fileChanges := s.analyzeDiff(diff)

	// Determine change type
	changeType := s.determineChangeType(diff, additions, deletions)

	// Get primary scope (directory or file type)
	scope := s.determineScope(stagedFiles)

	// Generate concise description
	description := s.generateDescription(stagedFiles, fileChanges, changeType)

	// Build the commit message (conventional commits format)
	if scope != "" {
		message = fmt.Sprintf("%s(%s): %s", changeType, scope, description)
	} else {
		message = fmt.Sprintf("%s: %s", changeType, description)
	}

	// Ensure message isn't too long (subject line should be <= 72 chars)
	if len(message) > 72 {
		message = message[:69] + "..."
	}

	// Build summary
	summary = fmt.Sprintf("+%d/-%d in %d file(s)", additions, deletions, len(stagedFiles))

	return message, summary
}

// analyzeDiff extracts statistics from diff
func (s *GitFlowSkill) analyzeDiff(diff string) (additions, deletions int, changes map[string]int) {
	changes = make(map[string]int)
	lines := strings.Split(diff, "\n")
	currentFile := ""

	for _, line := range lines {
		// Track current file
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
		}

		// Count additions/deletions
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
			if currentFile != "" {
				changes[currentFile]++
			}
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	return additions, deletions, changes
}

// determineChangeType categorizes the change type
func (s *GitFlowSkill) determineChangeType(diff string, additions, deletions int) string {
	// Check for specific patterns
	hasTests := strings.Contains(diff, "_test.go") || strings.Contains(diff, "_test.py") ||
		strings.Contains(diff, ".test.") || strings.Contains(diff, "Test")
	hasDocs := strings.Contains(diff, "README") || strings.Contains(diff, ".md") ||
		strings.Contains(diff, "docs/") || strings.Contains(diff, "CHANGELOG")
	hasConfig := strings.Contains(diff, ".yaml") || strings.Contains(diff, ".yml") ||
		strings.Contains(diff, ".json") || strings.Contains(diff, ".toml")

	// Determine by file type
	if hasDocs {
		return "docs"
	}
	if hasTests {
		return "test"
	}
	if hasConfig {
		return "config"
	}

	// Determine by change ratio
	if additions > 0 && deletions == 0 {
		return "feat"
	}
	if additions == 0 && deletions > 0 {
		return "remove"
	}
	if deletions > additions {
		return "refactor"
	}
	if strings.Contains(diff, "fix") || strings.Contains(diff, "bug") {
		return "fix"
	}

	return "update"
}

// determineScope extracts the primary scope from file paths
func (s *GitFlowSkill) determineScope(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// Single file - use extension or filename
	if len(files) == 1 {
		file := files[0]
		if idx := strings.LastIndex(file, "."); idx > 0 {
			ext := file[idx+1:]
			if ext == "go" {
				return "go"
			}
			if ext == "py" {
				return "python"
			}
			if ext == "js" || ext == "ts" {
				return "js"
			}
			if ext == "md" {
				return "docs"
			}
		}
		return files[0]
	}

	// Multiple files - find common directory
	dirs := make(map[string]int)
	for _, f := range files {
		if idx := strings.Index(f, "/"); idx > 0 {
			dir := f[:idx]
			dirs[dir]++
		}
	}

	// Find most common directory
	var maxDir string
	maxCount := 0
	for dir, count := range dirs {
		if count > maxCount {
			maxCount = count
			maxDir = dir
		}
	}

	if maxCount >= len(files)/2 {
		return maxDir
	}

	return ""
}

// generateDescription creates a human-readable description
func (s *GitFlowSkill) generateDescription(files []string, changes map[string]int, changeType string) string {
	if len(files) == 1 {
		// Single file change - describe what happened
		file := files[0]
		name := file
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			name = file[idx+1:]
		}

		switch changeType {
		case "feat", "add":
			return fmt.Sprintf("add %s", name)
		case "remove":
			return fmt.Sprintf("remove %s", name)
		case "fix":
			return fmt.Sprintf("fix issue in %s", name)
		case "refactor":
			return fmt.Sprintf("refactor %s", name)
		case "docs":
			return fmt.Sprintf("update %s documentation", name)
		case "test":
			return fmt.Sprintf("add tests for %s", name)
		default:
			return fmt.Sprintf("update %s", name)
		}
	}

	// Multiple files - summarize
	if len(files) <= 3 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			if idx := strings.LastIndex(f, "/"); idx >= 0 {
				names = append(names, f[idx+1:])
			} else {
				names = append(names, f)
			}
		}
		return fmt.Sprintf("%s %s", changeType, strings.Join(names, ", "))
	}

	// Many files
	return fmt.Sprintf("%d files", len(files))
}
