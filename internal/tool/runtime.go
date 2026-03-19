package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/codeintel"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/lsp"
)

type Definition struct {
	Name        string
	Description string
	Schema      JSONSchema
	Source      string
	Safe        bool
	Timeout     time.Duration
	Permissions []string
}

type BuildOptions struct {
	BaseDir   string
	Config    *config.Config
	Folders   *folder.FolderEngine
	CodeIndex *codeintel.Index
	LSP       *lsp.Broker
}

func BuildRunner(opts BuildOptions) (*ToolRunner, error) {
	runner := NewToolRunner()
	runner.baseDir = opts.BaseDir
	runner.Register(NewListFilesTool(opts.BaseDir))
	runner.Register(NewReadFileTool(opts.BaseDir))
	runner.Register(NewWriteFileTool(opts.BaseDir))
	runner.Register(NewRunCommandTool(opts.BaseDir, 30*time.Second))
	runner.Register(NewSearchCodeToolWithIndex(opts.BaseDir, opts.CodeIndex))
	runner.Register(NewFindSymbolToolWithIndex(opts.BaseDir, opts.CodeIndex))
	if opts.CodeIndex != nil {
		runner.RegisterWithDefinition(NewListSymbolsTool(opts.CodeIndex), Definition{
			Name:        "list_symbols",
			Description: "List indexed symbols in a scope",
			Schema: JSONSchema{
				Type: "object",
				Properties: map[string]*JSONProperty{
					"path":        {Type: "string", Description: "Optional directory scope"},
					"max_results": {Type: "number", Description: "Maximum results"},
				},
			},
			Source: "native",
			Safe:   true,
		})
		runner.RegisterWithDefinition(NewFindReferencesTool(opts.CodeIndex), Definition{
			Name:        "find_references",
			Description: "Find references for a symbol",
			Schema: JSONSchema{
				Type: "object",
				Properties: map[string]*JSONProperty{
					"symbol":      {Type: "string", Description: "Symbol name"},
					"path":        {Type: "string", Description: "Optional scope"},
					"max_results": {Type: "number", Description: "Maximum results"},
				},
				Required: []string{"symbol"},
			},
			Source: "native",
			Safe:   true,
		})
		runner.RegisterWithDefinition(NewFindCallersTool(opts.CodeIndex), Definition{
			Name:        "find_callers",
			Description: "Find likely callers for a symbol",
			Schema: JSONSchema{
				Type: "object",
				Properties: map[string]*JSONProperty{
					"symbol":      {Type: "string", Description: "Symbol name"},
					"path":        {Type: "string", Description: "Optional scope"},
					"max_results": {Type: "number", Description: "Maximum results"},
				},
				Required: []string{"symbol"},
			},
			Source: "native",
			Safe:   true,
		})
		runner.RegisterWithDefinition(NewFindImplementationsTool(opts.CodeIndex), Definition{
			Name:        "find_implementations",
			Description: "Find likely implementations for a symbol",
			Schema: JSONSchema{
				Type: "object",
				Properties: map[string]*JSONProperty{
					"symbol":      {Type: "string", Description: "Symbol name"},
					"path":        {Type: "string", Description: "Optional scope"},
					"max_results": {Type: "number", Description: "Maximum results"},
				},
				Required: []string{"symbol"},
			},
			Source: "native",
			Safe:   true,
		})
	}
	if opts.LSP != nil {
		runner.RegisterWithDefinition(NewDiagnosticsTool(opts.BaseDir, opts.LSP), Definition{
			Name:        "get_diagnostics",
			Description: "Read LSP diagnostics for a file",
			Schema: JSONSchema{
				Type: "object",
				Properties: map[string]*JSONProperty{
					"path": {Type: "string", Description: "Path to the file"},
				},
				Required: []string{"path"},
			},
			Source: "native",
			Safe:   true,
		})
	}
	if opts.Folders != nil {
		for _, def := range opts.Folders.ToolDefs() {
			manifest, err := NewManifestTool(opts.BaseDir, def)
			if err != nil {
				return nil, err
			}
			runner.RegisterWithDefinition(manifest, manifest.definition(opts.Config))
		}
	}
	return runner, nil
}

