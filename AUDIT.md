# Marcus AI Audit: Gap Analysis vs Claude Code

**Date:** 2026-03-19
**Purpose:** Identify what Marcus lacks to match Claude Code's capabilities

---

## Executive Summary

Marcus has completed **Phase 1** with a solid foundation: CLI, folder engine, providers, tool system, diff engine, flow engine, and TUI. However, significant gaps remain to match Claude Code's production-ready capabilities.

### Current State Assessment

| Category | Marcus Status | Claude Code Parity |
|----------|---------------|-------------------|
| Core Infrastructure | **Complete** | 90% |
| Provider Support | **Partial** | 60% |
| Tool System | **Partial** | 70% |
| Context Management | **Partial** | 50% |
| Memory System | **Basic** | 40% |
| Autonomy/Loop | **Basic** | 50% |
| Developer Experience | **Partial** | 60% |
| Production Hardening | **Missing** | 30% |

---

## 1. Provider Capabilities (CRITICAL GAP)

### What Marcus Has
- Anthropic HTTP provider
- Ollama provider (local LLMs)
- Basic streaming support
- Tool call parsing

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **OpenAI provider** | High | Blocks users without Anthropic access |
| **Groq provider** | Medium | Fast inference for coding models |
| **Gemini provider** | Low | Alternative provider option |
| **Model fallback** | High | Auto-failover when provider fails |
| **Rate limit handling** | High | Production reliability |
| **Token counting** | High | Context budget management |
| **Batch API support** | Medium | Cost optimization |
| **Response caching** | Medium | Already implemented but not tested |

**Implementation Effort:** 2-3 weeks

---

## 2. Tool System (HIGH GAP)

### What Marcus Has
- `read_file`, `write_file`, `run_command`
- `search_code`, `find_symbol`
- `list_symbols`, `find_references`
- `find_callers`, `find_implementations`
- `get_diagnostics` (LSP)
- Manifest-based custom tools

### What Marcus Lacks

| Missing Tool | Priority | Why It Matters |
|--------------|----------|----------------|
| **`patch_file`** | Critical | Claude Code edits with surgical precision, not full rewrites |
| **`list_directory`** | High | Explore unknown repos |
| **`glob_files`** | High | Find files by pattern (`**/*.go`) |
| **`edit_file`** | Critical | Line-based edits, insert/replace |
| **`create_file`** | Medium | Explicit new file creation |
| **`delete_file`** | Medium | File removal operations |
| **`run_in_background`** | High | Long-running commands don't block |
| **`fetch_url`** | Medium | API documentation, external context |
| **`git_operations`** | High | `git diff`, `git status`, `git log` as tools |
| **`test_runner`** | Medium | First-class test execution |
| **`lsp_operations`** | Medium | Go-to-definition, find-references via LSP |

**Current diff engine** is naive string replacement. Claude Code uses proper unified diff parsing and application.

**Implementation Effort:** 3-4 weeks

---

## 3. Context Management (HIGH GAP)

### What Marcus Has
- Basic context assembler
- Git state detection
- File hints from @mentions
- Session history
- TODO hint scanning

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **Token budget enforcement** | Critical | No hard cap on context size |
| **Relevance scoring** | Critical | Everything is included, not prioritized |
| **Smart file selection** | High | No auto-inclusion of related files |
| **Symbol-aware context** | High | Doesn't prioritize files with relevant symbols |
| **Context compaction** | Medium | No auto-summarization when near limit |
| **Image attachment** | Medium | Can't process screenshots |
| **Binary file handling** | Low | Graceful degradation for binaries |

**Current behavior:** Includes all available context. Claude Code uses **relevance ranking** to stay within token budgets.

**Implementation Effort:** 2-3 weeks

---

## 4. Memory System (MEDIUM GAP)

### What Marcus Has
- File-based memory storage
- User/project/reference scopes
- Basic recall with token matching
- User feedback capture

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **Episodic memory** | High | No conversation-level memory |
| **Pattern memory** | Medium | No learned behavior patterns |
| **Decision memory** | Medium | No tracking of architectural decisions |
| **Semantic search** | Medium | Keyword-only, no embeddings |
| **Memory expiration** | Low | No decay of stale memories |
| **Cross-project memory** | Low | Memory doesn't transfer between repos |
| **Memory prioritization** | Medium | All memories treated equally |

**Current behavior:** Basic key-value storage. Claude Code has **fact extraction**, **episodic recall**, and **pattern learning**.

**Implementation Effort:** 2 weeks

---

## 5. Autonomy / Loop Engine (MEDIUM GAP)

### What Marcus Has
- Iteration loop with max limit
- Tool execution with conversation history
- Stagnation detection (repeated plans)
- Step mode for pause/resume
- Basic verification (build/test detection)
- Dependency install auto-fix

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **Goal stack** | High | No tracking of multi-turn objectives |
| **Task DAG** | High | No workflow orchestration |
| **Self-correction** | Medium | Limited failure recovery |
| **Progress tracking** | Medium | No explicit progress reporting |
| **User interruption** | Medium | Can't gracefully pause mid-execution |
| **Checkpoint/restore** | Low | Can't save/restore long-running sessions |
| **Multi-agent coordination** | Low | Single agent only |

**Current behavior:** Linear iteration loop. Claude Code has **goal stacks**, **task graphs**, and **checkpoint recovery**.

**Implementation Effort:** 3-4 weeks

---

## 6. Developer Experience (MEDIUM GAP)

