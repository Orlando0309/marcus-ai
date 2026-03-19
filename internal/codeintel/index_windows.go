//go:build windows

package codeintel

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Language  string `json:"language"`
	Signature string `json:"signature,omitempty"`
}

type Match struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Preview string `json:"preview"`
}

type Index struct {
	baseDir string
	mu      sync.RWMutex
	files   map[string]fileIndex
}

type fileIndex struct {
	Language string
	Symbols  []Symbol
	Lines    []string
}

func NewIndex(baseDir string) *Index {
	return &Index{baseDir: baseDir, files: make(map[string]fileIndex)}
}

func (i *Index) Build(ctx context.Context) error {
	next := make(map[string]fileIndex)
	err := filepath.WalkDir(i.baseDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".venv", "__pycache__", ".marcus":
				if path != i.baseDir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(i.baseDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		entry, ok := i.indexFile(path, rel)
		if ok {
			next[rel] = entry
		}
		return nil
	})
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.files = next
	i.mu.Unlock()
	return nil
}

func (i *Index) Refresh(ctx context.Context, relPaths []string) error {
	_ = ctx
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, rel := range relPaths {
		rel = filepath.ToSlash(rel)
		abs := filepath.Join(i.baseDir, filepath.FromSlash(rel))
		entry, ok := i.indexFile(abs, rel)
		if ok {
			i.files[rel] = entry
		} else {
			delete(i.files, rel)
		}
	}
	return nil
}

func (i *Index) FindSymbol(name, scope string, max int) []Symbol {
	if max <= 0 {
		max = 20
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	var result []Symbol
	for path, entry := range i.files {
		if !withinScope(path, scope) {
			continue
		}
		for _, symbol := range entry.Symbols {
			if symbol.Name == name {
				result = append(result, symbol)
			}
		}
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Path == result[b].Path {
			return result[a].Line < result[b].Line
		}
		return result[a].Path < result[b].Path
	})
	if len(result) > max {
		result = result[:max]
	}
	return result
}

func (i *Index) ListSymbols(scope string, max int) []Symbol {
	if max <= 0 {
		max = 50
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	var result []Symbol
	for path, entry := range i.files {
		if !withinScope(path, scope) {
			continue
		}
		result = append(result, entry.Symbols...)
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Path == result[b].Path {
			return result[a].Line < result[b].Line
		}
		return result[a].Path < result[b].Path
	})
	if len(result) > max {
		result = result[:max]
	}
	return result
}

func (i *Index) FindReferences(name, scope string, max int) []Match {
	return i.Search(name, scope, false, max)
}

func (i *Index) FindCallers(name, scope string, max int) []Match {
	return i.Search(name+"(", scope, false, max)
}

func (i *Index) FindImplementations(name, scope string, max int) []Match {
	return i.Search(name, scope, false, max)
}

func (i *Index) Search(pattern, scope string, isRegex bool, max int) []Match {
	if max <= 0 {
		max = 20
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	var re *regexp.Regexp
	if isRegex {
		re = regexp.MustCompile(pattern)
	}
	var result []Match
	for path, entry := range i.files {
		if !withinScope(path, scope) {
			continue
		}
		for idx, line := range entry.Lines {
			matched := strings.Contains(line, pattern)
			if re != nil {
				matched = re.MatchString(line)
			}
			if matched {
				result = append(result, Match{Path: path, Line: idx + 1, Preview: strings.TrimSpace(line)})
			}
		}
	}
	if len(result) > max {
		result = result[:max]
	}
	return result
}

func (i *Index) indexFile(absPath, relPath string) (fileIndex, bool) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fileIndex{}, false
	}
	language := detectLanguage(absPath)
	if language == "" {
		return fileIndex{}, false
	}
	lines := strings.Split(string(data), "\n")
	return fileIndex{
		Language: language,
		Symbols:  extractSymbols(language, relPath, string(data)),
		Lines:    lines,
	}, true
}

func detectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	default:
		return ""
	}
}

func extractSymbols(language, relPath, content string) []Symbol {
	switch language {
	case "go":
		return extractGoSymbols(relPath, content)
	default:
		return extractRegexSymbols(language, relPath, content)
	}
}

func extractGoSymbols(relPath, content string) []Symbol {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, content, parser.ParseComments)
	if err != nil {
		return nil
	}
	var symbols []Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "function"
			if d.Recv != nil {
				kind = "method"
			}
			symbols = append(symbols, Symbol{Name: d.Name.Name, Kind: kind, Path: relPath, Line: fset.Position(d.Pos()).Line, Language: "go"})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					symbols = append(symbols, Symbol{Name: s.Name.Name, Kind: "type", Path: relPath, Line: fset.Position(s.Pos()).Line, Language: "go"})
				case *ast.ValueSpec:
					for _, name := range s.Names {
						symbols = append(symbols, Symbol{Name: name.Name, Kind: strings.ToLower(d.Tok.String()), Path: relPath, Line: fset.Position(name.Pos()).Line, Language: "go"})
					}
				}
			}
		}
	}
	return symbols
}

func extractRegexSymbols(language, relPath, content string) []Symbol {
	lines := strings.Split(content, "\n")
	var patterns []*regexp.Regexp
	switch language {
	case "python":
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)`),
			regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`),
		}
	default:
		patterns = []*regexp.Regexp{
			regexp.MustCompile(`^\s*function\s+([A-Za-z_][A-Za-z0-9_]*)`),
			regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`),
			regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)`),
			regexp.MustCompile(`^\s*interface\s+([A-Za-z_][A-Za-z0-9_]*)`),
			regexp.MustCompile(`^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)`),
		}
	}
	var symbols []Symbol
	for idx, line := range lines {
		for _, pattern := range patterns {
			match := pattern.FindStringSubmatch(line)
			if len(match) > 1 {
				symbols = append(symbols, Symbol{Name: match[1], Kind: "symbol", Path: relPath, Line: idx + 1, Language: language, Signature: strings.TrimSpace(line)})
				break
			}
		}
	}
	return symbols
}

func withinScope(path, scope string) bool {
	scope = filepath.ToSlash(strings.TrimSpace(scope))
	if scope == "" || scope == "." {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+"/")
}
