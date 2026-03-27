# Phase 3 Roadmap: Surpassing Claude Code

**Branch:** `feature/phase3-parity`
**Goal:** Achieve feature parity with Claude Code, then leverage Marcus's multi-agent architecture to surpass it.

---

## Overview

This roadmap tracks the implementation of critical features needed to match and exceed Claude Code's capabilities. Marcus already has superior architecture (multi-agent native, folder-driven, hooks system) - this phase adds the user-facing conveniences and ecosystem integrations.

---

## P0: Critical Path (Must Have)

### 1. Skills System (`internal/skill/`)

**Goal:** User-invocable slash commands like `/commit`, `/review-pr`, `/help`

**Tasks:**
- [ ] Create `internal/skill/` package with registry
- [ ] Implement skill interface: `Skill { Name(), Pattern(), Run(ctx, args) }`
- [ ] Add slash command parser to TUI input handler
- [ ] Build skill discovery from `~/.marcus/skills/` and `.marcus/skills/`
- [ ] Implement core skills:
  - [ ] `/commit` - Analyze diff, suggest message, create commit
  - [ ] `/help` - Show available commands and shortcuts
  - [ ] `/clear` - Clear conversation history
  - [ ] `/model <name>` - Switch provider/model dynamically
  - [ ] `/undo` - Quick undo last operation
  - [ ] `/status` - Show current session status
- [ ] Add skill documentation to help system
- [ ] Tests for skill registry and execution

**Files to Create:**
- `internal/skill/registry.go`
- `internal/skill/skill.go` (interface)
- `internal/skill/builtin/commit.go`
- `internal/skill/builtin/help.go`
- `internal/skill/builtin/clear.go`
- `internal/skill/builtin/model.go`
- `internal/skill/builtin/undo.go`
- `internal/skill/builtin/status.go`
- `internal/skill/parser.go`

---

### 2. MCP (Model Context Protocol) Integration (`internal/mcp/`)

**Goal:** Connect to MCP servers for external tool ecosystem

**Tasks:**
- [ ] Research MCP protocol specification
- [ ] Create `internal/mcp/` package
- [ ] Implement MCP client (stdio and SSE transports)
- [ ] Add server discovery from `~/.marcus/mcp.json`
- [ ] Bridge MCP tools to Marcus Tool system
- [ ] Add MCP tool listing to system prompts
- [ ] Implement 3 reference MCP integrations:
  - [ ] Filesystem MCP server
  - [ ] Git MCP server
  - [ ] Fetch MCP server
- [ ] Error handling for unavailable MCP servers
- [ ] Configuration UI in TUI

**Files to Create:**
- `internal/mcp/client.go`
- `internal/mcp/transport.go`
- `internal/mcp/discovery.go`
- `internal/mcp/bridge.go` (connects to tool system)
- `internal/mcp/config.go`

---

### 3. Remote Triggers (`internal/scheduler/`)

**Goal:** Scheduled agents, cron jobs, webhook triggers

**Tasks:**
- [ ] Create `internal/scheduler/` package
- [ ] Implement cron parser and scheduler
- [ ] Add trigger persistence (JSON file based)
- [ ] Build `/schedule` skill for TUI
- [ ] Implement trigger types:
  - [ ] Cron-based ("daily at 9am")
  - [ ] File-based (on git commit, on file change)
  - [ ] Webhook-based (HTTP endpoint)
- [ ] Background execution with proper isolation
- [ ] Results notification system
- [ ] `/triggers` skill to list/manage triggers
- [ ] Tests for scheduler and trigger execution

**Files to Create:**
- `internal/scheduler/scheduler.go`
- `internal/scheduler/trigger.go`
- `internal/scheduler/cron.go`
- `internal/scheduler/persistence.go`
- `internal/scheduler/webhook.go`
- `internal/skill/builtin/schedule.go`
- `internal/skill/builtin/triggers.go`

---

## P1: Feature Parity (Should Have)

### 4. Enhanced TUI Visual Polish (`internal/tui/`)

**Goal:** Match Claude Code's visual feedback and polish

**Tasks:**
- [ ] Implement badge system:
  - [ ] ✅ Success badges for applied edits
  - [ ] 🔴 Error badges with retry option
  - [ ] ⏳ Loading/progress indicators
  - [ ] 📎 File attachment badges
  - [ ] 🔗 Link badges for URLs
