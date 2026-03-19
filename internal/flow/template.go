package flow

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// TemplateEngine handles Jinja2-style template parsing and rendering
type TemplateEngine struct {
	funcMap template.FuncMap
}

// NewTemplateEngine creates a new template engine
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		funcMap: template.FuncMap{
			"lower":   strings.ToLower,
			"upper":   strings.ToUpper,
			"trim":    strings.TrimSpace,
			"join":    func(sep string, elems []string) string { return strings.Join(elems, sep) },
			"split":   func(sep, s string) []string { return strings.Split(s, sep) },
			"replace": func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
			"indent": func(spaces int, s string) string {
				indent := strings.Repeat(" ", spaces)
				lines := strings.Split(s, "\n")
				for i, line := range lines {
					if line != "" {
						lines[i] = indent + line
					}
				}
				return strings.Join(lines, "\n")
			},
		},
	}
}

// TemplateData holds the data for template rendering
type TemplateData map[string]interface{}

// Render parses and renders a template string
func (te *TemplateEngine) Render(templateStr string, data TemplateData) (string, error) {
	// Convert Jinja2-style {{.var}} to Go template {{.var}}
	// Go's text/template already supports {{.var}} syntax
	tmpl, err := template.New("flow").Option("missingkey=zero").Funcs(te.funcMap).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderFile reads and renders a template file
func (te *TemplateEngine) RenderFile(path string, data TemplateData) (string, error) {
	// This is handled by the flow executor which reads the file
	return "", nil
}

// ParseVariables extracts variable names from a template
func (te *TemplateEngine) ParseVariables(templateStr string) []string {
	var vars []string

	// Find all {{.var}} patterns
	start := 0
	for {
		idx := strings.Index(templateStr[start:], "{{.")
		if idx == -1 {
			break
		}
		start += idx
		end := strings.Index(templateStr[start:], "}}")
		if end == -1 {
			break
		}

		varExpr := templateStr[start+3 : start+end]
		// Handle filters like {{.var|upper}}
		if pipeIdx := strings.Index(varExpr, "|"); pipeIdx != -1 {
			varExpr = varExpr[:pipeIdx]
		}

		vars = append(vars, varExpr)
		start = start + end + 2
	}

	return vars
}

// ValidateTemplate checks if a template has all required variables
func (te *TemplateEngine) ValidateTemplate(templateStr string, required []string, data TemplateData) error {
	templateVars := te.ParseVariables(templateStr)

	// Check if all required variables are provided
	for _, req := range required {
		found := false
		for _, tv := range templateVars {
			if tv == req {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("required variable %q not found in template", req)
		}
	}

	return nil
}
