package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	ctxpkg "github.com/marcus-ai/marcus/internal/context"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/tool"
)

func (m *Model) sendToAI(content string) tea.Cmd {
	m.clearAgentContinuation()
	m.sideDiffLive = ""
	m.sideDiffTitle = ""
	m.streamDiffSnippet = ""
	if m.memoryManager != nil {
		if captured, err := m.memoryManager.CaptureUserFeedback(content); err == nil && len(captured) > 0 {
			var lines []string
			for _, entry := range captured {
				lines = append(lines, fmt.Sprintf("- [%s] %s", entry.Scope, entry.Title))
			}
			m.addItem("system", "Memory Updated", strings.Join(lines, "\n"), "")
		}
	}
	snapshot := m.contextAssembler.Assemble(content, m.session)
	if len(snapshot.TODOHints) > 0 {
		if synced, err := m.taskStore.SyncTODOHints(snapshot.TODOHints); err == nil && len(synced) > 0 {
			var lines []string
			for _, t := range synced {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", t.Status, t.ID, t.Title))
			}
			m.addItem("system", "TODO Tasks Synced", strings.Join(lines, "\n"), "")
			snapshot = m.contextAssembler.Assemble(content, m.session)
			m.refreshTaskBoard()
		}
	}
	m.streamBuffer.Reset()
	m.streaming = true
	m.activityIndex = len(m.transcript)
	m.addItem("system", "Marcus Working", "Assembling repo context and preparing the request...", "working")

	agent := m.selectAgent(content)
	m.currentAgent = agent
	if agent != nil {
		m.addItem("system", "Agent", agent.Role+": "+agent.Description, "")
		m.stepMode = agent.Rules.StepMode
	}

	return startAgentLoopCmd(m, content, snapshot, m.currentAgent)
}

func (m *Model) sendRecoveryLoop(content string) tea.Cmd {
	snapshot := m.contextAssembler.Assemble(content, m.session)
	m.streamBuffer.Reset()
	m.streaming = true
	m.activityIndex = len(m.transcript)
	m.addItem("system", "Marcus Retry", "Collecting failure context and retrying...", "retry")
	return startAgentLoopCmd(m, content, snapshot, m.currentAgent)
}

func (m *Model) addVerificationSummary(command string, exitCode int, ok bool, note string) {
	result := "FAILED"
	if ok {
		result = "PASSED"
	}
	body := fmt.Sprintf("Command: %s\nExit: %d\nResult: %s", command, exitCode, result)
	if strings.TrimSpace(note) != "" {
		body += "\nNote: " + note
	}
	meta := "verify-failed"
	if ok {
		meta = "verify-ok"
	}
	m.addItem("system", "Verification Summary", body, meta)
}

func waitForStream(stream <-chan provider.StreamChunk, context ctxpkg.Snapshot) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-stream
		if !ok {
			return streamChunkMsg{
				chunk:   provider.StreamChunk{Done: true},
				stream:  stream,
				context: context,
			}
		}
		return streamChunkMsg{
			chunk:   chunk,
			stream:  stream,
			context: context,
		}
	}
}

func startAgentLoopCmd(m *Model, content string, snapshot ctxpkg.Snapshot, agent *folder.AgentDef) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			m.runAgentLoopAsync(content, snapshot, agent, ch, nil)
			close(ch)
		}()
		return loopEventMsg{
			event: agentStatusMsg{
				body: "Starting agent loop...",
				meta: "working",
			},
			ch: ch,
		}
	}
}

func waitForLoopEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return loopEventMsg{event: nil, ch: ch}
		}
		return loopEventMsg{event: event, ch: ch}
	}
}

func (m *Model) resumeAgentLoopCmd(cont *agentContinuation) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			m.runAgentLoopAsync("", ctxpkg.Snapshot{}, m.currentAgent, ch, cont)
			close(ch)
		}()
		return loopEventMsg{
			event: agentStatusMsg{body: "Resuming agent after your approval...", meta: "working"},
			ch:    ch,
		}
	}
}