func (tr *ToolRunner) RegisterWithDefinition(tool Tool, def Definition) {
	if tr.tools == nil {
		tr.tools = make(map[string]Tool)
	}
	if tr.definitions == nil {
		tr.definitions = make(map[string]Definition)
	}
	tr.tools[tool.Name()] = tool
	if def.Name == "" {
		def = defaultDefinition(tool)
	}
	if def.Name == "" {
		def.Name = tool.Name()
	}
	if def.Description == "" {
		def.Description = tool.Description()
	}
	if def.Schema.Type == "" {
		def.Schema = tool.Schema()
	}
	tr.definitions[tool.Name()] = def
	switch t := tool.(type) {
	case *ReadFileTool:
		tr.baseDir = t.baseDir
	case *WriteFileTool:
		tr.baseDir = t.baseDir
	case *RunCommandTool:
		tr.baseDir = t.dir
	case *SearchCodeTool:
		tr.baseDir = t.baseDir
	}
}

func (tr *ToolRunner) Definitions() []Definition {
	defs := make([]Definition, 0, len(tr.definitions))
	for _, def := range tr.definitions {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}

func (tr *ToolRunner) Definition(name string) (Definition, bool) {
	def, ok := tr.definitions[name]
	return def, ok
}

func (tr *ToolRunner) Scoped(allowed []string) *ToolRunner {
	if len(allowed) == 0 {
		return tr
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	return &ToolRunner{
		baseDir:     tr.baseDir,
		tools:       tr.tools,
		definitions: tr.definitions,
		allowed:     allowedSet,
	}
}

func (tr *ToolRunner) IsSafe(name string) bool {
	if def, ok := tr.definitions[name]; ok {
		return def.Safe
	}
	return false
}

func (tr *ToolRunner) ensureAllowed(name string) error {
	if len(tr.allowed) == 0 {
		return nil
	}
	if _, ok := tr.allowed[name]; ok {
		return nil
	}
	return fmt.Errorf("tool %s not allowed in this flow", name)
}

func defaultDefinition(tool Tool) Definition {
	def := Definition{
		Name:        tool.Name(),
		Description: tool.Description(),
		Schema:      tool.Schema(),
		Source:      "native",
	}
	switch tool.Name() {
	case "list_files", "read_file", "search_code", "find_symbol", "list_symbols", "find_references", "find_callers", "find_implementations", "get_diagnostics":
		def.Safe = true
	}
	return def
}

type ManifestTool struct {
	baseDir string
	def     *folder.ToolDef
	script  string
}

func NewManifestTool(baseDir string, def *folder.ToolDef) (*ManifestTool, error) {
	script := manifestScript(def.Path)
	if script == "" {
		return nil, fmt.Errorf("tool %s has no runnable script", def.Name)
	}
	return &ManifestTool{baseDir: baseDir, def: def, script: script}, nil
}

func (t *ManifestTool) Name() string { return t.def.Name }
func (t *ManifestTool) Description() string {
	return valueOr(t.def.Description, t.def.Tool.Description)
}
func (t *ManifestTool) Schema() JSONSchema {
	var schema JSONSchema
	if len(t.def.Schema) > 0 {
		_ = json.Unmarshal(t.def.Schema, &schema)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema
}

func (t *ManifestTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	command, args := manifestCommand(t.script)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = t.def.Path
	cmd.Env = append(os.Environ(),
		"MARCUS_TOOL_INPUT="+string(input),
		"MARCUS_PROJECT_ROOT="+t.baseDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", t.Name(), strings.TrimSpace(string(output)))
	}
	if json.Valid(output) {
		return output, nil
	}
	return json.Marshal(map[string]any{
		"tool":   t.Name(),
		"output": strings.TrimSpace(string(output)),
	})
}

func (t *ManifestTool) definition(cfg *config.Config) Definition {
	def := Definition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema:      t.Schema(),
		Source:      "manifest",
		Permissions: append([]string(nil), t.def.Permissions...),
	}
	def.Safe = hasAutoApprovePermission(cfg, t.def.Permissions)
	if t.def.Timeout > 0 {
		def.Timeout = time.Duration(t.def.Timeout) * time.Second
	}
	return def
}

func hasAutoApprovePermission(cfg *config.Config, permissions []string) bool {
	if cfg == nil {
		return false
	}
	for _, permission := range permissions {
		for _, allowed := range cfg.Tools.Manifest.AutoApprovePermissions {
			if strings.EqualFold(strings.TrimSpace(permission), strings.TrimSpace(allowed)) {
				return true
			}
		}
	}
	return false
}

func manifestScript(root string) string {
	candidates := []string{"run.sh"}
	if runtime.GOOS == "windows" {
		candidates = []string{"run.ps1", "run.cmd", "run.bat"}
	}
	for _, name := range candidates {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func manifestCommand(script string) (string, []string) {
	switch strings.ToLower(filepath.Ext(script)) {
	case ".ps1":
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script}
	case ".cmd", ".bat":
		return "cmd", []string{"/c", script}
	default:
		return "sh", []string{script}
	}
}

type ListSymbolsTool struct{ index *codeintel.Index }

func NewListSymbolsTool(index *codeintel.Index) *ListSymbolsTool {
	return &ListSymbolsTool{index: index}
}
func (t *ListSymbolsTool) Name() string        { return "list_symbols" }
func (t *ListSymbolsTool) Description() string { return "List indexed symbols" }
func (t *ListSymbolsTool) Schema() JSONSchema  { return JSONSchema{Type: "object"} }
func (t *ListSymbolsTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	_ = json.Unmarshal(input, &params)
	return json.Marshal(map[string]any{"symbols": t.index.ListSymbols(params.Path, params.MaxResults)})
}

type FindReferencesTool struct{ index *codeintel.Index }

func NewFindReferencesTool(index *codeintel.Index) *FindReferencesTool {
	return &FindReferencesTool{index: index}
}
func (t *FindReferencesTool) Name() string        { return "find_references" }
func (t *FindReferencesTool) Description() string { return "Find references for a symbol" }
func (t *FindReferencesTool) Schema() JSONSchema  { return JSONSchema{Type: "object"} }
func (t *FindReferencesTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Symbol     string `json:"symbol"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	_ = json.Unmarshal(input, &params)
	return json.Marshal(map[string]any{"matches": t.index.FindReferences(params.Symbol, params.Path, params.MaxResults)})
}

type FindCallersTool struct{ index *codeintel.Index }

func NewFindCallersTool(index *codeintel.Index) *FindCallersTool {
	return &FindCallersTool{index: index}
}
func (t *FindCallersTool) Name() string        { return "find_callers" }
func (t *FindCallersTool) Description() string { return "Find likely callers for a symbol" }
func (t *FindCallersTool) Schema() JSONSchema  { return JSONSchema{Type: "object"} }
func (t *FindCallersTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Symbol     string `json:"symbol"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	_ = json.Unmarshal(input, &params)
	return json.Marshal(map[string]any{"matches": t.index.FindCallers(params.Symbol, params.Path, params.MaxResults)})
}

type FindImplementationsTool struct{ index *codeintel.Index }

func NewFindImplementationsTool(index *codeintel.Index) *FindImplementationsTool {
	return &FindImplementationsTool{index: index}
}
func (t *FindImplementationsTool) Name() string { return "find_implementations" }
func (t *FindImplementationsTool) Description() string {
	return "Find likely implementations for a symbol"
}
func (t *FindImplementationsTool) Schema() JSONSchema { return JSONSchema{Type: "object"} }
func (t *FindImplementationsTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Symbol     string `json:"symbol"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	_ = json.Unmarshal(input, &params)
	return json.Marshal(map[string]any{"matches": t.index.FindImplementations(params.Symbol, params.Path, params.MaxResults)})
}

type DiagnosticsTool struct {
	baseDir string
	broker  *lsp.Broker
}

func NewDiagnosticsTool(baseDir string, broker *lsp.Broker) *DiagnosticsTool {
	return &DiagnosticsTool{baseDir: baseDir, broker: broker}
}
func (t *DiagnosticsTool) Name() string        { return "get_diagnostics" }
func (t *DiagnosticsTool) Description() string { return "Get diagnostics for a file" }
func (t *DiagnosticsTool) Schema() JSONSchema  { return JSONSchema{Type: "object"} }
func (t *DiagnosticsTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	language := detectLspLanguage(path)
	if language == "" {
		return json.Marshal(map[string]any{"diagnostics": []any{}})
	}
	diagnostics, err := t.broker.Diagnostics(ctx, language, "file:///"+filepath.ToSlash(path))
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "unsupported") {
		return nil, err
	}
	return json.Marshal(map[string]any{"diagnostics": diagnostics})
}

func detectLspLanguage(path string) string {
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
