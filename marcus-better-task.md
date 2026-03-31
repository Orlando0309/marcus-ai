# Marcus > OpenCode — Implementation Plan

> **Goal:** Surpass OpenCode's capabilities by implementing missing core features while leveraging Marcus's existing advantages.

---

## Phase 1 — Core Loop Parity ✅ COMPLETE

### 1.1 Fix LoopEngine Tool-Call Integration ✅

**Status:** COMPLETE - Verified that TUI already has working streaming tool-call integration in `internal/tui/agent_loop.go`.

**What we found:**
- Streaming works via `m.providerRuntime.Stream()` (agent_loop.go:274)
- Tool calls extracted from `event.ToolCall` (agent_loop.go:310)
- Tool results fed back into messages for multi-turn execution

---

### 1.2 Doom Loop Detection ✅

**Status:** COMPLETE

**Files Created:**
- `internal/tool/doom_detector.go` — Hash-based cycle detection
- `internal/tool/doom_detector_test.go` — Full test coverage

**Files Modified:**
- `internal/config/loader.go` — Added `DoomLoopConfig` with window_size, threshold, ask_on_detect
- `internal/tui/model.go` — Added `doomDetector` field to Model
- `internal/tui/agent_loop.go` — Integrated doom check on every tool call

**Acceptance Criteria:**
```toml
# .marcus/marcus.toml
[autonomy.doom_loop]
enabled = true
window_size = 20
threshold = 3
ask_on_detect = true
```

---

### 1.3 FileTime Mtime Locking ✅

**Status:** COMPLETE

**Files Created:**
- `internal/file/time.go` — mtime tracking + assertion
- `internal/file/time_test.go` — Full test coverage

**Files Modified:**
- `internal/tool/tool.go` — Added file.Track() after read_file, file.Assert() before write_file
- `internal/tool/extra_files.go` — Added file.Assert()/Track() for patch_file, edit_file

**Acceptance Criteria:**
```bash
# User edits file while Marcus is thinking
# Marcus tries to apply edit → blocked with clear error
# "File test.go was modified externally. Re-reading..."
```

---

## Phase 2 — LSP Parity

### 2.1 Lazy LSP Client Pool

**Current State:** `broker.go` has basic client management but no lazy spawning.

**Target:** Spawn servers on-demand when file with matching extension is accessed.

**Files to Modify:**
- `internal/lsp/broker.go` — Add client pool, lazy spawning
- `internal/lsp/server.go` (new) — Define 40+ language servers

**Tasks:**
- [ ] Add `LSPConfig` per-language (command, extensions, root markers)
- [ ] Implement `NearestRoot()` walker (find package.json, tsconfig.json, etc.)
- [ ] Lazy client creation in `broker.clientFor()`
- [ ] Track client state (starting, ready, failed)

**Acceptance Criteria:**
```bash
# Open a .ts file → typescript-language-server spawns automatically
# Open a .go file → gopls spawns automatically
# No server running until file is accessed
```

---

### 2.2 Diagnostics Polling + Feedback Loop

**Current State:** No diagnostics feedback to agent.

**Target:** After file edits, wait for fresh diagnostics and feed errors back to model.

**Files to Modify:**
- `internal/lsp/broker.go` — Add `WaitForDiagnostics()`
- `internal/flow/loop.go` — Inject diagnostics after tool execution

**Tasks:**
- [ ] Subscribe to `textDocument/publishDiagnostics` notifications
- [ ] `LSP.WaitForDiagnostics(path, timeout)` — block for fresh errors
- [ ] After `write_file`/`edit_file`, poll for diagnostics
- [ ] Inject errors as user messages: "ERROR [line:col] message"

**Acceptance Criteria:**
```bash
# Marcus writes code with type error
# LSP sends diagnostic
# Marcus sees: "ERROR [12:5] Type 'string' is not assignable to 'number'"
# Marcus corrects the code automatically
```

---

### 2.3 Auto-Download Language Servers

**Current State:** User must manually install LSP servers.

**Target:** Auto-download and install missing language servers.

**Files to Create:**
- `internal/lsp/installer.go` — Auto-download logic

**Tasks:**
- [ ] Detect missing server (command not found)
- [ ] Download from GitHub releases / npm / go install
- [ ] Cache in `~/.marcus/lsp/`
- [ ] Respect `OPENCODE_DISABLE_LSP_DOWNLOAD=true`

**Acceptance Criteria:**
```bash
# User opens Python project without pyright
# Marcus downloads pyright automatically
# Server starts within 30 seconds
```

---

## Phase 3 — Multi-Agent Parity

### 3.1 Task Tool with Isolated Child Sessions

**Current State:** `task_tool.go` exists but doesn't create isolated sessions.

**Target:** Spawn child sessions with own context, permissions, step counter.

**Files to Modify:**
- `internal/tool/task_tool.go` — Create child session, run subagent loop
- `internal/session/store.go` — Support parent-child session linking

**Tasks:**
- [ ] `task` tool creates new `Session` with `parentSessionId`
- [ ] Child gets isolated message history
- [ ] Child inherits restricted permissions
- [ ] Child has own step counter (`steps` limit)
- [ ] Return result as `tool_result` to parent

**Acceptance Criteria:**
```bash
# User: "Search the codebase for all API endpoints"
# → build agent spawns @general subagent
# → general runs independently
# → Result returned to build agent
# → build agent continues with result
```

---

### 3.2 Per-Agent Permission System

**Current State:** Basic `RunCommandPolicy` with blocklist only.

