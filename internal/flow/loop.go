package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

// ContextBudget tracks estimated token usage across the loop run.
type ContextBudget struct {
	MaxTokens    int
	WarnAtPct    int
	CompactAtPct int
	// Running estimate (in tokens, 1 token ≈ 4 chars)
	EstimatedUsed int
	Warned        bool
	Compacted     int // number of times we've compacted
}

// Percent returns the estimated context usage as a percentage (0-100+).
func (b *ContextBudget) Percent() int {
	if b.MaxTokens == 0 {
		return 0
	}
	return (b.EstimatedUsed * 100) / b.MaxTokens
}

// ShouldCompact returns true when context has exceeded the compact threshold.
func (b *ContextBudget) ShouldCompact() bool {
	return b.Percent() >= b.CompactAtPct
}

// ShouldWarn returns true when the warn threshold is crossed and we haven't warned yet.
func (b *ContextBudget) ShouldWarn() bool {
	return b.Percent() >= b.WarnAtPct && !b.Warned
}

// MarkWarned records that a warning has been emitted.
func (b *ContextBudget) MarkWarned() {
	b.Warned = true
}

// EstimateTokens estimates token count from a string (rough: 1 token ≈ 4 chars).
func EstimateTokens(s string) int {
	return len(s) / 4
}

// Snapshot is the assembled prompt context (mirrors ctxpkg.Snapshot).
type Snapshot struct {
	Text            string
	Branch          string
	Dirty           bool
	FileHints       []string
	TODOHints       []task.TODOHint
	EstimatedTokens int
	Truncated       bool
	DroppedSections []string
}

// ContextAssembler is the subset of ctxpkg.Assembler that LoopEngine needs.
type ContextAssembler interface {
	Assemble(input string, sess *session.Session) Snapshot
}

// LoopEngine is a stateful, goal-aware execution engine that iteratively
// calls the model with tool results fed back as conversation messages.
type LoopEngine struct {
	engine           *Engine
	executor         *FlowExecutor
	taskStore        *task.Store
	memory           *memory.Manager
	contextAsm       ContextAssembler
	toolRunner       *tool.ToolRunner
	provider         *provider.Runtime
	cfg              *config.Config
	baseDir          string
	goalMu           sync.Mutex
	goalStack        []string
	selfCorrection   *SelfCorrectionEngine
	customSystemPrompt string  // Optional custom system prompt (e.g., for agent roles)
}

// LoopState carries the full state of one run or one step.
type LoopState struct {
	Iteration    int
	TaskID       string
	TaskStatus   string
	Progress     string
	Messages     []provider.Message
	ToolResults  []ToolResultEntry
	Decisions    []DecisionLogEntry
	Done         bool
	Blocked      bool
	FinishReason string
	Budget       ContextBudget
}

// ToolResultEntry records the outcome of a single tool call.
type ToolResultEntry struct {
	ToolName  string
	RawOutput json.RawMessage
	Success   bool
	Output    string
}

// DecisionLogEntry records what the model said and did in one iteration.
type DecisionLogEntry struct {
	Iteration  int
	ModelText  string
	Actions    []string
	TaskStatus string
	Timestamp  time.Time
}

// NewLoopEngine creates a LoopEngine wiring all dependencies.
func NewLoopEngine(
	engine *Engine,
	executor *FlowExecutor,
	taskStore *task.Store,
	mem *memory.Manager,
	contextAsm ContextAssembler,
	toolRunner *tool.ToolRunner,
	prov *provider.Runtime,
	cfg *config.Config,
	baseDir string,
) *LoopEngine {
	return &LoopEngine{
		engine:     engine,
		executor:   executor,
		taskStore:  taskStore,
		memory:     mem,
		contextAsm: contextAsm,
		toolRunner: toolRunner,
		provider:   prov,
		cfg:        cfg,
		baseDir:    baseDir,
	}
}

// EnableSelfCorrection enables the self-correction system
func (le *LoopEngine) EnableSelfCorrection(opts CorrectionOptions) {
	le.selfCorrection = NewSelfCorrectionEngine(opts)
}

// SetSystemPrompt sets a custom system prompt (e.g., for agent roles)
func (le *LoopEngine) SetSystemPrompt(prompt string) {
	le.customSystemPrompt = prompt
}

// GetSystemPrompt returns the current custom system prompt
func (le *LoopEngine) GetSystemPrompt() string {
	return le.customSystemPrompt
}

