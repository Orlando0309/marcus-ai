package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config holds the merged Marcus configuration.
type Config struct {
	MarcusVersion string `toml:"marcus_version"`
	Provider      string `toml:"provider"`
	// ProviderFallbacks lists extra providers tried in order if the primary errors at runtime (e.g. openai, ollama).
	ProviderFallbacks []string `toml:"provider_fallbacks"`
	Model             string   `toml:"model"`
	Temperature       float64  `toml:"temperature"`
	MaxTokens         int      `toml:"max_tokens"`
	HotReload         bool     `toml:"hot_reload"`
	Theme             string   `toml:"theme"`

	Project     ProjectConfig   `toml:"project"`
	Session     SessionConfig   `toml:"session"`
	Context     ContextConfig   `toml:"context"`
	Tools       ToolsConfig     `toml:"tools"`
	ProviderCfg ProviderConfig  `toml:"provider_runtime"`
	Reasoning   ReasoningConfig `toml:"reasoning"`
	Memory      MemoryConfig    `toml:"memory"`
	LSP         LSPConfig       `toml:"lsp"`
	Autonomy    AutonomyConfig  `toml:"autonomy"`
	Isolation   IsolationConfig `toml:"isolation"`
	Runtime     RuntimeConfig   `toml:"runtime"`
	Hooks       HooksConfig     `toml:"hooks"`

	GlobalPath  string `toml:"-"`
	ProjectPath string `toml:"-"`
	ProjectRoot string `toml:"-"`
}

type ProjectConfig struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Providers   []string `toml:"providers"`
	Flows       []string `toml:"flows"`
}

type SessionConfig struct {
	AutoSave   bool `toml:"auto_save"`
	AutoResume bool `toml:"auto_resume"`
	MaxTurns   int  `toml:"max_turns"`
}

type ContextConfig struct {
	AlwaysInclude     []string `toml:"always_include"`
	MaxFileBytes      int      `toml:"max_file_bytes"`
	MaxFilesPerPrompt int      `toml:"max_files_per_prompt"`
	MaxContextTokens  int      `toml:"max_context_tokens"` // hard cap on estimated context tokens
	WarnAtPercent     int      `toml:"warn_at_percent"`    // warn user when context reaches this %
	CompactAtPercent  int      `toml:"compact_at_percent"` // trigger auto-compact at this %
}

type ToolsConfig struct {
	RunCommand RunCommandToolConfig `toml:"run_command"`
	Manifest   ManifestToolConfig   `toml:"manifest"`
}

type HooksConfig struct {
	PreToolUse  []HookRule `toml:"pre_tool_use"`
	PostToolUse []HookRule `toml:"post_tool_use"`
}

type HookRule struct {
	Matcher  string   `toml:"matcher"`
	Commands []string `toml:"commands"`
}

type RunCommandToolConfig struct {
	AlwaysAllow       []string `toml:"always_allow"`
	AllowPrefixes     []string `toml:"allow_prefixes"`
	BlockedSubstrings []string `toml:"blocked_substrings"`
	// StrictAllowlist rejects any run_command that does not match AlwaysAllow or AllowPrefixes.
	StrictAllowlist bool `toml:"strict_allowlist"`
}

type ManifestToolConfig struct {
	AutoApprovePermissions []string `toml:"auto_approve_permissions"`
}

type ProviderConfig struct {
	CacheEnabled         bool `toml:"cache_enabled"`
	BatchEnabled         bool `toml:"batch_enabled"`
	MaxBatchSize         int  `toml:"max_batch_size"`
	RateLimitPerMinute   int  `toml:"rate_limit_per_minute"` // Max API requests per minute (0 = unlimited)
}

type ReasoningConfig struct {
	Effort       string `toml:"effort"`
	BudgetTokens int    `toml:"budget_tokens"`
}

type MemoryConfig struct {
	RecallLimit int `toml:"recall_limit"`
}

type LSPConfig struct {
	Enabled           bool   `toml:"enabled"`
	TimeoutSeconds    int    `toml:"timeout_seconds"`
	GoCommand         string `toml:"go_command"`
	PythonCommand     string `toml:"python_command"`
	JavaScriptCommand string `toml:"javascript_command"`
	TypeScriptCommand string `toml:"typescript_command"`
}

