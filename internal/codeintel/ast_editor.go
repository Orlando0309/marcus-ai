package codeintel

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ASTEditor performs structural code edits using AST manipulation
type ASTEditor struct {
	baseDir string
}

// NewASTEditor creates a new AST editor
func NewASTEditor(baseDir string) *ASTEditor {
	return &ASTEditor{baseDir: baseDir}
}

// ASTEditOp represents a structural edit operation
type ASTEditOp struct {
	Path      string     // File path
	Target    ASTTarget  // What to edit
	Operation string     // Type of operation
	NewValue  string     // New value (for renames, etc.)
	OldValue  string     // Old value (for find/replace)
}

// ASTTarget identifies a code element
type ASTTarget struct {
	Type     string // "function", "type", "method", "import", "const", "var"
	Name     string
	Package  string
	Receiver string // For methods
}

// ASTEditResult is the result of an AST edit
type ASTEditResult struct {
	Success      bool
	Error        string
	Modified     bool
	OriginalCode string
	NewCode      string
}

// Supported AST operations
const (
	ASTRename       = "rename"
	ASTAddImport    = "add_import"
	ASTRemoveImport = "remove_import"
	ASTAddParam     = "add_param"
	ASTRemoveParam  = "remove_param"
	ASTWrap         = "wrap"
	ASTExtract      = "extract"
	ASTMove         = "move"
)

// Edit performs an AST-based edit on a Go file
func (e *ASTEditor) Edit(op ASTEditOp) ASTEditResult {
	result := ASTEditResult{Success: true}

	// Resolve path
	path := e.resolvePath(op.Path)

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("read file: %v", err)
		return result
	}
	result.OriginalCode = string(content)

	// Parse file
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("parse file: %v", err)
		return result
	}

	// Perform edit based on operation type
	modified := false
	switch op.Operation {
	case ASTRename:
		modified = e.performRename(f, op.Target, op.NewValue)
	case ASTAddImport:
		modified = e.performAddImport(f, op.NewValue)
	case ASTRemoveImport:
		modified = e.performRemoveImport(f, op.OldValue)
	case ASTAddParam:
		modified = e.performAddParam(f, op.Target, op.NewValue)
	case ASTRemoveParam:
		modified = e.performRemoveParam(f, op.Target, op.OldValue)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("unsupported operation: %s", op.Operation)
		return result
	}

	if !modified {
		result.Modified = false
		result.NewCode = result.OriginalCode
		return result
	}

	// Format and write back
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("format file: %v", err)
		return result
	}

	result.NewCode = buf.String()
	result.Modified = true

	// Write back
	if err := os.WriteFile(path, []byte(result.NewCode), 0644); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("write file: %v", err)
		return result
	}

	return result
}

// performRename renames a symbol
func (e *ASTEditor) performRename(f *ast.File, target ASTTarget, newName string) bool {
	modified := false

	switch target.Type {
	case "function":
		ast.Inspect(f, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Name.Name == target.Name {
					fn.Name.Name = newName
					modified = true
				}
			}
			return true
		})
	case "type":
		ast.Inspect(f, func(n ast.Node) bool {
			switch t := n.(type) {
			case *ast.TypeSpec:
				if t.Name.Name == target.Name {
					t.Name.Name = newName
					modified = true
				}
			}
			return true
		})
	case "method":
		ast.Inspect(f, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Recv != nil && fn.Name.Name == target.Name {
					// Check receiver type matches
					if e.receiverMatches(fn.Recv, target.Receiver) {
						fn.Name.Name = newName
						modified = true
					}
				}
			}
			return true
		})
	case "const", "var":
		ast.Inspect(f, func(n ast.Node) bool {
			if vs, ok := n.(*ast.ValueSpec); ok {
				for i, name := range vs.Names {
					if name.Name == target.Name {
						vs.Names[i].Name = newName
						modified = true
					}
				}
			}
			return true
		})
	}

	return modified
}

// receiverMatches checks if a receiver matches the given receiver type
func (e *ASTEditor) receiverMatches(recv *ast.FieldList, receiverType string) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	for _, field := range recv.List {
		switch t := field.Type.(type) {
		case *ast.Ident:
			if t.Name == receiverType {
				return true
			}
		case *ast.StarExpr:
			if ident, ok := t.X.(*ast.Ident); ok {
				if ident.Name == receiverType {
					return true
				}
			}
		}
	}
	return false
}

// performAddImport adds an import
func (e *ASTEditor) performAddImport(f *ast.File, importPath string) bool {
	// Check if import already exists
	for _, imp := range f.Imports {
		if imp.Path.Value == `"`+importPath+`"` {
			return false
		}
	}

	// Create new import
	newImport := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + importPath + `"`,
		},
	}

	// Add to imports
	f.Imports = append(f.Imports, newImport)

	// If there's no import declaration, create one
	if len(f.Decls) > 0 {
		// Find position to insert import
		for i, decl := range f.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
				genDecl.Specs = append(genDecl.Specs, newImport)
				return true
			}
			// Insert before first non-import declaration
			if _, ok := decl.(*ast.GenDecl); ok {
				importDecl := &ast.GenDecl{
					Tok:   token.IMPORT,
					Specs: []ast.Spec{newImport},
				}
				f.Decls = append(f.Decls[:i], append([]ast.Decl{importDecl}, f.Decls[i:]...)...)
				return true
			}
		}
	}

	// Add new import declaration
	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{newImport},
	}
	f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
	return true
}

