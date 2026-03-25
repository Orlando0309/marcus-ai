# MARCUS

**Multi-Agent Reasoning & Coding Unified System**

A terminal-native, folder-driven AI coding assistant built to be composable, provider-agnostic, and deeply context-aware.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/go-1.21+-blue.svg)
![Status](https://img.shields.io/badge/status-alpha-yellow.svg)

---

## What is Marcus?

Marcus is a CLI-first AI coding assistant that uses **folders as the primary unit of abstraction**. Where most AI tools are black boxes, Marcus is a glass box — every behavior is a file, every workflow is a folder, every memory is inspectable.

### Core Features

- **Folder-Driven Architecture** — Every capability is a folder. Flows, tools, agents, and memory are all discovered from `.marcus/` directories
- **Provider Agnostic** — Swap Anthropic for Ollama, OpenAI, or Groq by changing one line in config
- **Composable** — Build complex behaviors from simple building blocks
- **Inspectable** — `cat` any file to understand what Marcus knows and why it behaves a certain way
- **Terminal UI** — Rich TUI with conversation viewport, context pane, and tools panel

### Advanced Features (New in Phase 1)

- **Self-Correction** — Automatically detects and fixes tool execution errors with retry logic
- **Semantic Code Search** — Find code by meaning using embeddings (OpenAI/Ollama)
- **AST-Aware Editing** — Structural code edits that preserve Go syntax (rename, imports, parameters)
- **Tool Composition** — Chain tools into reusable pipelines with variable passing
- **Task Planning** — Automatic goal decomposition with DAG-based execution
- **Multi-Agent Orchestration** — Parallel agent execution with 7 specialized roles (explorer, researcher, coder, reviewer, debugger, architect, planner)

---

## Quick Start

### Prerequisites

- Go 1.21+
- Anthropic API key OR local Ollama installation

### Build

```bash
git clone https://github.com/Orlando0309/marcus-ai.git
cd marcus-ai
go build -o marcus.exe ./cmd/marcus
```

### Configure

Create `~/.marcus/config.toml`:

```toml
[provider]
name = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-..."

# Or use Ollama:
# name = "ollama"
# model = "llama-3.1-70b"
# host = "http://localhost:11434"
```

### Initialize Project

```bash
./marcus.exe init
```

---

## Commands

```bash
# Version info
./marcus.exe --version

# Initialize Marcus in current directory
./marcus.exe init [directory]

# List discovered flows
./marcus.exe flow list

# Run a flow
./marcus.exe flow run code_edit

# Edit a file with AI assistance
./marcus.exe edit <file> "<instruction>"

# Open interactive TUI chat
./marcus.exe chat
```

---

## Architecture

```
marcus/
├── cmd/marcus/           # CLI entry point
├── internal/
│   ├── cli/              # Cobra commands
│   ├── config/           # Config loader (global + project)
│   ├── folder/           # Folder engine (discovers flows/tools/agents)
│   ├── provider/         # LLM adapters (Anthropic, Ollama, OpenAI, Groq)
│   ├── tool/             # Tool system + pipelines + composition
│   ├── flow/             # Flow engine + executor + self-correction
│   ├── diff/             # Unified diff parser/applier
│   ├── session/          # Session persistence
│   ├── tui/              # Bubble Tea TUI
│   ├── codeintel/        # Code intelligence (embeddings, AST, indexing)
│   ├── planner/          # Task planning + decomposition
│   └── agent/            # Multi-agent registry + coordinator
├── .marcus/              # Project Marcus directory
│   ├── flows/            # Discovered flows
│   ├── tools/            # Custom tools
│   ├── agents/           # Agent configurations
│   ├── memory/           # Persistent knowledge storage
│   └── marcus.toml       # Project config
├── prd.md                # Product Requirements Document
└── implmentation.md      # Implementation Guide
```

---

## Implementation Status

### Phase 1 (Complete ✅)

| Component | Status | Package |
|-----------|--------|---------|
| CLI | ✅ Done | `internal/cli/` |
| Config loader | ✅ Done | `internal/config/` |
| Folder engine | ✅ Done | `internal/folder/` |
| Anthropic provider | ✅ Done | `internal/provider/` |
| Ollama provider | ✅ Done | `internal/provider/` |
| Tool system | ✅ Done | `internal/tool/` |
| Diff engine | ✅ Done | `internal/diff/` |
| Flow engine | ✅ Done | `internal/flow/` |
| Chat TUI | ✅ Done | `internal/tui/` |
| **Self-Correction** | ✅ Done | `internal/flow/` |
| **Tool Composition** | ✅ Done | `internal/tool/` |
| **Semantic Search** | ✅ Done | `internal/codeintel/` |
| **AST Editing** | ✅ Done | `internal/codeintel/` |
| **Task Planning** | ✅ Done | `internal/planner/` |
| **Multi-Agent** | ✅ Done | `internal/agent/` |

### Verified Working

```bash
# Edit with Ollama
./marcus.exe edit test.go "add greet function"

# Interactive TUI
./marcus.exe chat

# Build semantic index
./marcus.exe flow run build_index

# Semantic code search
./marcus.exe flow run semantic_search "database connection"
```

### Roadmap

**Phase 2:** Vector memory, Knowledge graph, Workflow DAG execution
**Phase 3:** Advanced multi-agent collaboration, Pattern learning, Web tools

---

## How It Works

### 1. Folder Engine Discovery

When Marcus starts, it walks `.marcus/` directories to discover:
- `flows/` — Template-based workflows
- `tools/` — Executable tool definitions
- `agents/` — Agent configurations
- `memory/` — Persistent knowledge storage

### 2. Provider Abstraction

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error)
}
```

### 3. Tool System

Built-in tools:
- `read_file` — Read file contents
- `write_file` — Write file with diff confirmation
- `run_command` — Execute shell commands (sandboxed)

### 4. Diff Engine

Parses unified diff from LLM responses, applies with user confirmation:

```
$ ./marcus.exe edit main.go "add error handling"
--- a/main.go
+++ b/main.go
@@ -10,6 +10,10 @@ func main() {
+       if err != nil {
+           log.Fatal(err)
+       }
 }

