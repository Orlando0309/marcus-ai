package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marcus-ai/marcus/internal/diff"
)

// PatchFileTool applies a unified diff to an existing file.
type PatchFileTool struct {
	baseDir string
	list    *ListFilesTool
}

func NewPatchFileTool(baseDir string, list *ListFilesTool) *PatchFileTool {
	return &PatchFileTool{baseDir: baseDir, list: list}
}

func (t *PatchFileTool) Name() string { return "patch_file" }

func (t *PatchFileTool) Description() string {
	return "Apply a unified diff to a file (surgical edits; paths in the diff are ignored; only the target path is used)"
}

func (t *PatchFileTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":         {Type: "string", Description: "File path to patch (relative to project root)"},
			"unified_diff": {Type: "string", Description: "Complete unified diff body (may include ---/+++ headers)"},
		},
		Required: []string{"path", "unified_diff"},
	}
}

func (t *PatchFileTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path        string `json:"path"`
		UnifiedDiff string `json:"unified_diff"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	patches, err := diff.ParseUnifiedDiff(params.UnifiedDiff)
	if err != nil {
		return nil, fmt.Errorf("parse unified diff: %w", err)
	}
	next, err := diff.ApplyPatch(string(current), patches)
	if err != nil {
		return nil, fmt.Errorf("apply patch: %w", err)
	}
	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	if t.list != nil {
		t.list.invalidate()
	}
	return json.Marshal(map[string]any{
		"path":   params.Path,
		"pushed": true,
		"bytes":  len(next),
	})
}

// EditFileTool replaces a unique substring (or all occurrences) in a file.
type EditFileTool struct {
	baseDir string
	list    *ListFilesTool
}

func NewEditFileTool(baseDir string, list *ListFilesTool) *EditFileTool {
	return &EditFileTool{baseDir: baseDir, list: list}
}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Description() string {
	return "Replace old_string with new_string in a file (must match exactly once unless replace_all is true)"
}

func (t *EditFileTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":         {Type: "string", Description: "File path"},
			"old_string":   {Type: "string", Description: "Exact text to find"},
			"new_string":   {Type: "string", Description: "Replacement text"},
			"replace_all":  {Type: "boolean", Description: "Replace every occurrence"},
		},
		Required: []string{"path", "old_string", "new_string"},
	}
}

func (t *EditFileTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path        string `json:"path"`
		OldString   string `json:"old_string"`
		NewString   string `json:"new_string"`
		ReplaceAll  bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	s := string(current)
	if params.OldString == "" {
		return nil, fmt.Errorf("old_string must not be empty")
	}
	var next string
	var n int
	if params.ReplaceAll {
		n = strings.Count(s, params.OldString)
		if n == 0 {
			return nil, fmt.Errorf("old_string not found")
		}
		next = strings.ReplaceAll(s, params.OldString, params.NewString)
	} else {
		n = strings.Count(s, params.OldString)
		if n == 0 {
			return nil, fmt.Errorf("old_string not found")
		}
		if n != 1 {
			return nil, fmt.Errorf("old_string matches %d times; must be unique or set replace_all", n)
		}
		next = strings.Replace(s, params.OldString, params.NewString, 1)
	}
	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	if t.list != nil {
		t.list.invalidate()
	}
	return json.Marshal(map[string]any{
		"path":     params.Path,
		"edited":   true,
		"replaces": n,
		"bytes":    len(next),
	})
}

// GlobFilesTool finds files matching a glob pattern (supports * and **).
type GlobFilesTool struct {
	baseDir string
}

func NewGlobFilesTool(baseDir string) *GlobFilesTool {
	return &GlobFilesTool{baseDir: baseDir}
}

func (t *GlobFilesTool) Name() string { return "glob_files" }

func (t *GlobFilesTool) Description() string {
	return "Find files under the project matching a glob (e.g. **/*.go, cmd/*)"
}

func (t *GlobFilesTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"pattern":     {Type: "string", Description: "Glob pattern using / as separator"},
			"path":        {Type: "string", Description: "Optional subdirectory to search under (default: project root)"},
			"max_results": {Type: "number", Description: "Maximum matches (default 200)"},
		},
		Required: []string{"pattern"},
	}
}

func (t *GlobFilesTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 200
	}
	root := t.baseDir
	if root == "" {
		root, _ = os.Getwd()
	}
	if strings.TrimSpace(params.Path) != "" && params.Path != "." {
		sub, err := resolveToolPath(t.baseDir, params.Path)
		if err != nil {
			return nil, err
		}
		fi, err := os.Stat(sub)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("path is not a directory: %s", params.Path)
		}
		root = sub
	}
	pattern := filepath.ToSlash(strings.TrimSpace(params.Pattern))
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	var matches []string
	_ = filepath.WalkDir(root, func(full string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= params.MaxResults {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if matchGlobPattern(pattern, relSlash) {
			matches = append(matches, relSlash)
		}
		return nil
	})

	sort.Strings(matches)
	return json.Marshal(map[string]any{
		"pattern": pattern,
		"count":   len(matches),
		"files":   matches,
	})
}

// ListDirectoryTool lists immediate children of a directory (files and subdirs).
type ListDirectoryTool struct {
	baseDir string
}

func NewListDirectoryTool(baseDir string) *ListDirectoryTool {
	return &ListDirectoryTool{baseDir: baseDir}
}

func (t *ListDirectoryTool) Name() string { return "list_directory" }

func (t *ListDirectoryTool) Description() string {
	return "List files and subdirectories in a single directory (non-recursive)"
}

func (t *ListDirectoryTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":        {Type: "string", Description: "Directory path (default: project root)"},
			"max_results": {Type: "number", Description: "Max entries (default 500)"},
		},
	}
}

func (t *ListDirectoryTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 500
	}
	dir := t.baseDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if strings.TrimSpace(params.Path) != "" && params.Path != "." {
		var err error
		dir, err = resolveToolPath(t.baseDir, params.Path)
		if err != nil {
			return nil, err
		}
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", params.Path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	var out []entry
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if len(out) >= params.MaxResults {
			break
		}
		name := e.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, _ := filepath.Rel(t.baseDir, full)
		if t.baseDir == "" {
			rel = name
		}
		out = append(out, entry{
			Name:  name,
			Path:  filepath.ToSlash(rel),
			IsDir: e.IsDir(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	scope := params.Path
	if scope == "" {
		scope = "."
	}
	return json.Marshal(map[string]any{
		"path":    scope,
		"count":   len(out),
		"entries": out,
	})
}

// matchGlobPattern matches path (forward slashes, relative) against pattern.
func matchGlobPattern(pattern, path string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	path = filepath.ToSlash(path)
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "**") {
		if !strings.Contains(pattern, "/") {
			// "*.go" applies only at repo root, not under subdirs.
			if strings.Contains(path, "/") {
				return false
			}
			ok, _ := filepath.Match(pattern, path)
			return ok
		}
		ok, _ := filepath.Match(pattern, path)
		return ok
	}
	return matchDoubleStarGlob(pattern, path)
}

func matchDoubleStarGlob(pattern, path string) bool {
	i := strings.Index(pattern, "**")
	if i < 0 {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}
	before := strings.TrimSuffix(pattern[:i], "/")
	after := strings.TrimPrefix(pattern[i+2:], "/")
	if before != "" {
		if path != before && !strings.HasPrefix(path, before+"/") {
			return false
		}
		path = strings.TrimPrefix(path, before)
		path = strings.TrimPrefix(path, "/")
	}
	if after == "" {
		return true
	}
	if strings.Contains(after, "**") {
		if ok, _ := filepath.Match(after, path); ok {
			return true
		}
		for j := 0; j < len(path); j++ {
			if path[j] != '/' {
				continue
			}
			tail := path[j+1:]
			if matchDoubleStarGlob(after, tail) {
				return true
			}
		}
		return matchDoubleStarGlob(after, path)
	}
	if ok, _ := filepath.Match(after, path); ok {
		return true
	}
	for j := 0; j < len(path); j++ {
		if path[j] != '/' {
			continue
		}
		tail := path[j+1:]
		if ok, _ := filepath.Match(after, tail); ok {
			return true
		}
	}
	return false
}
