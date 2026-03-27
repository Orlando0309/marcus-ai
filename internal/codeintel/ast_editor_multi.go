package codeintel

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MultiLanguageASTEditor performs structural edits on multiple programming languages
type MultiLanguageASTEditor struct {
	baseDir string
}

// NewMultiLanguageASTEditor creates a new multi-language AST editor
func NewMultiLanguageASTEditor(baseDir string) *MultiLanguageASTEditor {
	return &MultiLanguageASTEditor{baseDir: baseDir}
}

// Language represents a supported programming language
type Language string

const (
	LangGo         Language = "go"
	LangPython     Language = "python"
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
)

// ASTOperation represents the type of AST operation
type ASTOperation string

const (
	OpRename       ASTOperation = "rename"
	OpAddImport    ASTOperation = "add_import"
	OpRemoveImport ASTOperation = "remove_import"
	OpAddParam     ASTOperation = "add_param"
	OpRemoveParam  ASTOperation = "remove_param"
	OpWrap         ASTOperation = "wrap"
	OpExtract      ASTOperation = "extract"
	OpAddMethod    ASTOperation = "add_method"
	OpRemoveMethod ASTOperation = "remove_method"
)

// MultiLanguageEditOp represents a cross-language edit operation
type MultiLanguageEditOp struct {
	Path      string       // File path
	Language  Language     // Target language
	Operation ASTOperation // Type of operation
	Target    ASTTarget    // What to edit
	NewValue  string       // New value
	OldValue  string       // Old value (for find/replace)
}

// MultiLanguageEditResult is the result of a multi-language edit
type MultiLanguageEditResult struct {
	Success      bool
	Error        string
	Modified     bool
	OriginalCode string
	NewCode      string
	Language     Language
}

// Edit performs an AST-based edit on a file
func (e *MultiLanguageASTEditor) Edit(op MultiLanguageEditOp) MultiLanguageEditResult {
	result := MultiLanguageEditResult{
		Success:  true,
		Language: op.Language,
	}

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

	// Detect language from file extension if not specified
	lang := op.Language
	if lang == "" {
		lang = e.detectLanguage(path)
	}

	// Perform edit based on language
	var modified bool
	var newCode string

	switch lang {
	case LangGo:
		return e.editGoFile(op, result)
	case LangPython:
		modified, newCode = e.editPythonFile(op, result)
	case LangTypeScript, LangJavaScript:
		modified, newCode = e.editTypeScriptFile(op, result)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("unsupported language: %s", lang)
		return result
	}

	result.Modified = modified
	result.NewCode = newCode

	if modified {
		// Write back
		if err := os.WriteFile(path, []byte(newCode), 0644); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("write file: %v", err)
			return result
		}
	}

	return result
}

// detectLanguage detects the language from file extension
func (e *MultiLanguageASTEditor) detectLanguage(path string) Language {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return LangGo
	case ".py":
		return LangPython
	case ".ts", ".tsx":
		return LangTypeScript
	case ".js", ".jsx":
		return LangJavaScript
	default:
		return ""
	}
}

// editGoFile performs AST edits on Go files
func (e *MultiLanguageASTEditor) editGoFile(op MultiLanguageEditOp, result MultiLanguageEditResult) MultiLanguageEditResult {
	// Reuse existing Go AST editor logic
	goEditor := &ASTEditor{baseDir: e.baseDir}
	goOp := ASTEditOp{
		Path:      op.Path,
		Target:    op.Target,
		Operation: string(op.Operation),
		NewValue:  op.NewValue,
		OldValue:  op.OldValue,
	}

	goResult := goEditor.Edit(goOp)
	result.Success = goResult.Success
	result.Error = goResult.Error
	result.Modified = goResult.Modified
	result.NewCode = goResult.NewCode
	return result
}

// editPythonFile performs edits on Python files
func (e *MultiLanguageASTEditor) editPythonFile(op MultiLanguageEditOp, result MultiLanguageEditResult) (bool, string) {
	switch op.Operation {
	case OpRename:
		return e.pythonRename(result.OriginalCode, op.Target.Name, op.NewValue)
	case OpAddImport:
		return e.pythonAddImport(result.OriginalCode, op.NewValue)
	case OpRemoveImport:
		return e.pythonRemoveImport(result.OriginalCode, op.OldValue)
	case OpAddMethod:
		return e.pythonAddMethod(result.OriginalCode, op.Target.Name, op.NewValue)
	default:
		return false, result.OriginalCode
	}
}

