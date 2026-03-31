# Phase 3 Testing Guide

This guide covers testing procedures for the Phase 3 Claude Code parity features.

## Table of Contents

1. [Skills System](#skills-system)
2. [MCP Integration](#mcp-integration)
3. [Remote Triggers](#remote-triggers)
4. [TUI Badge System](#tui-badge-system)

---

## Skills System

### Prerequisites

- Marcus binary built: `go build -o marcus.exe ./cmd/marcus`
- Project initialized: `./marcus.exe init .` (or existing `.marcus/` directory)

### Test Cases

#### 1.1 Help Skill (`/help`)

```bash
# Start chat TUI
./marcus.exe chat

# In the TUI, type:
/help
```

**Expected Result:**
- Transcript displays all available skills with patterns and descriptions
- Shows: `/commit`, `/clear`, `/help`, `/model`, `/newsession`, `/status`, `/undo`

#### 1.2 Clear Skill (`/clear`)

```bash
# In chat TUI, after some conversation:
/clear
```

**Expected Result:**
- Transcript is cleared
- System message: "Conversation cleared."

#### 1.3 Status Skill (`/status`)

```bash
# In chat TUI:
/status
```

**Expected Result:**
- Shows current provider/model (e.g., "anthropic/claude-sonnet-4-6")
- Shows project name
- Shows session context info

#### 1.4 Model Skill (`/model`)

```bash
# In chat TUI, switch provider:
/model ollama:qwen3.5:397b-cloud

# Verify switch:
/status
```

**Expected Result:**
- Status shows new provider/model
- Subsequent chat messages use the new provider

#### 1.5 New Session Skill (`/newsession`)

```bash
# In chat TUI after conversation:
/newsession
# or
/new
```

**Expected Result:**
- Session is reset
- System message: "New session started."

#### 1.6 Undo Skill (`/undo`)

```bash
# 1. Make some file changes through Marcus
# 2. In chat TUI:
/undo
```

**Expected Result:**
- Last batch of file writes is reverted
- System message showing reverted files

#### 1.7 Commit Skill (`/commit`)

**Prerequisites:** Git repository with staged changes

```bash
# Stage some changes
git add .

# In chat TUI:
/commit
```

**Expected Result:**
- Marcus analyzes staged changes
- Suggests commit message
- Executes `git commit` with the message
- Success badge appears

---

## MCP Integration

### Prerequisites

- MCP server installed (e.g., `npx -y @modelcontextprotocol/server-filesystem`)
- Node.js/npm available (for npm-based MCP servers)

### Configuration Setup

Create `~/.marcus/mcp.json`:

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/your/project"]
    }
  ]
}
```

### Test Cases

#### 2.1 MCP Server Discovery

```bash
# Start chat TUI:
./marcus.exe chat

# Wait for initialization...
# Look for MCP tools in the "Tools:" line of the welcome message
```

**Expected Result:**
- Welcome message lists MCP tools alongside native tools
- Example: "Tools: read_file, write_file, run_command, filesystem_read, filesystem_list"

#### 2.2 MCP Tool Execution

```bash
# In chat TUI:
list the files in the project root
```

**Expected Result:**
- Marcus uses filesystem MCP tool
- Tool call shows with blue bullet: "● filesystem_list"
- File listing displayed in result

#### 2.3 MCP Tool with Arguments

```bash
# In chat TUI:
read the contents of go.mod
```

**Expected Result:**
- Tool call shows: "● filesystem_read"
- File content displayed with proper formatting

#### 2.4 MCP Error Handling

```bash
# In chat TUI, request invalid path:
read /nonexistent/path/file.txt
```

**Expected Result:**
- Error badge (✗) displayed
- Error message in result body

---

## Remote Triggers

### Prerequisites

- Marcus built and project initialized
- Write access to `.marcus/triggers/` directory

### Test Cases

#### 3.1 Create Cron Trigger

```bash
# In chat TUI:
/schedule create --cron "*/2 * * * *" --flow "health_check"
```

**Expected Result:**
- Trigger created with ID
- System message: "Trigger created: trigger-<id>"

#### 3.2 List Triggers

```bash
# In chat TUI:
/triggers list
```

**Expected Result:**
- Table showing:
  - Trigger ID
  - Name
  - Type (cron)
  - Schedule expression
  - Enabled status
  - Next run time

#### 3.3 Manual Trigger Execution

```bash
# In chat TUI:
/triggers run <trigger-id>
```

**Expected Result:**
- Flow executes immediately
- System message: "Trigger <id> executed successfully"

#### 3.4 Disable/Enable Trigger

```bash
# Disable
/triggers disable <trigger-id>

# Enable
/triggers enable <trigger-id>
```

**Expected Result:**
- Status updated in `/triggers list`
- Disabled triggers don't fire

#### 3.5 Delete Trigger

```bash
# In chat TUI:
/schedule delete <trigger-id>
```

**Expected Result:**
- Trigger removed from list
- File deleted from `.marcus/triggers/`

#### 3.6 Automatic Cron Execution

```bash
# Create a trigger that fires every minute
/schedule create --cron "* * * * *" --flow "daily_report"

# Wait 1-2 minutes
```

**Expected Result:**
- Trigger fires automatically
- Results appear in session
- `RunCount` increments in trigger file

### Verify Trigger Persistence

```bash
# Check trigger files exist:
ls .marcus/triggers/

# View trigger JSON:
cat .marcus/triggers/trigger-<id>.json
```

Expected structure:
```json
{
  "id": "trigger-xxx",
  "name": "Cron Trigger",
  "type": "cron",
  "enabled": true,
  "config": {
    "cron_expression": "*/2 * * * *"
  },
  "action": {
    "type": "flow",
    "target": "health_check"
  },
  "run_count": 3,
  "error_count": 0,
  "created_at": "2026-03-27T..."
}
```

---

## TUI Badge System

### Test Cases

#### 4.1 Success Badge

```bash
# In chat TUI, request a file edit that succeeds:
edit main.go "add a comment at the top of the file"

# Apply with 'y'
```

**Expected Result:**
- Result item shows: "✓ Applied" in green
- Badge appears inline with the result title

#### 4.2 Error Badge

```bash
# In chat TUI, request an edit to a non-existent file:
edit /nonexistent/file.go "add comment"
```

**Expected Result:**
- Error badge: "✗ Failed" in red
- Error details in result body

#### 4.3 Multiple Actions Badge

```bash
# In chat TUI, request multiple file edits:
add a greeting function to main.go and create a new utils.go with helper functions
```

**Expected Result:**
- Each result item has its own badge
- Success badges for successful operations
- Error badges for failed operations

#### 4.4 Badge Persistence

```bash
# After several actions, scroll up in transcript
```

**Expected Result:**
- Badges remain visible on historical results
- Colors preserved (green for success, red for error)

### Visual Verification

Badges should appear as:
- **Success**: Green `✓ Applied` text
- **Error**: Red `✗ Failed` text
- Positioned after the result title, before the body content

---

## Integration Tests

### Full Workflow Test

```bash
# 1. Start fresh
./marcus.exe init test_project
cd test_project

# 2. Start chat
./marcus.exe chat

# 3. Test skills
/help
/status

# 4. Test MCP (if configured)
list files

# 5. Test file edit with badges
edit main.go "add hello function"
# Press 'y' to apply

# 6. Test undo
/undo

# 7. Test commit (stage changes first)
/commit

# 8. Test schedule
/schedule create --cron "0 9 * * *" --flow "daily_standup"
/triggers list

# 9. Clear and exit
/clear
/exit
```

---

## Troubleshooting

### MCP Server Not Found

**Symptoms:** MCP tools not appearing in welcome message

**Checks:**
```bash
# Verify MCP config exists and is valid JSON
cat ~/.marcus/mcp.json | jq .

# Test MCP server manually
npx -y @modelcontextprotocol/server-filesystem /path/to/project
# Should start without errors

# Check Marcus can see the config
./marcus.exe chat
# Look for MCP-related errors in startup
```

### Triggers Not Firing

**Symptoms:** Cron trigger doesn't execute at scheduled time

**Checks:**
```bash
# Verify trigger is enabled
ls .marcus/triggers/
cat .marcus/triggers/trigger-*.json | grep enabled

# Check cron expression is valid
# Expression format: minute hour day month day_of_week
# "*/2 * * * *" = every 2 minutes
```

### Skills Not Executing

**Symptoms:** `/command` treated as chat message

**Checks:**
```bash
# Verify slash command format
/help  # Should work
/help extra  # Should show error or ignore extra args

# Check skill is registered
# Look in initSkills() in model.go
```

---

## Test Checklist

- [ ] `/help` displays all skills
- [ ] `/clear` clears transcript
- [ ] `/status` shows current config
- [ ] `/model <provider:model>` switches provider
- [ ] `/newsession` resets session
- [ ] `/commit` analyzes diff and commits
- [ ] MCP tools appear in tool list
- [ ] MCP tools execute successfully
- [ ] MCP errors show error badges
- [ ] `/schedule create` creates trigger file
- [ ] `/triggers list` shows triggers
- [ ] Cron trigger fires automatically
- [ ] `/triggers run` executes manually
- [ ] Success badges appear on file edits
- [ ] Error badges appear on failures
- [ ] Badge colors match status (green/red)

---

## Automated Test Commands

Run the test suite:

```bash
# All tests
go test ./...

# Specific packages
go test ./internal/skill/...
go test ./internal/mcp/...
go test ./internal/scheduler/...
go test ./internal/tui/...

# Verbose output
go test -v ./internal/scheduler/...
```

Key test files:
- `internal/skill/registry_test.go` - Skill parsing and execution
- `internal/mcp/client_test.go` - MCP client and transport
- `internal/scheduler/cron_test.go` - Cron expression parsing
- `internal/scheduler/persistence_test.go` - Trigger storage
- `internal/tui/badge_test.go` - Badge rendering

---

## Notes

- All skill commands work in both interactive TUI and can be scripted
- MCP servers must be executable and available in PATH
- Triggers persist across Marcus restarts
- Badges are visual-only and don't affect functionality
- Cron expressions use local system time

---

*Generated for Phase 3 Testing - Marcus AI*
