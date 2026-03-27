package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/isolation"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/tool"
)

func (m *Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.thinkingCardIndex >= 0 && m.thinkingCardIndex < len(m.transcript) {
			m.tickThinkingCard()
		}
		return m, tickIfNeeded(m)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.renderTranscript()
		return m, nil
	case tea.MouseMsg:
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	case loopEventMsg:
		switch event := msg.event.(type) {
		case nil:
			return m, nil
		case agentStatusMsg:
			m.status = strings.TrimSpace(event.meta)
			if m.status == "" {
				m.status = "working"
			}
			m.updateActivity(event.body, event.meta, event.phase)
			return m, waitForLoopEvent(msg.ch)
		case agentStepMsg:
			m.addItem(event.kind, event.title, event.body, event.meta)
			return m, waitForLoopEvent(msg.ch)
		case sideDiffMsg:
			// Diff is now shown inline in the transcript
			m.sideDiffLive = event.text
			m.sideDiffTitle = event.title
			return m, waitForLoopEvent(msg.ch)
		case assistantResponseMsg:
			updatedModel, cmd := m.Update(event)
			if cmd != nil {
				return updatedModel, tea.Batch(cmd, waitForLoopEvent(msg.ch))
			}
			return updatedModel, waitForLoopEvent(msg.ch)
		case loopPausedMsg:

			return m, waitForLoopEvent(msg.ch)
		default:
			return m, waitForLoopEvent(msg.ch)
		}
	case streamOpenedMsg:
		if msg.err != nil {
			m.busy = false
			m.streaming = false
			m.status = "response failed"
			m.finishActivity("Provider Error", msg.err.Error(), "failed")
			return m, nil
		}
		m.streamBuffer.Reset()
		m.streamDiffSnippet = ""
		m.activeContext = msg.context
		m.streaming = true
		m.status = "streaming response"
		m.updateActivity("Calling provider and streaming response...", "live", "")
		return m, waitForStream(msg.stream, msg.context)
	case streamChunkMsg:
		if msg.chunk.Done {
			raw := m.streamBuffer.String()
			envelope := parseAssistantEnvelope(raw)
			return m, func() tea.Msg {
				return assistantResponseMsg{
					envelope: envelope,
					raw:      raw,
					context:  msg.context,
				}
			}
		}
		if msg.chunk.Text != "" {
			m.streamBuffer.WriteString(msg.chunk.Text)
			m.updateActivity(
				"Calling provider and streaming response...",
				fmt.Sprintf("Received %d characters so far.", m.streamBuffer.Len()),
				"",
			)
		}
		return m, waitForStream(msg.stream, msg.context)
	case assistantResponseMsg:
		m.busy = false
		m.streaming = false
		m.streamDiffSnippet = ""
		if msg.err != nil {
			m.status = "response failed"
			m.finishActivity("Provider Error", msg.err.Error(), "failed")
			m.CompletePlan()
			return m, nil
		}
		m.latestContext = msg.context
		message := visibleAssistantMessage(msg.envelope, msg.raw)
		m.finishActivity("Response Ready", "Marcus finished reasoning and prepared the response.", "complete")
		for _, result := range msg.autoResults {
			m.addItem("action", result.Proposal.Label(), "", "auto-run")
			m.addItem("result", result.Proposal.Label(), trimText(result.Output, 2000), "auto-run")
		}
		if msg.showItem || !msg.showItem && strings.TrimSpace(message) != "" && !hasAssistantTranscriptItem(m.transcript, message) {
			m.addItem("assistant", "Marcus", message, m.contextMeta(msg.context))
		}
		if strings.TrimSpace(message) != "" {
			m.session.AppendTurn("assistant", message, m.cfg.Session.MaxTurns)
		}
		if len(msg.envelope.Tasks) > 0 {
			applied, err := m.taskStore.ApplyUpdates(msg.envelope.Tasks)
			if err != nil {
				m.addItem("error", "Task Store", err.Error(), "")
			} else if len(applied) > 0 {
				var lines []string
				for _, t := range applied {
					lines = append(lines, fmt.Sprintf("- [%s] %s: %s", t.Status, t.ID, t.Title))
				}
				m.addItem("system", "Tasks Updated", strings.Join(lines, "\n"), "")
				m.refreshTaskBoard()
			}
		}
		m.pending = nil
		for _, proposal := range msg.envelope.Actions {
			preview, err := m.toolRunner.PreviewAction(proposal)
			if err != nil {
				m.addItem("error", "Action Preview Failed", fmt.Sprintf("%s\n\n%s", proposal.Label(), err), "")
				continue
			}
			m.pending = append(m.pending, pendingAction{Proposal: proposal, Preview: preview})

			// Format the action title in Claude Code style: "path/to/file — X lines"
			title := preview.Summary
			if proposal.Type == "write_file" && proposal.Path != "" {
				lineCount := strings.Count(preview.Diff, "\n+")
				if lineCount == 0 {
					lineCount = strings.Count(proposal.Content, "\n") + 1
				}
				title = fmt.Sprintf("%s — %d lines", proposal.Path, lineCount)
			}

			m.addItem("action", title, preview.Diff, proposal.Reason)
		}
		if len(m.pending) > 0 {
			m.sideDiffLive = ""
			m.sideDiffTitle = ""
			m.status = fmt.Sprintf("%d action(s) pending approval", len(m.pending))
			m.addItem("system", "Approval Required", "Press `y` to apply pending actions or `n` to discard them.", "")
			m.clampPendingDiffIndex()
		} else {
			m.status = "ready"
			m.pendingDiffIndex = 0
		}
		m.persistSession()
		return m, nil
	case appliedActionsMsg:
		if msg.err != nil {
			m.addItem("error", "Apply Failed", msg.err.Error(), "")
			m.status = "apply failed"

			m.agentContMu.Lock()
			cont := m.agentContinuation
			m.agentContMu.Unlock()

			if cont != nil {
				cont.toolResults = append(cont.toolResults, fmt.Sprintf("Apply Failed: %s", msg.err.Error()))
				m.busy = true
				m.retryCount = 0
				return m, m.resumeAgentLoopCmd(cont)
			}
			m.CompletePlan()
			return m, nil
		}
		if msg.session != nil && msg.session.Mode == isolation.ModeWorktree {
			m.addItem("system", "Isolation Active", fmt.Sprintf("Applied actions in worktree: %s", msg.session.Root), "worktree")
		}
		for _, result := range msg.results {
			body := result.Summary
			if result.Diff != "" {
				body += "\n\n" + result.Diff
			}
			if result.Output != "" {
				body += "\n\n" + trimText(result.Output, 1200)
			}
			meta := "applied"
			if !result.Success {
				meta = fmt.Sprintf("exit code %d", result.ExitCode)
			}
			m.addItem("result", result.Proposal.Label(), body, meta)
			m.session.AppendAction(result.Proposal.Label(), meta)
		}
		if paths := resultsToPaths(msg.results); len(paths) > 0 && m.codeIndex != nil && (msg.session == nil || msg.session.Mode == isolation.ModeInPlace) {
			_ = m.codeIndex.Refresh(context.Background(), paths)
		}
		if len(resultsToPaths(msg.results)) > 0 && m.contextAssembler != nil {
			m.contextAssembler.RefreshProjectMap()
		}
		if updates := m.reconcileTODOTasks(resultsToPaths(msg.results)); len(updates) > 0 {
			var lines []string
			for _, t := range updates {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", t.Status, t.ID, t.Title))
			}
			m.addItem("system", "Task Reconciliation", strings.Join(lines, "\n"), "")
			m.refreshTaskBoard()
		}
		m.pending = nil
		m.pendingDiffIndex = 0
		m.sideDiffLive = ""
		m.sideDiffTitle = ""
		m.status = "actions applied"
		m.addItem("system", "Apply Complete", "Pending actions were executed.", "")

		m.agentContMu.Lock()
		cont := m.agentContinuation
		m.agentContinuation = nil
		m.agentContMu.Unlock()
		if cont != nil {
			for _, r := range msg.results {
				out := trimText(r.Output, 2000)
				if strings.TrimSpace(out) == "" {
					out = trimText(r.Summary, 2000)
				}
				cont.toolResults = append(cont.toolResults, fmt.Sprintf("Tool (applied): %s\n%s", r.Proposal.Label(), out))
			}
		}

		m.persistSession()
		if prompt := m.recoveryPrompt(msg.results); prompt != "" && m.retryCount < m.cfg.Autonomy.RetryBudget {
			m.busy = true
			m.status = "diagnose"
			m.retryCount++
			m.addItem("system", "Verification Failed", "Marcus is diagnosing the failed verification and preparing a retry.", "retry")
			return m, m.sendRecoveryLoop(prompt)
		}
		if cont != nil {
			m.busy = true
			m.streaming = false
			m.retryCount = 0
			m.addItem("system", "Resuming", "Continuing the agent loop after your approval...", "")
			return m, m.resumeAgentLoopCmd(cont)
		}
		m.retryCount = 0
		return m, nil
	case undoPopMsg:
		if msg.err != nil {
			m.addItem("system", "Undo", msg.err.Error(), "")
			m.status = "undo failed"
			return m, nil
		}
		m.addItem("system", "Undo", fmt.Sprintf("Restored %d file(s).", msg.restored), "")
		m.status = "undone"
		if len(msg.paths) > 0 && m.codeIndex != nil {
			_ = m.codeIndex.Refresh(context.Background(), msg.paths)
		}
		m.persistSession()
		return m, nil
	}

	switch m.focusPane {
	case focusTranscript:
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd
	default:
		ta, cmd := m.textarea.Update(msg)
		m.textarea = ta
		return m, cmd
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlQ:
		m.persistSession()
		return m, tea.Quit
	case tea.KeyTab:
		m.cycleFocus()
		return m, nil
	case tea.KeyCtrlL:
		m.transcript = nil
		m.renderTranscript()
		return m, nil
	case tea.KeyCtrlY:
		if len(m.pending) == 0 {
			return m, nil
		}
		m.status = "applying actions"
		m.addItem("system", "Applying Pending Actions", fmt.Sprintf("Applying %d pending action(s)...", len(m.pending)), "")
		return m, m.applyPendingActions()
	case tea.KeyCtrlN:
		if len(m.pending) == 0 {
			return m, nil
		}
		m.clearAgentContinuation()
		for _, action := range m.pending {
			m.session.AppendAction(action.Proposal.Label(), "discarded")
		}
		m.pending = nil
		m.pendingDiffIndex = 0
		m.addItem("system", "Pending Actions Cleared", "Discarded all pending proposals.", "")
		m.status = "pending actions discarded"
		m.persistSession()
		return m, nil
	case tea.KeyRunes:
		switch strings.ToLower(msg.String()) {
		case "y":
			if len(m.pending) > 0 {
				m.status = "applying actions"
				m.addItem("system", "Applying Pending Actions", fmt.Sprintf("Applying %d pending action(s)...", len(m.pending)), "")
				return m, m.applyPendingActions()
			}
		case "n":
			if len(m.pending) > 0 {
				m.clearAgentContinuation()
				for _, action := range m.pending {
					m.session.AppendAction(action.Proposal.Label(), "discarded")
				}
				m.pending = nil
				m.pendingDiffIndex = 0
				m.addItem("system", "Pending Actions Cleared", "Discarded all pending proposals.", "")
				m.status = "pending actions discarded"
				m.persistSession()
				return m, nil
			}
		case "[":
			if len(m.pending) > 1 {
				m.pendingDiffIndex--
				if m.pendingDiffIndex < 0 {
					m.pendingDiffIndex = len(m.pending) - 1
				}
				return m, nil
			}
		case "]":
			if len(m.pending) > 1 {
				m.pendingDiffIndex = (m.pendingDiffIndex + 1) % len(m.pending)
				return m, nil
			}
		}
	case tea.KeySpace:

		if m.busy && m.stepPaused {

			m.stepPaused = false
			m.status = "resuming"
			m.addItem("system", "Resuming", "Continuing agent loop...", "")
			if m.stepSignal != nil {
				close(m.stepSignal)
				m.stepSignal = nil
			}
			return m, nil
		}

		m.stepMode = !m.stepMode
		if m.stepMode {
			m.addItem("system", "Step Mode", "Space pauses before each cooking phase.", "")
		} else {
			m.addItem("system", "Step Mode", "Step mode disabled.", "")
		}
		return m, nil
	case tea.KeyEnter:
		if m.focusPane != focusComposer || m.busy {
			return m, nil
		}
		content := strings.TrimSpace(m.textarea.Value())
		if content == "" {
			return m, nil
		}
		if content == "/newsession" || content == "/new" {
			return m.resetSession(), nil
		}
		if strings.EqualFold(content, "/undo") {
			m.textarea.SetValue("")
			return m, m.cmdUndoPop()
		}
		if strings.EqualFold(content, "/help") {
			m.textarea.SetValue("")
			m.addItem("system", "Help", renderHelpText(), "")
			m.status = "help"
			return m, nil
		}
		m.textarea.SetValue("")
		m.status = "thinking"
		m.busy = true
		m.retryCount = 0
		m.session.AppendTurn("user", content, m.cfg.Session.MaxTurns)
		m.addItem("user", "You", content, "")
		m.persistSession()
		return m, m.sendToAI(content)
	}

	switch m.focusPane {
	case focusComposer:
		ta, cmd := m.textarea.Update(msg)
		m.textarea = ta
		return m, cmd
	case focusTranscript:
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd
	}
	return m, nil
}