// editTypeScriptFile performs edits on TypeScript files
func (e *MultiLanguageASTEditor) editTypeScriptFile(op MultiLanguageEditOp, result MultiLanguageEditResult) (bool, string) {
	switch op.Operation {
	case OpRename:
		return e.typescriptRename(result.OriginalCode, op.Target.Name, op.NewValue)
	case OpAddImport:
		return e.typescriptAddImport(result.OriginalCode, op.NewValue)
	case OpRemoveImport:
		return e.typescriptRemoveImport(result.OriginalCode, op.OldValue)
	case OpAddMethod:
		return e.typescriptAddMethod(result.OriginalCode, op.Target.Name, op.NewValue, op.Target.Receiver)
	default:
		return false, result.OriginalCode
	}
}

// pythonRename renames a symbol in Python code using word boundaries
func (e *MultiLanguageASTEditor) pythonRename(code string, oldName, newName string) (bool, string) {
	// Use regex with word boundaries to rename
	pattern := `\b` + regexp.QuoteMeta(oldName) + `\b`
	re := regexp.MustCompile(pattern)

	newCode := re.ReplaceAllString(code, newName)
	return newCode != code, newCode
}

// pythonAddImport adds an import to Python code
func (e *MultiLanguageASTEditor) pythonAddImport(code string, importPath string) (bool, string) {
	// Check if import already exists
	if strings.Contains(code, "import "+importPath) || strings.Contains(code, "from "+importPath) {
		return false, code
	}

	// Find the best place to insert the import
	lines := strings.Split(code, "\n")
	insertIdx := 0
	inDocstring := false

	// Skip docstring if present
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && (strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, "'''")) {
			inDocstring = true
			continue
		}
		if inDocstring {
			if strings.Contains(trimmed, `"""`) || strings.Contains(trimmed, "'''") {
				inDocstring = false
				insertIdx = i + 1
			}
			continue
		}
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
			insertIdx = i + 1
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") && insertIdx > 0 {
			break
		}
	}

	// Insert the new import
	newImport := fmt.Sprintf("import %s", importPath)
	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, newImport)
	newLines = append(newLines, lines[insertIdx:]...)

	return true, strings.Join(newLines, "\n")
}

// pythonRemoveImport removes an import from Python code
func (e *MultiLanguageASTEditor) pythonRemoveImport(code string, importPath string) (bool, string) {
	lines := strings.Split(code, "\n")
	var newLines []string
	modified := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import "+importPath) ||
			strings.HasPrefix(trimmed, "from "+importPath) {
			modified = true
			continue
		}
		newLines = append(newLines, line)
	}

	return modified, strings.Join(newLines, "\n")
}

// pythonAddMethod adds a method to a Python class
func (e *MultiLanguageASTEditor) pythonAddMethod(code string, methodName, methodBody string) (bool, string) {
	// Find the class definition
	lines := strings.Split(code, "\n")
	insertIdx := -1

	for i, line := range lines {
		if strings.HasPrefix(line, "class ") && strings.Contains(line, ":") {
			// Find the end of the class header (look for first indented line)
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
					// Found first content, insert before it
					insertIdx = j
					break
				}
			}
			if insertIdx == -1 {
				insertIdx = i + 1
			}
			break
		}
	}

	if insertIdx == -1 {
		return false, code
	}

	// Insert the new method
	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, "", "    def "+methodBody)
	newLines = append(newLines, lines[insertIdx:]...)

	return true, strings.Join(newLines, "\n")
}

// typescriptRename renames a symbol in TypeScript code
func (e *MultiLanguageASTEditor) typescriptRename(code string, oldName, newName string) (bool, string) {
	// Use regex with word boundaries to rename
	pattern := `\b` + regexp.QuoteMeta(oldName) + `\b`
	re := regexp.MustCompile(pattern)

	newCode := re.ReplaceAllString(code, newName)
	return newCode != code, newCode
}

// typescriptAddImport adds an import to TypeScript code
func (e *MultiLanguageASTEditor) typescriptAddImport(code string, importPath string) (bool, string) {
	// Check if import already exists
	if strings.Contains(code, "from \""+importPath+"\"") ||
		strings.Contains(code, "from '"+importPath+"'") {
		return false, code
	}

	// Find existing imports or beginning of file
	lines := strings.Split(code, "\n")
	insertIdx := 0

	// Look for existing imports
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			insertIdx = i + 1
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "//") && insertIdx > 0 {
			break
		}
	}

	// Insert the new import (default import style)
	packageName := extractPackageName(importPath)
	newImport := fmt.Sprintf("import * as %s from \"%s\";", packageName, importPath)
	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, newImport)
	newLines = append(newLines, lines[insertIdx:]...)

	return true, strings.Join(newLines, "\n")
}

