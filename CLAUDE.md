# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**MARCUS** (Multi-Agent Reasoning & Coding Unified System) - A terminal-native AI coding assistant built to be composable, provider-agnostic, and deeply context-aware.

**Current State:** Phase 1 complete. CLI, folder engine, Anthropic + Ollama providers, tool system, diff engine, edit command, and TUI chat are functional and tested.

## Project Structure

```
marcus/
├── cmd/marcus/           # CLI entry point
├── internal/
│   ├── cli/              # Cobra commands (chat, edit, flow, init, version)
│   ├── config/           # Config loading (global + project scopes)
│   ├── folder/           # Folder engine (discovers flows/tools/agents)
│   ├── provider/         # LLM adapters (Anthropic, Ollama)
│   ├── tool/             # Tool system (read_file, write_file, run_command)
│   ├── flow/             # Flow engine + executor
│   ├── diff/             # Unified diff parser/applier
│   ├── session/          # Session persistence (TODO)
│   └── tui/              # TUI (TODO - Phase 2)
├── .marcus/              # Project Marcus directory
│   ├── flows/            # Discovered flows
│   └── marcus.toml       # Project config
├── prd.md                # Product Requirements Document (60KB)
├── implmentation.md      # Implementation guide (76KB)
└── CLAUDE.md             # AI assistant instructions
```

## Commands

```bash
# Build
go build -o marcus.exe ./cmd/marcus

# Test
./marcus.exe --version
./marcus.exe --help
./marcus.exe init [directory]
./marcus.exe flow list
./marcus.exe flow run <flow-name>
./marcus.exe edit <file> "<instruction>"
./marcus.exe chat  # Bubble Tea TUI with conversation/context/tools panes
```

## Implementation Status

### Phase 1 (Complete)

| Component | Status | Package | Notes |
|-----------|--------|---------|-------|
| **CLI** | Done | `internal/cli/` | chat, edit, flow, init, version |
| **Config loader** | Done | `internal/config/` | `~/.marcus/config.toml` + `.marcus/marcus.toml` |
| **Folder engine** | Done | `internal/folder/` | Discovers flows/tools/agents, hot-reload via fsnotify |
| **Anthropic provider** | Done | `internal/provider/` | HTTP API implementation |
| **Ollama provider** | Done | `internal/provider/` | qwen3.5:397b-cloud tested successfully |
| **Tool system** | Done | `internal/tool/` | read_file, write_file, run_command |
| **Diff engine** | Done | `internal/diff/` | Parse/apply unified diff, ANSI rendering |
| **Flow engine** | Done | `internal/flow/` | Template rendering, streaming execution |
| **Sample flow** | Done | `.marcus/flows/code_edit/` | Working end-to-end |
| **Chat TUI** | Done | `internal/tui/` | Bubble Tea with conversation/context/tools panes |

### Verified Working

```bash
$ ./marcus.exe edit test.go "add a function called greet"
# Sends to Ollama, shows diff, applies with confirmation ✓

$ ./marcus.exe chat
# Opens interactive TUI with conversation viewport, context pane, tools pane ✓
```

### Remaining Phase 1 Work

- [ ] **Session persistence** - Save/load conversations to `.marcus/sessions/`

### Phase 2 (Next)

- [ ] **TUI** - Bubble Tea for `marcus chat`
- [ ] **Multiple providers** - Add OpenAI, Groq
- [ ] **Memory system** - Facts, episodic, patterns
- [ ] **Context assembler** - Token budget, relevance scoring
- [ ] **Tree-sitter indexer** - Code symbol parsing

### Phase 3

- [ ] **Task system** - `.marcus/tasks/` queue
- [ ] **Loop Engine** - Autonomous execution
- [ ] **Goal stacks** - Track long-running sessions
- [ ] **Workflows** - DAG orchestration

## Key Interfaces

### Provider
```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, prompt string, opts CompletionOptions) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan StreamChunk, error)
}
```

### Tool
```go
type Tool interface {
    Name() string
    Description() string
    Schema() JSONSchema
    Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

### Folder Engine
```go
type FolderEngine struct {
    globalPath, projectPath string
    registry   *Registry
    watcher    *fsnotify.Watcher
}
```

## Technology Stack

| Component | Choice |
|-----------|--------|
| Language | Go |
| CLI | Cobra |
| Config | TOML (go-toml/v2) |
| Providers | Anthropic (HTTP), Ollama (HTTP) |
| File Watcher | fsnotify |
| Diff | Custom unified diff parser |
| TUI (Phase 2) | Bubble Tea |

## Development Notes

- Set `ANTHROPIC_API_KEY` for Anthropic provider
- Ollama uses `OLLAMA_HOST` env var (default: `http://localhost:11434`)
- Flows discovered from `.marcus/flows/` (project) and `~/.marcus/flows/` (global)
- Hot-reload watches `.marcus/` for changes
- Tool paths sandboxed to prevent escaping base directory
- Edit command: parses unified diff from LLM, applies with confirmation

## Testing

```bash
# Test edit with Ollama
echo 'package main\n\nfunc main() {\n}' > test.go
./marcus.exe edit test.go "add greet function"  # Type 'y' to apply
```