// SetSelfCorrectionProvider sets the LLM provider for self-correction
func (le *LoopEngine) SetSelfCorrectionProvider(p provider.Provider) {
	if le.selfCorrection != nil {
		le.selfCorrection.SetProvider(p)
	}
}

// DisableSelfCorrection disables the self-correction system
func (le *LoopEngine) DisableSelfCorrection() {
	le.selfCorrection = nil
}

// Run executes a full autonomous loop for the given goal and taskID.
// It loops until no tool calls are returned, maxIterations is hit, or the
// model signals completion/blocked via task updates.
func (le *LoopEngine) Run(ctx context.Context, goal, taskID string, maxIterations int) (*LoopState, error) {
	state := &LoopState{
		Iteration:  0,
		TaskID:     taskID,
		TaskStatus: task.StatusActive,
	}

	// Initialize context budget from config
	state.Budget = ContextBudget{
		WarnAtPct:    80,
		CompactAtPct: 90,
	}
	if le.cfg != nil {
		state.Budget.MaxTokens = le.cfg.Context.MaxContextTokens
		state.Budget.WarnAtPct = le.cfg.Context.WarnAtPercent
		state.Budget.CompactAtPct = le.cfg.Context.CompactAtPercent
	}

	// Mark task active
	if le.taskStore != nil && taskID != "" {
		if _, err := le.taskStore.ApplyUpdates([]task.Update{
			{ID: taskID, Title: goal, Status: task.StatusActive},
		}); err != nil {
			return nil, fmt.Errorf("activate task: %w", err)
		}
	}

	// Build initial context snapshot
	snapshot := le.contextAsm.Assemble(goal, nil)

	// Assemble initial system and user messages
	systemPrompt := le.buildSystemPrompt(goal)
	userPrompt := le.buildUserPrompt(goal, snapshot)

	messages := []provider.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	state.Messages = messages

	// Update budget with initial prompt size
	state.Budget.EstimatedUsed = EstimateTokens(systemPrompt + "\n" + userPrompt)

	iter := 0
	for iter < maxIterations {
		iter++
		state.Iteration = iter
		state.Progress = fmt.Sprintf("iteration %d: model call", iter)

		// Emit context usage progress
		pct := state.Budget.Percent()
		if state.Budget.MaxTokens > 0 {
			if pct >= 100 {
				fmt.Printf("\x1b[33m[iter %d] ⚠ context at %d%% (est. %d / %d tokens) — compacting...\x1b[0m\n",
					iter, pct, state.Budget.EstimatedUsed, state.Budget.MaxTokens)
			} else if state.Budget.ShouldWarn() {
				state.Budget.MarkWarned()
				fmt.Printf("\x1b[33m[iter %d] ⚠ context at %d%% (est. %d / %d tokens)\x1b[0m\n",
					iter, pct, state.Budget.EstimatedUsed, state.Budget.MaxTokens)
			} else {
				fmt.Printf("\x1b[36m[iter %d] context %d%% (est. %d / %d tokens)\x1b[0m\n",
					iter, pct, state.Budget.EstimatedUsed, state.Budget.MaxTokens)
			}
		}

		// Auto-compact if budget exceeded
		if state.Budget.ShouldCompact() {
			before := state.Budget.EstimatedUsed
			messages = le.compactMessages(messages)
			state.Budget.EstimatedUsed = EstimateTokens(le.messagesToText(messages))
			state.Budget.Compacted++
			state.Budget.Warned = false // reset so we warn again if needed
			fmt.Printf("\x1b[33m[iter %d] compact #%d: %d → %d tokens (est.)\x1b[0m\n",
				iter, state.Budget.Compacted, before, state.Budget.EstimatedUsed)
		}

		// Call provider
		req := provider.Request{
			Model:       le.cfg.Model,
			Temperature: le.cfg.Temperature,
			MaxTokens:   le.cfg.MaxTokens,
			JSON:        true,
			Messages:    messages,
			Tools:       le.providerToolSpecs(),
			Reasoning: provider.ReasoningOptions{
				Effort:       le.cfg.Reasoning.Effort,
				BudgetTokens: le.cfg.Reasoning.BudgetTokens,
			},
		}

		resp, err := le.provider.Complete(ctx, req)
		if err != nil {
			state.FinishReason = "error"
			return state, fmt.Errorf("provider complete: %w", err)
		}

		// Update budget with response size
		state.Budget.EstimatedUsed += resp.Usage.TotalTokens
		state.Progress = fmt.Sprintf("iteration %d: parse response", iter)

		modelText := resp.Text
		messages = append(messages, provider.Message{Role: "assistant", Content: modelText})

		// If no tool calls from provider, try to parse XML tool calls from text
		toolCalls := resp.ToolCalls
		if len(toolCalls) == 0 {
			toolCalls = le.parseXMLToolCalls(modelText)
		}

		// Parse envelope to extract actions and task updates
		envelope := le.parseEnvelope(modelText)

		// Log the decision
		var actionNames []string
		for _, a := range envelope.Actions {
			actionNames = append(actionNames, a.Label())
		}
		decision := DecisionLogEntry{
			Iteration:  iter,
			ModelText:  trimText(modelText, 500),
			Actions:    actionNames,
			TaskStatus: le.currentTaskStatus(envelope.Tasks),
			Timestamp:  time.Now().UTC(),
		}
		state.Decisions = append(state.Decisions, decision)

		// Apply task updates
		if len(envelope.Tasks) > 0 && le.taskStore != nil {
			applied, err := le.taskStore.ApplyUpdates(envelope.Tasks)
			if err == nil {
				state.TaskStatus = le.currentTaskStatus(envelope.Tasks)
				for _, t := range applied {
					if t.ID == taskID {
						state.TaskStatus = t.Status
					}
				}
			}
		}

		// Check if model signaled done or blocked
		if state.TaskStatus == task.StatusDone || state.TaskStatus == task.StatusBlocked {
			state.Done = state.TaskStatus == task.StatusDone
			state.Blocked = state.TaskStatus == task.StatusBlocked
			state.FinishReason = state.TaskStatus
			break
		}

		if len(toolCalls) == 0 {
			state.FinishReason = "done"
			state.Done = true
			break
		}

		state.Progress = fmt.Sprintf("iteration %d: run %d tool(s)", iter, len(toolCalls))
		// Execute tool calls
		for _, tc := range toolCalls {
			raw, err := le.toolRunner.Run(ctx, tc.Name, tc.Input)
			output := formatToolOutput(tc.Name, raw, err)
			success := err == nil
			state.ToolResults = append(state.ToolResults, ToolResultEntry{
				ToolName:  tc.Name,
				RawOutput: raw,
				Success:   success,
				Output:    output,
			})

			// Self-correction: verify and attempt to fix errors
			if le.selfCorrection != nil {
				// Parse tool input for verification
				var inputMap map[string]any
				_ = json.Unmarshal(tc.Input, &inputMap)
				// Convert tool call to action proposal for verification
				action := tool.ActionProposal{
					Type:   tc.Name,
					Input:  inputMap,
					Reason: "Auto-generated from tool call",
				}
				correctionResult := le.selfCorrection.CorrectAction(ctx, action, output, err)
				if correctionResult.Attempts > 0 {
					if correctionResult.Success && correctionResult.FixedAction != nil {
						// Retry with fixed action
						fixedPayload := correctionResult.FixedAction.Payload()
						raw, err = le.toolRunner.Run(ctx, tc.Name, fixedPayload)
						output = formatToolOutput(tc.Name, raw, err)
						success = err == nil
						// Update the last tool result
						state.ToolResults[len(state.ToolResults)-1] = ToolResultEntry{
							ToolName:  tc.Name,
							RawOutput: raw,
							Success:   success,
							Output:    output + "\n[Self-corrected after " + fmt.Sprintf("%d", correctionResult.Attempts) + " attempt(s)]",
						}
					} else if correctionResult.Attempts > 0 {
						output += "\n[Self-correction attempted: " + fmt.Sprintf("%d", correctionResult.Attempts) + " attempt(s), failed]"
					}
				}
			}

			content := fmt.Sprintf("[TOOL RESULT %s]\n%s", tc.Name, output)
			if err != nil {
				content = fmt.Sprintf("[TOOL ERROR %s]: %v", tc.Name, err)
			}
			messages = append(messages, provider.Message{Role: "user", Content: content})
			state.Budget.EstimatedUsed += EstimateTokens(content)
		}

		// Refresh context snapshot (but not the conversation history part)
		snapshot = le.contextAsm.Assemble(goal, nil)
	}

	state.Messages = messages
	if state.FinishReason == "" {
		if iter >= maxIterations {
			state.FinishReason = "iteration_limit"
		} else {
			state.FinishReason = "done"
		}
	}

	// Log decision to memory
	le.logDecision(state)

	return state, nil
}