// typescriptRemoveImport removes an import from TypeScript code
func (e *MultiLanguageASTEditor) typescriptRemoveImport(code string, importPath string) (bool, string) {
	lines := strings.Split(code, "\n")
	var newLines []string
	modified := false

	for _, line := range lines {
		if strings.Contains(line, "from \""+importPath+"\"") ||
			strings.Contains(line, "from '"+importPath+"'") {
			modified = true
			continue
		}
		newLines = append(newLines, line)
	}

	return modified, strings.Join(newLines, "\n")
}

// typescriptAddMethod adds a method to a TypeScript class
func (e *MultiLanguageASTEditor) typescriptAddMethod(code string, methodName, methodBody, receiverType string) (bool, string) {
	// Find the class or interface definition
	lines := strings.Split(code, "\n")
	insertIdx := -1
	braceCount := 0
	inClass := false

	for i, line := range lines {
		if strings.Contains(line, "class ") || strings.Contains(line, "interface ") {
			inClass = true
		}
		if inClass {
			for _, char := range line {
				if char == '{' {
					braceCount++
				} else if char == '}' {
					braceCount--
				}
			}
			// Found closing brace of class
			if braceCount == 0 && strings.Contains(line, "}") {
				insertIdx = i
				break
			}
		}
	}

	if insertIdx == -1 {
		return false, code
	}

	// Insert the new method before the closing brace
	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, "    "+methodBody)
	newLines = append(newLines, lines[insertIdx:]...)

	return true, strings.Join(newLines, "\n")
}

// Preview shows a preview of the edit without applying it
func (e *MultiLanguageASTEditor) Preview(op MultiLanguageEditOp) (MultiLanguageEditResult, error) {
	result := MultiLanguageEditResult{
		Language: op.Language,
	}

	// Resolve path
	path := e.resolvePath(op.Path)

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read file: %w", err)
	}
	result.OriginalCode = string(content)

	// Detect language
	lang := op.Language
	if lang == "" {
		lang = e.detectLanguage(path)
	}

	// Perform edit (but don't write)
	var modified bool
	var newCode string

	switch lang {
	case LangGo:
		goEditor := &ASTEditor{baseDir: e.baseDir}
		goOp := ASTEditOp{
			Path:      op.Path,
			Target:    op.Target,
			Operation: string(op.Operation),
			NewValue:  op.NewValue,
			OldValue:  op.OldValue,
		}
		goResult, err := goEditor.Preview(goOp)
		if err != nil {
			return result, err
		}
		result.Success = goResult.Success
		result.Error = goResult.Error
		result.Modified = goResult.Modified
		result.NewCode = goResult.NewCode
		return result, nil
	case LangPython:
		modified, newCode = e.editPythonFile(op, result)
	case LangTypeScript, LangJavaScript:
		modified, newCode = e.editTypeScriptFile(op, result)
	default:
		return result, fmt.Errorf("unsupported language: %s", lang)
	}

	result.Modified = modified
	result.NewCode = newCode
	return result, nil
}

// SupportedLanguages returns the list of supported languages
func (e *MultiLanguageASTEditor) SupportedLanguages() []Language {
	return []Language{LangGo, LangPython, LangTypeScript, LangJavaScript}
}

// CanEdit checks if a file can be edited
func (e *MultiLanguageASTEditor) CanEdit(path string) bool {
	lang := e.detectLanguage(path)
	return lang != ""
}

// GetLanguage gets the language for a file path
func (e *MultiLanguageASTEditor) GetLanguage(path string) Language {
	return e.detectLanguage(path)
}

// resolvePath resolves a relative path to absolute
func (e *MultiLanguageASTEditor) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.baseDir, path)
}

// extractPackageName extracts a package name from an import path
func extractPackageName(importPath string) string {
	parts := strings.Split(importPath, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".ts")
	name = strings.TrimSuffix(name, ".tsx")
	name = strings.TrimSuffix(name, ".js")
	return sanitizeIdentifier(name)
}

// sanitizeIdentifier sanitizes a string to be a valid identifier
func sanitizeIdentifier(s string) string {
	var buf bytes.Buffer
	for i, r := range s {
		if i == 0 && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
			buf.WriteRune('_')
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			buf.WriteRune(r)
		}
	}
	result := buf.String()
	if result == "" {
		return "_default"
	}
	return result
}