type AutonomyConfig struct {
	MaxIterations    int            `toml:"max_iterations"`
	RetryBudget      int            `toml:"retry_budget"`
	VerifyAfterApply bool           `toml:"verify_after_apply"`
	DoomLoop         DoomLoopConfig `toml:"doom_loop"`
}

type DoomLoopConfig struct {
	Enabled     bool `toml:"enabled"`
	WindowSize  int  `toml:"window_size"`
	Threshold   int  `toml:"threshold"`
	AskOnDetect bool `toml:"ask_on_detect"` // If true, prompt user when doom detected; otherwise auto-block
}

type IsolationConfig struct {
	Enabled         bool `toml:"enabled"`
	PreferWorktree  bool `toml:"prefer_worktree"`
	RiskyFileWrites int  `toml:"risky_file_writes"`
}

type RuntimeConfig struct {
	DefaultMode string `toml:"default_mode"`
}

// DefaultConfig returns a usable baseline config.
func DefaultConfig() *Config {
	return &Config{
		MarcusVersion: "1.0.0",
		Provider:      "ollama",
		Model:         "qwen3.5:397b-cloud",
		Temperature:   0.3,
		MaxTokens:     4096,
		HotReload:     true,
		Theme:         "marcus-dark",
		Session: SessionConfig{
			AutoSave:   true,
			AutoResume: true,
			MaxTurns:   200,
		},
		Context: ContextConfig{
			AlwaysInclude: []string{
				".marcus/context/PROJECT_MAP.md",
				".marcus/context/ARCHITECTURE.md",
				".marcus/context/CONVENTIONS.md",
				".marcus/context/TASKS.md",
			},
			MaxFileBytes:      8192,
			MaxFilesPerPrompt: 4,
			MaxContextTokens:  100000,
			WarnAtPercent:     80,
			CompactAtPercent:  90,
		},
		ProviderCfg: ProviderConfig{
			CacheEnabled: true,
			BatchEnabled: true,
			MaxBatchSize: 4,
		},
		Reasoning: ReasoningConfig{
			Effort:       "medium",
			BudgetTokens: 2048,
		},
		Memory: MemoryConfig{
			RecallLimit: 8,
		},
		LSP: LSPConfig{
			Enabled:        true,
			TimeoutSeconds: 10,
		},
		Autonomy: AutonomyConfig{
			MaxIterations:    100000,
			RetryBudget:      2,
			VerifyAfterApply: true,
			DoomLoop: DoomLoopConfig{
				Enabled:     true,
				WindowSize:  20,
				Threshold:   3,
				AskOnDetect: true,
			},
		},
		Isolation: IsolationConfig{
			Enabled:         true,
			PreferWorktree:  false,
			RiskyFileWrites: 4,
		},
		Tools: ToolsConfig{
			RunCommand: RunCommandToolConfig{
				BlockedSubstrings: []string{
					"| sh", "| bash", "|sh ", "; sh ", "; bash ",
					"&& curl ", "| curl ", "; curl ",
					"rm -rf /", "rm -rf \\", "mkfs.", "dd if=/dev/",
				},
			},
			Manifest: ManifestToolConfig{
				AutoApprovePermissions: []string{"read", "safe", "inspect"},
			},
		},
		Runtime: RuntimeConfig{
			DefaultMode: "build",
		},
	}
}

// Load discovers and merges configuration from global and project scopes.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}

	globalPath := filepath.Join(homeDir, ".marcus", "config.toml")
	if err := mergeConfigFile(cfg, globalPath); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}
	cfg.GlobalPath = globalPath

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get wd: %w", err)
	}

	projectPath, projectRoot := findProjectConfig(cwd)
	if projectPath != "" {
		if err := mergeConfigFile(cfg, projectPath); err != nil {
			return nil, fmt.Errorf("failed to parse project config: %w", err)
		}
		cfg.ProjectPath = projectPath
		cfg.ProjectRoot = projectRoot
	}

	if cfg.Project.Name == "" && cfg.ProjectRoot != "" {
		cfg.Project.Name = filepath.Base(cfg.ProjectRoot)
	}
	if cfg.Provider == "" && len(cfg.Project.Providers) > 0 {
		cfg.Provider = cfg.Project.Providers[0]
	}

	return cfg, nil
}

func mergeConfigFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return toml.Unmarshal(data, cfg)
}

