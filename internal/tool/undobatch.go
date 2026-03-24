package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UndoBatch captures pre-mutation file state for rollback and user /undo.
type UndoBatch struct {
	Root    string
	Entries []UndoFileEntry
}

// UndoFileEntry is one file's prior contents (or absence).
type UndoFileEntry struct {
	Rel     string
	HadFile bool
	Data    []byte
}

// MutatingPaths returns unique repo-relative paths touched by file-mutating proposals.
func MutatingPaths(proposals []ActionProposal) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range proposals {
		switch p.Type {
		case "write_file", "patch_file", "edit_file", "delete_file", "create_file":
			rel := strings.TrimSpace(p.Path)
			if rel == "" || seen[rel] {
				continue
			}
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out
}

// SnapshotUndoBatch reads current file bytes for all mutating paths (before apply).
func SnapshotUndoBatch(root string, proposals []ActionProposal) (UndoBatch, error) {
	rels := MutatingPaths(proposals)
	if len(rels) == 0 {
		return UndoBatch{Root: root}, nil
	}
	if root == "" {
		return UndoBatch{}, fmt.Errorf("snapshot: empty root")
	}
	var entries []UndoFileEntry
	for _, rel := range rels {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		b, err := os.ReadFile(full)
		if err != nil {
			entries = append(entries, UndoFileEntry{Rel: rel, HadFile: false})
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		entries = append(entries, UndoFileEntry{Rel: rel, HadFile: true, Data: cp})
	}
	return UndoBatch{Root: root, Entries: entries}, nil
}

// RestoreUndoBatch restores files to the captured state.
func RestoreUndoBatch(batch UndoBatch) error {
	for _, e := range batch.Entries {
		full := filepath.Join(batch.Root, filepath.FromSlash(e.Rel))
		if e.HadFile {
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(full, e.Data, 0o644); err != nil {
				return err
			}
		} else {
			_ = os.Remove(full)
		}
	}
	return nil
}

// ApplyProposalsInTransaction applies proposals in order; on first error restores prior file state.
// The returned UndoBatch is the pre-apply snapshot (for stacking on user /undo after success).
func (tr *ToolRunner) ApplyProposalsInTransaction(ctx context.Context, proposals []ActionProposal) ([]ActionResult, UndoBatch, error) {
	if tr == nil {
		return nil, UndoBatch{}, fmt.Errorf("nil runner")
	}
	batch, err := SnapshotUndoBatch(tr.baseDir, proposals)
	if err != nil {
		return nil, UndoBatch{}, err
	}
	var results []ActionResult
	for _, p := range proposals {
		r, err := tr.ApplyAction(ctx, p)
		if err != nil {
			_ = RestoreUndoBatch(batch)
			return nil, UndoBatch{}, fmt.Errorf("%s: %w", p.Label(), err)
		}
		results = append(results, r)
	}
	return results, batch, nil
}
