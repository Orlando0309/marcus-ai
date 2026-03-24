package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/marcus-ai/marcus/internal/lsp"
)

// LSPDefinitionTool wraps go-to-definition.
type LSPDefinitionTool struct {
	baseDir string
	broker  *lsp.Broker
}

func NewLSPDefinitionTool(baseDir string, broker *lsp.Broker) *LSPDefinitionTool {
	return &LSPDefinitionTool{baseDir: baseDir, broker: broker}
}

func (t *LSPDefinitionTool) Name() string { return "lsp_definition" }

func (t *LSPDefinitionTool) Description() string {
	return "Go to definition for a position in a file (0-based line/character)"
}

func (t *LSPDefinitionTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":      {Type: "string", Description: "File path"},
			"line":      {Type: "number", Description: "Line (0-based)"},
			"character": {Type: "number", Description: "Character (0-based)"},
		},
		Required: []string{"path", "line", "character"},
	}
}

func (t *LSPDefinitionTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.broker == nil {
		return nil, fmt.Errorf("LSP not configured")
	}
	var params struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	lang := detectLspLanguage(path)
	if lang == "" {
		return nil, fmt.Errorf("unsupported language for LSP")
	}
	uri := "file:///" + filepath.ToSlash(path)
	locs, err := t.broker.Definition(ctx, lang, uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"locations": locs})
}

// LSPReferencesTool wraps find-references.
type LSPReferencesTool struct {
	baseDir string
	broker  *lsp.Broker
}

func NewLSPReferencesTool(baseDir string, broker *lsp.Broker) *LSPReferencesTool {
	return &LSPReferencesTool{baseDir: baseDir, broker: broker}
}

func (t *LSPReferencesTool) Name() string { return "lsp_references" }

func (t *LSPReferencesTool) Description() string {
	return "Find references for a symbol at a position (0-based line/character)"
}

func (t *LSPReferencesTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":      {Type: "string", Description: "File path"},
			"line":      {Type: "number", Description: "Line (0-based)"},
			"character": {Type: "number", Description: "Character (0-based)"},
		},
		Required: []string{"path", "line", "character"},
	}
}

func (t *LSPReferencesTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.broker == nil {
		return nil, fmt.Errorf("LSP not configured")
	}
	var params struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	lang := detectLspLanguage(path)
	if lang == "" {
		return nil, fmt.Errorf("unsupported language for LSP")
	}
	uri := "file:///" + filepath.ToSlash(path)
	locs, err := t.broker.References(ctx, lang, uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"locations": locs})
}

// LSPHoverTool returns hover documentation at a position.
type LSPHoverTool struct {
	baseDir string
	broker  *lsp.Broker
}

func NewLSPHoverTool(baseDir string, broker *lsp.Broker) *LSPHoverTool {
	return &LSPHoverTool{baseDir: baseDir, broker: broker}
}

func (t *LSPHoverTool) Name() string { return "lsp_hover" }

func (t *LSPHoverTool) Description() string {
	return "Hover documentation for a symbol at a position (0-based line/character)"
}

func (t *LSPHoverTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":      {Type: "string", Description: "File path"},
			"line":      {Type: "number", Description: "Line (0-based)"},
			"character": {Type: "number", Description: "Character (0-based)"},
		},
		Required: []string{"path", "line", "character"},
	}
}

func (t *LSPHoverTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.broker == nil {
		return nil, fmt.Errorf("LSP not configured")
	}
	var params struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	lang := detectLspLanguage(path)
	if lang == "" {
		return nil, fmt.Errorf("unsupported language for LSP")
	}
	uri := "file:///" + filepath.ToSlash(path)
	text, err := t.broker.Hover(ctx, lang, uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"hover": text})
}

// LSPRenameTool requests a workspace rename (returns LSP WorkspaceEdit JSON; does not apply edits).
type LSPRenameTool struct {
	baseDir string
	broker  *lsp.Broker
}

func NewLSPRenameTool(baseDir string, broker *lsp.Broker) *LSPRenameTool {
	return &LSPRenameTool{baseDir: baseDir, broker: broker}
}

func (t *LSPRenameTool) Name() string { return "lsp_rename" }

func (t *LSPRenameTool) Description() string {
	return "Propose a workspace rename for a symbol at a position; returns raw LSP WorkspaceEdit JSON (not applied automatically)"
}

func (t *LSPRenameTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path":      {Type: "string", Description: "File path"},
			"line":      {Type: "number", Description: "Line (0-based)"},
			"character": {Type: "number", Description: "Character (0-based)"},
			"new_name":  {Type: "string", Description: "New symbol name"},
		},
		Required: []string{"path", "line", "character", "new_name"},
	}
}

func (t *LSPRenameTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.broker == nil {
		return nil, fmt.Errorf("LSP not configured")
	}
	var params struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		NewName   string `json:"new_name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	path, err := resolveToolPath(t.baseDir, params.Path)
	if err != nil {
		return nil, err
	}
	lang := detectLspLanguage(path)
	if lang == "" {
		return nil, fmt.Errorf("unsupported language for LSP")
	}
	uri := "file:///" + filepath.ToSlash(path)
	raw, err := t.broker.Rename(ctx, lang, uri, params.Line, params.Character, params.NewName)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return json.Marshal(map[string]any{"workspace_edit": nil})
	}
	return json.Marshal(map[string]any{"workspace_edit": json.RawMessage(raw)})
}