func (m *Model) cycleFocus() {
	m.focusPane++
	if m.focusPane > focusTranscript {
		m.focusPane = focusComposer
	}
	if m.focusPane == focusComposer {
		m.textarea.Focus()
	} else {
		m.textarea.Blur()
	}
}

func (m *Model) pushUndoBatch(batch tool.UndoBatch) {
	if batch.Root == "" || len(batch.Entries) == 0 {
		return
	}
	m.undoMu.Lock()
	defer m.undoMu.Unlock()
	m.undoStack = append(m.undoStack, batch)
}

func (m *Model) cmdUndoPop() tea.Cmd {
	return func() tea.Msg {
		m.undoMu.Lock()
		n := len(m.undoStack)
		if n == 0 {
			m.undoMu.Unlock()
			return undoPopMsg{err: fmt.Errorf("nothing to undo")}
		}
		batch := m.undoStack[n-1]
		m.undoStack = m.undoStack[:n-1]
		m.undoMu.Unlock()
		if err := tool.RestoreUndoBatch(batch); err != nil {
			return undoPopMsg{err: err}
		}
		paths := make([]string, 0, len(batch.Entries))
		for _, e := range batch.Entries {
			paths = append(paths, e.Rel)
		}
		return undoPopMsg{restored: len(batch.Entries), paths: paths}
	}
}