- [ ] Add syntax highlighting in diff previews
- [ ] Implement rich artifact rendering (multi-file)
- [ ] Add hover tooltips for actions
- [ ] Improve streaming response smoothness
- [ ] Add keyboard shortcut hints overlay (`/?`)
- [ ] Theme improvements (consistent colors)
- [ ] Compact view mode for small terminals
- [ ] Tests for new UI components

**Files to Modify:**
- `internal/tui/view.go` - Add badge rendering
- `internal/tui/styles.go` - New badge styles
- `internal/tui/model.go` - Badge state management
- `internal/tui/update.go` - Badge lifecycle

**Files to Create:**
- `internal/tui/badge.go` (badge system)
- `internal/tui/syntax.go` (syntax highlighting)
- `internal/tui/overlay.go` (help overlay)

---

### 5. Context Window Expansion (`internal/context/`)

**Goal:** Support 200K+ token context windows

**Tasks:**
- [ ] Increase default `MaxContextTokens` to 200K
- [ ] Implement sliding window for conversation history
- [ ] Add smart truncation (preserve system/first messages)
- [ ] Implement context compaction strategies:
  - [ ] Summarize old conversation turns
  - [ ] Compress file contents with embeddings
  - [ ] Priority-based section dropping
- [ ] Add token count display in TUI status bar
- [ ] Warn at 80%, compact at 90%, truncate at 95%
- [ ] Tests for context management

**Files to Modify:**
- `internal/context/assembler.go`
- `internal/context/budget.go`
- `internal/config/loader.go` (new defaults)

---

### 6. Plan Mode Enhancement (`internal/planner/`)

**Goal:** Interactive planning mode for complex tasks

**Tasks:**
- [ ] Enhance existing planner with confirmation steps
- [ ] Add `/plan` skill for explicit plan mode
- [ ] Implement interactive plan editing:
  - [ ] Add/remove steps
  - [ ] Reorder steps
  - [ ] Edit step descriptions
- [ ] Plan persistence across sessions
- [ ] Plan templates (common patterns)
- [ ] Visual plan timeline in TUI
- [ ] Export plan to markdown

**Files to Modify:**
- `internal/planner/decomposer.go`
- `internal/tui/view.go` (plan display)

**Files to Create:**
- `internal/skill/builtin/plan.go`

---

## P2: Differentiation (Nice to Have)

### 7. Advanced Multi-Agent Features

**Goal:** Leverage Marcus's native multi-agent architecture

**Tasks:**
- [ ] Implement parallel workflow execution
- [ ] Add agent marketplace (folder-based sharing)
- [ ] Create specialized agents:
  - [ ] Security reviewer agent
  - [ ] Performance optimizer agent
  - [ ] Documentation writer agent
  - [ ] Test generator agent
- [ ] Agent-to-agent delegation protocol
- [ ] Collective intelligence (agent voting)

---

### 8. Memory System Enhancement

**Goal:** Automatic fact extraction and semantic memory

**Tasks:**
- [ ] Automatic fact extraction from conversations
- [ ] Episodic memory (conversation summaries)
- [ ] Pattern memory (code pattern recognition)
- [ ] Memory browser in TUI (`/memory` skill)
- [ ] Memory search and management
- [ ] Export/import memory

---

## Implementation Order

```
Week 1-2:  Skills System (Core framework + 3 skills)
Week 3-4:  MCP Integration (Basic client + filesystem bridge)
Week 5-6:  Remote Triggers (Cron + persistence)
Week 7-8:  Visual Polish (Badges + artifacts)
Week 9-10: Context Expansion + Plan Mode
Week 11+:  Multi-Agent features + Memory enhancements
```

---

## Success Criteria

- [ ] User can type `/commit` and get a suggested commit message
- [ ] MCP filesystem server tools appear in tool list
- [ ] Scheduled trigger fires and executes in background
- [ ] Success/error badges appear on tool operations
- [ ] Context window handles 200K tokens without truncation errors
- [ ] All new features have tests with >80% coverage

---

## Notes

- Keep folder-driven architecture - all configs in files
- Each major feature gets its own sub-package
- Maintain backward compatibility with existing flows/tools
- Document breaking changes in CHANGELOG.md

---

**Last Updated:** 2026-03-27
**Next Review:** When P0 items complete
