package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/marcus-ai/marcus/internal/skill"
)

// CommitSkill generates a commit message from staged changes
type CommitSkill struct{}

func (c *CommitSkill) Name() string { return "commit" }

func (c *CommitSkill) Pattern() string { return "/commit" }

func (c *CommitSkill) Description() string {
	return "Generate and create a git commit from staged changes"
}

func (c *CommitSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	// Check if we're in a git repository
	if deps.ProjectRoot == "" {
		return skill.Result{
			Message: "Not in a project directory",
			Done:    true,
		}, nil
	}

	// Get staged diff
	stagedDiff, err := c.getStagedDiff(ctx, deps.ProjectRoot)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to get staged diff: %v", err),
			Done:    true,
		}, nil
	}

	// Check if there are staged changes
	if strings.TrimSpace(stagedDiff) == "" {
		// Check for unstaged changes
		unstagedDiff, _ := c.getUnstagedDiff(ctx, deps.ProjectRoot)
		if strings.TrimSpace(unstagedDiff) != "" {
			return skill.Result{
				Message: "No staged changes found.\nThere are unstaged changes. Run 'git add <files>' first, or use /commit --all to stage and commit all changes.",
				Done:    true,
			}, nil
		}

		return skill.Result{
			Message: "No changes to commit. Stage some files with 'git add' first.",
			Done:    true,
		}, nil
	}

	// Check for --all flag
	if len(args) > 0 && args[0] == "--all" {
		return c.commitAll(ctx, args, deps)
	}

	// Generate commit message using provider
	commitMsg, err := c.generateCommitMessage(ctx, stagedDiff, deps)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to generate commit message: %v", err),
			Done:    true,
		}, nil
	}

	// Execute git commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	cmd.Dir = deps.ProjectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create commit:\n%s\n\nError: %v", string(output), err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Created commit:\n%s\n\n%s", commitMsg, strings.TrimSpace(string(output))),
		Done:    true,
	}, nil
}

func (c *CommitSkill) commitAll(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	// Stage all changes first
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = deps.ProjectRoot
	if output, err := addCmd.CombinedOutput(); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to stage changes:\n%s\n\nError: %v", string(output), err),
			Done:    true,
		}, nil
	}

	// Get the staged diff
	stagedDiff, err := c.getStagedDiff(ctx, deps.ProjectRoot)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to get staged diff: %v", err),
			Done:    true,
		}, nil
	}

	if strings.TrimSpace(stagedDiff) == "" {
		return skill.Result{
			Message: "No changes to commit after staging.",
			Done:    true,
		}, nil
	}

	// Generate commit message
	commitMsg, err := c.generateCommitMessage(ctx, stagedDiff, deps)
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to generate commit message: %v", err),
			Done:    true,
		}, nil
	}

	// Execute git commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	cmd.Dir = deps.ProjectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create commit:\n%s\n\nError: %v", string(output), err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Staged all changes and created commit:\n%s\n\n%s", commitMsg, strings.TrimSpace(string(output))),
		Done:    true,
	}, nil
}

func (c *CommitSkill) getStagedDiff(ctx context.Context, projectRoot string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--no-color")
	cmd.Dir = projectRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func (c *CommitSkill) getUnstagedDiff(ctx context.Context, projectRoot string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-color")
	cmd.Dir = projectRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func (c *CommitSkill) generateCommitMessage(ctx context.Context, diff string, deps skill.Dependencies) (string, error) {
	// For now, use a simple heuristic based on the diff
	// In a full implementation, this would call the provider

	// Truncate diff if too long
	maxDiffLen := 4000
	if len(diff) > maxDiffLen {
		diff = diff[:maxDiffLen] + "\n... (truncated)"
	}

	// Parse the diff to understand what changed
	var additions, deletions int
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	// Determine the type of change
	var changeType string
	if additions > 0 && deletions == 0 {
		changeType = "Add"
	} else if additions == 0 && deletions > 0 {
		changeType = "Remove"
	} else if additions > deletions {
		changeType = "Update"
	} else if deletions > additions {
		changeType = "Refactor"
	} else {
		changeType = "Update"
	}

	// Extract affected files
	files := c.extractAffectedFiles(diff)
	if len(files) == 1 {
		return fmt.Sprintf("%s %s", changeType, files[0]), nil
	} else if len(files) <= 3 {
		return fmt.Sprintf("%s %s", changeType, strings.Join(files, ", ")), nil
	} else {
		return fmt.Sprintf("%s %d files", changeType, len(files)), nil
	}
}

func (c *CommitSkill) extractAffectedFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			if file != "/dev/null" && !seen[file] {
				files = append(files, file)
				seen[file] = true
			}
		}
	}

	return files
}
