package isolation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/tool"
)

const (
	ModeInPlace  = "in_place"
	ModeWorktree = "worktree"
)

type Session struct {
	Mode string
	Root string
	Name string
}

type Manager struct {
	root string
	cfg  config.IsolationConfig
}

func NewManager(projectRoot string, cfg config.IsolationConfig) *Manager {
	return &Manager{root: projectRoot, cfg: cfg}
}

func (m *Manager) Prepare(ctx context.Context, proposals []tool.ActionProposal) (*Session, error) {
	if !m.cfg.Enabled || m.root == "" {
		return &Session{Mode: ModeInPlace, Root: m.root, Name: "main"}, nil
	}
	if !m.shouldUseWorktree(proposals) {
		return &Session{Mode: ModeInPlace, Root: m.root, Name: "main"}, nil
	}
	if !isGitRepo(ctx, m.root) {
		return &Session{Mode: ModeInPlace, Root: m.root, Name: "main"}, nil
	}
	name := "marcus-" + time.Now().UTC().Format("20060102-150405")
	target := filepath.Join(filepath.Dir(m.root), name)
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", target)
	cmd.Dir = m.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("create worktree: %s", strings.TrimSpace(string(output)))
	}
	return &Session{Mode: ModeWorktree, Root: target, Name: name}, nil
}

func (m *Manager) shouldUseWorktree(proposals []tool.ActionProposal) bool {
	if m.cfg.PreferWorktree {
		return true
	}
	writes := 0
	for _, proposal := range proposals {
		if proposal.Type == "write_file" {
			writes++
		}
		if proposal.Type == "run_command" && strings.Contains(strings.ToLower(proposal.Command), "install") {
			return true
		}
	}
	return writes >= max(1, m.cfg.RiskyFileWrites)
}

func (m *Manager) Cleanup(ctx context.Context, session *Session) error {
	if session == nil || session.Mode != ModeWorktree || session.Root == "" || m.root == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", session.Root)
	cmd.Dir = m.root
	_, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(session.Root)
	}
	return nil
}

func isGitRepo(ctx context.Context, root string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}