func (m *Model) runAgentLoopAsync(content string, snapshot ctxpkg.Snapshot, agent *folder.AgentDef, ch chan<- tea.Msg, resume *agentContinuation) {
	if resume == nil {
		m.clearAgentContinuation()
	}

	toolResults := []string{}
	currentSnapshot := snapshot
	var lastRaw string
	lastActionSignature := ""
	stagnationCount := 0
	startLoop := 1

	maxIterations := 10
	if agent != nil && agent.Autonomy.IterationLimit > 0 {
		maxIterations = agent.Autonomy.IterationLimit
	} else if m.cfg != nil {
		maxIterations = max(1, m.cfg.Autonomy.MaxIterations)
	}

	if resume != nil {
		content = resume.userContent
		toolResults = append([]string(nil), resume.toolResults...)
		startLoop = resume.startLoop
		maxIterations = resume.maxIterations
		lastActionSignature = resume.lastActionSignature
		stagnationCount = resume.stagnationCount
		currentSnapshot = m.contextAssembler.Assemble(content, m.session)
	}

	goalContent := content
	requestContent := goalContent

	consecutivePlanOnlyResponses := 0

	// Track live card index for thinking updates
	var thinkingCardIndex int = -1

	cookingPhases := []string{"Preheating", "Frying", "Sizzling", "Simmering", "Browning", "Searing", "Roasting", "Grilling", "Baking", "Finishing"}
	cookPhase := func(i int) string {
		if i >= 1 && i <= len(cookingPhases) {
			return cookingPhases[i-1]
		}
		return cookingPhases[len(cookingPhases)-1]
	}

	for loop := startLoop; loop <= maxIterations; loop++ {
		iterStart := time.Now()
		phase := cookPhase(loop)
		m.addItem("iteration", phase, "Planning, execution, and verification", "")

		if m.stepMode {
			m.stepPaused = true
			m.status = "paused (step mode)"
			m.stepSignal = make(chan struct{})
			m.addItem("system", "Step Mode Active", fmt.Sprintf("%s: press Space to continue", phase), "")
			ch <- agentStatusMsg{body: fmt.Sprintf("paused before %s", phase), meta: "paused"}

			ch <- loopPausedMsg{iteration: loop}
			<-m.stepSignal
			m.stepPaused = false
			m.status = "running"
		}

		ch <- agentStatusMsg{
			body:  fmt.Sprintf("%s: thinking and calling provider...", phase),
			meta:  "planning",
			phase: phase,
		}

		prompt := buildPromptForAgent(m.toolRunner, currentSnapshot, m.session, requestContent, toolResults, agent)
		request := provider.Request{
			Model:       m.cfg.Model,
			Temperature: m.cfg.Temperature,
			MaxTokens:   m.cfg.MaxTokens,
			JSON:        true,
			Messages: []provider.Message{
				{Role: "system", Content: buildAgentSystemPrompt(agent)},
				{Role: "user", Content: prompt},
			},
			Tools: providerToolSpecs(m.toolRunner),
		}
		ctx := context.Background()
		stream, err := m.providerRuntime.Stream(ctx, request)
		if err != nil {
			m.clearAgentContinuation()
			ch <- assistantResponseMsg{err: err}
			return
		}

		// Stream thinking text in real-time
		var streamBuffer strings.Builder
		charCount := 0
		toolCallsSeen := 0
		for event := range stream {
			if event.Done {
				break
			}

			if event.Text != "" {
				streamBuffer.WriteString(event.Text)
				charCount += len(event.Text)

				if charCount%200 < len(event.Text) {
					ch <- agentStatusMsg{
						body:  fmt.Sprintf("Thinking... (%d chars)", charCount),
						meta:  "thinking",
						phase: phase,
					}
				}

				if thinkingCardIndex >= 0 {
					m.updateTranscriptItem(thinkingCardIndex, "thinking", "Marcus is thinking...", thinkingCardBody(charCount, toolCallsSeen), fmt.Sprintf("%d chars", charCount))
				} else {
					thinkingCardIndex = len(m.transcript)
					m.addItem("thinking", "Marcus is thinking...", thinkingCardBody(charCount, toolCallsSeen), fmt.Sprintf("%d chars", charCount))
				}
			}

			if event.ToolCall != nil {
				toolCallsSeen++
				body := formatToolCallHuman(event.ToolCall.Name, event.ToolCall.Input)
				m.addItem("tool_call", fmt.Sprintf("Tool #%d: %s", toolCallsSeen, event.ToolCall.Name), body, "provider-call")
				if thinkingCardIndex >= 0 {
					m.updateTranscriptItem(thinkingCardIndex, "thinking", "Marcus is thinking...", thinkingCardBody(charCount, toolCallsSeen), fmt.Sprintf("%d chars", charCount))
				}
				ch <- agentStatusMsg{
					body:  fmt.Sprintf("Provider requested tool: %s", event.ToolCall.Name),
					meta:  "tool-call",
					phase: phase,
				}
			}
		}

		if thinkingCardIndex >= 0 && thinkingCardIndex < len(m.transcript) {
			m.transcript[thinkingCardIndex].Kind = "thinking"
		}
		thinkingCardIndex = -1

		lastRaw = streamBuffer.String()
		envelope := parseAssistantEnvelope(lastRaw)

		modelMessage := visibleAssistantMessage(envelope, lastRaw)
		m.addItem("assistant", "Marcus", modelMessage, "")

		elapsed := time.Since(iterStart).Round(time.Second)
		ch <- agentStatusMsg{
			body:  fmt.Sprintf("%s — done in %v: %d action(s) parsed", phase, elapsed, len(envelope.Actions)),
			meta:  "analyzing",
			phase: phase,
		}

		if len(envelope.Actions) == 0 {
			maxPlanOnlyRetries := 4
			if consecutivePlanOnlyResponses < maxPlanOnlyRetries && loop < maxIterations {
				consecutivePlanOnlyResponses++
				maxTurns := 50
				if m.cfg != nil {
					maxTurns = m.cfg.Session.MaxTurns
				}
				if strings.TrimSpace(modelMessage) != "" {
					m.session.AppendTurn("assistant", modelMessage, maxTurns)
				}
				toolResults = append(toolResults, planOnlyNoActionsNudge(consecutivePlanOnlyResponses, maxPlanOnlyRetries))
				requestContent = planOnlyRetryPrompt(goalContent, modelMessage, consecutivePlanOnlyResponses, maxPlanOnlyRetries)
				m.addItem("system", "No Tool Actions", fmt.Sprintf("Response had no actions in JSON — nudging for concrete tool proposals (%d/%d).", consecutivePlanOnlyResponses, maxPlanOnlyRetries), "retry")
				ch <- agentStatusMsg{
					body:  fmt.Sprintf("%s: empty actions[] — retrying for concrete tool calls", phase),
					meta:  "retry",
					phase: phase,
				}
				currentSnapshot = m.contextAssembler.Assemble(goalContent, m.session)
				continue
			}
			m.clearAgentContinuation()
			ch <- assistantResponseMsg{
				envelope: assistantEnvelope{
					Message: fmt.Sprintf("Marcus stopped after %d plan-only responses without concrete tool actions. The model kept describing implementation but did not emit executable steps.", maxPlanOnlyRetries),
				},
				raw:      lastRaw,
				context:  currentSnapshot,
				showItem: true,
			}
			return
		}
		consecutivePlanOnlyResponses = 0
		requestContent = goalContent

		actionSignature := actionPlanSignature(envelope.Actions)
		if actionSignature != "" && actionSignature == lastActionSignature {
			stagnationCount++
		} else {
			stagnationCount = 0
		}
		lastActionSignature = actionSignature
		maxStagnationRetries := 4
		if stagnationCount >= 2 {
			if stagnationCount < maxStagnationRetries && loop < maxIterations {
				toolResults = append(toolResults, stagnationNoProgressNudge(stagnationCount, maxStagnationRetries))
				requestContent = stagnationRetryPrompt(goalContent, modelMessage, stagnationCount, maxStagnationRetries)
				m.addItem("system", "Loop Guard", fmt.Sprintf("Marcus detected a repeated action plan and is nudging for a different next step (%d/%d before stop).", stagnationCount, maxStagnationRetries), "retry")
				ch <- agentStatusMsg{
					body:  fmt.Sprintf("%s: repeated action plan — asking for a different concrete step", phase),
					meta:  "retry",
					phase: phase,
				}
				currentSnapshot = m.contextAssembler.Assemble(goalContent, m.session)
				continue
			}
			m.clearAgentContinuation()
			m.addItem("system", "Loop Guard", "Marcus detected repeated identical action plans and paused to avoid a retry loop. Please adjust the request or approve a different action path.", "stopped")
			ch <- assistantResponseMsg{
				envelope: assistantEnvelope{
					Message: "I am repeating the same plan and stopped to avoid a loop. I need a revised approach or additional context.",
				},
				raw:      lastRaw,
				context:  currentSnapshot,
				showItem: true,
			}
			return
		}

		for i, action := range envelope.Actions {
			detail := formatActionHuman(action)
			reason := valueOr(action.Reason, "")
			if reason != "" {
				detail = detail + "\nReason: " + reason
			}
			m.addItem("action", fmt.Sprintf("Proposal #%d: %s", i+1, action.Label()), detail, "pending-review")
		}

		var side strings.Builder
		for _, a := range envelope.Actions {
			prev, err := m.toolRunner.PreviewAction(a)
			if err != nil {
				continue
			}
			if strings.TrimSpace(prev.Diff) != "" {
				side.WriteString("// ")
				side.WriteString(prev.Summary)
				side.WriteString("\n")
				side.WriteString(prev.Diff)
				side.WriteString("\n\n")
			}
		}
		if side.Len() > 0 {
			ch <- sideDiffMsg{text: strings.TrimSpace(side.String()), title: "Proposed changes (live preview)"}
		}

		if m.stepMode {
			m.stepPaused = true
			m.status = "step: review actions"
			m.stepPending = true
			m.stepSignal = make(chan struct{})
			ch <- agentStatusMsg{body: "paused: review proposals above", meta: "paused"}
			ch <- loopPausedMsg{iteration: loop}
			<-m.stepSignal
			m.stepPaused = false
			m.stepPending = false
			m.status = "running"
		}

		safeActions, pendingActions := m.partitionActions(envelope.Actions)

		txBatch, snapErr := tool.SnapshotUndoBatch(m.projectRoot, safeActions)
		if snapErr != nil {
			m.clearAgentContinuation()
			ch <- assistantResponseMsg{err: snapErr}
			return
		}
		mutating := len(tool.MutatingPaths(safeActions)) > 0

		wroteFiles := false
		for _, action := range safeActions {
			m.addItem("action", "Running: "+action.Label(), valueOr(action.Reason, "safe auto-run"), "auto-run")
			ch <- agentStatusMsg{
				body:  fmt.Sprintf("Executing: %s", action.Label()),
				meta:  "running-tool",
				phase: phase,
			}
			result, err := m.toolRunner.ApplyAction(context.Background(), action)
			if err != nil {
				if mutating {
					_ = tool.RestoreUndoBatch(txBatch)
				}
				errStr := fmt.Sprintf("Error running tool: %s", err.Error())
				toolResults = append(toolResults, fmt.Sprintf("Tool: %s\n%s", action.Label(), errStr))
				m.addItem("tool_result", "Error: "+action.Label(), errStr, "failed")
				ch <- agentStatusMsg{
					body:  fmt.Sprintf("Failed: %s", action.Label()),
					meta:  "tool-error",
					phase: phase,
				}
				wroteFiles = false
				break
			}
			toolResults = append(toolResults, fmt.Sprintf("Tool: %s\n%s", action.Label(), result.Output))
			m.addItem("tool_result", "Result: "+action.Label(), trimText(result.Output, 1500), "auto")
			ch <- agentStatusMsg{
				body:  fmt.Sprintf("Completed: %s", action.Label()),
				meta:  "tool-done",
				phase: phase,
			}
			switch action.Type {
			case "write_file", "patch_file", "edit_file", "create_file":
				wroteFiles = true
			}
		}
		if mutating {
			m.pushUndoBatch(txBatch)
		}

		if wroteFiles {
			verifyCmd := m.detectVerifyCommand()
			if verifyCmd != "" {
				m.addItem("action", "Verifying: "+verifyCmd, "auto-run build/test check", "auto-run")
				ch <- agentStatusMsg{
					body:  fmt.Sprintf("Running verification: %s", verifyCmd),
					meta:  "running-tool",
					phase: phase,
				}
				verifyResult, verifyErr := m.toolRunner.ApplyAction(context.Background(), tool.ActionProposal{
					Type:    "run_command",
					Command: verifyCmd,
					Reason:  "verification after file changes",
				})
				if verifyErr == nil {
					toolResults = append(toolResults, fmt.Sprintf("Verification: %s\n%s", verifyCmd, verifyResult.Output))
					successMeta := "passed"
					if !verifyResult.Success {
						successMeta = "FAILED"
					}
					m.addItem("tool_result", "Verify: "+verifyCmd, trimText(verifyResult.Output, 1500), successMeta)
					ch <- agentStatusMsg{
						body:  fmt.Sprintf("Verification result: exit %d — %s", verifyResult.ExitCode, verifyCmd),
						meta:  "tool-done",
						phase: phase,
					}

					if !verifyResult.Success {
						if installCmd := m.detectDependencyInstallCommand(verifyResult.Output); installCmd != "" {
							m.addItem("action", "Auto-install dependency", installCmd, "auto-repair")
							ch <- agentStatusMsg{
								body:  fmt.Sprintf("Trying dependency fix: %s", installCmd),
								meta:  "running-tool",
								phase: phase,
							}
							installResult, installErr := m.toolRunner.ApplyAction(context.Background(), tool.ActionProposal{
								Type:    "run_command",
								Command: installCmd,
								Reason:  "automatic dependency install after verification failure",
							})
							if installErr == nil {
								toolResults = append(toolResults, fmt.Sprintf("Dependency install: %s\n%s", installCmd, installResult.Output))
								meta := "FAILED"
								if installResult.Success {
									meta = "passed"
								}
								m.addItem("tool_result", "Deps: "+installCmd, trimText(installResult.Output, 1200), meta)
								if installResult.Success {
									reverifyResult, reverifyErr := m.toolRunner.ApplyAction(context.Background(), tool.ActionProposal{
										Type:    "run_command",
										Command: verifyCmd,
										Reason:  "re-verify after dependency install",
									})
									if reverifyErr == nil {
										toolResults = append(toolResults, fmt.Sprintf("Re-verify: %s\n%s", verifyCmd, reverifyResult.Output))
										reMeta := "FAILED"
										if reverifyResult.Success {
											reMeta = "passed"
										}
										m.addItem("tool_result", "Re-verify: "+verifyCmd, trimText(reverifyResult.Output, 1500), reMeta)
										verifyResult = reverifyResult
									}
								}
							}
						}

						failMsg := fmt.Sprintf("Build/test failed:\n%s\n\nFix the errors above and retry.", verifyResult.Output)
						m.addVerificationSummary(verifyCmd, verifyResult.ExitCode, false, "Automatic verification failed")
						m.addItem("system", "Build Failed", failMsg, "error")
						ch <- agentStatusMsg{
							body:  "Build failed — feeding error back to model for self-correction",
							meta:  "retry",
							phase: phase,
						}

						toolResults = append(toolResults, failMsg)
					} else {
						m.addVerificationSummary(verifyCmd, verifyResult.ExitCode, true, "Checks passed")
						m.addItem("system", "Build Passed", "All checks passed.", "complete")
					}
				}
			}
		}

		if len(pendingActions) > 0 {
			var previewErrors []string
			for _, action := range pendingActions {
				_, err := m.toolRunner.PreviewAction(action)
				if err != nil {
					errStr := fmt.Sprintf("Action Validation Error: %s", err.Error())
					toolResults = append(toolResults, fmt.Sprintf("Tool %s failed: %s", action.Label(), errStr))
					m.addItem("tool_result", "Validation Error: "+action.Label(), errStr, "failed")
					previewErrors = append(previewErrors, errStr)
				}
			}

			if len(previewErrors) > 0 {
				ch <- agentStatusMsg{
					body:  "Validation failed — feeding error back to model",
					meta:  "retry",
					phase: phase,
				}
				currentSnapshot = m.contextAssembler.Assemble(goalContent, m.session)
				continue
			}

			m.addItem("system", "Approval Required", fmt.Sprintf("%d action(s) need your approval — press y to apply, n to discard", len(pendingActions)), "")
			m.stashAgentContinuationForPending(goalContent, loop, maxIterations, toolResults, lastActionSignature, stagnationCount)
			ch <- assistantResponseMsg{
				envelope: assistantEnvelope{Message: "Actions proposed — some need approval.", Actions: pendingActions},
				raw:      lastRaw,
				context:  currentSnapshot,
				showItem: false,
			}
			return
		}

		currentSnapshot = m.contextAssembler.Assemble(goalContent, m.session)
	}

	m.clearAgentContinuation()
	ch <- assistantResponseMsg{
		envelope: assistantEnvelope{
			Message: fmt.Sprintf("Reached iteration limit (%d). You can ask Marcus to continue.", maxIterations),
		},
		raw:      lastRaw,
		context:  currentSnapshot,
		showItem: false,
	}
}

