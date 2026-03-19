# PRD — MARCUS
### **M**ulti-Agent **R**easoning & **C**oding **U**nified **S**ystem
**Version:** 1.0.0-alpha  
**Status:** Draft  
**Author:** Architecture Proposal  
**Date:** 2026-03-18

---

> *Marcus is a terminal-native, folder-driven AI coding assistant built to be composable, provider-agnostic, and deeply context-aware. Where most AI tools are black boxes, Marcus is a glass box — every behavior is a file, every workflow is a folder, every memory is inspectable.*

---

## Table of Contents

1. [Vision & Philosophy](#1-vision--philosophy)
2. [Core Design Principles](#2-core-design-principles)
3. [Folder-Driven Architecture](#3-folder-driven-architecture)
4. [The Marcus Runtime](#4-the-marcus-runtime)
5. [Coding Intelligence](#5-coding-intelligence)
6. [Memory System](#6-memory-system)
7. [Provider Management](#7-provider-management)
8. [Workflow & Flow Engine](#8-workflow--flow-engine)
9. [Interactive Console (TUI)](#9-interactive-console-tui)
10. [Tool System](#10-tool-system)
11. [Agent System](#11-agent-system)
12. [Context Management](#12-context-management)
13. [Plugin Architecture](#13-plugin-architecture)
14. [Security Model](#14-security-model)
15. [Configuration Reference](#15-configuration-reference)
16. [CLI Reference](#16-cli-reference)
17. [Roadmap](#17-roadmap)

---

## 1. Vision & Philosophy

### 1.1 The Problem with Existing AI Coding Tools

Today's AI coding assistants share a fundamental flaw: they are **opaque by design**. Behavior is hardcoded. Memory is hidden. Workflows are rigid. You can't inspect *why* the AI did what it did, you can't modify *how* it reasons, and you can't compose *complex behaviors* from simple building blocks.

Marcus is built on a different premise:

> **"If a human can understand a folder, a human can understand Marcus."**

### 1.2 What Marcus Is

Marcus is a **CLI-first AI coding assistant** that:

- Uses **folders as the primary unit of abstraction** for flows, workflows, agents, memory, and tools
- Is **provider-agnostic** — swap Anthropic for OpenAI for Gemini by changing one line
- Has **persistent, structured memory** at project, workspace, and global levels
- Exposes a **rich interactive TUI** (terminal UI) that rivals graphical IDEs
- Is **composable** — every capability is a plugin, every plugin is a folder
- Is **inspectable** — you can `cat` any file to understand what Marcus knows and why it behaves a certain way

### 1.3 What Marcus Is Not

- Not a GUI application (though a web dashboard is a stretch goal)
- Not a replacement for your editor (it integrates *with* your editor)
- Not locked to any model, provider, or cloud
- Not a monolithic binary — it's a runtime that orchestrates folders

---

## 2. Core Design Principles

### P1 — Folders Are Truth
Every behavior, every memory, every workflow lives in a folder. No hidden state. No opaque databases (unless you opt in). Open Marcus's working directory and you understand Marcus.

### P2 — Composability Over Configuration
Instead of a 500-line config file, Marcus is built from composable units. A "flow" is a folder. A "tool" is a folder. An "agent" is a folder. Complex behaviors emerge from combining simple folders.

### P3 — Provider Agnosticism
Marcus never assumes a specific LLM. The provider layer is a thin adapter. Switching from Claude to GPT-4o to Llama 3 is a one-line change. Models are capabilities, not identities.

### P4 — Context is King
Every action Marcus takes is grounded in rich context: the current file, the git diff, the recent conversation, the project's coding standards, the team's past decisions. Context is assembled automatically and is always inspectable.

### P5 — Fail Loudly, Recover Gracefully
Marcus prefers explicit errors over silent degradation. When a tool fails, it says why. When context is insufficient, it asks. When a workflow is ambiguous, it confirms.

### P6 — Human in the Loop by Default
Marcus never applies changes without showing a diff. Never commits without confirmation. Never makes destructive operations silently. Autonomy is opt-in, not opt-out.

---

## 3. Folder-Driven Architecture

### 3.1 The Marcus Home Directory

When Marcus is initialized globally, it creates `~/.marcus/`:

```
~/.marcus/
├── config.toml                  # Global configuration
├── providers/                   # LLM provider definitions
│   ├── anthropic.toml
│   ├── openai.toml
│   ├── groq.toml
│   └── local.toml               # Ollama / local models
├── memory/                      # Global memory
│   ├── index.json               # Memory index
│   ├── facts/                   # Persistent facts (key-value)
│   ├── episodic/                # Conversation summaries
│   └── semantic/                # Embeddings store (optional)
├── tools/                       # Global tools
│   ├── web_search/
│   ├── run_command/
│   └── read_url/
├── agents/                      # Reusable agent definitions
│   ├── reviewer/
│   ├── architect/
│   └── debugger/
├── flows/                       # Global reusable flows
│   ├── code_review/
│   ├── refactor/
│   └── explain/
├── plugins/                     # Installed plugins
└── themes/                      # TUI themes
    ├── dracula.toml
    ├── nord.toml
    └── marcus-dark.toml
```

### 3.2 The Project Marcus Directory

When Marcus is initialized in a project, it creates `.marcus/` at the project root:

```
.marcus/
├── marcus.toml                  # Project-level config (overrides global)
├── context/                     # Project context files
│   ├── ARCHITECTURE.md          # Auto-read on every session
│   ├── CONVENTIONS.md           # Coding conventions
│   ├── GLOSSARY.md              # Domain-specific terminology
│   └── TECH_STACK.md            # Technologies in use
├── memory/                      # Project memory (overrides global)
│   ├── decisions/               # Architecture decisions log
│   ├── patterns/                # Detected code patterns
│   └── todos/                   # Persistent todos across sessions
├── flows/                       # Project-specific flows
│   ├── deploy/
│   ├── test_suite/
│   └── pr_review/
├── tools/                       # Project-specific tools
│   ├── run_tests/
│   └── build/
├── agents/                      # Project-specific agents
│   └── domain_expert/
├── sessions/                    # Session history
│   ├── 2026-03-18T10-23-41/
│   │   ├── conversation.jsonl
│   │   ├── context_snapshot.json
│   │   └── summary.md
│   └── latest -> 2026-03-18T10-23-41/
└── workspace.json               # Current workspace state
```

### 3.3 Flow Folder Anatomy

A **flow** is the fundamental unit of behavior in Marcus. Every flow is a folder:

```
flows/code_review/
├── flow.toml                    # Flow definition and metadata
├── prompt.md                    # The main prompt template
├── context.md                   # What context to inject
├── steps/                       # Multi-step flows
│   ├── 01_analyze.md
│   ├── 02_suggest.md
│   └── 03_apply.md
├── tools.toml                   # Which tools this flow can use
├── examples/                    # Few-shot examples
│   ├── good_review.md
│   └── bad_review.md
├── hooks/                       # Lifecycle hooks (scripts)
│   ├── pre_run.sh
│   └── post_run.sh
└── tests/                       # Flow tests
    ├── test_001.toml
    └── test_002.toml
```

**flow.toml:**
```toml
[flow]
name = "code_review"
description = "Performs a thorough code review of staged changes"
version = "1.0.0"
author = "team"

[model]
provider = "default"             # Uses project/global default
temperature = 0.3                # Lower = more consistent
max_tokens = 4096

[input]
requires = ["diff", "file_list"]
optional = ["ticket_description", "pr_title"]

[output]
format = "markdown"
save_to = ".marcus/memory/reviews/{timestamp}.md"

[behavior]
stream = true
confirm_before_apply = true
auto_fix = false
```

### 3.4 Workflow Folder Anatomy

A **workflow** orchestrates multiple flows in sequence or parallel:

```
workflows/feature_ship/
├── workflow.toml                # Workflow definition
├── dag.toml                     # Directed acyclic graph of steps
├── steps/
│   ├── 01_review/               # → runs flow: code_review
│   │   └── step.toml
│   ├── 02_test_generation/      # → runs flow: generate_tests
│   │   └── step.toml
│   ├── 03_docs_update/          # → runs flow: update_docs
│   │   └── step.toml
│   └── 04_pr_description/       # → runs flow: write_pr
│       └── step.toml
└── conditions/
    ├── skip_if_draft.toml       # Conditional execution
    └── require_clean_tree.toml
```

**dag.toml:**
```toml
[dag]
name = "feature_ship"
strategy = "sequential"          # or "parallel" or "conditional"

[[step]]
id = "review"
flow = "code_review"
depends_on = []

[[step]]
id = "tests"
flow = "generate_tests"
depends_on = ["review"]
condition = "review.passed == true"

[[step]]
id = "docs"
flow = "update_docs"
depends_on = ["review"]
parallel_with = ["tests"]

[[step]]
id = "pr"
flow = "write_pr"
depends_on = ["tests", "docs"]
```

---

## 4. The Marcus Runtime

### 4.1 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         MARCUS RUNTIME                       │
│                                                              │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────────┐   │
│  │   TUI    │    │   CLI    │    │    LSP Bridge        │   │
│  │ (Bubble  │    │ (Cobra)  │    │  (Editor Integration)│   │
│  │   Tea)   │    │          │    │                      │   │
│  └────┬─────┘    └────┬─────┘    └──────────┬───────────┘   │
│       │               │                      │               │
│  ─────┴───────────────┴──────────────────────┴─────────────  │
│                    EVENT BUS                                  │
│  ─────────────────────────────────────────────────────────── │
│                                                              │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────┐ │
│  │   Session   │  │    Flow      │  │   Workflow          │ │
│  │   Manager   │  │   Engine     │  │   Orchestrator      │ │
│  └──────┬──────┘  └──────┬───────┘  └──────────┬──────────┘ │
│         │                │                      │            │
│  ┌──────┴──────────────────────────────────────┴──────────┐  │
│  │                   CONTEXT ASSEMBLER                     │  │
│  └──────────────────────────┬──────────────────────────────┘ │
│                             │                                 │
│  ┌──────────────────────────┴──────────────────────────────┐  │
│  │                   PROVIDER ROUTER                        │  │
│  └──────┬──────────────┬──────────────┬────────────────────┘  │
│         │              │              │                        │
│  ┌──────┴──┐    ┌──────┴──┐    ┌─────┴────┐                  │
│  │Anthropic│    │ OpenAI  │    │  Local   │  ...              │
│  │ Adapter │    │ Adapter │    │ Adapter  │                   │
│  └─────────┘    └─────────┘    └──────────┘                  │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌────────────────────────┐  │
│  │  Memory    │  │   Tool     │  │      Agent             │  │
│  │  Manager   │  │  Runner    │  │    Coordinator         │  │
│  └────────────┘  └────────────┘  └────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Bootstrap Sequence

When Marcus starts (`marcus` or `marcus chat`):

1. **Load global config** from `~/.marcus/config.toml`
2. **Walk up the directory tree** to find `.marcus/` (project context)
3. **Merge configs** (project overrides global, env vars override both)
4. **Validate providers** — ping configured providers, report status
5. **Load memory** — deserialize active memory layers
6. **Scan flows/tools/agents** — index available capabilities
7. **Restore session** — optionally resume last session
8. **Start TUI or stream mode** depending on flags

### 4.3 The Folder Watcher

Marcus runs a background folder watcher on `.marcus/` using filesystem events. When you edit a flow's `prompt.md`, the change is hot-reloaded without restarting Marcus. This makes iterating on flows as fast as editing a text file.

```toml
# marcus.toml
[runtime]
hot_reload = true                # Watch .marcus/ for changes
watch_debounce_ms = 300
```

---

## 5. Coding Intelligence

### 5.1 Code Understanding Layer

Marcus maintains a **live code index** of the current project. This is not a full LSP — it's a lightweight semantic layer built specifically for AI context assembly:

```
.marcus/
└── index/
    ├── symbols.json             # Function/class/type index
    ├── imports.json             # Import graph
    ├── call_graph.json          # Who calls what
    ├── file_hashes.json         # Change detection
    └── embeddings/              # Optional: semantic search
        └── *.vec
```

**Symbol indexing** is language-aware using Tree-sitter parsers:

| Language       | Parser        | Indexed Constructs                        |
|----------------|---------------|--------------------------------------------|
| TypeScript/JS  | tree-sitter   | functions, classes, interfaces, exports    |
| Python         | tree-sitter   | functions, classes, decorators, imports    |
| Go             | tree-sitter   | funcs, types, interfaces, packages         |
| Rust           | tree-sitter   | fns, structs, traits, impls, modules       |
| Java/Kotlin    | tree-sitter   | classes, methods, interfaces, annotations  |
| C/C++          | tree-sitter   | functions, structs, classes, macros        |
| Ruby/PHP       | tree-sitter   | classes, methods, modules                  |

### 5.2 Context-Aware Code Generation

When Marcus generates code, it automatically injects:

- **Local conventions** from `.marcus/context/CONVENTIONS.md`
- **Existing patterns** detected in the codebase (naming, structure)
- **Import style** (are they using named imports? default exports? barrel files?)
- **Test framework** in use (jest, vitest, pytest, go test, etc.)
- **Error handling patterns** from existing code
- **Type patterns** (strict TypeScript? using zod? pydantic?)

This context assembly happens automatically via the **Context Assembler** (see §12).

### 5.3 Code Operations

Marcus supports these first-class code operations:

#### `marcus edit <file> "<instruction>"`
Applies a targeted edit to a specific file. Shows a diff before applying.

```
$ marcus edit src/auth.ts "add rate limiting to the login function"

  Reading src/auth.ts...
  Analyzing surrounding context...
  Generating edit...

  ┌─ PROPOSED EDIT ──────────────────────────────────────────┐
  │  src/auth.ts                                              │
  │                                                           │
  │  + import { rateLimit } from '../middleware/rateLimit'    │
  │                                                           │
  │  - async function login(req: Request) {                   │
  │  + async function login(req: Request) {                   │
  │  +   await rateLimit(req, { max: 5, window: '15m' })      │
  │      const { email, password } = req.body                 │
  │      ...                                                  │
  └───────────────────────────────────────────────────────────┘

  Apply this edit? [y/N/e(dit)/v(iew full file)]
```

#### `marcus create <path> "<description>"`
Creates a new file from a description, respecting project conventions.

#### `marcus refactor <file> --flow refactor/extract_function`
Runs a specific flow against a file.

#### `marcus explain <file>:<line_range>`
Explains a code section with full project context.

#### `marcus fix`
Reads stderr/stdout from the last failed command and attempts to fix the relevant code.

```bash
$ npm test 2>&1 | marcus fix
# OR
$ marcus fix --from-last-error
```

### 5.4 Diff & Apply Engine

Marcus uses a custom diff/apply engine that handles:

- **Fuzzy matching** — if the AI's context was slightly stale, Marcus finds the right location anyway
- **Conflict detection** — checks for conflicts with uncommitted changes
- **Atomic application** — all-or-nothing: either the full edit applies cleanly or nothing changes
- **Rollback** — every applied change is saved to `.marcus/sessions/latest/patches/` for rollback

```bash
$ marcus rollback                # Undo last Marcus edit
$ marcus rollback --steps 3      # Undo last 3 Marcus edits
```

### 5.5 Test-Driven Mode

In TDD mode, Marcus generates tests first, then implementation:

```bash
$ marcus tdd "create a currency converter that handles 150+ currencies"

  Mode: Test-Driven Development
  
  Step 1/3: Writing tests...
  Step 2/3: Running tests (expect failure)...
  Step 3/3: Writing implementation to pass tests...
  
  ✓ All 12 tests passing
```

---

## 6. Memory System

### 6.1 Memory Architecture

Marcus has a **three-tier memory system**, all stored as inspectable files:

```
TIER 1: SESSION MEMORY (volatile)
  In-memory during session
  Lost when Marcus exits (unless summarized)

TIER 2: PROJECT MEMORY (persistent, local)
  .marcus/memory/
  Scoped to the project
  Committed to git (team-shared memory)

TIER 3: GLOBAL MEMORY (persistent, local)
  ~/.marcus/memory/
  Scoped to the user/machine
  Personal preferences, cross-project patterns
```

### 6.2 Memory Types

#### 6.2.1 Factual Memory
Key-value pairs of things Marcus has learned:

```
.marcus/memory/facts/
├── tech_stack.md          # "This project uses Prisma + PostgreSQL"
├── conventions.md         # "We use kebab-case for file names"
├── team.md                # "Alice owns the auth module"
└── infrastructure.md      # "Deployed on Railway, staging on branch 'staging'"
```

Each file is human-readable and human-editable. Marcus reads these on every session.

#### 6.2.2 Episodic Memory
Summaries of past sessions:

```
.marcus/memory/episodic/
├── 2026-03-15.md          # "Refactored auth to use JWT, discussed migration plan"
├── 2026-03-16.md          # "Added rate limiting, fixed the Redis connection bug"
└── 2026-03-17.md          # "Started work on payment integration, using Stripe"
```

At the end of each session, Marcus automatically generates a summary and saves it. On the next session start, it reads the last N days of episodic memory.

#### 6.2.3 Code Pattern Memory
Detected patterns Marcus has observed or been taught:

```
.marcus/memory/patterns/
├── error_handling.md      # "Always wrap DB calls in try/catch, use AppError class"
├── api_routes.md          # "Routes follow REST, versioned at /api/v1/"
└── testing.md             # "Tests use vitest, factories in tests/factories/"
```

#### 6.2.4 Decision Log
Architecture decisions for long-term context:

```
.marcus/memory/decisions/
├── ADR-001-database-choice.md
├── ADR-002-auth-strategy.md
└── ADR-003-state-management.md
```

Marcus can reference these when making suggestions that touch architectural boundaries.

### 6.3 Memory Operations

```bash
$ marcus memory list                    # Show all memories
$ marcus memory add "We use Prettier with 2-space indent"
$ marcus memory forget "old convention" # Remove a memory
$ marcus memory search "auth"           # Search memories
$ marcus memory edit                    # Open memory files in $EDITOR
$ marcus memory sync                    # Re-index memory from files
```

### 6.4 Memory Injection Strategy

Marcus uses a **relevance-weighted** injection strategy. Not all memory is injected into every prompt — that would waste tokens. Instead:

1. **Always injected:** facts, tech_stack, conventions (small, high-signal)
2. **Recency-injected:** last 3 episodic summaries
3. **Query-injected:** memory entries that match the current task (via keyword or embedding similarity)
4. **Explicitly injected:** user can pin specific memory entries with `@memory`

### 6.5 Memory from Conversation

Marcus passively learns during conversation. When you tell Marcus something new:

> *"By the way, we switched from Mongoose to Prisma last month"*

Marcus will offer to save this:

```
  ℹ  I noticed a new fact. Save to memory?
  "The project migrated from Mongoose to Prisma"
  [y/N/edit]
```

You can also force-save with `!remember <text>` during any conversation.

---

## 7. Provider Management

### 7.1 Provider Architecture

Every provider is a folder in `providers/`:

```
~/.marcus/providers/
├── anthropic/
│   ├── provider.toml
│   ├── models.toml
│   └── adapter.js             # Optional: custom adapter override
├── openai/
│   ├── provider.toml
│   └── models.toml
├── groq/
│   ├── provider.toml
│   └── models.toml
├── ollama/
│   ├── provider.toml
│   └── models.toml
└── custom_azure/              # Custom enterprise endpoint
    ├── provider.toml
    └── adapter.js
```

### 7.2 Provider Definition

**provider.toml:**
```toml
[provider]
name = "anthropic"
type = "openai_compatible"     # or "native", "custom"
base_url = "https://api.anthropic.com"
auth_type = "api_key"          # or "oauth", "none", "azure_ad"
auth_env = "ANTHROPIC_API_KEY" # env var holding the key
enabled = true

[limits]
requests_per_minute = 60
tokens_per_minute = 100000
max_parallel_requests = 5

[features]
streaming = true
function_calling = true
vision = true
embeddings = false             # Use OpenAI for embeddings

[fallback]
on_rate_limit = "groq"         # Fallback provider on rate limit
on_error = "openai"            # Fallback on error
```

**models.toml:**
```toml
[[model]]
id = "claude-opus-4-6"
alias = "opus"
context_window = 200000
max_output_tokens = 32000
cost_per_1k_input = 0.015
cost_per_1k_output = 0.075
capabilities = ["code", "analysis", "vision", "long_context"]
recommended_for = ["complex_refactoring", "architecture", "review"]

[[model]]
id = "claude-sonnet-4-6"
alias = "sonnet"
context_window = 200000
max_output_tokens = 16000
cost_per_1k_input = 0.003
cost_per_1k_output = 0.015
capabilities = ["code", "analysis", "vision"]
recommended_for = ["general", "chat", "edit"]
default = true
```

### 7.3 Model Routing

Marcus has an intelligent model router that selects the best model for each task:

```toml
# marcus.toml
[routing]
strategy = "capability"        # or "cost", "speed", "manual"

[routing.rules]
# Task-based routing
"code.review" = "anthropic/opus"
"code.edit" = "anthropic/sonnet"
"code.explain" = "anthropic/sonnet"
"chat.general" = "anthropic/sonnet"
"analysis.architecture" = "anthropic/opus"
"generation.quick" = "groq/llama-3-70b"    # Fast + cheap

# Context-based routing
long_context_threshold = 50000             # tokens
long_context_model = "anthropic/sonnet"    # Best long context

# Cost controls
max_cost_per_session = 2.00               # USD
cost_alert_threshold = 1.50               # Warn before hitting limit
```

### 7.4 Provider CLI

```bash
$ marcus providers list

  ┌─ PROVIDERS ──────────────────────────────────────────────┐
  │  ✓ anthropic    claude-sonnet-4-6 (default)   ● online   │
  │  ✓ openai       gpt-4o                         ● online   │
  │  ✓ groq         llama-3-70b                    ● online   │
  │  ✗ ollama       llama3.2:latest                ○ offline  │
  └───────────────────────────────────────────────────────────┘

$ marcus providers add openrouter
$ marcus providers test anthropic
$ marcus providers set-default groq
$ marcus use claude-opus-4-6          # Switch model for session
$ marcus use --provider ollama llama3 # Switch to local model
```

### 7.5 Cost Tracking

Marcus tracks token usage and cost per session and project:

```bash
$ marcus cost

  ┌─ COST REPORT ────────────────────────────────────────────┐
  │  Today                $0.23                              │
  │  This week            $1.84                              │
  │  This month           $12.40                             │
  │                                                          │
  │  By provider:                                            │
  │    anthropic          $10.20   (82%)                     │
  │    groq               $2.20    (18%)                     │
  │                                                          │
  │  By operation:                                           │
  │    code edits         $6.10    (49%)                     │
  │    chat               $4.30    (35%)                     │
  │    reviews            $2.00    (16%)                     │
  └───────────────────────────────────────────────────────────┘
```

---

## 8. Workflow & Flow Engine

### 8.1 Flow Execution Model

A flow execution goes through these stages:

```
INPUT → CONTEXT ASSEMBLY → PROMPT RENDERING → LLM CALL → OUTPUT PARSING → ACTION
  │            │                  │               │              │           │
  │     Injects memory,     Renders Jinja2    Streams or     Parses code   Applies
  │     code index,         templates with    buffers        blocks,       diffs,
  │     file content,       all context       response       JSON, etc.    runs cmds
  │     conventions         variables
  │
  └── Validates required inputs are present
```

### 8.2 Prompt Templates

Marcus uses **Jinja2-based prompt templates** in flow `prompt.md` files:

```markdown
# Code Review

You are reviewing code changes for the project: **{{ project.name }}**.

## Project Context
{{ context.conventions }}

## Recent History
{{ memory.recent_decisions | join('\n') }}

## Changed Files
{% for file in input.changed_files %}
### {{ file.path }}
```{{ file.language }}
{{ file.content }}
```
{% endfor %}

## Diff
```diff
{{ input.diff }}
```

Review these changes for:
1. Correctness and logic errors
2. Adherence to project conventions
3. Missing error handling
4. Performance issues
5. Security concerns

{{ examples.good_review }}
```

### 8.3 Multi-Step Flows

Flows can define multiple steps that execute sequentially, passing outputs forward:

```
flows/implement_feature/
├── flow.toml
├── steps/
│   ├── 01_understand.md       # Step 1: Understand the requirement
│   ├── 02_plan.md             # Step 2: Create implementation plan
│   ├── 03_implement.md        # Step 3: Write the code
│   └── 04_test.md             # Step 4: Write tests
└── state.schema.json          # Schema for inter-step state
```

Each step can access outputs from previous steps via `{{ steps.01_understand.output }}`.

### 8.4 Workflow Orchestration

The workflow orchestrator handles:

- **Sequential execution** with output passing
- **Parallel execution** for independent steps
- **Conditional branching** based on step outputs
- **Retry logic** with configurable backoff
- **Human checkpoints** that pause and wait for user approval
- **Rollback** if a later step fails

```bash
$ marcus workflow run feature_ship

  ┌─ WORKFLOW: feature_ship ─────────────────────────────────┐
  │                                                          │
  │  [1/4] code_review        ████████████  ✓ complete       │
  │  [2/4] generate_tests     ████████░░░░  ⟳ running...     │
  │  [3/4] update_docs        ░░░░░░░░░░░░  ○ pending        │
  │  [4/4] write_pr           ░░░░░░░░░░░░  ○ pending        │
  │                                                          │
  │  Elapsed: 00:01:23   Tokens: 12,340   Cost: $0.04        │
  └───────────────────────────────────────────────────────────┘
```

### 8.5 Flow Sharing & Registry

Flows can be published to and installed from a registry:

```bash
$ marcus flow install marcus-hub/nextjs-best-practices
$ marcus flow publish ./flows/my_review
$ marcus flow list --remote          # Browse registry
$ marcus flow update --all           # Update installed flows
```

---

## 9. Interactive Console (TUI)

### 9.1 TUI Architecture

The Marcus TUI is built with **Bubble Tea** (Go) — a functional, Elm-inspired TUI framework. It provides a rich, keyboard-driven interface that feels native to the terminal.

### 9.2 TUI Layout

```
┌──────────────────────────────────────────────────────────────────┐
│ MARCUS  v1.0  ●anthropic/sonnet  ~/proj/myapp  git:main*  $0.03 │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─ CONVERSATION ──────────────────────────────────────────────┐ │
│  │                                                             │ │
│  │  ╭─ You ───────────────────────────────────────────────╮   │ │
│  │  │ Can you refactor the auth module to use refresh     │   │ │
│  │  │ tokens?                                             │   │ │
│  │  ╰─────────────────────────────────────────────────────╯   │ │
│  │                                                             │ │
│  │  ╭─ Marcus ────────────────────────────────────────────╮   │ │
│  │  │ I'll refactor the auth module. Let me first         │   │ │
│  │  │ understand the current implementation...            │   │ │
│  │  │                                                     │   │ │
│  │  │  📁 Reading: src/auth/index.ts                      │   │ │
│  │  │  📁 Reading: src/auth/middleware.ts                  │   │ │
│  │  │  🔧 Checking: memory/facts/tech_stack.md             │   │ │
│  │  │                                                     │   │ │
│  │  │ Here's my plan:                                     │   │ │
│  │  │  1. Add refresh token generation to login()         │   │ │
│  │  │  2. Create new /auth/refresh endpoint              │   │ │
│  │  │  3. Update middleware to handle token expiry        │   │ │
│  │  │                                                     │   │ │
│  │  │ Should I proceed? [y/N/view plan]                   │   │ │
│  │  ╰─────────────────────────────────────────────────────╯   │ │
│  │                                                             │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─ CONTEXT ──────────┐  ┌─ TOOLS ────────────────────────────┐ │
│  │ src/auth/index.ts  │  │ ✓ read_file     ✓ write_file       │ │
│  │ src/auth/middleware │  │ ✓ run_command   ✓ search_code      │ │
│  │ memory: auth facts  │  │ ✓ git_diff      ✗ web_search       │ │
│  └────────────────────┘  └────────────────────────────────────┘ │
│                                                                  │
├──────────────────────────────────────────────────────────────────┤
│  > _                                        [?] help  [/] cmds  │
└──────────────────────────────────────────────────────────────────┘
```

### 9.3 Status Bar

The top status bar always shows:
- **Marcus version**
- **Current provider/model** with latency indicator
- **Current directory** (project root detection)
- **Git branch** + dirty indicator
- **Session cost**

### 9.4 Input Features

The input box supports:

| Feature | Description |
|---|---|
| **Multi-line input** | `Shift+Enter` for newlines |
| **File mention** | `@src/auth.ts` to attach a file |
| **Memory mention** | `@memory:auth` to pin a memory |
| **Flow trigger** | `/flow code_review` to run a flow |
| **Slash commands** | `/edit`, `/explain`, `/fix`, `/commit` |
| **History** | `↑/↓` to navigate input history |
| **Autocomplete** | `Tab` to complete file paths, commands |
| **Inline diff** | Diffs rendered inline with syntax highlighting |
| **Yank to clipboard** | `y` in response view to copy code blocks |

### 9.5 Keyboard Navigation

```
Global:
  Ctrl+C          Exit
  Ctrl+L          Clear screen
  Ctrl+Z          Undo last action
  Ctrl+R          Retry last request
  Tab             Focus next pane
  ?               Show help overlay

Conversation pane:
  j/k             Scroll up/down
  gg/G            Jump to top/bottom
  /               Search conversation
  y               Copy selection
  Enter           Expand code block

Context pane:
  a               Add file to context
  d               Remove file from context
  c               Clear context

Command palette (Ctrl+P):
  Fuzzy search all Marcus commands, flows, tools
```

### 9.6 Slash Commands

```
/edit <file> <instruction>   Edit a file with AI
/create <path> <description> Create a new file
/explain [file[:line]]       Explain code
/fix                         Fix last error
/review                      Code review staged changes
/commit                      AI-generated commit message
/test                        Generate or run tests
/docs                        Update documentation
/flow <name> [args]          Run a specific flow
/workflow <name>             Run a workflow
/memory [add|list|forget]    Manage memory
/use <model>                 Switch model
/cost                        Show cost report
/context [add|clear|list]    Manage context window
/diff                        Show current git diff
/rollback [n]                Undo last n Marcus changes
/settings                    Open settings
/help [command]              Show help
```

### 9.7 Inline Code Rendering

Code blocks in responses are rendered with:
- **Syntax highlighting** via Chroma
- **File path header** with language badge
- **Line numbers** (toggleable)
- **Diff view** for modifications (green additions, red deletions)
- **Action buttons**: `[a]pply  [c]opy  [v]iew  [e]dit`

### 9.8 Streaming

All responses stream character-by-character. Marcus shows a **thinking indicator** during the pre-streaming phase:

```
  Marcus is thinking ···
  Reading 3 files · Checking memory · Building context
```

During streaming, tool calls are shown inline:

```
  Marcus ──────────────────────────────────────────
  Let me look at your current implementation...
  
  🔧 read_file("src/auth/index.ts")  ✓ 142 lines
  🔧 search_code("refreshToken")    ✓ 3 matches
  
  Based on what I see, the current auth system...
```

### 9.9 Notification System

Non-intrusive notifications in the top-right corner:

```
                                    ╭───────────────────╮
                                    │ ✓ Edit applied     │
                                    │   src/auth/index.ts│
                                    ╰───────────────────╯
```

Notification types: success, warning, error, info, cost-alert.

---

## 10. Tool System

### 10.1 Tool as a Folder

Every tool is a folder:

```
tools/read_file/
├── tool.toml                    # Tool definition
├── description.md               # What the tool does (shown to LLM)
├── schema.json                  # Input/output JSON schema
├── run.sh                       # Tool implementation
└── tests/
    └── test_001.toml
```

**tool.toml:**
```toml
[tool]
name = "read_file"
description = "Reads the content of a file"
version = "1.0.0"
enabled = true
timeout_seconds = 10
requires_confirmation = false
safe = true                      # Safe tools run without confirmation

[permissions]
allow_paths = ["."]              # Relative to project root
deny_paths = [".marcus/", ".env"]
```

**schema.json:**
```json
{
  "input": {
    "type": "object",
    "properties": {
      "path": { "type": "string", "description": "File path to read" },
      "line_range": {
        "type": "array",
        "items": { "type": "number" },
        "minItems": 2,
        "maxItems": 2
      }
    },
    "required": ["path"]
  },
  "output": {
    "type": "object",
    "properties": {
      "content": { "type": "string" },
      "line_count": { "type": "number" },
      "language": { "type": "string" }
    }
  }
}
```

### 10.2 Built-in Tools

Marcus ships with these core tools:

| Tool | Description | Safe |
|------|-------------|------|
| `read_file` | Read file content with optional line range | ✓ |
| `write_file` | Write/overwrite a file | Confirm |
| `edit_file` | Apply targeted edits | Confirm |
| `list_files` | List directory contents | ✓ |
| `search_code` | Grep/ast-grep code search | ✓ |
| `run_command` | Execute shell command | Confirm |
| `git_diff` | Get current git diff | ✓ |
| `git_log` | Recent commit history | ✓ |
| `git_blame` | Blame lines in a file | ✓ |
| `find_definition` | Find symbol definition | ✓ |
| `find_references` | Find symbol references | ✓ |
| `web_search` | Search the web (optional) | ✓ |
| `fetch_url` | Fetch URL content | ✓ |
| `read_image` | Read image file (vision models) | ✓ |

### 10.3 Tool Confirmation

Tools marked `requires_confirmation = true` always show:

```
  Marcus wants to run:
  ┌─────────────────────────────────────────────────────────┐
  │  run_command("npm test -- --coverage")                  │
  └─────────────────────────────────────────────────────────┘
  [y] Run  [n] Skip  [e] Edit command  [!] Always allow this
```

You can build an **allow list** for commands you trust:

```toml
# .marcus/marcus.toml
[tools.run_command]
always_allow = [
  "npm test",
  "npm run lint",
  "go test ./...",
  "cargo test"
]
```

---

## 11. Agent System

### 11.1 Agent Architecture

An agent in Marcus is a **persistent entity with a role, memory, tools, and behavior**. Unlike flows (which are single-task), agents can carry on multi-turn autonomous work.

```
agents/reviewer/
├── agent.toml                   # Agent definition
├── system_prompt.md             # Agent's system prompt
├── tools.toml                   # Tools the agent can use
├── memory.toml                  # Agent-specific memory config
└── examples/                    # Example interactions
```

**agent.toml:**
```toml
[agent]
name = "reviewer"
role = "Senior Code Reviewer"
description = "Reviews code for quality, security, and conventions"
version = "1.0.0"

[model]
provider = "anthropic"
model = "claude-opus-4-6"
temperature = 0.2

[behavior]
max_iterations = 10              # Max tool call cycles
think_aloud = true               # Show reasoning
autonomous = false               # Require human approval
checkpoint_every = 3             # Pause every N iterations

[memory]
use_project_memory = true
use_global_memory = false
save_summaries = true
```

### 11.2 Multi-Agent Coordination

Marcus supports multi-agent workflows where specialized agents collaborate:

```toml
# workflows/full_feature/dag.toml
[[step]]
id = "architect"
agent = "architect"
task = "Design the implementation approach"

[[step]]
id = "coder"
agent = "coder"
task = "Implement the design from architect"
depends_on = ["architect"]
context_from = ["architect.output"]

[[step]]
id = "reviewer"
agent = "reviewer"
task = "Review the implementation"
depends_on = ["coder"]
context_from = ["architect.output", "coder.output"]

[[step]]
id = "tester"
agent = "tester"
task = "Write tests for the implementation"
depends_on = ["coder"]
parallel_with = ["reviewer"]
```

### 11.3 Built-in Agents

| Agent | Role | Default Model |
|-------|------|---------------|
| `architect` | System design and ADRs | opus |
| `coder` | Implementation and refactoring | sonnet |
| `reviewer` | Code review and quality | opus |
| `debugger` | Bug investigation and fixing | sonnet |
| `tester` | Test generation and analysis | sonnet |
| `documenter` | Documentation writing | sonnet |
| `security` | Security analysis | opus |
| `optimizer` | Performance optimization | sonnet |

---

## 12. Context Management

### 12.1 Context Assembly Pipeline

Before every LLM call, the Context Assembler runs:

```
1. SEED CONTEXT
   └─ Load: system prompt, current flow, agent role

2. INJECT MEMORY
   └─ Load: relevant facts, conventions, recent episodes

3. INJECT CODE INDEX
   └─ Load: relevant symbols, imports for current task

4. INJECT EXPLICIT FILES
   └─ Load: @-mentioned files, open editor files (if LSP)

5. INJECT CONVERSATION HISTORY
   └─ Load: current session + (summarized) older context

6. INJECT TASK CONTEXT
   └─ Load: git diff, recently edited files, error output

7. TRIM TO FIT
   └─ Token budget enforcement, priority-based trimming

8. FINAL PROMPT
   └─ Rendered template with all context injected
```

### 12.2 Context Window Management

Marcus is context-window-aware. It tracks token usage and uses a **priority-based trimming** strategy when the context is too large:

| Priority | Content | Drop When? |
|----------|---------|------------|
| 1 (keep) | System prompt | Never |
| 2 (keep) | Current task / user message | Never |
| 3 (keep) | Current file(s) | Never |
| 4 | Active conversation (last 10 turns) | If over 80% |
| 5 | Injected memory | If over 70% |
| 6 | Code index excerpts | If over 60% |
| 7 | Older conversation history | If over 50% |
| 8 (drop) | Low-relevance context | First to drop |

```bash
$ marcus context show             # Show current context
$ marcus context size             # Show token usage
$ marcus context add src/auth.ts  # Explicitly add a file
$ marcus context clear            # Clear injected context
$ marcus context pin src/auth.ts  # Pin file (never auto-remove)
```

### 12.3 Smart File Selection

When you ask Marcus to do something, it automatically selects relevant files using:

1. **Explicit mentions** in your message (`@src/auth.ts`)
2. **Task keywords** matched against the symbol index
3. **Recent edits** (files you modified in the last hour)
4. **Import graph** (if you mention `auth`, it also includes `auth/utils`, `auth/types`)
5. **Test files** if the task mentions testing

---

## 13. Plugin Architecture

### 13.1 Plugin System

Everything in Marcus is a plugin. The core itself uses the same plugin interface as user plugins. A plugin is a folder:

```
plugins/my-plugin/
├── plugin.toml                  # Plugin manifest
├── flows/                       # Flows contributed by the plugin
├── tools/                       # Tools contributed by the plugin
├── agents/                      # Agents contributed by the plugin
├── themes/                      # TUI themes contributed
├── providers/                   # Providers contributed
└── hooks/                       # Lifecycle hooks
    ├── on_session_start.sh
    ├── on_edit_applied.sh
    └── on_session_end.sh
```

### 13.2 Plugin Installation

```bash
$ marcus plugin install github:user/marcus-plugin-github-copilot
$ marcus plugin install npm:marcus-prisma-plugin
$ marcus plugin install ./local-plugin
$ marcus plugin list
$ marcus plugin enable my-plugin
$ marcus plugin disable my-plugin
$ marcus plugin update --all
```

### 13.3 First-Party Plugins

These are maintained by the Marcus core team:

| Plugin | Description |
|--------|-------------|
| `marcus-github` | GitHub PRs, issues, workflows |
| `marcus-linear` | Linear issues and projects |
| `marcus-notion` | Notion pages and databases |
| `marcus-docker` | Docker container management |
| `marcus-aws` | AWS CLI integration |
| `marcus-vercel` | Vercel deployments |
| `marcus-jest` | Jest test runner integration |
| `marcus-storybook` | Storybook component flows |

---

## 14. Security Model

### 14.1 Principle of Least Privilege

Every tool, flow, and agent declares exactly what it needs. Marcus enforces these declarations:

```toml
# tools/run_command/tool.toml
[permissions]
allow_commands = ["npm", "npx", "node", "git"]
deny_commands = ["rm -rf", "sudo", "curl | bash"]
allow_env_vars = ["NODE_ENV", "PORT"]
deny_env_vars = ["*_KEY", "*_SECRET", "*_TOKEN"]
allow_network = false
allow_fs_write = true
allow_fs_read = true
```

### 14.2 Secret Management

Marcus never sends API keys or secrets to LLMs. The Context Assembler has a **secret scrubber** that:

1. Reads `.env` and `.gitignore` patterns
2. Removes any matching strings from context before sending
3. Warns if it detects potential secrets in user messages

```toml
# marcus.toml
[security]
scrub_env_files = [".env", ".env.local", ".env.production"]
scrub_patterns = ["sk-*", "ghp_*", "AKIA*"]
warn_on_secret_in_input = true
```

### 14.3 Audit Log

Every action Marcus takes is logged to `.marcus/audit.log`:

```
2026-03-18T10:23:41Z  read_file       src/auth/index.ts
2026-03-18T10:23:41Z  read_file       src/auth/middleware.ts
2026-03-18T10:23:43Z  llm_call        anthropic/sonnet  4231 tokens
2026-03-18T10:23:51Z  write_file      src/auth/index.ts  (confirmed)
2026-03-18T10:23:51Z  write_file      src/auth/middleware.ts  (confirmed)
```

---

## 15. Configuration Reference

### 15.1 Global config (`~/.marcus/config.toml`)

```toml
[marcus]
version = "1.0.0"
telemetry = false                # Never send usage data
auto_update = true

[defaults]
provider = "anthropic"
model = "claude-sonnet-4-6"
theme = "marcus-dark"
editor = "$EDITOR"

[memory]
auto_save_facts = true           # Offer to save detected facts
auto_summarize_session = true    # Save episodic summary on exit
max_episodic_days = 30           # How far back to load episodic memory
embedding_provider = "none"      # "openai" for semantic search

[session]
auto_restore = false             # Restore last session on start
history_limit = 1000             # Max commands in history

[ui]
streaming = true
syntax_highlight = true
line_numbers = true
diff_style = "unified"           # or "side-by-side"
show_token_count = true
show_cost = true
```

### 15.2 Project config (`.marcus/marcus.toml`)

```toml
[project]
name = "my-app"
description = "E-commerce platform"

[defaults]
provider = "anthropic"
model = "claude-sonnet-4-6"

[context]
always_include = [
  ".marcus/context/ARCHITECTURE.md",
  ".marcus/context/CONVENTIONS.md"
]
auto_include_patterns = [
  "src/**/*.ts",
  "!src/**/*.test.ts"
]

[tools.run_command]
always_allow = ["npm test", "npm run lint"]

[routing.rules]
"code.review" = "anthropic/opus"
"chat.general" = "groq/llama-3-70b"

[security]
scrub_env_files = [".env", ".env.local"]
```

---

## 16. CLI Reference

```
marcus                           Start interactive TUI
marcus chat                      Start chat (alias for marcus)
marcus <message>                 One-shot message, no TUI

marcus edit <file> <instruction>
marcus create <path> <description>
marcus explain [file[:line]]
marcus fix [--from-last-error]
marcus review [--staged|--branch <branch>]
marcus commit [--dry-run]
marcus test [--generate|--run|--coverage]
marcus docs [--generate|--update]

marcus flow run <name> [args]
marcus flow list
marcus flow install <source>
marcus flow create <name>
marcus flow edit <name>
marcus flow test <name>

marcus workflow run <name>
marcus workflow list
marcus workflow visualize <name>

marcus memory list
marcus memory add <text>
marcus memory forget <text>
marcus memory search <query>
marcus memory edit

marcus providers list
marcus providers add <name>
marcus providers test [name]
marcus providers set-default <name>

marcus context show
marcus context add <file>
marcus context clear

marcus cost [--today|--week|--month]
marcus sessions list
marcus sessions restore [id]

marcus plugin install <source>
marcus plugin list
marcus plugin enable <name>
marcus plugin disable <name>

marcus init                      Initialize Marcus in current project
marcus init --global             Re-initialize global Marcus config
marcus update                    Update Marcus to latest version
marcus upgrade                   Upgrade all plugins and flows

Global flags:
  --model <model>                Override model for this command
  --provider <provider>          Override provider for this command
  --no-confirm                   Skip confirmations (dangerous!)
  --dry-run                      Show what would happen, don't apply
  --verbose                      Show debug info
  --json                         Output as JSON (for scripting)
```

---

## 17. Roadmap

### v1.0 — Foundation
- [ ] Folder-driven flow and workflow engine
- [ ] Multi-provider support (Anthropic, OpenAI, Groq, Ollama)
- [ ] Three-tier memory system
- [ ] Interactive TUI (Bubble Tea)
- [ ] Core tools (read/write/edit/search/run)
- [ ] Code index (Tree-sitter)
- [ ] Session management
- [ ] Cost tracking

### v1.1 — Intelligence
- [ ] Semantic memory (embeddings-based recall)
- [ ] Proactive suggestions (Marcus notices things)
- [ ] Pattern learning (adapts to your codebase style over time)
- [ ] Test failure auto-fix loop

### v1.2 — Collaboration
- [ ] Shared team memory (synced via git or server)
- [ ] Flow sharing registry (marcus-hub)
- [ ] Multi-user sessions (pair programming with AI)
- [ ] Review request integration (GitHub/GitLab)

### v1.3 — Ecosystem
- [ ] LSP server (editor integrations: Neovim, VSCode, Zed)
- [ ] Web dashboard (browser-based TUI alternative)
- [ ] Marcus Cloud (hosted memory, shared flows, team plans)
- [ ] Mobile companion (review and approve on mobile)

### v2.0 — Autonomy
- [ ] Long-horizon task agent (multi-day autonomous work)
- [ ] Self-improving flows (Marcus optimizes its own prompts)
- [ ] Cross-project reasoning (insights across your whole workspace)
- [ ] Voice interface (terminal + voice)

---

## Appendix A: Design Rationale

### Why Go?
Marcus is implemented in Go. Go compiles to a single binary, has excellent TUI libraries (Bubble Tea, Lip Gloss), fast startup time, and strong concurrency support for parallel agent execution.

### Why Folders over Databases?
Databases are opaque. Folders are git-friendly, human-readable, editor-friendly, and diffable. The entire Marcus state can be versioned, reviewed in PRs, and shared with a team. When something breaks, you `cat` a file to debug it.

### Why Jinja2 for prompts?
Prompt templates need conditionals, loops, and variable injection. Jinja2 is the most widely understood templating language among developers. Alternatives (Handlebars, Liquid) were considered but Jinja2's Python-like syntax is more familiar to the target audience.

### Why Bubble Tea for the TUI?
Bubble Tea's Elm-architecture model makes complex interactive TUIs predictable and testable. The reactive model handles streaming output elegantly. Lip Gloss provides CSS-like styling that makes the TUI look polished without reinventing rendering.

### Why not a GUI?
Terminal-first is not a compromise — it's a philosophy. CLI tools compose. They work over SSH. They work in CI. They work with every editor. A GUI would require a separate install, a browser, or an Electron app. The terminal is already open.

---

## Appendix B: Comparison with Claude Code

| Feature | Claude Code | Marcus |
|---------|-------------|--------|
| Provider | Anthropic only | Any (Claude, GPT, Gemini, Local) |
| Behavior customization | Limited | Full (folder-based flows) |
| Memory | Session-only | Three-tier persistent memory |
| Workflows | No | Full DAG orchestration |
| Extensibility | No plugin system | Full plugin architecture |
| State inspection | Opaque | Every file is inspectable |
| Team sharing | No | Via git (memory, flows, agents) |
| Cost tracking | No | Full cost dashboard |
| Offline/local | No | Yes (Ollama adapter) |
| Flow registry | No | marcus-hub |
| Multi-agent | No | Yes |

Marcus is not a replacement for Claude Code — it's the tool you build when you want the *same idea* but with full control, transparency, and composability.

---

*This document is a living PRD. File issues and PRs at `github.com/marcus-ai/marcus`.*