// performRemoveImport removes an import
func (e *ASTEditor) performRemoveImport(f *ast.File, importPath string) bool {
	modified := false

	// Remove from f.Imports
	newImports := make([]*ast.ImportSpec, 0, len(f.Imports))
	for _, imp := range f.Imports {
		if imp.Path.Value != `"`+importPath+`"` {
			newImports = append(newImports, imp)
		} else {
			modified = true
		}
	}
	f.Imports = newImports

	// Remove from declarations
	for _, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			newSpecs := make([]ast.Spec, 0, len(genDecl.Specs))
			for _, spec := range genDecl.Specs {
				if imp, ok := spec.(*ast.ImportSpec); ok {
					if imp.Path.Value != `"`+importPath+`"` {
						newSpecs = append(newSpecs, spec)
					}
				} else {
					newSpecs = append(newSpecs, spec)
				}
			}
			genDecl.Specs = newSpecs
		}
	}

	return modified
}

// performAddParam adds a parameter to a function
func (e *ASTEditor) performAddParam(f *ast.File, target ASTTarget, param string) bool {
	modified := false

	// Parse param (e.g., "ctx context.Context")
	paramName, paramType := e.parseParam(param)
	if paramName == "" {
		return false
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Name.Name == target.Name {
				// Create new parameter
				newParam := &ast.Field{
					Names: []*ast.Ident{{Name: paramName}},
					Type:  e.parseType(paramType),
				}

				if fn.Type.Params == nil {
					fn.Type.Params = &ast.FieldList{}
				}
				fn.Type.Params.List = append(fn.Type.Params.List, newParam)
				modified = true
			}
		}
		return true
	})

	return modified
}

// performRemoveParam removes a parameter from a function
func (e *ASTEditor) performRemoveParam(f *ast.File, target ASTTarget, paramName string) bool {
	modified := false

	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Name.Name == target.Name {
				if fn.Type.Params != nil {
					newParams := make([]*ast.Field, 0)
					for _, field := range fn.Type.Params.List {
						keep := true
						for _, name := range field.Names {
							if name.Name == paramName {
								keep = false
								modified = true
								break
							}
						}
						if keep {
							newParams = append(newParams, field)
						}
					}
					fn.Type.Params.List = newParams
				}
			}
		}
		return true
	})

	return modified
}

// parseParam parses a parameter string like "ctx context.Context"
func (e *ASTEditor) parseParam(param string) (name, typ string) {
	parts := strings.Fields(param)
	if len(parts) >= 2 {
		return parts[0], strings.Join(parts[1:], " ")
	}
	return "", ""
}

// parseType parses a type string into an AST expression
func (e *ASTEditor) parseType(typeStr string) ast.Expr {
	// Handle pointer types
	if strings.HasPrefix(typeStr, "*") {
		return &ast.StarExpr{
			X: &ast.Ident{Name: strings.TrimPrefix(typeStr, "*")},
		}
	}
	// Simple identifier
	return &ast.Ident{Name: typeStr}
}

// resolvePath resolves a relative path to absolute
func (e *ASTEditor) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.baseDir, path)
}

// CanEdit checks if a file can be edited (Go files only for now)
func (e *ASTEditor) CanEdit(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".go"
}

// Preview shows a preview of the edit without applying it
func (e *ASTEditor) Preview(op ASTEditOp) (ASTEditResult, error) {
	result := ASTEditResult{Success: true}

	// Resolve path
	path := e.resolvePath(op.Path)

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read file: %w", err)
	}
	result.OriginalCode = string(content)

	// Parse file
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return result, fmt.Errorf("parse file: %w", err)
	}

	// Perform edit
	modified := false
	switch op.Operation {
	case ASTRename:
		modified = e.performRename(f, op.Target, op.NewValue)
	case ASTAddImport:
		modified = e.performAddImport(f, op.NewValue)
	case ASTRemoveImport:
		modified = e.performRemoveImport(f, op.OldValue)
	case ASTAddParam:
		modified = e.performAddParam(f, op.Target, op.NewValue)
	case ASTRemoveParam:
		modified = e.performRemoveParam(f, op.Target, op.OldValue)
	default:
		return result, fmt.Errorf("unsupported operation: %s", op.Operation)
	}

	if !modified {
		result.NewCode = result.OriginalCode
		result.Modified = false
		return result, nil
	}

	// Format
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return result, fmt.Errorf("format file: %w", err)
	}

	result.NewCode = buf.String()
	result.Modified = true
	return result, nil
}