func (m *Model) stashAgentContinuationForPending(
	content string,
	loop, maxIterations int,
	toolResults []string,
	lastSig string,
	stag int,
) {
	next := loop + 1
	maxIt := maxIterations
	if next > maxIt {
		maxIt = next
	}
	m.agentContMu.Lock()
	m.agentContinuation = &agentContinuation{
		userContent:         content,
		startLoop:           next,
		maxIterations:       maxIt,
		toolResults:         append([]string(nil), toolResults...),
		lastActionSignature: lastSig,
		stagnationCount:     stag,
	}
	m.agentContMu.Unlock()
}

func (m *Model) partitionActions(actions []tool.ActionProposal) ([]tool.ActionProposal, []tool.ActionProposal) {
	var safe []tool.ActionProposal
	var pending []tool.ActionProposal

	agent := m.currentAgent

	writeCount := 0
	cmdCount := 0
	for _, a := range actions {
		if a.Type == "write_file" {
			writeCount++
		}
		if a.Type == "run_command" {
			cmdCount++
		}
	}

	safeActions := map[string]bool{
		"list_files": true, "read_file": true,
		"search_code": true, "find_symbol": true, "list_symbols": true,
	}
	if agent != nil {
		for _, a := range agent.Rules.SafeActions {
			safeActions[a] = true
		}
	}

	autoRunPrefixes := []string{
		"go build", "cargo build", "npm run", "ruff", "go test", "go vet",
		"golangci-lint", "python -m", "mvn", "gradle", "make",
	}
	if agent != nil {
		autoRunPrefixes = append(autoRunPrefixes, agent.Rules.AutoRunCommands...)
	}

	writeIf := "first_wave"
	if agent != nil {
		writeIf = agent.Rules.WriteIf
	}

	for _, action := range actions {
		switch {

		case action.Type != "run_command" && (safeActions[action.Type] || m.toolRunner.IsSafe(action.Type)):
			safe = append(safe, action)

		case action.Type == "write_file":
			switch writeIf {
			case "always":
				safe = append(safe, action)
			case "first_wave":
				if writeCount == 1 && cmdCount == 0 {
					safe = append(safe, action)
				} else {
					pending = append(pending, action)
				}
			default:
				pending = append(pending, action)
			}

		case action.Type == "run_command":
			if isAutoRunCommand(action.Command, autoRunPrefixes) {
				safe = append(safe, action)
			} else {
				pending = append(pending, action)
			}

		default:
			pending = append(pending, action)
		}
	}
	return safe, pending
}

func isAutoRunCommand(command string, prefixes []string) bool {
	command = strings.TrimSpace(command)
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if command == prefix || strings.HasPrefix(command, prefix+" ") {
			return true
		}
	}
	return false
}

func (m *Model) isAutoApprovedCommand(command string) bool {
	command = strings.TrimSpace(command)
	for _, allowed := range m.cfg.Tools.RunCommand.AlwaysAllow {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if command == allowed || strings.HasPrefix(command, allowed+" ") {
			return true
		}
	}
	return false
}