Apply this change? [y/N] y
```

---

## Advanced Capabilities

### Self-Correction & Error Recovery

Marcus automatically detects and fixes tool execution errors:

- **Error Pattern Detection** — Recognizes compile errors, test failures, permission issues
- **Retry Logic** — Exponential backoff with configurable max retries
- **Error Classification** — Distinguishes between transient and permanent errors
- **Confidence Scoring** — Estimates success probability for each action

### Semantic Code Search

Find code by meaning, not just keywords:

```bash
# Search by concept
./marcus.exe flow run semantic_search "function that handles user authentication"

# Results ranked by semantic similarity
[
  { "path": "auth/login.go", "score": 0.92, "content": "func AuthenticateUser(...)" },
  { "path": "middleware/auth.go", "score": 0.85, "content": "func RequireAuth(...)" }
]
```

**Features:**
- OpenAI and Ollama embedding providers
- Cosine similarity ranking
- Language-specific filtering
- Persistent index with save/load

### AST-Aware Editing

Make structural code edits that preserve syntax:

```bash
# Rename a function across the codebase
./marcus.exe flow run ast_edit --operation rename --target "OldFunctionName" --new-value "NewFunctionName"

# Add an import
./marcus.exe flow run ast_edit --operation add_import --value "github.com/new/package"

# Preview changes before applying
./marcus.exe flow run ast_edit --preview ...
```

**Supported Operations:**
- Rename (functions, types, methods, constants, variables)
- Add/Remove imports
- Add/Remove function parameters

### Tool Composition

Chain tools into reusable pipelines:

```toml
# .marcus/pipelines/search_and_fix.toml
name = "search_and_fix"
description = "Find code and apply fixes"

[[steps]]
tool = "search_code"
input = { pattern = "${search_pattern}" }
output_to = "matches"

[[steps]]
tool = "read_file"
input = { path = "${matches.0.path}" }
condition = "matches exists"
output_to = "content"

[[steps]]
tool = "edit_file"
input = { path = "${matches.0.path}", old_string = "...", new_string = "..." }
condition = "content contains bug"
```

**Predefined Pipelines:**
- `search_and_read` — Find files and read contents
- `analyze_and_fix` — Run tests and apply fixes
- `explore_and_plan` — Explore structure and create plan

### Task Planning

Automatic goal decomposition with DAG-based execution:

```bash
# Create a plan for a complex task
./marcus.exe flow run plan "Implement user authentication system"