// Step executes a single iteration and returns control to the caller.
// This is used for TUI step-by-step mode where the user approves writes.
func (le *LoopEngine) Step(ctx context.Context, goal, taskID string, messages []provider.Message, snapshot Snapshot) (*LoopState, error) {
	state := &LoopState{
		Iteration:  0,
		TaskID:     taskID,
		TaskStatus: task.StatusActive,
		Progress:   "step: model call",
	}

	req := provider.Request{
		Model:       le.cfg.Model,
		Temperature: le.cfg.Temperature,
		MaxTokens:   le.cfg.MaxTokens,
		JSON:        true,
		Messages:    messages,
		Tools:       le.providerToolSpecs(),
		Reasoning: provider.ReasoningOptions{
			Effort:       le.cfg.Reasoning.Effort,
			BudgetTokens: le.cfg.Reasoning.BudgetTokens,
		},
	}

	resp, err := le.provider.Complete(ctx, req)
	if err != nil {
		state.FinishReason = "error"
		return state, fmt.Errorf("provider complete: %w", err)
	}

	state.Progress = "step: parse response"
	state.Iteration = 1
	modelText := resp.Text
	messages = append(messages, provider.Message{Role: "assistant", Content: modelText})
	state.Messages = messages

	envelope := le.parseEnvelope(modelText)

	var actionNames []string
	for _, a := range envelope.Actions {
		actionNames = append(actionNames, a.Label())
	}
	decision := DecisionLogEntry{
		Iteration:  1,
		ModelText:  trimText(modelText, 500),
		Actions:    actionNames,
		TaskStatus: le.currentTaskStatus(envelope.Tasks),
		Timestamp:  time.Now().UTC(),
	}
	state.Decisions = append(state.Decisions, decision)

	if len(envelope.Tasks) > 0 && le.taskStore != nil {
		applied, _ := le.taskStore.ApplyUpdates(envelope.Tasks)
		state.TaskStatus = le.currentTaskStatus(envelope.Tasks)
		for _, t := range applied {
			if t.ID == taskID {
				state.TaskStatus = t.Status
			}
		}
	}

	if resp.ToolCalls == nil || len(resp.ToolCalls) == 0 {
		state.Done = true
		state.FinishReason = "done"
		return state, nil
	}

	state.Progress = fmt.Sprintf("step: run %d tool(s)", len(resp.ToolCalls))
	// Execute tools and append results as user messages
	for _, tc := range resp.ToolCalls {
		raw, err := le.toolRunner.Run(ctx, tc.Name, tc.Input)
		output := formatToolOutput(tc.Name, raw, err)
		state.ToolResults = append(state.ToolResults, ToolResultEntry{
			ToolName:  tc.Name,
			RawOutput: raw,
			Success:   err == nil,
			Output:    output,
		})
		content := fmt.Sprintf("[TOOL RESULT %s]\n%s", tc.Name, output)
		if err != nil {
			content = fmt.Sprintf("[TOOL ERROR %s]: %v", tc.Name, err)
		}
		messages = append(messages, provider.Message{Role: "user", Content: content})
	}
	state.Messages = messages

	state.FinishReason = "tool_calls_returned"
	return state, nil
}

