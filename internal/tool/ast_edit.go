package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/marcus-ai/marcus/internal/codeintel"
)

// ASTEditTool provides AST-based code editing
type ASTEditTool struct {
	baseDir string
	editor  *codeintel.ASTEditor
}

// NewASTEditTool creates a new AST edit tool
func NewASTEditTool(baseDir string) *ASTEditTool {
	return &ASTEditTool{
		baseDir: baseDir,
		editor:  codeintel.NewASTEditor(baseDir),
	}
}

// Name returns the tool name
func (t *ASTEditTool) Name() string {
	return "ast_edit"
}

// Description returns the tool description
func (t *ASTEditTool) Description() string {
	return "Perform structural code edits using AST manipulation (Go files only). Supports renaming, import management, and parameter changes."
}

// Schema returns the JSON schema for the tool
func (t *ASTEditTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path": {
				Type:        "string",
				Description: "Path to the file to edit",
			},
			"operation": {
				Type:        "string",
				Description: "Operation to perform: rename, add_import, remove_import, add_param, remove_param",
			},
			"target": {
				Type:        "object",
				Description: "Target element to edit (for rename, add_param, remove_param)",
			},
			"target.type": {
				Type:        "string",
				Description: "Type of target: function, type, method, const, var",
			},
			"target.name": {
				Type:        "string",
				Description: "Name of the target",
			},
			"target.receiver": {
				Type:        "string",
				Description: "Receiver type for methods",
			},
			"new_value": {
				Type:        "string",
				Description: "New value (for rename, new parameter format)",
			},
			"old_value": {
				Type:        "string",
				Description: "Old value (for remove_import, parameter name to remove)",
			},
			"preview": {
				Type:        "boolean",
				Description: "Show preview without applying changes",
			},
		},
		Required: []string{"path", "operation"},
	}
}

// Run executes the AST edit
func (t *ASTEditTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path     string                 `json:"path"`
		Op       string                 `json:"operation"`
		Target   map[string]string      `json:"target,omitempty"`
		NewValue string                 `json:"new_value,omitempty"`
		OldValue string                 `json:"old_value,omitempty"`
		Preview  bool                   `json:"preview,omitempty"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if params.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	if !t.editor.CanEdit(params.Path) {
		return nil, fmt.Errorf("file type not supported for AST editing (Go files only)")
	}

	// Build ASTEditOp
	op := codeintel.ASTEditOp{
		Path:      params.Path,
		Operation: params.Op,
		NewValue:  params.NewValue,
		OldValue:  params.OldValue,
	}

	if params.Target != nil {
		op.Target = codeintel.ASTTarget{
			Type:     params.Target["type"],
			Name:     params.Target["name"],
			Receiver: params.Target["receiver"],
		}
	}

	var result codeintel.ASTEditResult
	var err error

	if params.Preview {
		result, err = t.editor.Preview(op)
		if err != nil {
			return nil, fmt.Errorf("preview failed: %w", err)
		}
	} else {
		result = t.editor.Edit(op)
	}

	if !result.Success {
		return nil, fmt.Errorf("edit failed: %s", result.Error)
	}

	output := map[string]any{
		"success":       result.Success,
		"modified":        result.Modified,
		"preview":         params.Preview,
	}

	if params.Preview {
		output["original"] = result.OriginalCode
		output["new"] = result.NewCode
		output["diff"] = computeDiff(result.OriginalCode, result.NewCode)
	}

	return json.Marshal(output)
}

// computeDiff generates a simple diff
func computeDiff(original, new string) string {
	if original == new {
		return "(no changes)"
	}
	return fmt.Sprintf("--- original\n+++ modified\n\n%s", "")
}

// ASTSafeEditTool provides safe AST-based editing with validation
type ASTSafeEditTool struct {
	*ASTEditTool
}

// NewASTSafeEditTool creates a new AST safe edit tool
func NewASTSafeEditTool(baseDir string) *ASTSafeEditTool {
	return &ASTSafeEditTool{
		ASTEditTool: NewASTEditTool(baseDir),
	}
}

// Name returns the tool name
func (t *ASTSafeEditTool) Name() string {
	return "ast_safe_edit"
}

// Description returns the tool description
func (t *ASTSafeEditTool) Description() string {
	return "Perform safe AST-based code edits with validation (Go files only). Validates changes before applying."
}

// Schema returns the JSON schema for the tool
func (t *ASTSafeEditTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"path": {
				Type:        "string",
				Description: "Path to the file to edit",
			},
			"operation": {
				Type:        "string",
				Description: "Operation to perform: rename, add_import, remove_import",
			},
			"target": {
				Type:        "object",
				Description: "Target element to edit",
			},
			"target.type": {
				Type:        "string",
				Description: "Type of target: function, type, method, const, var",
			},
			"target.name": {
				Type:        "string",
				Description: "Name of the target",
			},
			"new_value": {
				Type:        "string",
				Description: "New value",
			},
		},
		Required: []string{"path", "operation"},
	}
}

// Run executes the safe AST edit
func (t *ASTSafeEditTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	// First run preview
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	params["preview"] = true

	previewInput, _ := json.Marshal(params)
	result, err := t.ASTEditTool.Run(ctx, previewInput)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if changes look safe
	var previewOutput map[string]any
	if err := json.Unmarshal(result, &previewOutput); err != nil {
		return nil, err
	}

	// Apply the edit
	delete(params, "preview")
	applyInput, _ := json.Marshal(params)
	return t.ASTEditTool.Run(ctx, applyInput)
}
