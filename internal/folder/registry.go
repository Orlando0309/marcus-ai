package folder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Registry holds all discovered Marcus units
type Registry struct {
	Flows    map[string]*FlowDef
	Tools    map[string]*ToolDef
	Agents   map[string]*AgentDef
	Memories map[string]*MemoryDef
	AllNames []string
	ByTag    map[string][]string
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		Flows:    make(map[string]*FlowDef),
		Tools:    make(map[string]*ToolDef),
		Agents:   make(map[string]*AgentDef),
		Memories: make(map[string]*MemoryDef),
		ByTag:    make(map[string][]string),
	}
}

// Replace replaces a unit in the registry
func (r *Registry) Replace(unit interface{}) {
	switch u := unit.(type) {
	case *FlowDef:
		r.Flows[u.Name] = u
	case *ToolDef:
		r.Tools[u.Name] = u
	case *AgentDef:
		r.Agents[u.Name] = u
	}
}

// FlowDef defines a flow loaded from flow.toml
type FlowDef struct {
	Name        string         `toml:"-"`
	Path        string         `toml:"-"`
	Flow        FlowMetadata   `toml:"flow"`
	Model       ModelConfig    `toml:"model"`
	Input       InputConfig    `toml:"input"`
	Output      OutputConfig   `toml:"output"`
	Behavior    BehaviorConfig `toml:"behavior"`
	Tools       []string       `toml:"tools"`
	Description string         `toml:"description"`
	Version     string         `toml:"version"`
	Author      string         `toml:"author"`
	Tags        []string       `toml:"tags"`
	ContextPath string         `toml:"-"`
}

type FlowMetadata struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Version     string `toml:"version"`
	Author      string `toml:"author"`
	Extends     string `toml:"extends"`
}

type ModelConfig struct {
	Provider    string  `toml:"provider"`
	Model       string  `toml:"model"`
	Temperature float64 `toml:"temperature"`
	MaxTokens   int     `toml:"max_tokens"`
}

type InputConfig struct {
	Requires []string `toml:"requires"`
	Optional []string `toml:"optional"`
}

type OutputConfig struct {
	Format string `toml:"format"`
	SaveTo string `toml:"save_to"`
	Memory bool   `toml:"save_to_memory"`
}

type BehaviorConfig struct {
	Stream             bool `toml:"stream"`
	ConfirmBeforeApply bool `toml:"confirm_before_apply"`
	AutoFix            bool `toml:"auto_fix"`
}

// ToolDef defines a tool loaded from tool.toml
type ToolDef struct {
	Name        string          `toml:"-"`
	Path        string          `toml:"-"`
	Type        string          `toml:"type"` // "shell" or "native"
	Tool        ToolMetadata    `toml:"tool"`
	Schema      json.RawMessage `toml:"schema"`
	Description string          `toml:"description"`
	Timeout     int             `toml:"timeout"`
	Permissions []string        `toml:"permissions"`
}

type ToolMetadata struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Type        string `toml:"type"`
}

// AgentDef defines an agent loaded from agent.toml
type AgentDef struct {
	Name        string           `toml:"-"`
	Path        string           `toml:"-"`
	Agent       AgentMetadata    `toml:"agent"`
	Role        string           `toml:"role"`
	Goals       []string         `toml:"goals"`
	Tools       []string         `toml:"tools"`
	Flows       []string         `toml:"flows"`
	Description string           `toml:"description"`
	Autonomy    AutonomyConfig   `toml:"autonomy"`
	Rules       RulesConfig      `toml:"rules"`
}

type AgentMetadata struct {
	Name        string `toml:"name"`
	Role        string `toml:"role"`
	Description string `toml:"description"`
}

// AutonomyConfig controls the agent's autonomous behavior.
type AutonomyConfig struct {
	IterationLimit int      `toml:"iteration_limit"`
	Verification   string   `toml:"verification"`    // "detect", "always", "never", or specific command
	ConfirmWrites bool     `toml:"confirm_writes"`   // if true, stop before writes
	SystemPrompt  string   `toml:"system_prompt"`    // optional path to system.md
}

// RulesConfig controls which actions are safe, auto-approved, or require confirmation.
type RulesConfig struct {
	SafeActions      []string `toml:"safe_actions"`        // always auto-run
	AutoRunCommands []string `toml:"auto_run_commands"`   // run_command patterns that are auto-approved
	WriteIf         string   `toml:"write_if"`            // "always", "first_wave", "never"
	StepMode        bool     `toml:"step_mode"`           // default step mode setting
}

// MemoryDef defines a memory layer
type MemoryDef struct {
	Name        string `toml:"-"`
	Path        string `toml:"-"`
	Type        string `toml:"type"` // "facts", "episodic", "patterns"
	Description string `toml:"description"`
}

// validateFlow validates a flow folder
func validateFlow(path string) error {
	// Required: flow.toml
	if _, err := os.Stat(filepath.Join(path, "flow.toml")); os.IsNotExist(err) {
		return fmt.Errorf("flow.toml is required")
	}

	if _, err := os.Stat(filepath.Join(path, "prompt.md")); os.IsNotExist(err) {
		return fmt.Errorf("prompt.md is required")
	}

	if info, err := os.Stat(filepath.Join(path, "flow.toml")); err == nil && info.IsDir() {
		return fmt.Errorf("flow.toml must be a file")
	}

	return nil
}

// HasPrompt checks if a flow has a specific prompt file
func (f *FlowDef) HasPrompt(name string) bool {
	promptPath := filepath.Join(f.Path, name+".md")
	_, err := os.Stat(promptPath)
	return err == nil
}

// GetPromptPath returns the path to a prompt file
func (f *FlowDef) GetPromptPath(name string) string {
	if name == "" {
		// Default to prompt.md
		return filepath.Join(f.Path, "prompt.md")
	}
	return filepath.Join(f.Path, name+".md")
}

// ReadPrompt reads a prompt file
func (f *FlowDef) ReadPrompt(name string) (string, error) {
	path := f.GetPromptPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	prompt := string(data)
	if f.ContextPath != "" {
		if contextData, err := os.ReadFile(f.ContextPath); err == nil && len(contextData) > 0 {
			prompt += "\n\n## Flow Context\n" + string(contextData)
		}
	}
	return prompt, nil
}

// HasTool checks if a flow uses a specific tool
func (f *FlowDef) HasTool(name string) bool {
	for _, tool := range f.Tools {
		if tool == name {
			return true
		}
	}
	return false
}

// TagsString returns tags as comma-separated string
func (f *FlowDef) TagsString() string {
	return strings.Join(f.Tags, ",")
}

// ReadSystemPrompt reads the agent's system.md file if present.
func (a *AgentDef) ReadSystemPrompt() (string, error) {
	if a.Path == "" {
		return "", nil
	}
	path := filepath.Join(a.Path, "system.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadRules reads the agent's rules.md file if present.
func (a *AgentDef) ReadRules() (string, error) {
	if a.Path == "" {
		return "", nil
	}
	path := filepath.Join(a.Path, "rules.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// validateAgent validates an agent folder.
func validateAgent(path string) error {
	if _, err := os.Stat(filepath.Join(path, "agent.toml")); os.IsNotExist(err) {
		return fmt.Errorf("agent.toml is required")
	}
	return nil
}