// BuildInitialMessages constructs the starting message list for a goal.
func (le *LoopEngine) BuildInitialMessages(goal string, sess *session.Session) ([]provider.Message, Snapshot) {
	snapshot := le.contextAsm.Assemble(goal, sess)
	systemPrompt := le.buildSystemPrompt(goal)
	userPrompt := le.buildUserPrompt(goal, snapshot)
	return []provider.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, snapshot
}

type stepEnvelope struct {
	Actions []tool.ActionProposal `json:"actions"`
	Tasks   []task.Update         `json:"tasks"`
}

func (le *LoopEngine) buildSystemPrompt(goal string) string {
	var parts []string

	// Base Marcus instructions (always needed for proper tool use and JSON response)
	marcosBase := `You are Marcus, a terminal-native coding assistant. Work methodically: read the project map and existing context first, identify the one most relevant file or subsystem, and prefer a targeted read/search over broad repository scans. Follow the current pattern, make the smallest safe change, verify the result, and capture durable repo facts in the project map when they will help future tasks. Return JSON with "message", "actions", and "tasks" fields. Keep "message" human-readable and concise. Mark tasks active when implementing, done when complete, blocked when stuck. IMPORTANT: Use todo_write to track all tasks, especially for multi-step goals.`

	// Use custom system prompt if set (e.g., for agent roles)
	if le.customSystemPrompt != "" {
		parts = append(parts, le.customSystemPrompt)
		// Append Marcus base instructions so agents know about todo_write and JSON format
		parts = append(parts, "\n\n---\n\n"+marcosBase)
	} else {
		parts = append(parts, marcosBase)
	}

	if goal != "" {
		parts = append(parts, fmt.Sprintf("\n\nCurrent Goal: %s", goal))
	}
	le.goalMu.Lock()
	stack := append([]string(nil), le.goalStack...)
	le.goalMu.Unlock()
	if len(stack) > 0 {
		var lines []string
		for i, g := range stack {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, g))
		}
		parts = append(parts, "\n\nGoal stack:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "")
}

