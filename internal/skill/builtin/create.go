package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/marcus-ai/marcus/internal/skill"
)

// CreateSkill creates new skills from templates
type CreateSkill struct{}

func (c *CreateSkill) Name() string { return "create" }

func (c *CreateSkill) Pattern() string { return "/create" }

func (c *CreateSkill) Description() string {
	return "Create a new skill from a template (e.g., /create skill myskill)"
}

func (c *CreateSkill) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return c.showHelp(), nil
	}

	subcommand := args[0]
	switch subcommand {
	case "skill":
		return c.createSkill(args[1:], deps)
	case "flow":
		return c.createFlow(args[1:], deps)
	case "agent":
		return c.createAgent(args[1:], deps)
	case "tool":
		return c.createTool(args[1:], deps)
	default:
		return skill.Result{
			Message: fmt.Sprintf("Unknown create type: %s\nUse: skill, flow, agent, or tool", subcommand),
			Done:    true,
		}, nil
	}
}

func (c *CreateSkill) showHelp() skill.Result {
	var lines []string
	lines = append(lines, "Create new Marcus components from templates")
	lines = append(lines, "")
	lines = append(lines, "Usage:")
	lines = append(lines, "  /create skill <name>     - Create a new skill (YAML + Go handler)")
	lines = append(lines, "  /create flow <name>      - Create a new flow")
	lines = append(lines, "  /create agent <name>     - Create a new agent")
	lines = append(lines, "  /create tool <name>      - Create a new tool")
	lines = append(lines, "")
	lines = append(lines, "Examples:")
	lines = append(lines, "  /create skill weather    - Creates .marcus/skills/weather.yaml + .go")
	lines = append(lines, "  /create flow deploy      - Creates .marcus/flows/deploy/flow.toml")
	lines = append(lines, "  /create agent reviewer   - Creates .marcus/agents/reviewer/")
	lines = append(lines, "  /create tool fetch       - Creates .marcus/tools/fetch.json")
	lines = append(lines, "")
	lines = append(lines, "Skill Architecture (Hybrid):")
	lines = append(lines, "  - YAML defines metadata: name, pattern, description, handler")
	lines = append(lines, "  - Go file implements the logic (or use 'run: shell command')")
	lines = append(lines, "  - Skills auto-discovered from .marcus/skills/*.yaml")

	return skill.Result{
		Message: strings.Join(lines, "\n"),
		Done:    true,
	}
}

func (c *CreateSkill) createSkill(args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return skill.Result{
			Message: "Skill name required. Usage: /create skill <name> [--shell]",
			Done:    true,
		}, nil
	}

	// Check for --shell flag
	isShell := false
	for _, arg := range args[1:] {
		if arg == "--shell" || arg == "-s" {
			isShell = true
			break
		}
	}

	name := args[0]
	skillName := sanitizeName(name)
	pattern := "/" + strings.ToLower(skillName)

	// Determine where to create the skill
	skillDir := filepath.Join(deps.ProjectRoot, ".marcus", "skills")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create skills directory: %v", err),
			Done:    true,
		}, nil
	}

	yamlPath := filepath.Join(skillDir, skillName+".yaml")
	goPath := filepath.Join(skillDir, skillName+".go")

	// Check if files exist
	if _, err := os.Stat(yamlPath); err == nil {
		return skill.Result{
			Message: fmt.Sprintf("Skill already exists: %s", yamlPath),
			Done:    true,
		}, nil
	}

	// If shell skill, just create YAML with run command
	if isShell {
		return c.createShellSkill(skillName, pattern, yamlPath, deps)
	}

	// Generate YAML definition (metadata)
	yamlContent := fmt.Sprintf(`name: %s
pattern: %s
description: Custom skill: %s
args: []
handler: skills/%s.go
timeout: 30s
`, skillName, pattern, skillName, skillName)

	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write skill YAML: %v", err),
			Done:    true,
		}, nil
	}

	// Generate Go handler
	structName := capitalize(skillName) + "Skill"
	tmpl := `package skills

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/marcus-ai/marcus/internal/skill"
)

// {{.StructName}} is a custom skill
type {{.StructName}} struct{}

func (s *{{.StructName}}) Run(ctx context.Context, args []string, deps skill.Dependencies) (skill.Result, error) {
	// TODO: Implement your skill logic here
	// Args from command: /{{.SkillName}} arg1 arg2

	// Example: read from environment
	skillArgs := os.Getenv("MARCUS_SKILL_ARGS")
	_ = skillArgs // comma-separated args

	return skill.Result{
		Message: "{{.SkillName}} skill executed successfully!",
		Done:    true,
	}, nil
}
`

	data := map[string]string{
		"StructName": structName,
		"SkillName":  skillName,
	}

	t := template.Must(template.New("skill").Parse(tmpl))
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to generate skill template: %v", err),
			Done:    true,
		}, nil
	}

	// Write the Go file
	if err := os.WriteFile(goPath, []byte(buf.String()), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write skill file: %v", err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Created skill: %s\n\nYAML definition: %s\nGo handler: %s\nPattern: %s\n\nThe skill is automatically discovered.\nJust restart Marcus to use it.", skillName, yamlPath, goPath, pattern),
		Done:    true,
	}, nil
}