# Plan output with dependencies
Step 1: Research authentication libraries (no deps)
Step 2: Design database schema (depends: Step 1)
Step 3: Implement login endpoint (depends: Step 2)
Step 4: Add middleware (depends: Step 3)
Step 5: Write tests (depends: Step 3, Step 4)
```

**Features:**
- LLM-based task decomposition
- Dependency graph (DAG) construction
- Parallel execution of independent steps
- Automatic plan revision on failure

### Multi-Agent Orchestration

Deploy specialized agents for complex tasks:

| Role | Purpose | Tools |
|------|---------|-------|
| `explorer` | Map codebase structure | list_files, read_file, search_code |
| `researcher` | Find solutions & best practices | fetch_url, search_code |
| `coder` | Write implementation | read_file, write_file, edit_file |
| `reviewer` | Review code for quality | read_file, run_command |
| `debugger` | Diagnose and fix bugs | run_command, search_code, edit_file |
| `architect` | Design system architecture | read_file, write_file |
| `planner` | Create execution plans | read_file, list_files |

```bash
# Execute with multiple agents in parallel
./marcus.exe flow run multi_agent \
  --agents "explorer,coder,reviewer" \
  --goal "Add logging to the auth module"

# Results are synthesized automatically
Synthesis:
1. Explorer found 3 relevant files
2. Coder added structured logging
3. Reviewer approved changes with minor suggestions
```

---

## Philosophy

### What Marcus Is

- CLI-first with rich TUI
- Folder-driven, not config-file-driven
- Provider-agnostic
- Composable and extensible
- Fully inspectable

### What Marcus Is Not

- Not a GUI application
- Not locked to any model or cloud
- Not a monolithic binary
- Not opaque — everything is a file

---

## Configuration

### Global Config (`~/.marcus/config.toml`)

```toml
[provider]
name = "anthropic"
model = "claude-sonnet-4-5-20250929"

[settings]
theme = "dark"
confirm_apply = true
```

### Project Config (`.marcus/marcus.toml`)

```toml
[provider]
name = "ollama"
model = "llama-3.1-70b"

[project]
name = "my-app"
language = "go"

# Embeddings configuration for semantic search
[embeddings]
provider = "ollama"           # openai, ollama, mock
model = "nomic-embed-text"
base_url = "http://localhost:11434"

# Planning configuration
[planning]
enabled = true
decomposition_threshold = 3   # Steps before auto-decomposition
max_plan_depth = 5

# Agent configuration
[agents]
enabled = true
max_parallel = 4
result_synthesis = true

# Self-correction configuration
[self_correction]
max_retries = 3
confidence_threshold = 0.7
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key | Required for Anthropic |
| `OPENAI_API_KEY` | OpenAI API key | Required for OpenAI embeddings |
| `OLLAMA_HOST` | Ollama host | `http://localhost:11434` |
| `MARCUS_LOG_LEVEL` | Log verbosity (debug, info, warn, error) | `info` |

---

## Testing

```bash
# Test edit command
echo 'package main\n\nfunc main() {\n}' > test.go
./marcus.exe edit test.go "add greet function"
# Type 'y' to apply the diff
```

---

## Project Structure Discovery

Marcus discovers capabilities from two scopes:

1. **Global** (`~/.marcus/`) — System-wide flows, tools, agents
2. **Project** (`.marcus/`) — Project-specific overrides

Project entries override global entries with the same name.

---

## Design Principles

1. **Folders Are Truth** — No hidden state, no opaque databases
2. **Composability Over Configuration** — Complex behaviors from simple folders
3. **Provider Agnosticism** — Models are capabilities, not identities
4. **Context is King** — Every action grounded in rich, inspectable context
5. **Fail Loudly, Recover Gracefully** — Explicit errors over silent degradation
6. **Human in the Loop** — Autonomy is opt-in, not opt-out

---

## Documentation

- [PRD](prd.md) — Product Requirements Document
- [Implementation Guide](implmentation.md) — Engineering deep-dive
- [CLAUDE.md](CLAUDE.md) — AI assistant development guidelines
- [AUDIT_2.md](AUDIT_2.md) — Architecture audit and gap analysis

---

## License

MIT License — see LICENSE for details

---

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [fsnotify](https://github.com/fsnotify/fsnotify) — File watching
- [go-toml](https://github.com/pelletier/go-toml) — TOML parsing