**Target:** Per-agent permissions (allow/deny/ask) with glob patterns.

**Files to Create:**
- `internal/agent/permission.go` — Permission rule engine
- `internal/agent/permission_test.go`

**Files to Modify:**
- `internal/safety/policy.go` — Integrate permission checks
- `internal/tool/tool.go` — Check permission before tool execution

**Tasks:**
- [ ] Define `PermissionRule` struct (tool, pattern, action)
- [ ] Merge permissions: defaults → global config → agent-specific
- [ ] Support glob patterns: `"*.env" → "ask"`, `"src/**" → "allow"`
- [ ] Add `ask` mode (prompt user before executing)

**Acceptance Criteria:**
```toml
# .marcus/marcus.toml
[agent.build.permission]
"*.env" = "deny"
"src/**" = "allow"
"bash" = "ask"

[agent.plan.permission]
edit = "deny"
bash = "deny"
read = "allow"
```

---

### 3.3 Permission Inheritance + Recursion Guards

**Current State:** No recursion protection.

**Target:** Subagents cannot spawn further subagents unless explicitly allowed.

**Files to Modify:**
- `internal/tool/task_tool.go` — Check `task` permission before spawning

**Tasks:**
- [ ] Check if agent has explicit `task` permission
- [ ] Default: subagents have NO task permission (one-level nesting)
- [ ] Allow explicit override: `"task": "allow"` in agent config
- [ ] Warn about unlimited recursion risk in docs

**Acceptance Criteria:**
```bash
# build (primary) spawns general (subagent)
# general tries to spawn another subagent → blocked
# "Subagents cannot spawn further subagents by default"
```

---

## Phase 4 — Surpass OpenCode

### 4.1 Enhanced Context Budgeting (Marcus Advantage)

**Current State:** `ContextBudget` exists with token estimation and auto-compact.

**Target:** Enhance with relevance scoring, section dropping.

**Files to Modify:**
- `internal/context/budget.go` — Add relevance scoring
- `internal/context/assembler.go` — Drop low-relevance sections

**Tasks:**
- [ ] Score context sections by relevance to goal
- [ ] Drop lowest-relevance sections when over budget
- [ ] Track dropped sections in `Snapshot.DroppedSections`

---

### 4.2 Self-Correction Enhancements (Marcus Advantage)

**Current State:** `SelfCorrectionEngine` exists with basic retry logic.

**Target:** LLM-powered error diagnosis and fix generation.

**Files to Modify:**
- `internal/flow/self_correction.go` — Add LLM-powered fixes

**Tasks:**
- [ ] Send error + original action to LLM
- [ ] Request specific fix suggestion
- [ ] Apply fix and retry
- [ ] Track success rate in memory

---

### 4.3 Vector-Store Semantic Search (Marcus Advantage)

**Current State:** `vector_store.go` with Chroma/pgvector backends.

**Target:** Semantic code search powered by embeddings.

**Files to Modify:**
- `internal/tool/semantic_search.go` — Integrate with tool system
- `internal/codeintel/embeddings.go` — Generate embeddings on file changes

**Tasks:**
- [ ] Generate embeddings for all code files
- [ ] `semantic_search` tool queries vector store
- [ ] Return top-K relevant files for queries
- [ ] Integrate into context assembler

---

## Summary Checklist

### Phase 1 — Core Loop Parity
- [ ] 1.1 LoopEngine tool-call integration
- [ ] 1.2 Doom loop detection
- [ ] 1.3 FileTime mtime locking

### Phase 2 — LSP Parity
- [ ] 2.1 Lazy LSP client pool
- [ ] 2.2 Diagnostics polling + feedback
- [ ] 2.3 Auto-download language servers

### Phase 3 — Multi-Agent Parity
- [ ] 3.1 Task tool with isolated child sessions
- [ ] 3.2 Per-agent permission system
- [ ] 3.3 Permission inheritance + recursion guards

### Phase 4 — Surpass OpenCode
- [ ] 4.1 Enhanced context budgeting
- [ ] 4.2 Self-correction enhancements
- [ ] 4.3 Vector-store semantic search

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        USER (TUI / CLI)                          │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                    ┌────────▼────────┐
                    │   Session       │  ← message history, agent state
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  LoopEngine     │  ← ReAct loop (Phase 1)
                    │  (prompt.ts)    │
                    └────────┬────────┘
           ┌─────────────────┼──────────────────┐
           │                 │                  │
    ┌──────▼──────┐  ┌───────▼──────┐  ┌───────▼──────┐
    │ AI Provider │  │  Tool Layer  │  │  LSP Layer   │
    │ (Claude,    │  │  bash, edit, │  │  diagnostics,│
    │  OpenAI,    │  │  read, write,│  │  hover, def, │
    │  Gemini…)   │  │  task, lsp…  │  │  symbols     │
    └─────────────┘  └───────┬──────┘  └──────────────┘
                             │
                    ┌────────▼────────┐
                    │  Task Tool      │  ← spawns child sessions (Phase 3)
                    │  (Delegation)   │
                    └────────┬────────┘
                    ┌────────▼────────────────┐
                    │  Child Session          │
                    │  (subagent: general,    │
                    │   explore, custom…)     │
                    └─────────────────────────┘
```

---

## Next Steps

1. **Start Phase 1** — Begin with 1.1 (LoopEngine tool-call integration)
2. **Test incrementally** — Verify each sub-task works before moving on
3. **Document as we go** — Update this file with lessons learned