### What Marcus Has
- Single-pane TUI (Bubble Tea)
- Conversation transcript
- Pending action approval (y/n)
- Session persistence
- Task board
- Cooking metaphor spinner

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **Multi-pane layout** | High | No side-by-side file view |
| **Diff viewer** | Critical | Shows raw diff, not rendered |
| **File explorer** | Medium | Can't browse project tree in TUI |
| **Inline chat** | High | Can't chat within file context |
| **Slash commands** | Medium | Limited to `/newsession` |
| **Keyboard shortcuts** | Medium | Basic Tab/Ctrl+Y/N |
| **Syntax highlighting** | Medium | Transcript is plain text |
| **Undo/redo** | High | No action reversal |
| **Session search** | Low | Can't search past conversations |

**Current behavior:** Functional single-pane TUI. Claude Code has **multi-pane layouts**, **inline chat**, **rich diff rendering**.

**Implementation Effort:** 2-3 weeks

---

## 7. Production Hardening (CRITICAL GAP)

### What Marcus Has
- Basic error handling
- Path sandboxing
- Tool timeouts
- Config validation

### What Marcus Lacks

| Feature | Priority | Impact |
|---------|----------|--------|
| **Comprehensive logging** | Critical | No structured logs for debugging |
| **Metrics/telemetry** | High | No usage tracking |
| **Crash reporting** | Medium | Silent failures |
| **Graceful degradation** | High | Hard crashes on edge cases |
| **Input validation** | High | Trusts LLM output too much |
| **Security audit** | Critical | No security review |
| **Performance profiling** | Medium | No benchmarks |
| **Load testing** | Medium | Unknown scaling limits |
| **Documentation** | High | README exists, no API docs |
| **Integration tests** | Critical | Manual testing only |
| **E2E test suite** | Critical | No automated testing |

**Current behavior:** Works in happy path. Production reliability unproven.

**Implementation Effort:** 4-6 weeks

---

## 8. Missing Core Features

### 8.1 Inline Edit Mode
Claude Code can edit files **inline** within the editor. Marcus requires explicit `edit` command.

**Gap:** No LSP integration for inline edits.

### 8.2 Multi-File Operations
Claude Code handles **multi-file refactors** in a single operation. Marcus processes files sequentially.

**Gap:** No transactional multi-file changes.

### 8.3 Code Review Mode
Claude Code has dedicated **review mode** for PR analysis. Marcus has generic flows.

**Gap:** No specialized review workflow.

### 8.4 Test Generation
Claude Code auto-generates tests for new code. Marcus requires explicit flow.

**Gap:** No built-in test generation capability.

### 8.5 Documentation Generation
Claude Code generates docs from code. Marcus has no dedicated doc flow.

**Gap:** No auto-doc generation.

---

## 9. Architecture Gaps

### 9.1 Event Bus
Claude Code uses an **event bus** for decoupled communication. Marcus has direct coupling.

**Gap:** Tightly coupled components, harder to extend.

### 9.2 Plugin System
Claude Code has a **plugin SDK**. Marcus has folder-based tools only.

**Gap:** Can't extend without code changes.

### 9.3 LSP Integration
Marcus has LSP broker but **underutilized**. Claude Code uses LSP for:
- Go-to-definition
- Find-references
- Rename-symbol
- Inline type info

**Gap:** LSP only used for diagnostics.

---

## 10. Roadmap to Parity

### Phase 2 (8-12 weeks)
1. **Critical tools:** `patch_file`, `edit_file`, `glob_files`
2. **Token budget:** Context assembler with hard caps
3. **OpenAI provider:** Alternative to Anthropic
4. **Proper diff engine:** Unified diff parser/applier
5. **Integration tests:** E2E test suite

### Phase 3 (8-12 weeks)
1. **Goal stack:** Multi-turn objective tracking
2. **Memory upgrade:** Episodic + pattern memory
3. **TUI upgrade:** Multi-pane, diff viewer
4. **LSP operations:** Full LSP tool integration
5. **Production hardening:** Logging, metrics, error handling

### Phase 4 (8-12 weeks)
1. **Plugin SDK:** Extensibility layer
2. **Workflow DAG:** Task orchestration
3. **Multi-agent:** Coordinator pattern
4. **Security audit:** Formal review
5. **Documentation:** API docs, user guide

---

## Summary: What Would Make Marcus "Claude Code"

| Priority | Feature | Effort |
|----------|---------|--------|
| **P0** | Proper diff engine (unified diff parse/apply) | 1 week |
| **P0** | `patch_file` and `edit_file` tools | 1 week |
| **P0** | Token budget + relevance scoring | 2 weeks |
| **P0** | OpenAI provider | 1 week |
| **P0** | Integration/E2E tests | 2 weeks |
| **P1** | Goal stack + task tracking | 2 weeks |
| **P1** | Multi-pane TUI with diff viewer | 2 weeks |
| **P1** | Comprehensive logging | 1 week |
| **P1** | LSP operations (GTD, references) | 1 week |
| **P2** | Memory upgrade (episodic, patterns) | 2 weeks |
| **P2** | Workflow DAG orchestration | 3 weeks |

**Total to parity:** ~16-20 weeks (4-5 months)

---

## Conclusion

Marcus has a **solid foundation** with the folder-driven architecture, provider abstraction, and TUI. The gaps are **implementable**, not architectural rewrites.

**Focus areas for immediate parity:**
1. Fix diff engine (naive string replacement → proper unified diff)
2. Add surgical edit tools (`patch_file`, `edit_file`)
3. Implement token budget with relevance scoring
4. Add OpenAI provider for accessibility
5. Build E2E test suite for reliability

Marcus is **Phase 1 complete**. With focused work on the gaps above, Marcus could reach **Claude Code parity in 4-5 months**.
