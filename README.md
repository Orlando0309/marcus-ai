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
│   ├── provider/         # LLM adapters (Anthropic, Ollama)
│   ├── tool/             # Tool system (read_file, write_file, run_command)
│   ├── flow/             # Flow engine + executor
│   ├── diff/             # Unified diff parser/applier
│   ├── session/          # Session persistence
│   └── tui/              # Bubble Tea TUI
├── .marcus/              # Project Marcus directory
│   ├── flows/            # Discovered flows
│   ├── tools/            # Custom tools
│   ├── agents/           # Agent configurations
│   └── marcus.toml       # Project config
├── prd.md                # Product Requirements Document
└── implmentation.md      # Implementation Guide
```

---

## Implementation Status

### Phase 1 (Complete)

| Component | Status | Package |
|-----------|--------|---------|
| CLI | Done | `internal/cli/` |
| Config loader | Done | `internal/config/` |
| Folder engine | Done | `internal/folder/` |
| Anthropic provider | Done | `internal/provider/` |
| Ollama provider | Done | `internal/provider/` |
| Tool system | Done | `internal/tool/` |
| Diff engine | Done | `internal/diff/` |
| Flow engine | Done | `internal/flow/` |
| Chat TUI | Done | `internal/tui/` |

### Verified Working

```bash
# Edit with Ollama
./marcus.exe edit test.go "add greet function"

# Interactive TUI
./marcus.exe chat
```

### Roadmap

**Phase 2:** Memory system, Context assembler, Tree-sitter indexer
**Phase 3:** Task system, Loop Engine, Goal stacks, Workflows

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
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key | Required for Anthropic |
| `OLLAMA_HOST` | Ollama host | `http://localhost:11434` |

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