func (le *LoopEngine) buildUserPrompt(goal string, snapshot Snapshot) string {
	var sections []string
	sections = append(sections, fmt.Sprintf("Goal: %s", goal))
	sections = append(sections, fmt.Sprintf("\n\n%s", snapshot.Text))
	return strings.TrimSpace(strings.Join(sections, "\n"))
}

// messagesToText returns the concatenated text of all messages (for token estimation).
func (le *LoopEngine) messagesToText(messages []provider.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// compactMessages removes verbose tool-result content and summarises the
// conversation history to keep the context window under budget.
// It always keeps the system prompt and the last user/assistant exchange intact.
func (le *LoopEngine) compactMessages(messages []provider.Message) []provider.Message {
	if len(messages) <= 2 {
		return messages
	}

	// Keep system prompt
	systemPrompt := messages[0]
	if systemPrompt.Role != "system" {
		// No system prompt found, nothing to compact
		return messages
	}

	// Identify the last assistant message index
	lastAssistant := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			lastAssistant = i
			break
		}
	}

	// Strategy: keep system, build a summary of the middle conversation,
	// keep the last assistant message and all messages after it.
	var compacted []provider.Message
	compacted = append(compacted, systemPrompt)

	if lastAssistant > 1 {
		// Summarise everything between system and last assistant turn
		var history strings.Builder
		for i := 1; i < lastAssistant; i++ {
			role := messages[i].Role
			content := messages[i].Content
			// Truncate each historical message to 200 chars
			if len(content) > 200 {
				content = content[:200] + "\n... [truncated]"
			}
			history.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
		}
		summary := fmt.Sprintf("[Prior conversation summarised — %d messages condensed]:\n%s",
			lastAssistant-1, history.String())
		if le.memory != nil {
			_ = le.memory.UpdateProjectSummary(summary)
		}
		compacted = append(compacted, provider.Message{Role: "user", Content: summary})
	}

	// Append last assistant turn (with its tool-call results attached)
	for i := lastAssistant; i < len(messages); i++ {
		// Truncate long tool results
		m := messages[i]
		if len(m.Content) > 2000 {
			m.Content = m.Content[:2000] + "\n[TOOL OUTPUT TRUNCATED]"
		}
		compacted = append(compacted, m)
	}

	return compacted
}

func (le *LoopEngine) providerToolSpecs() []provider.ToolSpec {
	if le.toolRunner == nil {
		return nil
	}
	defs := le.toolRunner.Definitions()
	specs := make([]provider.ToolSpec, 0, len(defs))
	for _, def := range defs {
		raw, _ := json.Marshal(def.Schema)
		specs = append(specs, provider.ToolSpec{
			Name:        def.Name,
			Description: def.Description,
			Schema:      raw,
		})
	}
	return specs
}

func (le *LoopEngine) parseEnvelope(text string) stepEnvelope {
	var env stepEnvelope
	text = strings.TrimSpace(text)
	if start := strings.Index(text, "{"); start != -1 {
		if end := strings.LastIndex(text, "}"); end > start {
			if err := json.Unmarshal([]byte(text[start:end+1]), &env); err == nil {
				return env
			}
		}
	}
	return env
}

