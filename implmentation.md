# MARCUS — HOW IT ACTUALLY WORKS
### Implementation Deep-Dive: Folder Engine, Universal Plugins, Long-Running Autonomy & Task Tracking
**Document Type:** Engineering Implementation Guide  
**Companion to:** PRD-MARCUS v1.0.0  
**Date:** 2026-03-18

---

> *The PRD tells you what Marcus is. This document tells you how it works — the real mechanics, the tricky parts, and the decisions that make it hold together under pressure.*

---

## Table of Contents

1. [The Folder Engine — Full Mechanics](#1-the-folder-engine--full-mechanics)
2. [Making Marcus Universal, Not Just a Coder](#2-making-marcus-universal-not-just-a-coder)
3. [The Plugin Contract — How Anything Becomes a Capability](#3-the-plugin-contract--how-anything-becomes-a-capability)
4. [Long-Running Autonomy — Working All Day Without Interruption](#4-long-running-autonomy--working-all-day-without-interruption)
5. [The Loop Engine](#5-the-loop-engine)
6. [Task Tracking System](#6-task-tracking-system)
7. [The Context Assembler — Deep Implementation](#7-the-context-assembler--deep-implementation)
8. [The Conversation State Machine](#8-the-conversation-state-machine)
9. [How Streaming Works End-to-End](#9-how-streaming-works-end-to-end)
10. [Error Recovery & Resilience](#10-error-recovery--resilience)
11. [The Scheduler — Time-Based & Trigger-Based Execution](#11-the-scheduler--time-based--trigger-based-execution)
12. [Building the TUI Layer](#12-building-the-tui-layer)
13. [How Memory Learns Over Time](#13-how-memory-learns-over-time)
14. [The Wire Format — How Everything Talks](#14-the-wire-format--how-everything-talks)
15. [Implementation Order — What to Build First](#15-implementation-order--what-to-build-first)

---

## 1. The Folder Engine — Full Mechanics

### 1.1 The Problem with Config Files

Most tools use a single config file to define behavior. This breaks at scale: the file becomes huge, merging changes causes conflicts, you can't compose behaviors, and you can't understand "why is it doing that?" without reading 800 lines of YAML.

Marcus solves this with the **Folder Engine** — a runtime that treats a directory tree as an executable specification.

### 1.2 How the Folder Engine Boots

When Marcus starts, the Folder Engine does a **discovery walk**:

```
DISCOVERY WALK ALGORITHM:

1. Start from ~/.marcus/ (global scope)
2. Walk up from $CWD until .marcus/ found (project scope)
3. For each scope, in order: global → project → workspace

For each directory found:
  a. Read all *.toml files → parse into Registry
  b. Read all *.md files → parse into PromptStore
  c. Read all *.json files → parse into SchemaStore
  d. Register *.sh files → register as Hooks
  e. Register subdirectories → register as namespaced units

Scoping rule:
  Project entry OVERRIDES global entry with same name.
  Workspace entry OVERRIDES project entry with same name.
```

In Go, this looks like:

```go
type FolderEngine struct {
    globalPath   string
    projectPath  string
    registry     *Registry
    watcher      *fsnotify.Watcher
    mu           sync.RWMutex
}

func (fe *FolderEngine) Boot() error {
    // Walk all scopes in order, later scopes override earlier
    for _, scope := range []string{fe.globalPath, fe.projectPath} {
        if err := fe.walkScope(scope); err != nil {
            return err
        }
    }
    // Start hot-reload watcher
    return fe.startWatcher()
}

func (fe *FolderEngine) walkScope(root string) error {
    return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        
        rel, _ := filepath.Rel(root, path)
        parts := strings.Split(rel, string(os.PathSeparator))
        
        // Depth 1 = category (flows, tools, agents, plugins, memory...)
        // Depth 2 = unit name
        // Depth 3+ = unit internals
        
        if d.IsDir() && len(parts) == 2 {
            return fe.registerUnit(path, parts[0], parts[1])
        }
        return nil
    })
}
```

### 1.3 The Registry

The Registry is the in-memory index of everything Marcus knows how to do:

```go
type Registry struct {
    Flows     map[string]*FlowDef      // "code_review" → FlowDef
    Tools     map[string]*ToolDef      // "read_file" → ToolDef
    Agents    map[string]*AgentDef     // "reviewer" → AgentDef
    Workflows map[string]*WorkflowDef  // "feature_ship" → WorkflowDef
    Plugins   map[string]*PluginDef    // "marcus-github" → PluginDef
    Memories  map[string]*MemoryDef    // memory layer definitions
    Providers map[string]*ProviderDef  // "anthropic" → ProviderDef
    
    // Index for autocomplete and fuzzy search
    AllNames  []string
    ByTag     map[string][]string
    ByCapability map[string][]string
}
```

When you type `/flow `, Marcus autocompletes from `Registry.Flows`. When a flow references `tools = ["read_file"]`, Marcus resolves it from `Registry.Tools`. Everything is connected through this single registry.

### 1.4 Hot Reload Without Restart

The folder watcher reloads only the changed unit:

```go
func (fe *FolderEngine) handleFSEvent(event fsnotify.Event) {
    // Debounce: wait 300ms for writes to settle
    fe.debounce(300*time.Millisecond, func() {
        path := event.Name
        unitPath := fe.findUnitRoot(path)  // walk up to unit folder
        
        // Re-parse just this unit
        unit, err := fe.parseUnit(unitPath)
        if err != nil {
            fe.notify(NotifError, fmt.Sprintf("Reload failed: %v", err))
            return
        }
        
        // Swap in registry under write lock
        fe.mu.Lock()
        fe.registry.Replace(unit)
        fe.mu.Unlock()
        
        fe.notify(NotifInfo, fmt.Sprintf("Reloaded: %s", unit.Name))
    })
}
```

This means: edit `flows/code_review/prompt.md`, save it, and the next time `code_review` runs, it uses the new prompt. No restart needed.

### 1.5 Unit Validation

Every unit is validated on load against a schema. Invalid units are reported but don't crash Marcus:

```go
type ValidationResult struct {
    Unit     string
    Valid     bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

// Example validation for a flow:
func validateFlow(path string) ValidationResult {
    result := ValidationResult{Unit: path}
    
    // Required: flow.toml must exist
    if !fileExists(filepath.Join(path, "flow.toml")) {
        result.Errors = append(result.Errors, ValidationError{
            Field: "flow.toml",
            Message: "flow.toml is required",
        })
    }
    
    // Required: at least one prompt file
    prompts, _ := glob(path, "*.md")
    if len(prompts) == 0 {
        result.Errors = append(result.Errors, ValidationError{
            Field: "prompt",
            Message: "at least one .md prompt file is required",
        })
    }
    
    // Warning: no examples folder (reduces quality)
    if !dirExists(filepath.Join(path, "examples")) {
        result.Warnings = append(result.Warnings, ValidationWarning{
            Message: "no examples/ folder — few-shot quality may be lower",
        })
    }
    
    result.Valid = len(result.Errors) == 0
    return result
}
```

### 1.6 Folder Namespacing & Inheritance

Folders can inherit from other folders using `extends`:

```toml
# flows/quick_review/flow.toml
[flow]
name = "quick_review"
extends = "code_review"          # Inherits everything from code_review

[model]
temperature = 0.5                # Override just the temperature

# The prompt is also inherited unless flows/quick_review/prompt.md exists
# If prompt.md exists here, it OVERRIDES the parent's prompt
# If it doesn't, the parent's prompt.md is used
```

This lets you build a library of specialized flows without duplicating everything.

---

## 2. Making Marcus Universal, Not Just a Coder

### 2.1 The Core Insight: Capability Domains

Marcus is not a coding assistant. Marcus is a **general-purpose autonomous agent runtime** that ships with coding capabilities by default. The distinction matters: the engine has no concept of "code" baked in. It only knows:

- **Flows** (single tasks)
- **Workflows** (orchestrated tasks)
- **Tools** (actions)
- **Memory** (state)
- **Providers** (intelligence)

Coding is just one domain of flows and tools. Email drafting, document generation, calendar management, data analysis — these are other domains. Each domain is a plugin.

### 2.2 Domain Architecture

A domain is a named collection of related flows, tools, and agents:

```
plugins/
├── coding/                      ← Ships by default
│   ├── plugin.toml
│   ├── flows/
│   │   ├── edit_code/
│   │   ├── review_code/
│   │   ├── explain_code/
│   │   └── generate_tests/
│   ├── tools/
│   │   ├── read_file/
│   │   ├── write_file/
│   │   └── run_command/
│   └── agents/
│       ├── coder/
│       └── reviewer/
│
├── email/                       ← Install: marcus plugin install marcus/email
│   ├── plugin.toml
│   ├── flows/
│   │   ├── draft_email/
│   │   ├── reply_email/
│   │   ├── summarize_thread/
│   │   └── triage_inbox/
│   ├── tools/
│   │   ├── send_email/          ← Uses SMTP/Gmail API
│   │   ├── read_inbox/
│   │   └── search_emails/
│   └── agents/
│       └── email_assistant/
│
├── writing/                     ← Install: marcus plugin install marcus/writing
│   ├── flows/
│   │   ├── draft_document/
│   │   ├── edit_prose/
│   │   ├── summarize/
│   │   ├── translate/
│   │   └── tone_rewrite/
│   └── tools/
│       ├── read_document/
│       └── export_to_pdf/
│
├── calendar/
├── data_analysis/
├── research/
└── social_media/
```

### 2.3 The Universal Prompt Engine

The prompt engine is domain-agnostic. It doesn't care if you're writing code or writing an email. It renders a Jinja2 template with context variables. The template decides what domain it's operating in.

This is the full variable space available to any prompt:

```jinja2
{# SYSTEM VARIABLES — Always available #}
{{ marcus.version }}             {# "1.0.0" #}
{{ marcus.session_id }}          {# "sess_20260318_102341" #}
{{ now }}                        {# "2026-03-18T10:23:41Z" #}
{{ user.name }}                  {# From global config #}
{{ user.timezone }}

{# PROJECT VARIABLES — Available when in a project #}
{{ project.name }}
{{ project.description }}
{{ project.tech_stack }}

{# MEMORY VARIABLES — Injected by context assembler #}
{{ memory.facts }}               {# All facts as a string #}
{{ memory.recent_episodes }}     {# Last N session summaries #}
{{ memory.relevant }}            {# Query-relevant memories #}
{{ memory.get("key") }}          {# Specific memory lookup #}

{# INPUT VARIABLES — Passed when flow is invoked #}
{{ input.raw }}                  {# The user's raw message #}
{{ input.files }}                {# Attached files #}
{{ input.params }}               {# Named parameters #}

{# PREVIOUS STEP OUTPUTS — In multi-step flows #}
{{ steps.step_name.output }}
{{ steps.step_name.metadata }}

{# TOOL OUTPUTS — Available after tool calls #}
{{ tools.last_result }}

{# CONVERSATION HISTORY #}
{{ conversation.last_n(5) }}     {# Last 5 turns #}
{{ conversation.summary }}       {# Auto-summary of long conversations #}

{# DOMAIN-SPECIFIC — Added by plugins #}
{{ email.thread }}               {# email plugin #}
{{ git.diff }}                   {# coding plugin #}
{{ calendar.today }}             {# calendar plugin #}
```

### 2.4 The `draft_email` Flow — A Complete Example

Here's a real, complete flow for email drafting to illustrate how non-coding domains work:

```
plugins/email/flows/draft_email/
├── flow.toml
├── prompt.md
├── context.md
├── examples/
│   ├── formal_reply.md
│   ├── sales_outreach.md
│   └── internal_update.md
└── tools.toml
```

**flow.toml:**
```toml
[flow]
name = "draft_email"
description = "Draft a professional email based on intent and context"
domain = "email"
version = "1.0.0"

[model]
provider = "default"
temperature = 0.7               # Higher for creative writing
max_tokens = 1024

[input]
required = ["intent"]           # What you want to say
optional = [
  "recipient_name",
  "recipient_role",
  "thread",                     # Existing email thread to reply to
  "tone",                       # formal, casual, assertive, warm
  "length",                     # short, medium, long
  "language",                   # fr, en, es, etc.
]

[output]
format = "email"
fields = ["subject", "body", "suggested_actions"]
save_to_memory = false

[behavior]
stream = true
confirm_before_apply = false    # Email drafts don't "apply"
show_alternatives = 2           # Generate 2 alternative versions
```

**prompt.md:**
```markdown
You are a professional communication assistant helping {{ user.name }} draft emails.

{% if memory.facts %}
## About the User
{{ memory.facts }}
{% endif %}

{% if memory.get("communication_style") %}
## Their Communication Style
{{ memory.get("communication_style") }}
{% endif %}

## Task
Draft an email with the following intent:
{{ input.intent }}

{% if input.recipient_name %}
**Recipient:** {{ input.recipient_name }}{% if input.recipient_role %}, {{ input.recipient_role }}{% endif %}
{% endif %}

**Tone:** {{ input.tone | default("professional") }}
**Length:** {{ input.length | default("medium") }}
**Language:** {{ input.language | default("en") }}

{% if input.thread %}
## Existing Thread (reply to this)
{{ input.thread }}
{% endif %}

{% if examples %}
## Examples of Good Emails in This Style
{{ examples.formal_reply }}
{% endif %}

## Instructions
1. Write a clear, compelling subject line
2. Write the email body
3. End with a clear call to action if appropriate
4. Suggest 2 follow-up actions the user might want to take

Format your response as:
**SUBJECT:** [subject line]

**BODY:**
[email body]

**SUGGESTED ACTIONS:**
- [action 1]
- [action 2]
```

**Usage:**
```bash
$ marcus flow run draft_email \
    --intent "Follow up on the proposal I sent last week, ask for their decision timeline" \
    --recipient_name "Sarah Chen" \
    --recipient_role "VP of Engineering" \
    --tone "warm but professional"
```

Or interactively:
```
> /draft-email Follow up with Sarah Chen (VP Eng) on last week's proposal, ask for timeline
```

### 2.5 Making Marcus Aware of Non-Code Domains

The engine needs to route user messages to the right domain without the user always specifying. This is done by the **Intent Classifier**, a lightweight LLM call that runs before the main response:

```go
type Intent struct {
    Domain     string    // "coding", "email", "writing", "general"
    Flow       string    // suggested flow name, if obvious
    Confidence float64   // 0.0 - 1.0
    Entities   map[string]string // extracted entities
}

func (m *Marcus) classifyIntent(userMessage string) Intent {
    // This is a fast, cheap call to a small/fast model
    // It only needs to return a JSON intent object
    
    prompt := fmt.Sprintf(`
Classify this user message into a domain and suggest the best flow.
Available domains: %s
Available flows: %s

User message: "%s"

Respond in JSON: {"domain": "...", "flow": "...", "confidence": 0.0, "entities": {}}
`, m.registry.Domains(), m.registry.FlowNames(), userMessage)

    result := m.provider.Complete(prompt, CompletionOptions{
        Model:     "groq/llama-3-8b",   // Fast & cheap for classification
        MaxTokens: 100,
        JSON:      true,
    })
    
    var intent Intent
    json.Unmarshal([]byte(result), &intent)
    return intent
}
```

When the classifier returns high confidence, Marcus routes silently. When confidence is low, it asks:

```
  > Write something to convince my boss to approve the budget

  ℹ  I can help with that. What format works best?
  
  [1] Email to your boss
  [2] Slack message
  [3] Document / memo
  [4] Presentation talking points
```

---

## 3. The Plugin Contract — How Anything Becomes a Capability

### 3.1 The Contract

Every plugin must satisfy exactly one contract: **a folder with a valid `plugin.toml`**. That's it. Everything else is optional contributions.

```toml
# plugin.toml — minimum viable plugin
[plugin]
name = "marcus-email"
version = "1.0.0"
description = "Email drafting and management"
author = "marcus-team"
marcus_version = ">=1.0.0"

# What this plugin contributes
[contributes]
flows = true
tools = true
agents = false
providers = false
themes = false
hooks = ["on_session_start"]

# What this plugin needs
[requires]
tools = []                       # No tool dependencies
env_vars = ["GMAIL_CLIENT_ID"]   # Required env vars
permissions = ["network"]        # network, filesystem, shell
```

### 3.2 How Plugins Register Capabilities

When Marcus loads a plugin, it walks the plugin's subdirectories and registers contributions into the global Registry, namespaced by plugin name:

```
Plugin: marcus-email
  flows/draft_email   → registered as "email.draft_email"
  flows/reply_email   → registered as "email.reply_email"
  tools/send_email    → registered as "email.send_email"
  tools/read_inbox    → registered as "email.read_inbox"
```

Short names also work: `draft_email` resolves to `email.draft_email` if unambiguous.

### 3.3 Tool Implementation Flexibility

Tools can be implemented in any language via shell scripts, or natively in Go via a Go interface:

**Shell-based tool (works in any language):**
```bash
# tools/send_email/run.sh
#!/bin/bash
# Marcus passes input as JSON on stdin
INPUT=$(cat)

TO=$(echo $INPUT | jq -r '.to')
SUBJECT=$(echo $INPUT | jq -r '.subject')
BODY=$(echo $INPUT | jq -r '.body')

# Use any tool you want
python3 send_gmail.py "$TO" "$SUBJECT" "$BODY"

# Output result as JSON on stdout
echo '{"sent": true, "message_id": "msg_123"}'
```

**Native Go tool (faster, better integrated):**
```go
// Implements the Tool interface
type SendEmailTool struct {
    gmailClient *gmail.Client
}

func (t *SendEmailTool) Name() string { return "send_email" }
func (t *SendEmailTool) Schema() JSONSchema { return sendEmailSchema }

func (t *SendEmailTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
    var params SendEmailParams
    json.Unmarshal(input, &params)
    
    msgID, err := t.gmailClient.Send(params.To, params.Subject, params.Body)
    if err != nil {
        return nil, fmt.Errorf("email send failed: %w", err)
    }
    
    result := SendEmailResult{Sent: true, MessageID: msgID}
    return json.Marshal(result)
}
```

### 3.4 Plugin Hooks

Plugins can hook into Marcus lifecycle events:

```
hooks/
├── on_session_start.sh          # Runs when Marcus starts
├── on_session_end.sh            # Runs when Marcus exits
├── on_edit_applied.sh           # Runs after any file edit
├── on_message_before.sh         # Runs before LLM call (can modify input)
├── on_message_after.sh          # Runs after LLM call (can modify output)
├── on_task_complete.sh          # Runs when a task finishes
├── on_error.sh                  # Runs on any error
└── on_memory_save.sh            # Runs when something is saved to memory
```

**Example:** The email plugin's `on_session_start.sh` checks for unread emails and adds them to context:

```bash
#!/bin/bash
# on_session_start.sh — called by Marcus on boot

UNREAD=$(python3 check_gmail.py --count)

if [ "$UNREAD" -gt "0" ]; then
    # Write to Marcus's context pipe
    echo "{\"type\": \"context_inject\", \"key\": \"email.unread_count\", \"value\": $UNREAD}" >&3
    echo "{\"type\": \"notification\", \"message\": \"$UNREAD unread emails\"}" >&3
fi
```

### 3.5 Plugin Isolation & Sandboxing

Plugins run in a constrained environment:

```toml
# Global security policy for plugins
[security.plugins]
allow_network = false            # Must declare network in plugin.toml
allow_shell = false              # Must declare shell in plugin.toml
allow_fs_write = false           # Must declare filesystem in plugin.toml
allow_env_access = false         # Must declare specific env vars

# Plugin must declare permissions upfront
# Marcus shows user a permission request on install:
#
# Installing marcus-email requires:
#   • Network access (to send/receive emails via Gmail API)
#   • Environment variable: GMAIL_CLIENT_ID
#   • Environment variable: GMAIL_CLIENT_SECRET
# Allow? [y/N]
```

---

## 4. Long-Running Autonomy — Working All Day Without Interruption

### 4.1 The Core Challenge

An AI that runs for 8 hours faces problems that a simple chatbot never encounters:

1. **Context window exhaustion** — conversations grow longer than any model supports
2. **Token cost explosion** — sending the full history on every call is wasteful
3. **Task drift** — the AI loses track of what it was doing after many steps
4. **Tool failure accumulation** — small errors compound over time
5. **Human attention** — user goes to lunch; what does Marcus do when stuck?
6. **State recovery** — if Marcus crashes after 3 hours, it must resume from where it was
7. **Goal coherence** — Marcus must always know "what am I working toward?"

Each of these has a specific solution in Marcus.

### 4.2 The Goal Stack

Every autonomous session starts with a **goal stack** — a structured declaration of what Marcus is working toward. This is what prevents drift.

```
.marcus/sessions/latest/
└── goal_stack.json
```

```json
{
  "root_goal": {
    "id": "goal_root_001",
    "description": "Implement the payment integration feature",
    "created_at": "2026-03-18T09:00:00Z",
    "status": "in_progress",
    "success_criteria": [
      "Stripe checkout endpoint exists at /api/payments/checkout",
      "Webhook handler exists at /api/payments/webhook",
      "All tests pass",
      "No TypeScript errors"
    ]
  },
  "active_goal": {
    "id": "goal_003",
    "parent": "goal_root_001",
    "description": "Write unit tests for the checkout endpoint",
    "status": "in_progress",
    "started_at": "2026-03-18T10:45:00Z"
  },
  "completed_goals": [
    { "id": "goal_001", "description": "Create Stripe client wrapper", "completed_at": "2026-03-18T09:30:00Z" },
    { "id": "goal_002", "description": "Implement checkout endpoint", "completed_at": "2026-03-18T10:44:00Z" }
  ],
  "blocked_goals": [],
  "next_goals": [
    { "id": "goal_004", "description": "Implement webhook handler", "depends_on": "goal_003" }
  ]
}
```

The goal stack is re-injected into context at the start of every LLM call. Marcus always knows where it is in the larger work.

### 4.3 The Working Memory Compressor

As a session grows, Marcus applies **progressive summarization** to keep context size manageable:

```
CONVERSATION COMPRESSION STAGES:

Stage 1: Full fidelity (messages 1-20)
  → All messages kept verbatim in context
  → ~10k tokens

Stage 2: Selective compression (messages 21-60)
  → Tool call inputs/outputs compressed to 1-line summaries
  → Code blocks replaced with "edited src/foo.ts (142→156 lines)"
  → Human messages kept verbatim
  → ~8k tokens for 40 messages

Stage 3: Episodic summary (messages 61+)
  → Entire block summarized into a structured episode:
    "Built Stripe client wrapper. Implemented checkout endpoint
     that creates a PaymentIntent and returns client_secret.
     Wrote 8 unit tests, all passing. Currently writing
     tests for webhook handler."
  → ~500 tokens for 60 messages

Stage 4: Goal anchoring (every time)
  → Goal stack always injected fresh
  → Last 5 messages always kept verbatim
  → Most recent code state injected fresh
```

The compressor runs automatically when context exceeds 70% of the model's window:

```go
func (ca *ContextAssembler) compressIfNeeded(conv *Conversation, budget TokenBudget) {
    currentTokens := ca.estimateTokens(conv)
    
    if currentTokens < budget.Warning {
        return // No compression needed
    }
    
    if currentTokens < budget.Critical {
        // Stage 2: compress tool outputs
        ca.compressToolOutputs(conv)
        return
    }
    
    // Stage 3: full episode summarization
    // Find the oldest uncompressed block of 40+ messages
    block := conv.OldestUncompressedBlock(40)
    if block == nil { return }
    
    summary := ca.summarizeBlock(block)  // LLM call to compress
    conv.ReplaceBlock(block, summary)
    
    // Save to episodic memory for future sessions
    ca.memory.SaveEpisode(summary)
}
```

### 4.4 The Autonomy Levels

Marcus has five autonomy levels. The user sets the level before starting a long session:

```bash
$ marcus run --autonomy supervised "implement the payment integration"
$ marcus run --autonomy autonomous "implement the payment integration"
```

| Level | Name | Behavior |
|-------|------|----------|
| 0 | **Manual** | Every action requires confirmation |
| 1 | **Supervised** (default) | Safe reads auto-approved, writes need confirmation |
| 2 | **Guided** | All file operations auto-approved, shell commands need confirmation |
| 3 | **Autonomous** | Everything auto-approved except destructive operations |
| 4 | **Headless** | Fully autonomous, no interaction, logs only |

Levels can be overridden per-tool:

```toml
# tools/run_command/tool.toml
[autonomy_overrides]
# Even at level 4, these always need confirmation
always_confirm = ["rm", "drop table", "delete from", "git push --force"]
```

### 4.5 Stuck Detection

A system that runs all day must know when it's stuck. Marcus monitors for these stuck signals:

```go
type StuckDetector struct {
    history    []Action
    thresholds StuckThresholds
}

type StuckThresholds struct {
    SameErrorCount    int           // 3 same errors → stuck
    NoProgressTime    time.Duration // 10 min no file changes → stuck
    MaxRetries        int           // 5 retries on same step → stuck
    LoopDetectWindow  int           // Same actions in last N steps → stuck
}

func (sd *StuckDetector) Check() StuckStatus {
    // Pattern 1: Same error message repeated
    if sd.countRepeatedErrors() >= sd.thresholds.SameErrorCount {
        return StuckStatus{
            Stuck: true,
            Reason: "same error repeated 3+ times",
            Suggestion: "try a different approach or ask for help",
        }
    }
    
    // Pattern 2: Action loop (doing same thing over and over)
    if sd.detectActionLoop(sd.thresholds.LoopDetectWindow) {
        return StuckStatus{
            Stuck: true,
            Reason: "detected action loop",
            Suggestion: "step back and re-evaluate the approach",
        }
    }
    
    // Pattern 3: No progress for too long
    if sd.timeSinceLastProgress() > sd.thresholds.NoProgressTime {
        return StuckStatus{
            Stuck: true,
            Reason: "no meaningful progress in 10 minutes",
            Suggestion: "the current approach may be wrong",
        }
    }
    
    return StuckStatus{Stuck: false}
}
```

When stuck, Marcus doesn't spin forever. It escalates:

```
STUCK ESCALATION PROTOCOL:

Level 1: Self-recover
  → Generate 3 alternative approaches
  → Pick the most promising
  → Try for N more iterations

Level 2: Decompose
  → Break the current task into smaller sub-tasks
  → Attempt each sub-task separately

Level 3: Ask for help (if human available)
  → Pause execution
  → Send notification to user
  → Present the problem clearly with context
  → Wait for guidance

Level 4: Park and move on (if headless)
  → Log the stuck task with full context
  → Mark as BLOCKED in task tracker
  → Move to next available task
  → Resume blocked task at next session
```

---

## 5. The Loop Engine

### 5.1 What is the Loop Engine?

The Loop Engine is the core of autonomous operation. It's a state machine that:

1. Picks the next task from the task queue
2. Assembles context for that task
3. Calls the LLM
4. Executes tool calls returned by the LLM
5. Evaluates whether the task is complete
6. Updates the task tracker
7. Goes back to step 1

It runs indefinitely until the queue is empty, it's told to stop, or it hits a blocking condition.

### 5.2 The Loop State Machine

```
                     ┌─────────────────────┐
                     │        IDLE         │
                     │  (no tasks queued)  │
                     └──────────┬──────────┘
                                │ task arrives
                                ▼
                     ┌─────────────────────┐
              ┌─────▶│     TASK_PICKUP     │◀────────────────┐
              │      │  (select next task) │                 │
              │      └──────────┬──────────┘                 │
              │                 │                            │
              │                 ▼                            │
              │      ┌─────────────────────┐                 │
              │      │  CONTEXT_ASSEMBLY   │                 │
              │      │ (build LLM context) │                 │
              │      └──────────┬──────────┘                 │
              │                 │                            │
              │                 ▼                            │
              │      ┌─────────────────────┐                 │
              │      │     LLM_CALLING     │                 │
              │      │  (stream response)  │                 │
              │      └──────────┬──────────┘                 │
              │                 │                            │
              │       ┌─────────┴──────────┐                │
              │       │                    │                 │
              │       ▼                    ▼                 │
              │  [tool calls]        [text response]         │
              │       │                    │                 │
              │       ▼                    │                 │
              │  ┌─────────────┐           │                 │
              │  │ TOOL_EXEC   │           │                 │
              │  │ (run tools) │           │                 │
              │  └──────┬──────┘           │                 │
              │         │                  │                 │
              │         ▼                  ▼                 │
              │  ┌──────────────────────────────────────┐    │
              │  │         COMPLETION_CHECK              │    │
              │  │  Is the task done? (LLM evaluation)  │    │
              │  └──────┬────────────┬───────────────────┘   │
              │         │            │                        │
              │     [done]       [not done]              [stuck]
              │         │            │                        │
              │         ▼            └────────────────────────┘
              │  ┌──────────────┐
              │  │TASK_COMPLETE │
              │  │(update state)│
              │  └──────┬───────┘
              │         │
              └─────────┘  (pick next task)
```

### 5.3 The Loop Engine Implementation

```go
type LoopEngine struct {
    taskQueue      *TaskQueue
    contextAssembler *ContextAssembler
    provider       Provider
    toolRunner     *ToolRunner
    stuckDetector  *StuckDetector
    goalStack      *GoalStack
    config         LoopConfig
    state          LoopState
    eventBus       *EventBus
}

type LoopConfig struct {
    MaxIterationsPerTask int
    MaxTotalIterations   int
    CheckpointEvery      int           // Save state every N iterations
    PauseOnHuman         bool          // Pause when human message detected
    AutonomyLevel        int
    TickInterval         time.Duration // Min time between iterations
}

func (le *LoopEngine) Run(ctx context.Context) error {
    le.state = StateRunning
    iteration := 0
    
    for {
        select {
        case <-ctx.Done():
            return le.gracefulShutdown()
        default:
        }
        
        // Checkpoint regularly
        if iteration % le.config.CheckpointEvery == 0 {
            le.checkpoint()
        }
        
        // Get next task
        task, ok := le.taskQueue.Next()
        if !ok {
            le.state = StateIdle
            le.eventBus.Emit(EventQueueEmpty{})
            
            if le.config.WaitForTasks {
                time.Sleep(le.config.TickInterval)
                continue
            }
            return nil // Done!
        }
        
        // Execute the task
        result := le.executeTask(ctx, task)
        
        // Handle result
        switch result.Status {
        case TaskDone:
            le.goalStack.Complete(task.GoalID)
            le.taskQueue.MarkDone(task.ID)
            le.memory.SaveTaskOutcome(task, result)
            
        case TaskBlocked:
            le.taskQueue.Park(task.ID, result.BlockReason)
            le.eventBus.Emit(EventTaskBlocked{Task: task, Reason: result.BlockReason})
            
        case TaskFailed:
            le.taskQueue.Fail(task.ID, result.Error)
            if le.config.StopOnFailure {
                return result.Error
            }
            
        case TaskNeedsHuman:
            le.pause(task, result.HumanQuestion)
            // Execution stops here until human responds
        }
        
        iteration++
    }
}

func (le *LoopEngine) executeTask(ctx context.Context, task *Task) TaskResult {
    taskCtx := &TaskContext{
        Task:     task,
        GoalStack: le.goalStack,
        Iteration: 0,
    }
    
    for taskCtx.Iteration < le.config.MaxIterationsPerTask {
        // Check for stuck condition
        if stuck := le.stuckDetector.Check(); stuck.Stuck {
            if err := le.handleStuck(ctx, taskCtx, stuck); err != nil {
                return TaskResult{Status: TaskBlocked, BlockReason: stuck.Reason}
            }
        }
        
        // Assemble context
        context := le.contextAssembler.Assemble(AssemblyRequest{
            Task:      task,
            GoalStack: le.goalStack,
            History:   taskCtx.History,
        })
        
        // Call LLM
        response, err := le.provider.Complete(ctx, context)
        if err != nil {
            return TaskResult{Status: TaskFailed, Error: err}
        }
        
        taskCtx.History = append(taskCtx.History, response)
        
        // Execute any tool calls
        if len(response.ToolCalls) > 0 {
            toolResults := le.toolRunner.RunAll(ctx, response.ToolCalls)
            taskCtx.History = append(taskCtx.History, toolResults...)
            le.stuckDetector.RecordActions(response.ToolCalls)
        }
        
        // Check completion
        if le.isTaskComplete(ctx, taskCtx) {
            return TaskResult{Status: TaskDone, Output: taskCtx.FinalOutput()}
        }
        
        // Check if LLM needs human input
        if response.NeedsHuman {
            return TaskResult{
                Status: TaskNeedsHuman,
                HumanQuestion: response.HumanQuestion,
            }
        }
        
        taskCtx.Iteration++
        
        // Respect tick interval (don't hammer the API)
        time.Sleep(le.config.TickInterval)
    }
    
    return TaskResult{Status: TaskBlocked, BlockReason: "max iterations reached"}
}
```

### 5.4 Completion Detection

This is subtle: how does Marcus know when a task is done? It uses a two-signal approach:

**Signal 1: Explicit completion declaration**
The LLM can declare completion by calling a special `task_complete` tool:

```json
{
  "tool": "task_complete",
  "input": {
    "summary": "Implemented the checkout endpoint",
    "outputs": ["src/payments/checkout.ts", "src/payments/checkout.test.ts"],
    "notes": "Used PaymentIntent API as discussed"
  }
}
```

**Signal 2: Completion evaluation**
Every N iterations, Marcus runs a separate evaluation LLM call:

```go
func (le *LoopEngine) isTaskComplete(ctx context.Context, taskCtx *TaskContext) bool {
    // Quick check: did LLM call task_complete?
    if taskCtx.HasExplicitCompletion() { return true }
    
    // Only run eval every 3 iterations (saves tokens)
    if taskCtx.Iteration % 3 != 0 { return false }
    
    evalPrompt := fmt.Sprintf(`
Task: %s
Success criteria:
%s

Actions taken so far:
%s

Current state:
%s

Has this task been completed? Answer with JSON:
{"complete": true/false, "reason": "...", "missing": ["..."]}
`, taskCtx.Task.Description,
   taskCtx.Task.SuccessCriteria,
   taskCtx.ActionSummary(),
   taskCtx.CurrentState())
    
    result := le.provider.Complete(ctx, evalPrompt, CompletionOptions{
        Model:     "groq/llama-3-8b",  // Fast evaluator
        MaxTokens: 200,
        JSON:      true,
    })
    
    var eval CompletionEval
    json.Unmarshal([]byte(result.Text), &eval)
    return eval.Complete
}
```

### 5.5 The Pause/Resume Protocol

When Marcus needs to pause (human input needed, end of day, `Ctrl+C`), it saves full state:

```go
func (le *LoopEngine) checkpoint() {
    state := LoopCheckpoint{
        Timestamp:    time.Now(),
        LoopState:    le.state,
        GoalStack:    le.goalStack.Snapshot(),
        TaskQueue:    le.taskQueue.Snapshot(),
        ActiveTask:   le.activeTask,
        History:      le.history.Snapshot(),
        ContextState: le.contextAssembler.Snapshot(),
    }
    
    // Write atomically (write to .tmp, then rename)
    data, _ := json.MarshalIndent(state, "", "  ")
    tmpPath := ".marcus/sessions/latest/checkpoint.json.tmp"
    finalPath := ".marcus/sessions/latest/checkpoint.json"
    
    os.WriteFile(tmpPath, data, 0644)
    os.Rename(tmpPath, finalPath)  // Atomic on most filesystems
}
```

Resuming:
```bash
$ marcus resume                   # Resume last session
$ marcus resume --session 2026-03-17T09-00-00
```

---

## 6. Task Tracking System

### 6.1 Tasks as Files

Every task is a file in `.marcus/tasks/`:

```
.marcus/tasks/
├── queue/                       # Pending tasks (sorted by priority)
│   ├── 001_implement_checkout.md
│   ├── 002_write_tests.md
│   └── 003_update_docs.md
├── active/                      # Currently executing
│   └── 002_write_tests.md       # (symlink or copy)
├── done/                        # Completed tasks
│   ├── 000_setup_stripe.md
│   └── 001_implement_checkout.md
├── blocked/                     # Waiting on something
│   └── 004_deploy.md
└── failed/                      # Failed tasks
```

Each task file is a human-readable markdown with TOML front-matter:

```markdown
---
id: task_002
title: "Write unit tests for checkout endpoint"
priority: high
created_at: 2026-03-18T09:00:00Z
started_at: 2026-03-18T10:45:00Z
goal_id: goal_root_001
depends_on: [task_001]
assigned_to: agent.tester
tags: [testing, payments]
---

## Description
Write comprehensive unit tests for the `/api/payments/checkout` endpoint
implemented in task_001.

## Success Criteria
- [ ] Tests for happy path (successful payment)
- [ ] Tests for invalid card
- [ ] Tests for insufficient funds
- [ ] Tests for network timeout
- [ ] Minimum 80% code coverage on payments module

## Context
The endpoint is at `src/payments/checkout.ts`.
It uses Stripe's PaymentIntent API.
Test framework: vitest with supertest for HTTP testing.

## Progress Log
2026-03-18T10:45:12Z Created test file at src/payments/checkout.test.ts
2026-03-18T10:46:30Z Wrote happy path test - passing
2026-03-18T10:47:15Z Wrote invalid card test - passing
```

### 6.2 Task Operations

```bash
$ marcus task add "Write migration for users table"
$ marcus task add --priority critical "Fix prod bug: login broken"
$ marcus task list
$ marcus task list --status blocked
$ marcus task show task_002
$ marcus task done task_002
$ marcus task block task_004 --reason "Waiting for staging environment"
$ marcus task unblock task_004
$ marcus task next                # What should Marcus work on next?
$ marcus task import tasks.md     # Import tasks from a markdown checklist
```

### 6.3 Task Priority Algorithm

Marcus uses a scoring algorithm to determine which task to work on next:

```go
type TaskScore struct {
    Priority     float64  // explicit priority: critical=10, high=7, medium=5, low=3
    Urgency      float64  // deadline proximity
    Dependents   float64  // how many other tasks depend on this
    Age          float64  // how long has it been waiting
    Context      float64  // how much context is already loaded (efficiency)
    GoalAlign    float64  // alignment with root goal
}

func (tq *TaskQueue) Score(task *Task) float64 {
    s := tq.computeScore(task)
    return (s.Priority * 0.35) +
           (s.Urgency  * 0.25) +
           (s.Dependents * 0.20) +
           (s.Age      * 0.10) +
           (s.Context  * 0.05) +
           (s.GoalAlign * 0.05)
}
```

### 6.4 The Daily Briefing

When Marcus starts in the morning:

```
  ╭─────────────────────────────────────────────────────────────╮
  │  MARCUS — Good morning, Alex. Here's your day:             │
  ├─────────────────────────────────────────────────────────────┤
  │                                                             │
  │  📋 TASKS (8 total)                                        │
  │  ┌─────────────────────────────────────────────────────┐   │
  │  │ ● 2 critical   ██ invoice bug, deploy blocker       │   │
  │  │ ● 3 high       ███ tests, docs, code review         │   │
  │  │ ● 3 medium     ███ refactoring, feature work        │   │
  │  └─────────────────────────────────────────────────────┘   │
  │                                                             │
  │  Yesterday I completed:                                     │
  │  ✓ Implemented checkout endpoint (src/payments/checkout.ts) │
  │  ✓ Added Stripe client wrapper                             │
  │  ✓ 3 of 8 checkout tests written                           │
  │                                                             │
  │  Blocked (waiting on you):                                 │
  │  ⚠ Deploy to staging — needs your Railway API key          │
  │                                                             │
  │  I suggest starting with:                                  │
  │  → Fix invoice bug (critical, 2 dependents)                │
  │                                                             │
  │  Start working? [y] Yes  [n] No  [c] Choose task           │
  ╰─────────────────────────────────────────────────────────────╯
```

### 6.5 Notification & Alert System

For long-running sessions, Marcus sends notifications through configured channels:

```toml
# marcus.toml
[notifications]
on_task_complete = ["terminal", "slack"]
on_blocked = ["terminal", "slack", "email"]
on_error = ["terminal"]
on_daily_summary = ["email"]

[notifications.slack]
webhook_url_env = "SLACK_WEBHOOK_URL"
channel = "#marcus-updates"

[notifications.email]
to = "alex@company.com"
smtp_env = "SMTP_URL"
```

---

## 7. The Context Assembler — Deep Implementation

### 7.1 The Assembly Budget

Every LLM call has a token budget. The assembler fills this budget using a priority-ranked list of context items:

```go
type ContextBudget struct {
    Total       int   // Model's max context window
    Reserved    int   // Reserved for response
    Available   int   // Total - Reserved
    
    // Allocations (sum must <= Available)
    SystemPrompt  int  // Fixed: ~500 tokens
    GoalStack     int  // Fixed: ~300 tokens
    Memory        int  // Dynamic: up to 2000 tokens
    CodeIndex     int  // Dynamic: up to 3000 tokens  
    Files         int  // Dynamic: up to 10000 tokens
    History       int  // Dynamic: fills the rest
    CurrentInput  int  // Fixed: actual user message
}

func NewContextBudget(model *ModelDef, inputTokens int) ContextBudget {
    available := model.ContextWindow - model.MaxOutputTokens - inputTokens
    return ContextBudget{
        Total:        model.ContextWindow,
        Reserved:     model.MaxOutputTokens,
        Available:    available,
        SystemPrompt: min(500, available * 5/100),
        GoalStack:    min(300, available * 3/100),
        Memory:       min(2000, available * 15/100),
        CodeIndex:    min(3000, available * 20/100),
        Files:        min(10000, available * 40/100),
        History:      available - 500 - 300 - 2000 - 3000 - 10000,
    }
}
```

### 7.2 Relevance Scoring for Context Items

The assembler doesn't blindly include all files or all memories. It scores each candidate:

```go
type ContextItem struct {
    Type       string   // "file", "memory", "symbol", "history"
    Content    string
    TokenCount int
    Score      float64  // Relevance to current task
    Source     string   // Where it came from
}

func (ca *ContextAssembler) scoreFile(file *File, task *Task) float64 {
    score := 0.0
    
    // Direct mention in task description
    if strings.Contains(task.Description, file.Name) { score += 0.5 }
    
    // Recently modified
    age := time.Since(file.ModTime)
    if age < 1*time.Hour { score += 0.3 }
    if age < 24*time.Hour { score += 0.1 }
    
    // Contains symbols mentioned in task
    for _, sym := range ca.extractSymbols(task.Description) {
        if file.ContainsSymbol(sym) { score += 0.2 }
    }
    
    // In the import graph of directly-mentioned files
    if ca.isImportedBy(file, task.ExplicitFiles) { score += 0.15 }
    
    // Has recent git changes
    if ca.git.RecentlyChanged(file.Path) { score += 0.1 }
    
    return score
}
```

### 7.3 The Assembly Order

Context is assembled in this strict order to ensure the most important things are always included:

```go
func (ca *ContextAssembler) Assemble(req AssemblyRequest) PromptContext {
    budget := NewContextBudget(ca.model, req.InputTokens)
    ctx := &PromptContext{}
    
    // 1. System prompt (always, non-negotiable)
    ctx.System = ca.buildSystemPrompt(req.Flow, req.Agent)
    budget.SystemPrompt -= ca.count(ctx.System)
    
    // 2. Goal stack (always, non-negotiable)
    ctx.GoalStack = ca.goalStack.Render()
    budget.GoalStack -= ca.count(ctx.GoalStack)
    
    // 3. Memory (high priority, but trimmed if needed)
    ctx.Memory = ca.selectMemory(req.Task, budget.Memory)
    
    // 4. Explicit files (user @-mentioned, always include)
    for _, f := range req.ExplicitFiles {
        ctx.Files = append(ctx.Files, ca.readFile(f))
    }
    budget.Files -= ca.countFiles(ctx.Files)
    
    // 5. Auto-selected files (relevance-ranked, fill remaining file budget)
    candidates := ca.rankCandidateFiles(req.Task)
    for _, c := range candidates {
        if budget.Files <= 0 { break }
        if c.TokenCount <= budget.Files {
            ctx.Files = append(ctx.Files, c)
            budget.Files -= c.TokenCount
        }
    }
    
    // 6. Code index excerpts (relevant symbols)
    ctx.CodeIndex = ca.selectSymbols(req.Task, budget.CodeIndex)
    
    // 7. Conversation history (fill remaining budget)
    ctx.History = ca.compressHistory(req.History, budget.History)
    
    return *ctx
}
```

---

## 8. The Conversation State Machine

### 8.1 Conversation States

```
                    ┌──────────┐
             ┌─────▶│  READY   │◀────────────────────┐
             │      └────┬─────┘                     │
             │           │ user input                 │
             │           ▼                            │
             │      ┌──────────┐   needs info    ┌────┴─────┐
             │      │CLASSIFYING│───────────────▶│CLARIFYING│
             │      └────┬─────┘                 └────┬─────┘
             │           │ classified                  │ user answers
             │           ▼                            │
             │      ┌──────────┐◀────────────────────┘
             │      │ASSEMBLING│ (build context)
             │      └────┬─────┘
             │           │ context ready
             │           ▼
             │      ┌──────────┐
             │      │STREAMING │ (LLM is responding)
             │      └────┬─────┘
             │           │
             │    ┌───────┴──────────┐
             │    │                  │
             │    ▼                  ▼
             │ [tool calls]    [text complete]
             │    │                  │
             │    ▼                  │
             │ ┌──────────┐          │
             │ │EXECUTING │          │
             │ │  TOOLS   │          │
             │ └────┬─────┘          │
             │      │ results        │
             │      ▼                │
             │ ┌──────────┐          │
             │ │CONFIRMING│          │
             │ │(if needed)│         │
             │ └────┬─────┘          │
             │      │ approved       │
             │      ▼                ▼
             │ ┌────────────────────────┐
             │ │      RESPONDING        │
             │ │  (display to user)     │
             │ └───────────┬────────────┘
             │             │
             └─────────────┘
```

### 8.2 Multi-Turn State Preservation

Between turns, the state machine saves a minimal "turn record":

```json
{
  "turn_id": "turn_042",
  "timestamp": "2026-03-18T10:47:22Z",
  "input": "add error handling to the webhook handler",
  "intent": { "domain": "coding", "flow": "edit_code", "confidence": 0.95 },
  "context_summary": "3 files loaded, 1.2k tokens memory, goal: payment integration",
  "actions": [
    { "tool": "read_file", "input": "src/payments/webhook.ts", "tokens": 234 },
    { "tool": "write_file", "input": "src/payments/webhook.ts", "confirmed": true }
  ],
  "output_summary": "Added try-catch around Stripe event processing, added specific handlers for payment_failed and payment_expired events",
  "tokens_used": 3421,
  "cost_usd": 0.0051
}
```

---

## 9. How Streaming Works End-to-End

### 9.1 The Streaming Pipeline

```
LLM API (SSE stream)
    │
    ▼
StreamParser
    │ parses text chunks / tool call deltas
    ▼
StreamRouter
    ├── TextChunk → TextRenderer → TUI update
    ├── ToolCallStart → ToolCallUI (show spinner)
    ├── ToolCallArgs → ToolCallUI (stream args)
    ├── ToolCallComplete → ToolRunner.Schedule()
    └── StreamEnd → CompletionHandler
         │
         ▼
    ToolRunner (parallel execution of tool calls)
         │ results
         ▼
    HistoryAppender (add to conversation)
         │
         ▼
    LoopEngine.ContinueIfNeeded()
```

### 9.2 Streaming Tool Call Rendering

Tool calls stream their arguments incrementally. Marcus renders them in real-time:

```
  Marcus ─────────────────────────────────────────────────────
  Let me look at the webhook handler and add error handling...
  
  🔧 read_file
     path: "src/payments/webhook.ts" ✓ reading...
     ████████████████ done (156 lines)
  
  🔧 write_file
     path: "src/payments/webhook.ts"
     ┌──────────────────────────────────────────────────────┐
     │ + try {                                              │
     │ +   const event = stripe.webhooks.constructEvent(   │
     │ ...streaming...                                      │
```

### 9.3 Backpressure Handling

The TUI can render slower than the stream arrives. Marcus uses a buffered channel with backpressure:

```go
type StreamBuffer struct {
    chunks chan StreamChunk
    done   chan struct{}
}

func (sb *StreamBuffer) Feed(chunk StreamChunk) {
    select {
    case sb.chunks <- chunk:
        // Buffered successfully
    case <-time.After(50 * time.Millisecond):
        // TUI is slow, drop non-critical chunks (mid-word updates)
        // but never drop tool calls or completion signals
        if chunk.Type == ChunkTypeText && !chunk.IsLast {
            return // Drop this intermediate text chunk
        }
        sb.chunks <- chunk // Block for critical chunks
    }
}
```

---

## 10. Error Recovery & Resilience

### 10.1 Error Classification

Not all errors are equal. Marcus classifies errors to respond appropriately:

```go
type ErrorClass int

const (
    ErrorTransient    ErrorClass = iota // Retry with backoff (rate limit, timeout)
    ErrorRecoverable                    // Try different approach
    ErrorUserInput                      // Ask user to clarify
    ErrorPermission                     // Ask user to grant permission
    ErrorFatal                          // Stop everything, report
)

func classifyError(err error) ErrorClass {
    switch {
    case isRateLimit(err):       return ErrorTransient
    case isNetworkTimeout(err):  return ErrorTransient
    case isModelOverloaded(err): return ErrorTransient
    case isToolFailed(err):      return ErrorRecoverable
    case isAmbiguousInput(err):  return ErrorUserInput
    case isPermissionDenied(err): return ErrorPermission
    default:                     return ErrorFatal
    }
}
```

### 10.2 Retry Strategy

```go
type RetryConfig struct {
    MaxAttempts  int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   float64
    Jitter       bool
}

var DefaultRetry = RetryConfig{
    MaxAttempts:  5,
    InitialDelay: 1 * time.Second,
    MaxDelay:     60 * time.Second,
    Multiplier:   2.0,
    Jitter:       true,  // Add random jitter to prevent thundering herd
}

func (m *Marcus) withRetry(ctx context.Context, fn func() error) error {
    config := DefaultRetry
    delay := config.InitialDelay
    
    for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
        err := fn()
        if err == nil { return nil }
        
        if classifyError(err) != ErrorTransient { return err }
        
        if attempt == config.MaxAttempts { return err }
        
        // Exponential backoff with jitter
        if config.Jitter {
            jitter := time.Duration(rand.Int63n(int64(delay) / 2))
            delay = delay + jitter
        }
        
        m.notify(NotifWarning, fmt.Sprintf("Retrying in %s (attempt %d/%d)", delay, attempt, config.MaxAttempts))
        
        select {
        case <-ctx.Done(): return ctx.Err()
        case <-time.After(delay):
        }
        
        delay = min(time.Duration(float64(delay)*config.Multiplier), config.MaxDelay)
    }
    return nil
}
```

### 10.3 Provider Failover

When a provider fails, Marcus fails over automatically:

```go
func (pr *ProviderRouter) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    providers := pr.orderedProviders(req)  // Primary + fallbacks
    
    for _, provider := range providers {
        resp, err := pr.withRetry(ctx, func() error {
            return provider.Complete(ctx, req)
        })
        
        if err == nil {
            if provider.Name != req.PreferredProvider {
                pr.notify(fmt.Sprintf("Used fallback: %s (primary %s unavailable)", 
                    provider.Name, req.PreferredProvider))
            }
            return resp, nil
        }
        
        pr.logger.Warn("Provider failed, trying next", 
            "provider", provider.Name, "error", err)
    }
    
    return nil, fmt.Errorf("all providers failed")
}
```

---

## 11. The Scheduler — Time-Based & Trigger-Based Execution

### 11.1 Marcus as a Daemon

Marcus can run as a background daemon that executes tasks on a schedule:

```bash
$ marcus daemon start             # Start background daemon
$ marcus daemon status            # Check daemon status
$ marcus daemon stop              # Stop daemon
$ marcus daemon logs              # Tail daemon logs
```

### 11.2 Scheduled Tasks

Tasks can have schedules defined in their front-matter:

```markdown
---
id: task_daily_standup
title: "Generate daily standup notes"
schedule: "0 9 * * 1-5"           # 9am every weekday (cron syntax)
trigger: none
autonomy: 4                         # Run fully headlessly
---

## Description
Read yesterday's git commits and completed tasks.
Draft standup notes: what was done, what's next, any blockers.
Send to Slack #standup channel.
```

### 11.3 Trigger-Based Tasks

Tasks can also be triggered by filesystem events, git events, or external webhooks:

```markdown
---
id: task_review_pr
title: "Review incoming PR"
trigger:
  type: webhook
  path: /webhooks/github
  event: pull_request.opened
autonomy: 2
notify_on_complete: true
---

Review the incoming pull request. Check for:
- Code quality and conventions
- Missing tests
- Security issues
- Performance concerns

Post the review as a GitHub comment.
```

```toml
# triggers.toml in .marcus/
[[trigger]]
id = "on_test_fail"
type = "command_exit"
watch_command = "npm test"
on_exit_code = [1, 2]
run_flow = "debug_test_failure"

[[trigger]]
id = "on_new_file"  
type = "filesystem"
watch_path = "src/**/*.ts"
on_event = "created"
run_flow = "add_file_to_index"
```

### 11.4 The Scheduler Implementation

```go
type Scheduler struct {
    tasks    []*ScheduledTask
    cron     *cron.Cron
    triggers []*Trigger
    engine   *LoopEngine
    mu       sync.Mutex
}

func (s *Scheduler) Boot() error {
    // Load all scheduled tasks from .marcus/tasks/
    for _, task := range s.loadScheduledTasks() {
        if task.Schedule != "" {
            s.cron.AddFunc(task.Schedule, func() {
                s.engine.taskQueue.Push(task)
            })
        }
    }
    
    // Register filesystem triggers
    for _, trigger := range s.triggers {
        if trigger.Type == "filesystem" {
            s.watchFS(trigger)
        }
    }
    
    // Start HTTP server for webhook triggers
    s.startWebhookServer()
    
    s.cron.Start()
    return nil
}
```

---

## 12. Building the TUI Layer

### 12.1 Bubble Tea Architecture

Bubble Tea uses the Elm architecture: Model → Update → View. The entire TUI is a pure function of state.

```go
// The entire TUI state
type Model struct {
    // Layout
    width, height int
    focusedPane   Pane
    
    // Conversation
    messages      []Message
    viewport      viewport.Model  // Scrollable message list
    textarea      textarea.Model  // Input box
    
    // Status
    provider      string
    model         string
    sessionCost   float64
    gitBranch     string
    
    // Context panel
    contextFiles  []ContextFile
    contextTokens int
    
    // Tools panel
    activeTools   []ToolStatus
    
    // Streaming state
    streaming     bool
    streamBuffer  strings.Builder
    toolSpinners  map[string]*spinner.Model
    
    // Notifications
    notifications []Notification
    
    // State
    loopRunning   bool
    taskCount     int
    activeTask    *Task
}

// The update function — pure, handles all events
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    
    case tea.KeyMsg:
        return m.handleKey(msg)
    
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
        return m.relayout()
    
    case StreamChunkMsg:
        m.streamBuffer.WriteString(msg.Chunk)
        return m, nil
    
    case ToolStartMsg:
        m.toolSpinners[msg.ToolID] = newSpinner()
        return m, tickSpinner(msg.ToolID)
    
    case ToolCompleteMsg:
        delete(m.toolSpinners, msg.ToolID)
        return m, nil
    
    case NotificationMsg:
        m.notifications = append(m.notifications, msg.Notification)
        return m, clearNotificationAfter(msg.Notification.ID, 4*time.Second)
    
    case TaskUpdateMsg:
        m.activeTask = msg.Task
        m.taskCount = msg.Total
        return m, nil
    }
    
    return m, nil
}
```

### 12.2 Syntax Highlighting Inline

Code blocks in responses are syntax-highlighted using Chroma:

```go
func renderCodeBlock(content, language string, width int) string {
    lexer := lexers.Get(language)
    if lexer == nil { lexer = lexers.Fallback }
    
    style := styles.Get("dracula")  // Matches TUI theme
    formatter := formatters.Get("terminal256")
    
    var buf bytes.Buffer
    iterator, _ := lexer.Tokenise(nil, content)
    formatter.Format(&buf, style, iterator)
    
    // Add line numbers
    lines := strings.Split(buf.String(), "\n")
    var numbered []string
    for i, line := range lines {
        numbered = append(numbered, fmt.Sprintf("%3d  %s", i+1, line))
    }
    
    return lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("#6272a4")).
        Width(width - 4).
        Render(strings.Join(numbered, "\n"))
}
```

### 12.3 The Command Palette

`Ctrl+P` opens a fuzzy-searchable command palette:

```go
type CommandPalette struct {
    input     textinput.Model
    list      list.Model
    allItems  []CommandItem  // Loaded from registry
}

type CommandItem struct {
    Title       string
    Description string
    Category    string  // "flow", "tool", "command", "file"
    Action      func() tea.Cmd
}

func (cp *CommandPalette) FilteredItems() []CommandItem {
    query := cp.input.Value()
    if query == "" { return cp.allItems[:20] }  // Top 20 most used
    
    // Fuzzy match using a simple scoring algorithm
    var scored []ScoredItem
    for _, item := range cp.allItems {
        score := fuzzyScore(query, item.Title)
        if score > 0 {
            scored = append(scored, ScoredItem{item, score})
        }
    }
    
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score > scored[j].Score
    })
    
    result := make([]CommandItem, min(len(scored), 10))
    for i, s := range result {
        result[i] = scored[i].Item
    }
    return result
}
```

---

## 13. How Memory Learns Over Time

### 13.1 Passive Learning During Conversation

Marcus watches conversation for learnable facts using a lightweight extractor:

```go
type FactExtractor struct {
    patterns []ExtractionPattern
}

type ExtractionPattern struct {
    Trigger  *regexp.Regexp
    Type     string
    Template string
}

var patterns = []ExtractionPattern{
    {
        // "we use X" / "we're using X" / "the project uses X"
        Trigger:  regexp.MustCompile(`(?i)(we|this project|the app|we're) (use[sd]?|using) ([^.!?]+)`),
        Type:     "tech_fact",
        Template: "Uses: $3",
    },
    {
        // "X is owned by Y" / "Y is responsible for X"
        Trigger:  regexp.MustCompile(`(?i)(\w+) (is owned|is maintained|is responsible for|works on) ([^.!?]+)`),
        Type:     "ownership",
        Template: "$1: $3",
    },
    {
        // "never do X" / "always do X" / "don't X"
        Trigger:  regexp.MustCompile(`(?i)(never|always|don't|do not) ([^.!?]+)`),
        Type:     "convention",
        Template: "Convention: $1 $2",
    },
}

func (fe *FactExtractor) Extract(message string) []ExtractedFact {
    var facts []ExtractedFact
    for _, p := range patterns {
        matches := p.Trigger.FindAllStringSubmatch(message, -1)
        for _, m := range matches {
            fact := applyTemplate(p.Template, m)
            facts = append(facts, ExtractedFact{
                Content: fact,
                Type:    p.Type,
                Source:  message,
                Confidence: 0.8,
            })
        }
    }
    return facts
}
```

When facts are extracted, Marcus surfaces them with a low-friction prompt:

```
  ℹ  Noted something new — save to memory?
  "The project uses Prisma with PostgreSQL"
  [y] Save  [n] Skip  [e] Edit  [!] Always save this type
```

### 13.2 Session Summarization

At the end of every session (or when compressing history), Marcus generates a structured summary:

```go
const summarizationPrompt = `
Summarize this work session. Be specific and technical.
Focus on: what was built, what decisions were made, what's still in progress.

Format:
## Completed
- [specific things done]

## Decisions Made
- [architecture/approach decisions and why]

## In Progress
- [what's still being worked on]

## Blockers / Notes
- [anything important for the next session]

Conversation:
{{conversation}}
`
```

The summary is saved as an episodic memory entry and injected at the start of future sessions.

### 13.3 Cross-Session Pattern Detection

Over time, Marcus notices patterns in how you work:

```go
// Runs weekly, analyzes recent sessions
func (m *MemoryManager) detectPatterns() {
    sessions := m.loadRecentSessions(30)  // Last 30 days
    
    // Pattern: files you always edit together
    coEdits := analyzeCoEditPatterns(sessions)
    for pair, count := range coEdits {
        if count >= 5 {
            m.savePattern(Pattern{
                Type: "co_edit",
                Description: fmt.Sprintf("When editing %s, you often also edit %s", pair.A, pair.B),
                Data: pair,
            })
        }
    }
    
    // Pattern: flows you use most
    flowUsage := analyzeFlowUsage(sessions)
    // Use to reorder autocomplete suggestions
    
    // Pattern: times of day you work on what
    workPatterns := analyzeTimePatterns(sessions)
    // Use to schedule appropriate tasks
}
```

---

## 14. The Wire Format — How Everything Talks

### 14.1 Internal Event Bus

All Marcus components communicate through a typed event bus. This decouples everything and makes the system easy to extend:

```go
type Event interface {
    EventType() string
}

// All events
type UserInputEvent struct { Message string; Attachments []File }
type LLMStreamChunkEvent struct { Chunk string; Done bool }
type ToolCallEvent struct { ID string; Tool string; Input json.RawMessage }
type ToolResultEvent struct { ID string; Output json.RawMessage; Error error }
type TaskStartEvent struct { Task *Task }
type TaskCompleteEvent struct { Task *Task; Result TaskResult }
type MemorySaveEvent struct { Key string; Value string; Type MemoryType }
type NotificationEvent struct { Level string; Message string }
type SessionCheckpointEvent struct { Path string }
type LoopStateChangeEvent struct { From LoopState; To LoopState }

type EventBus struct {
    subscribers map[string][]chan Event
    mu          sync.RWMutex
}

func (eb *EventBus) Subscribe(eventType string) <-chan Event {
    ch := make(chan Event, 32)
    eb.mu.Lock()
    eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
    eb.mu.Unlock()
    return ch
}

func (eb *EventBus) Emit(event Event) {
    eb.mu.RLock()
    subs := eb.subscribers[event.EventType()]
    eb.mu.RUnlock()
    
    for _, ch := range subs {
        select {
        case ch <- event:
        default:
            // Drop if subscriber is slow (non-blocking)
        }
    }
}
```

### 14.2 The Marcus IPC Protocol

For editor integrations and the web dashboard, Marcus exposes a local Unix socket with a JSON-RPC protocol:

```
Socket: /tmp/marcus-{pid}.sock

Request:
{
  "jsonrpc": "2.0",
  "id": "req_001",
  "method": "marcus.complete",
  "params": {
    "message": "explain this function",
    "context": { "file": "src/auth.ts", "line": 42 }
  }
}

Response (streaming):
{ "jsonrpc": "2.0", "id": "req_001", "result": { "type": "chunk", "text": "This function..." } }
{ "jsonrpc": "2.0", "id": "req_001", "result": { "type": "chunk", "text": " handles..." } }
{ "jsonrpc": "2.0", "id": "req_001", "result": { "type": "done", "usage": { "tokens": 234, "cost": 0.001 } } }
```

---

## 15. Implementation Order — What to Build First

Building Marcus in the right order matters. Here's a phased plan that gets you to a usable tool as fast as possible.

### Phase 1 — The Skeleton (Weeks 1-2)
Goal: Something you can actually type into.

```
Week 1:
  ✓ Basic CLI with cobra (marcus chat, marcus edit, marcus flow)
  ✓ Single provider adapter (Anthropic only)
  ✓ Simple streaming output (no TUI yet, just stdout)
  ✓ Basic folder engine: boot, discover flows, load prompt.md
  ✓ One working flow: code_edit

Week 2:
  ✓ File reading tool (read_file)
  ✓ File writing tool (write_file + diff preview)
  ✓ Confirmation prompts for writes
  ✓ Basic session save/load (JSONL)
  ✓ run_command tool
```

**Milestone:** You can say `marcus edit src/auth.ts "add rate limiting"` and it works.

### Phase 2 — The Foundation (Weeks 3-4)
Goal: The engine is solid enough for daily use.

```
Week 3:
  ✓ Full Bubble Tea TUI (conversation + input + status bar)
  ✓ Syntax-highlighted code blocks in TUI
  ✓ Multiple providers (add OpenAI, Groq)
  ✓ Provider failover
  ✓ Basic memory system (facts + episodic summaries)

Week 4:
  ✓ Context assembler with token budget
  ✓ Tree-sitter code indexer
  ✓ Diff/apply engine with rollback
  ✓ /slash commands
  ✓ @file mentions in input
```

**Milestone:** You can use Marcus as your daily driver for coding tasks.

### Phase 3 — The Power Features (Weeks 5-7)
Goal: Marcus can work autonomously.

```
Week 5:
  ✓ Task system (task files + queue)
  ✓ Loop engine (basic autonomous execution)
  ✓ Goal stack
  ✓ Stuck detection (basic)

Week 6:
  ✓ Workflow orchestration (DAG engine)
  ✓ Multi-step flows
  ✓ Context compression (progressive summarization)
  ✓ Session checkpoint + resume

Week 7:
  ✓ Plugin system (folder-based, with permissions)
  ✓ First non-coding plugin: marcus-writing
  ✓ Intent classifier
  ✓ Universal prompt engine (domain-agnostic)
```

**Milestone:** `marcus run --autonomy autonomous "implement the payment feature"` works.

### Phase 4 — The Ecosystem (Weeks 8-10)
Goal: Marcus is a platform, not just a tool.

```
Week 8:
  ✓ marcus-email plugin
  ✓ Scheduler + cron tasks
  ✓ Filesystem + webhook triggers
  ✓ Notification system (terminal + Slack)

Week 9:
  ✓ IPC protocol (Unix socket + JSON-RPC)
  ✓ Neovim plugin (calls Marcus via IPC)
  ✓ Cost tracking dashboard
  ✓ Pattern memory (cross-session learning)

Week 10:
  ✓ Plugin registry (install from GitHub)
  ✓ Flow sharing (publish/install flows)
  ✓ Daily briefing
  ✓ Web dashboard (read-only, monitor long-running sessions)
```

**Milestone:** Marcus works all day, handles emails and code, you can install community plugins.

---

### Key Technical Decisions Summary

| Decision | Choice | Reason |
|----------|--------|--------|
| Language | Go | Single binary, fast startup, great TUI libs, strong concurrency |
| TUI framework | Bubble Tea + Lip Gloss | Elm architecture, composable, battle-tested |
| Template engine | Jinja2 (via go-jinja2) | Most familiar to developers, powerful, readable |
| Code parsing | Tree-sitter | Fast, accurate, 40+ languages, incremental parsing |
| Config format | TOML | Human-friendly, less noisy than YAML, typed |
| Event bus | In-process channels | Simple, fast, no external dependency |
| IPC | Unix socket + JSON-RPC | Universal, works with any editor/language |
| Storage | Files + JSONL | Inspectable, git-friendly, no database dependency |
| Embeddings | Optional (OpenAI) | Not required for v1, adds cost complexity |
| Task format | Markdown + TOML front-matter | Human-readable, editable in any editor |
| Prompt format | Jinja2 Markdown | Written like docs, powerful like code |

---

*Marcus is built on one conviction: a tool you can fully understand is a tool you can fully trust. Every design decision points back to that.*