func (m *Model) applyPendingActions() tea.Cmd {
	pending := append([]pendingAction(nil), m.pending...)
	return func() tea.Msg {
		ctx := context.Background()
		proposals := make([]tool.ActionProposal, 0, len(pending))
		for _, action := range pending {
			proposals = append(proposals, action.Proposal)
		}
		isoSession, err := m.isolationManager.Prepare(ctx, proposals)
		if err != nil {
			return appliedActionsMsg{err: err}
		}
		runner, err := tool.BuildRunner(tool.BuildOptions{
			BaseDir:        isoSession.Root,
			Config:         m.cfg,
			Folders:        m.flowEngine.FolderEngine(),
			CodeIndex:      m.codeIndex,
			LSP:            m.lspBroker,
			SubagentRunner: flow.NewSubagentRunner(m.flowEngine.FolderEngine(), m.cfg, isoSession.Root),
		})
		if err != nil {
			return appliedActionsMsg{err: err}
		}
		results, undoBatch, err := runner.ApplyProposalsInTransaction(ctx, proposals)
		if err != nil {
			return appliedActionsMsg{err: err}
		}
		m.pushUndoBatch(undoBatch)
		return appliedActionsMsg{results: results, session: isoSession}
	}
}

func (m *Model) resetSession() tea.Model {
	m.clearAgentContinuation()
	m.persistSession()
	m.sideDiffLive = ""
	m.sideDiffTitle = ""
	m.streamDiffSnippet = ""
	m.undoMu.Lock()
	m.undoStack = nil
	m.undoMu.Unlock()
	now := time.Now().UTC()
	m.session = &session.Session{
		ID:        now.Format("2006-01-02T15-04-05"),
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.pending = nil
	m.transcript = nil
	m.streamBuffer.Reset()
	m.status = "new session"
	m.activityIndex = -1
	m.taskBoardIndex = -1
	m.thinkingCardIndex = -1
	m.currentThinkingCard = -1
	m.textarea.SetValue("")
	m.bootstrapTranscript()
	m.persistSession()
	m.addItem("system", "New Session", "Started a fresh Marcus session.", "")
	return m
}

func (m *Model) clearAgentContinuation() {
	m.agentContMu.Lock()
	m.agentContinuation = nil
	m.agentContMu.Unlock()
}