// parseXMLToolCalls parses XML-format tool calls like <write_file><path>...</path>...</write_file>
func (le *LoopEngine) parseXMLToolCalls(text string) []provider.ToolCall {
	var calls []provider.ToolCall

	// Look for <tool_name>...</tool_name> patterns
	for _, toolName := range []string{"write_file", "read_file", "edit_file", "run_command", "glob_files", "search_code"} {
		openTag := "<" + toolName + ">"
		closeTag := "</" + toolName + ">"

		start := strings.Index(text, openTag)
		for start != -1 {
			end := strings.Index(text[start:], closeTag)
			if end == -1 {
				break
			}
			end += start

			content := text[start+len(openTag) : end]

			// Extract path if present
			path := ""
			if pathStart := strings.Index(content, "<path>"); pathStart != -1 {
				pathEnd := strings.Index(content[pathStart:], "</path>")
				if pathEnd != -1 {
					path = content[pathStart+6 : pathStart+pathEnd]
				}
			}

			// Extract content if present
			fileContent := ""
			if contentStart := strings.Index(content, "<content>"); contentStart != -1 {
				contentEnd := strings.Index(content[contentStart:], "</content>")
				if contentEnd != -1 {
					fileContent = content[contentStart+9 : contentStart+contentEnd]
				}
			}

			// Extract command if present
			command := ""
			if cmdStart := strings.Index(content, "<command>"); cmdStart != -1 {
				cmdEnd := strings.Index(content[cmdStart:], "</command>")
				if cmdEnd != -1 {
					command = content[cmdStart+9 : cmdStart+cmdEnd]
				}
			}

			// Build input JSON
			inputMap := make(map[string]interface{})
			if path != "" {
				inputMap["path"] = path
			}
			if fileContent != "" {
				inputMap["content"] = fileContent
			}
			if command != "" {
				inputMap["command"] = command
			}

			inputJSON, _ := json.Marshal(inputMap)
			calls = append(calls, provider.ToolCall{
				ID:    fmt.Sprintf("xml-%s-%d", toolName, len(calls)),
				Name:  toolName,
				Input: inputJSON,
			})

			// Find next occurrence
			start = strings.Index(text[end:], openTag)
			if start != -1 {
				start += end
			}
		}
	}

	return calls
}

func (le *LoopEngine) currentTaskStatus(updates []task.Update) string {
	for _, u := range updates {
		if u.Status != "" {
			switch u.Status {
			case task.StatusDone, task.StatusActive, task.StatusBlocked:
				return u.Status
			}
		}
	}
	return ""
}

func (le *LoopEngine) logDecision(state *LoopState) {
	if le.memory == nil || len(state.Decisions) == 0 {
		return
	}
	for _, d := range state.Decisions {
		actionSummary := strings.Join(d.Actions, ", ")
		if actionSummary == "" {
			actionSummary = "(none)"
		}
		content := fmt.Sprintf(
			"Iteration %d | Task: %s | Status: %s\nModel: %s\nActions: %s",
			d.Iteration, state.TaskID, d.TaskStatus, d.ModelText, actionSummary,
		)
		tags := []string{"loop", fmt.Sprintf("iter-%d", d.Iteration), state.TaskStatus}
		if state.TaskID != "" {
			tags = append(tags, "task:"+state.TaskID)
		}
		le.memory.Remember("decisions", "loop-decision", fmt.Sprintf("Loop iter %d", d.Iteration), content, "loop-engine", tags...)
		_ = le.memory.AppendEpisodic("assistant", d.ModelText)
	}
}

// PushGoal adds a sub-goal to the stack (most recent listed last).
func (le *LoopEngine) PushGoal(g string) {
	g = strings.TrimSpace(g)
	if g == "" {
		return
	}
	le.goalMu.Lock()
	defer le.goalMu.Unlock()
	le.goalStack = append(le.goalStack, g)
}

// PopGoal removes the last pushed goal.
func (le *LoopEngine) PopGoal() {
	le.goalMu.Lock()
	defer le.goalMu.Unlock()
	if len(le.goalStack) == 0 {
		return
	}
	le.goalStack = le.goalStack[:len(le.goalStack)-1]
}

// Goals returns a copy of the goal stack.
func (le *LoopEngine) Goals() []string {
	le.goalMu.Lock()
	defer le.goalMu.Unlock()
	return append([]string(nil), le.goalStack...)
}

// ToolRunner returns the tool runner used by this loop engine.
func (le *LoopEngine) ToolRunner() *tool.ToolRunner {
	return le.toolRunner
}

// Provider returns the provider runtime used by this loop engine.
func (le *LoopEngine) Provider() *provider.Runtime {
	return le.provider
}

// TaskStore returns the task store used by this loop engine.
func (le *LoopEngine) TaskStore() *task.Store {
	return le.taskStore
}

// ContextAssembler returns the context assembler used by this loop engine.
func (le *LoopEngine) ContextAssembler() ContextAssembler {
	return le.contextAsm
}

func trimText(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[:limit]) + "\n...[truncated]"
}
