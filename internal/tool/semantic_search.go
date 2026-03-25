package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/marcus-ai/marcus/internal/codeintel"
)

// SemanticSearchTool provides semantic code search
type SemanticSearchTool struct {
	baseDir string
	index   *codeintel.SemanticIndex
}

// NewSemanticSearchTool creates a new semantic search tool
func NewSemanticSearchTool(baseDir string, index *codeintel.SemanticIndex) *SemanticSearchTool {
	return &SemanticSearchTool{
		baseDir: baseDir,
		index:   index,
	}
}

// Name returns the tool name
func (t *SemanticSearchTool) Name() string {
	return "semantic_search"
}

// Description returns the tool description
func (t *SemanticSearchTool) Description() string {
	return "Search for code using semantic similarity. Finds code that matches the meaning of your query, not just keywords."
}

// Schema returns the JSON schema for the tool
func (t *SemanticSearchTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"query": {
				Type:        "string",
				Description: "The search query describing what you're looking for (e.g., 'function that handles user authentication', 'database connection pool')",
			},
			"limit": {
				Type:        "number",
				Description: "Maximum number of results to return (default: 10)",
			},
			"language": {
				Type:        "string",
				Description: "Filter by programming language (e.g., 'go', 'python', 'javascript')",
			},
		},
		Required: []string{"query"},
	}
}

// Run executes the semantic search
func (t *SemanticSearchTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.index == nil {
		return nil, fmt.Errorf("semantic index not initialized")
	}

	var params struct {
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
		Language string `json:"language,omitempty"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if params.Limit <= 0 {
		params.Limit = 10
	}

	var results []codeintel.SemanticSearchResult
	var err error

	if params.Language != "" {
		results, err = t.index.SearchWithFilter(ctx, params.Query, []string{params.Language}, params.Limit)
	} else {
		results, err = t.index.Search(ctx, params.Query, params.Limit)
	}

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	output := map[string]any{
		"query":   params.Query,
		"results": results,
		"count":   len(results),
	}

	return json.Marshal(output)
}

// SemanticIndexTool provides operations for the semantic index
type SemanticIndexTool struct {
	baseDir string
	index   *codeintel.SemanticIndex
}

// NewSemanticIndexTool creates a new semantic index tool
func NewSemanticIndexTool(baseDir string, index *codeintel.SemanticIndex) *SemanticIndexTool {
	return &SemanticIndexTool{
		baseDir: baseDir,
		index:   index,
	}
}

// Name returns the tool name
func (t *SemanticIndexTool) Name() string {
	return "semantic_index"
}

// Description returns the tool description
func (t *SemanticIndexTool) Description() string {
	return "Manage the semantic code index: build, refresh, save, load, or get statistics"
}

// Schema returns the JSON schema for the tool
func (t *SemanticIndexTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"action": {
				Type:        "string",
				Description: "Action to perform: build, refresh, save, load, stats, clear",
			},
			"paths": {
				Type:        "array",
				Description: "List of file paths to index (for build/refresh)",
			},
			"index_path": {
				Type:        "string",
				Description: "Path to save/load the index (for save/load)",
			},
		},
		Required: []string{"action"},
	}
}

// Run executes the semantic index operation
func (t *SemanticIndexTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.index == nil {
		return nil, fmt.Errorf("semantic index not initialized")
	}

	var params struct {
		Action    string   `json:"action"`
		Paths     []string `json:"paths,omitempty"`
		IndexPath string   `json:"index_path,omitempty"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	output := make(map[string]any)

	switch params.Action {
	case "build":
		if err := t.index.Build(ctx, params.Paths); err != nil {
			return nil, fmt.Errorf("build failed: %w", err)
		}
		output["message"] = "Index built successfully"
		output["indexed_files"] = t.index.Size()

	case "refresh":
		if err := t.index.Refresh(ctx, params.Paths); err != nil {
			return nil, fmt.Errorf("refresh failed: %w", err)
		}
		output["message"] = "Index refreshed successfully"
		output["indexed_files"] = t.index.Size()

	case "save":
		path := params.IndexPath
		if path == "" {
			path = ".marcus/semantic/index.json"
		}
		if err := t.index.Save(path); err != nil {
			return nil, fmt.Errorf("save failed: %w", err)
		}
		output["message"] = "Index saved successfully"
		output["path"] = path

	case "load":
		path := params.IndexPath
		if path == "" {
			path = ".marcus/semantic/index.json"
		}
		if err := t.index.Load(path); err != nil {
			return nil, fmt.Errorf("load failed: %w", err)
		}
		output["message"] = "Index loaded successfully"
		output["indexed_files"] = t.index.Size()

	case "stats":
		output["indexed_files"] = t.index.Size()
		output["indexed_file_list"] = t.index.GetIndexedFiles()

	case "clear":
		t.index.Clear()
		output["message"] = "Index cleared successfully"

	default:
		return nil, fmt.Errorf("unknown action: %s", params.Action)
	}

	return json.Marshal(output)
}
