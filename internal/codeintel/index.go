//go:build !windows

package codeintel

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
	tsjs "github.com/smacker/go-tree-sitter/javascript"
	tspy "github.com/smacker/go-tree-sitter/python"
	tsts "github.com/smacker/go-tree-sitter/typescript/typescript"
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
	return &Index{
		baseDir: baseDir,
		files:   make(map[string]fileIndex),
	}
}

func (i *Index) Build(ctx context.Context) error {
	if i.baseDir == "" {
		return nil
	}
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
		entry, ok := i.indexFile(ctx, path, rel)
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
	if i.baseDir == "" {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, relPath := range relPaths {
		relPath = filepath.ToSlash(relPath)
		abs := filepath.Join(i.baseDir, filepath.FromSlash(relPath))
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			delete(i.files, relPath)
			continue
		}
		entry, ok := i.indexFile(ctx, abs, relPath)
		if ok {
			i.files[relPath] = entry
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
				if len(result) >= max {
					return result
				}
			}
		}
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Path == result[b].Path {
			return result[a].Line < result[b].Line
		}
		return result[a].Path < result[b].Path
	})
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
	var result []Match
	var re *regexp.Regexp
	if isRegex {
		re = regexp.MustCompile(pattern)
	}
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
				result = append(result, Match{
					Path:    path,
					Line:    idx + 1,
					Preview: strings.TrimSpace(line),
				})
				if len(result) >= max {
					return result
				}
			}
		}
	}
	return result
}

func (i *Index) indexFile(ctx context.Context, absPath, relPath string) (fileIndex, bool) {
	_ = ctx
	language, lang := detectLanguage(absPath)
	if lang == nil {
		return fileIndex{}, false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fileIndex{}, false
	}
	root, err := sitter.ParseCtx(context.Background(), data, lang)
	if err != nil || root == nil {
		return fileIndex{}, false
	}
	lines := strings.Split(string(data), "\n")
	return fileIndex{
		Language: language,
		Symbols:  extractSymbols(language, relPath, data, root),
		Lines:    lines,
	}, true
}

func detectLanguage(path string) (string, *sitter.Language) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go", tsgo.GetLanguage()
	case ".py":
		return "python", tspy.GetLanguage()
	case ".js", ".jsx":
		return "javascript", tsjs.GetLanguage()
	case ".ts", ".tsx":
		return "typescript", tsts.GetLanguage()
	default:
		return "", nil
	}
}

func extractSymbols(language, relPath string, content []byte, root *sitter.Node) []Symbol {
	var symbols []Symbol
	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if symbol, ok := nodeSymbol(language, relPath, content, node); ok {
			symbols = append(symbols, symbol)
		}
		for idx := 0; idx < int(node.NamedChildCount()); idx++ {
			child := node.NamedChild(idx)
			walk(child)
		}
	}
	walk(root)
	return dedupeSymbols(symbols)
}

func nodeSymbol(language, relPath string, content []byte, node *sitter.Node) (Symbol, bool) {
	nodeType := node.Type()
	switch language {
	case "go":
		switch nodeType {
		case "function_declaration":
			return symbolFromNode(language, relPath, content, node, "function"), true
		case "method_declaration":
			return symbolFromNode(language, relPath, content, node, "method"), true
		case "type_spec":
			return symbolFromNode(language, relPath, content, node, "type"), true
		case "const_spec":
			return symbolFromFirstIdentifier(language, relPath, content, node, "const")
		case "var_spec":
			return symbolFromFirstIdentifier(language, relPath, content, node, "var")
		}
	case "python":
		switch nodeType {
		case "function_definition":
			return symbolFromNode(language, relPath, content, node, "function"), true
		case "class_definition":
			return symbolFromNode(language, relPath, content, node, "class"), true
		}
	case "javascript", "typescript":
		switch nodeType {
		case "function_declaration":
			return symbolFromNode(language, relPath, content, node, "function"), true
		case "class_declaration":
			return symbolFromNode(language, relPath, content, node, "class"), true
		case "method_definition":
			return symbolFromNode(language, relPath, content, node, "method"), true
		case "interface_declaration":
			return symbolFromNode(language, relPath, content, node, "interface"), true
		case "type_alias_declaration":
			return symbolFromNode(language, relPath, content, node, "type"), true
		case "lexical_declaration", "variable_declaration":
			return symbolFromFirstIdentifier(language, relPath, content, node, "variable")
		}
	}
	return Symbol{}, false
}

func symbolFromNode(language, relPath string, content []byte, node *sitter.Node, kind string) Symbol {
	nameNode := node.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = strings.TrimSpace(nameNode.Content(content))
	}
	if name == "" {
		name = fallbackIdentifier(node, content)
	}
	return Symbol{
		Name:      name,
		Kind:      kind,
		Path:      relPath,
		Line:      int(node.StartPoint().Row) + 1,
		Language:  language,
		Signature: trimNodeContent(node.Content(content)),
	}
}

func symbolFromFirstIdentifier(language, relPath string, content []byte, node *sitter.Node, kind string) (Symbol, bool) {
	name := fallbackIdentifier(node, content)
	if name == "" {
		return Symbol{}, false
	}
	return Symbol{
		Name:      name,
		Kind:      kind,
		Path:      relPath,
		Line:      int(node.StartPoint().Row) + 1,
		Language:  language,
		Signature: trimNodeContent(node.Content(content)),
	}, true
}

func fallbackIdentifier(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() == "identifier" || strings.HasSuffix(node.Type(), "_identifier") || node.Type() == "type_identifier" || node.Type() == "property_identifier" {
		return strings.TrimSpace(node.Content(content))
	}
	for idx := 0; idx < int(node.NamedChildCount()); idx++ {
		child := node.NamedChild(idx)
		if name := fallbackIdentifier(child, content); name != "" {
			return name
		}
	}
	return ""
}

func trimNodeContent(input string) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len(input) > 160 {
		return input[:160] + "..."
	}
	return input
}

func dedupeSymbols(input []Symbol) []Symbol {
	seen := make(map[string]struct{}, len(input))
	var result []Symbol
	for _, symbol := range input {
		if symbol.Name == "" {
			continue
		}
		key := symbol.Path + "|" + symbol.Name + "|" + symbol.Kind + "|" + strconvI(symbol.Line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, symbol)
	}
	return result
}

func withinScope(path, scope string) bool {
	scope = filepath.ToSlash(strings.TrimSpace(scope))
	if scope == "" || scope == "." {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+"/")
}

func strconvI(v int) string {
	return strconv.Itoa(v)
}