func (c *CreateSkill) createShellSkill(skillName, pattern, yamlPath string, deps skill.Dependencies) (skill.Result, error) {
	// Generate YAML with run command
	yamlContent := fmt.Sprintf(`name: %s
pattern: %s
description: Shell skill: %s
args: []
run: "echo 'Implement your command here'"
timeout: 30s
`, skillName, pattern, skillName)

	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write skill YAML: %v", err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Created shell skill: %s\n\nYAML definition: %s\nPattern: %s\n\nEdit the 'run:' field to implement your command.\nUse ${args[0]}, ${args[1]} for arguments.\n\nThe skill is automatically discovered.", skillName, yamlPath, pattern),
		Done:    true,
	}, nil
}

func (c *CreateSkill) createFlow(args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return skill.Result{
			Message: "Flow name required. Usage: /create flow <name>",
			Done:    true,
		}, nil
	}

	name := args[0]
	flowName := sanitizeName(name)

	// Create flow directory
	flowDir := filepath.Join(deps.ProjectRoot, ".marcus", "flows", flowName)
	if err := os.MkdirAll(flowDir, 0755); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create flow directory: %v", err),
			Done:    true,
		}, nil
	}

	// Check if already exists
	flowToml := filepath.Join(flowDir, "flow.toml")
	if _, err := os.Stat(flowToml); err == nil {
		return skill.Result{
			Message: fmt.Sprintf("Flow already exists: %s", flowDir),
			Done:    true,
		}, nil
	}

	// Create flow.toml
	tomlContent := fmt.Sprintf(`name = "%s"
description = "Generated flow: %s"
version = "1.0.0"

triggers = ["manual"]

[[steps]]
name = "step1"
type = "prompt"
prompt = "Hello from %s flow!"
`, flowName, flowName, flowName)

	if err := os.WriteFile(flowToml, []byte(tomlContent), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write flow.toml: %v", err),
			Done:    true,
		}, nil
	}

	// Create README
	readmePath := filepath.Join(flowDir, "README.md")
	readmeContent := fmt.Sprintf("# %s Flow\n\nGenerated on %s\n\n## Description\n\nDescribe your flow here.\n\n## Usage\n\n```bash\nmarcus flow run %s\n```\n", capitalize(flowName), time.Now().Format("2006-01-02"), flowName)

	os.WriteFile(readmePath, []byte(readmeContent), 0644)

	return skill.Result{
		Message: fmt.Sprintf("Created flow: %s\nPath: %s\n\nTo run:\n  marcus flow run %s", flowName, flowDir, flowName),
		Done:    true,
	}, nil
}

func (c *CreateSkill) createAgent(args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return skill.Result{
			Message: "Agent name required. Usage: /create agent <name>",
			Done:    true,
		}, nil
	}

	name := args[0]
	agentName := sanitizeName(name)

	// Create agent directory
	agentDir := filepath.Join(deps.ProjectRoot, ".marcus", "agents", agentName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create agent directory: %v", err),
			Done:    true,
		}, nil
	}

	// Create agent.toml
	agentToml := filepath.Join(agentDir, "agent.toml")
	if _, err := os.Stat(agentToml); err == nil {
		return skill.Result{
			Message: fmt.Sprintf("Agent already exists: %s", agentDir),
			Done:    true,
		}, nil
	}

	tomlContent := fmt.Sprintf(`name = "%s"
description = "Generated agent: %s"
system_prompt = """
You are %s, a helpful AI assistant.
Your role is to assist with tasks efficiently and accurately.
"""

[capabilities]
file_access = true
web_search = false
code_execution = false
`, agentName, agentName, capitalize(agentName))

	if err := os.WriteFile(agentToml, []byte(tomlContent), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write agent.toml: %v", err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Created agent: %s\nPath: %s\n\nEdit agent.toml to customize behavior.", agentName, agentDir),
		Done:    true,
	}, nil
}

func (c *CreateSkill) createTool(args []string, deps skill.Dependencies) (skill.Result, error) {
	if len(args) == 0 {
		return skill.Result{
			Message: "Tool name required. Usage: /create tool <name>",
			Done:    true,
		}, nil
	}

	name := args[0]
	toolName := sanitizeName(name)

	// Create tools directory
	toolsDir := filepath.Join(deps.ProjectRoot, ".marcus", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to create tools directory: %v", err),
			Done:    true,
		}, nil
	}

	// Create tool definition
	toolPath := filepath.Join(toolsDir, toolName+".json")
	if _, err := os.Stat(toolPath); err == nil {
		return skill.Result{
			Message: fmt.Sprintf("Tool already exists: %s", toolPath),
			Done:    true,
		}, nil
	}

	jsonContent := fmt.Sprintf(`{
  "name": "%s",
  "description": "Generated tool: %s",
  "parameters": {
    "type": "object",
    "properties": {
      "input": {
        "type": "string",
        "description": "Input parameter"
      }
    },
    "required": ["input"]
  },
  "run": "echo 'Implement tool logic in .marcus/tools/%s.sh'"
}
`, toolName, toolName, toolName)

	if err := os.WriteFile(toolPath, []byte(jsonContent), 0644); err != nil {
		return skill.Result{
			Message: fmt.Sprintf("Failed to write tool JSON: %v", err),
			Done:    true,
		}, nil
	}

	return skill.Result{
		Message: fmt.Sprintf("Created tool: %s\nPath: %s\n\nEdit the JSON to define parameters and implementation.", toolName, toolPath),
		Done:    true,
	}, nil
}

// sanitizeName removes invalid characters from names
func sanitizeName(name string) string {
	// Remove non-alphanumeric characters except underscore
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
	}
	return strings.ToLower(result.String())
}

// capitalize capitalizes the first letter
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
