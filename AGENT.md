# Project map (development-only)

## Stack
- Language: Go
- Interface: Bubble Tea terminal UI
- Runtime shape: file-backed `.marcus/` configuration, flows, agents, tasks, sessions, and memory

## Core directories
- `internal/tui/` -> transcript rendering, approvals, streaming agent loop
- `internal/provider/` -> provider adapters plus request/runtime translation
- `internal/context/` -> assembled prompt context from docs, git state, tasks, and attached files
- `internal/flow/` -> flows, loop engine, and flow execution
- `internal/folder/` -> discovers agents, flows, tools, and memory from `.marcus/`
- `internal/tool/` -> read/write/command/search tool implementations

## Key files
- `internal/tui/tui.go` -> main interactive coding loop and transcript behavior
- `internal/tui/styles.go` -> transcript card rendering
- `internal/provider/runtime.go` -> converts structured requests into provider calls
- `internal/provider/anthropic.go` -> Anthropic request/stream handling
- `.marcus/agents/coding_agent/system.md` -> coding-agent behavior rules
- `.marcus/agents/general_agent/system.md` -> analysis/chat behavior rules
- `.marcus/marcus.toml` -> project runtime config

## Patterns in use
- Keep the transcript readable; do not surface raw machine JSON when a concise summary will do.
- Prefer targeted reads/searches over broad repository scans.
- Keep runtime behavior file-backed and inspectable inside `.marcus/`.
- Favor small, reviewable diffs and explicit verification after writes.

## Current focus
- Make Marcus feel closer to Claude Code in terminal UX.
- Strengthen the agent’s engineering discipline with map-first, methodical behavior.

## Map maintenance
- Update this file when you discover durable project structure, a load-bearing file, a recurring gotcha, or a new pattern worth preserving.
- This file is optional and intended for development convenience, not required runtime context.