// findProjectConfig walks up the directory tree to find `.marcus/marcus.toml`.
func findProjectConfig(start string) (configPath, rootDir string) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".marcus", "marcus.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// EnsureGlobalDir creates `~/.marcus/` if it doesn't exist.
func EnsureGlobalDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(homeDir, ".marcus"), 0755)
}

// InitProject creates a rich `.marcus/` project scaffold.
func InitProject(root string) error {
	marcusPath := filepath.Join(root, ".marcus")
	dirs := []string{
		marcusPath,
		filepath.Join(marcusPath, "agents"),
		filepath.Join(marcusPath, "agents", "coding_agent"),
		filepath.Join(marcusPath, "agents", "general_agent"),
		filepath.Join(marcusPath, "context"),
		filepath.Join(marcusPath, "flows"),
		filepath.Join(marcusPath, "flows", "app_plan"),
		filepath.Join(marcusPath, "flows", "app_scaffold"),
		filepath.Join(marcusPath, "flows", "chat"),
		filepath.Join(marcusPath, "flows", "code_edit"),
		filepath.Join(marcusPath, "flows", "create_todo"),
		filepath.Join(marcusPath, "tools"),
		filepath.Join(marcusPath, "tools", "list_python_files"),
		filepath.Join(marcusPath, "memory"),
		filepath.Join(marcusPath, "memory", "user"),
		filepath.Join(marcusPath, "memory", "project"),
		filepath.Join(marcusPath, "memory", "reference"),
		filepath.Join(marcusPath, "memory", "decisions"),
		filepath.Join(marcusPath, "memory", "patterns"),
		filepath.Join(marcusPath, "cache"),
		filepath.Join(marcusPath, "cache", "provider"),
		filepath.Join(marcusPath, "sessions"),
		filepath.Join(marcusPath, "tasks"),
		filepath.Join(marcusPath, "tasks", "queue"),
		filepath.Join(marcusPath, "tasks", "active"),
		filepath.Join(marcusPath, "tasks", "done"),
		filepath.Join(marcusPath, "tasks", "blocked"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	projectName := filepath.Base(root)
	configContents := strings.TrimSpace(fmt.Sprintf(`
marcus_version = "1.0.0"
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.3
max_tokens = 4096
hot_reload = true
theme = "marcus-dark"

[project]
name = %q
description = "Project-level Marcus runtime configuration"

[session]
auto_save = true
auto_resume = true
max_turns = 200

[context]
always_include = [
  ".marcus/context/PROJECT_MAP.md",
  ".marcus/context/ARCHITECTURE.md",
  ".marcus/context/CONVENTIONS.md",
  ".marcus/context/TASKS.md",
]
max_file_bytes = 8192
max_files_per_prompt = 4
max_context_tokens = 100000
warn_at_percent = 80
compact_at_percent = 90

[tools.run_command]
always_allow = ["go test ./...", "go build ./...", "cargo test", "cargo build", "npm test", "npm run build", "python -m pytest", "python -m compileall -q ."]

[tools.manifest]
auto_approve_permissions = ["read", "safe", "inspect"]

[provider_runtime]
cache_enabled = true
batch_enabled = true
max_batch_size = 4

[memory]
recall_limit = 8

[lsp]
enabled = true
timeout_seconds = 10
go_command = "gopls"
python_command = "pylsp"
javascript_command = ""
typescript_command = ""

[autonomy]
max_iterations = 100000
retry_budget = 2
verify_after_apply = true

[isolation]
enabled = true
prefer_worktree = false
risky_file_writes = 4

[runtime]
default_mode = "build"
`, projectName)) + "\n"

	codingAgentSystem := `# Coding Agent — System Prompt

You are **Marcus Coding Agent**, an autonomous software engineer for real repositories.

## Core mindset
- Model the system before editing it.
- Prefer the smallest safe diff that solves the task.
- Match existing patterns instead of inventing new ones.
- Verify every meaningful change.
- Explain progress briefly and keep machine-only structure out of user-facing prose.

## Orientation before edits
Before proposing writes:
1. Read the generated project map and project docs already in context first.
2. Identify the single most likely file or subsystem involved.
3. Use targeted reads and searches to confirm the existing pattern.
4. Only broaden exploration if the first file is not enough.

Do not scan the whole repository by default. Avoid broad "list_files" passes when a targeted "read_file", "search_code", or symbol lookup will answer the question faster.

## Iteration loop
1. **Orient** — understand the stack, structure, conventions, and the relevant existing pattern.
2. **Inspect** — use safe tools to confirm the exact file(s) and imports involved.
3. **Plan** — choose the smallest viable change and the right verification step.
4. **Execute** — write files and run commands as needed.
5. **Verify** — build, run the relevant test, or perform the narrowest useful smoke check.
6. **Complete** — finish only when the requested outcome is actually satisfied.

## Engineering discipline
- Search for the concept before creating new files or abstractions.
- Keep refactors separate from feature or bug-fix work unless the user asked for both.
- When debugging, reproduce first, form one hypothesis, add one observation, then change code.
- Read failures carefully and fix the specific cause rather than guessing.
- Tests should express intent and behavior, not implementation trivia.
- Comments should explain *why* when the code is not already obvious.

## Output rules
- Always return valid JSON with "message", "actions", and "tasks".
- Keep "message" brief, human-readable, and free of raw JSON.
- When writing files, always include complete file contents.
- Put verification commands after the writes they validate.
- For "run_command", emit the exact shell command Marcus should run on the current platform. Do not use JSON-style escaped quotes like "\\\"" inside the command string.
- Use the task system: mark tasks "active" when starting, "done" when complete, "blocked" when stuck.
- Never use markdown fences in your JSON response.
- If a command is not auto-approved, it will be shown to the user for approval.

## Project map discipline
- Marcus maintains ".marcus/context/PROJECT_MAP.md" from a bounded repo scan.
- Start from that generated map instead of scanning directories when it already answers where to begin.
- If you learn a durable repo fact that future tasks should know, update the generated map before finishing.
`

	codingAgentRules := "# Coding Agent \u2014 Partition Rules\n\nThese rules define which actions Marcus auto-runs vs. asks approval for.\n\n## Safe actions (always auto-run)\n- `list_files` \u2014 browse directory structure\n- `read_file` \u2014 inspect file contents\n- `search_code` \u2014 regex search across files\n- `find_symbol` \u2014 find a function/type definition\n- `list_symbols` \u2014 list symbols in a file\n\n## Auto-run commands (no approval needed)\nAny `run_command` that starts with one of these prefixes is auto-approved:\n- `go build`\n- `go build ./...`\n- `cargo build`\n- `cargo build 2>&1`\n- `npm run build`\n- `npm run build 2>&1`\n- `ruff check`\n- `ruff check .`\n- `ruff format`\n- `go test`\n- `go test ./...`\n- `go vet`\n- `golangci-lint run`\n- `python -m py_compile`\n- `mvn compile`\n- `gradle build`\n- `make`\n- `cmake --build`\n- Any `go run` that runs a specific file (not a general `go run`)\n\n## Write file policy\n- `write_if = \"first_wave\"` \u2014 auto-run when there is exactly ONE write_file action and NO run_command in the same batch\n- If there are multiple writes or a write + run_command mix, all writes go to pending (ask user approval)\n\n## Verification\n- `verification = \"detect\"` \u2014 after any write_file, detect the project type and run the appropriate build command automatically\n- If `verification = \"always\"`, always run verification after writes\n- If `verification = \"never\"`, skip verification after writes\n- To override the detected command, add a specific command: e.g. `verification = \"go test ./...\"`\n\n## Step mode\n- `step_mode = false` \u2014 by default, run autonomously without pausing\n- If `step_mode = true`, pause after each iteration and wait for user to press Space to continue\n"

	generalAgentSystem := `# General Agent — System Prompt

You are **Marcus**, a general-purpose terminal assistant for software projects.

## Investigation discipline
- Start from the most relevant file or symbol.
- Prefer targeted reads/searches over broad scans.
- Use hypothesis-driven debugging when diagnosing failures.

## Output rules
- For action-oriented responses, return JSON with "message", "actions", and "tasks".
- For simple questions, plain text is allowed.
- Keep explanations concise and technical.
`

	generalAgentRules := "# General Agent \u2014 Partition Rules\n\n## Safe actions (always auto-run)\n- `list_files` \u2014 browse directory\n- `read_file` \u2014 read any file\n- `search_code` \u2014 search files\n- `find_symbol` \u2014 find definitions\n\n## Auto-run commands\nNone \u2014 all run_command actions require user approval.\n\n## Write file policy\n`write_if = \"never\"` \u2014 any write_file requires explicit user approval.\n\n## Verification\n`verification = \"never\"` \u2014 no automatic build verification.\n\n## Step mode\n`step_mode = true` \u2014 pause between iterations so the user can follow along and approve actions.\n"

	architectureMd := "# Architecture\n\nDocument this repository's architecture here.\n\nSuggested sections:\n- Runtime components and boundaries\n- Main data/control flows\n- Key source-of-truth files\n- External integrations\n"

	conventionsMd := "# Conventions\n\nDocument repository-specific conventions here.\n\nSuggested sections:\n- Naming and folder rules\n- Testing/verification expectations\n- Command safety and approval rules\n- Platform compatibility constraints\n"

	tasksMd := "# Tasks\n\nTrack durable, high-priority goals here.\n\nSuggested sections:\n- Current goals\n- Milestones\n- Blockers\n"

	codeEditPrompt := "# Code Edit Instruction\n\nYou are an AI coding assistant. Your task is to edit code files based on user instructions.\n\n## File to Edit\n{{.file}}\n\n## Current Content\n```\n{{.content}}\n```\n\n## User Instruction\n{{.instruction}}\n\n## Your Task\n1. Understand the current code structure\n2. Apply the user's instruction precisely\n3. Return ONLY the new/modified code, not the entire file\n4. Use a unified diff format to show changes\n\nFormat your response as:\n```diff\n@@ -original_line_count,+new_line_count @@\n-original line\n+new line\n```\n\nOr if the change is simple, just describe what to change:\n- Line X: change Y to Z\n- Add function ABC after line N\n"

	files := map[string]string{
		filepath.Join(marcusPath, "marcus.toml"): configContents,

		// Agents
		filepath.Join(marcusPath, "agents", "coding_agent", "agent.toml"): `[agent]
name = "coding_agent"
role = "coding"
description = "Autonomous coding agent that writes files, runs builds, fixes errors, and iterates until the task is complete"

[autonomy]
iteration_limit = 100000
verification = "detect"
confirm_writes = false

[rules]
safe_actions = ["list_files", "read_file", "search_code", "find_symbol", "list_symbols"]
auto_run_commands = ["go build", "go build ./...", "cargo build", "cargo build 2>&1", "npm run build", "npm run build 2>&1", "ruff check", "ruff check .", "ruff format", "go test", "go test ./...", "go vet", "golangci-lint run", "python -m py_compile", "mvn compile", "gradle build", "make", "cmake --build"]
write_if = "first_wave"
step_mode = false
`,
		filepath.Join(marcusPath, "agents", "coding_agent", "system.md"): codingAgentSystem,
		filepath.Join(marcusPath, "agents", "coding_agent", "rules.md"):  codingAgentRules,
		filepath.Join(marcusPath, "agents", "general_agent", "agent.toml"): `[agent]
name = "general_agent"
role = "general"
description = "General-purpose conversational agent for questions, analysis, and non-coding tasks"

[autonomy]
iteration_limit = 100000
verification = "never"
confirm_writes = true

[rules]
safe_actions = ["list_files", "read_file", "search_code", "find_symbol"]
auto_run_commands = []
write_if = "never"
step_mode = true
`,
		filepath.Join(marcusPath, "agents", "general_agent", "system.md"): generalAgentSystem,
		filepath.Join(marcusPath, "agents", "general_agent", "rules.md"):  generalAgentRules,

		// Context
		filepath.Join(marcusPath, "context", "ARCHITECTURE.md"): architectureMd,
		filepath.Join(marcusPath, "context", "CONVENTIONS.md"):  conventionsMd,
		filepath.Join(marcusPath, "context", "TASKS.md"):        tasksMd,

		// Flows \u2014 app_plan
		filepath.Join(marcusPath, "flows", "app_plan", "flow.toml"): `[flow]
name = "app_plan"
description = "Plan an application before scaffolding"
version = "1.0.0"
author = "marcus"

[model]
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.2
max_tokens = 4096

[input]
requires = ["description"]
optional = ["stack", "constraints"]

[output]
format = "text"
save_to = ""

[behavior]
stream = false
confirm_before_apply = true
auto_fix = false

tools = []
`,
		filepath.Join(marcusPath, "flows", "app_plan", "prompt.md"): `# App Planning Flow

Plan a new application from this description:

{{.description}}
`,
		filepath.Join(marcusPath, "flows", "app_plan", "context.md"): "Plan tasks and folder structure before proposing file writes.\n",

		// Flows \u2014 app_scaffold
		filepath.Join(marcusPath, "flows", "app_scaffold", "flow.toml"): `[flow]
name = "app_scaffold"
description = "Generate a starter scaffold for a new application"
version = "1.0.0"
author = "marcus"

[model]
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.2
max_tokens = 4096

[input]
requires = ["description"]
optional = ["stack", "output_dir"]

[output]
format = "text"
save_to = ""

[behavior]
stream = false
confirm_before_apply = true
auto_fix = false

tools = ["write_file", "run_command"]
`,
		filepath.Join(marcusPath, "flows", "app_scaffold", "prompt.md"): `# App Scaffold Flow

Create a starter scaffold for:

{{.description}}
`,
		filepath.Join(marcusPath, "flows", "app_scaffold", "context.md"): "Keep scaffolding minimal, reviewable, and backed by durable tasks.\n",

		// Flows \u2014 chat
		filepath.Join(marcusPath, "flows", "chat", "flow.toml"): `[flow]
name = "chat"
description = "Interactive chat conversation with Marcus AI"
version = "1.0.0"
author = "marcus"

[model]
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.7
max_tokens = 4096

[input]
requires = ["message"]
optional = ["conversation_history"]

[output]
format = "text"
save_to = ""

[behavior]
stream = false
confirm_before_apply = false
auto_fix = false
`,
		filepath.Join(marcusPath, "flows", "chat", "prompt.md"): `# Marcus Chat

You are Marcus, an AI coding assistant. You are helpful, precise, and knowledgeable about software development.

## Conversation History
{{range .conversation_history}}
{{.Role}}: {{.Content}}
{{end}}

## User Message
{{.message}}

Please respond helpfully and concisely. If the user asks about code, provide accurate technical information.
`,

		// Flows \u2014 code_edit
		filepath.Join(marcusPath, "flows", "code_edit", "flow.toml"): `[flow]
name = "code_edit"
description = "Edit code files based on a natural language instruction"
version = "1.0.0"
author = "marcus"

[model]
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.3
max_tokens = 4096

[input]
requires = ["file", "instruction"]
optional = ["context"]

[output]
format = "diff"
save_to = ""

[behavior]
stream = true
confirm_before_apply = true
auto_fix = false
`,
		filepath.Join(marcusPath, "flows", "code_edit", "prompt.md"): codeEditPrompt,

		// Flows \u2014 create_todo
		filepath.Join(marcusPath, "flows", "create_todo", "flow.toml"): `[flow]
name = "create_todo"
description = "Create a todo list application"
version = "1.0.0"
author = "marcus"

[model]
provider = "ollama"
model = "qwen3.5:397b-cloud"
temperature = 0.3
max_tokens = 4096

[input]
requires = ["description"]
optional = []

[output]
format = "text"
save_to = ""

[behavior]
stream = false
confirm_before_apply = false
auto_fix = false

tools = ["write_file", "run_command"]
`,
		filepath.Join(marcusPath, "flows", "create_todo", "prompt.md"): `# Create Todo Application

Create a todo list application based on the user's description.

## User Request
{{.description}}

## Instructions
1. Create a simple, functional todo list application
2. Use appropriate language/framework for the request
3. Include all necessary files for the application to run
4. Provide clear instructions on how to use it

Return the complete file content for each file needed.
`,

		// Tools
		filepath.Join(marcusPath, "tools", "list_python_files", "tool.toml"): `type = "shell"
description = "List Python files in the current project"
timeout = 10
permissions = ["read", "inspect"]

[tool]
name = "list_python_files"
description = "List Python files in the project"
`,
		filepath.Join(marcusPath, "tools", "list_python_files", "run.ps1"): `$root = $env:MARCUS_PROJECT_ROOT
if ([string]::IsNullOrWhiteSpace($root)) {
  $root = (Get-Location).Path
}

$files = Get-ChildItem -Path $root -Recurse -File -Include *.py |
  Where-Object { $_.FullName -notmatch '\\.git\\|\\node_modules\\|\\__pycache__\\|\\\.venv\\' } |
  ForEach-Object { $_.FullName.Substring($root.Length).TrimStart('\') -replace '\\','/' }

$result = @{
  files = $files
  count = $files.Count
}

$result | ConvertTo-Json -Depth 5
`,
	}

	for path, content := range files {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}
