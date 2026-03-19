package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marcus-ai/marcus/internal/codeintel"
	"github.com/marcus-ai/marcus/internal/config"
	ctxpkg "github.com/marcus-ai/marcus/internal/context"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/isolation"
	"github.com/marcus-ai/marcus/internal/lsp"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

type transcriptItem struct {
	Kind  string
	Title string
	Body  string
	Meta  string
}

type pendingAction struct {
	Proposal tool.ActionProposal
	Preview  tool.ActionPreview
}

// flowContextAssembler wraps ctxpkg.Assembler so it satisfies flow.ContextAssembler.
type flowContextAssembler struct {
	delegate       *ctxpkg.Assembler
	toFlowSnapshot func(ctxpkg.Snapshot) flow.Snapshot
}

func (f flowContextAssembler) Assemble(input string, sess *session.Session) flow.Snapshot {
	return f.toFlowSnapshot(f.delegate.Assemble(input, sess))
}

// Model is the Marcus single-pane TUI model.
type Model struct {
	provider         provider.Provider
	providerRuntime  *provider.Runtime
	flowEngine       *flow.Engine
	loopEngine       *flow.LoopEngine
	toolRunner       *tool.ToolRunner
	codeIndex        *codeintel.Index
	lspBroker        *lsp.Broker
	memoryManager    *memory.Manager
	isolationManager *isolation.Manager
	cfg              *config.Config
	styles           Styles
	viewport         viewport.Model
	textarea         textarea.Model
	width            int
	height           int
	ready            bool
	inputFocused     bool
	busy             bool
	status           string
	transcript       []transcriptItem
	pending          []pendingAction
	session          *session.Session
	sessionStore     *session.Store
	contextAssembler *ctxpkg.Assembler
	taskStore        *task.Store
	latestContext    ctxpkg.Snapshot
	activeContext    ctxpkg.Snapshot
	streamBuffer     strings.Builder
	streaming        bool
	activityIndex    int
	taskBoardIndex   int
	retryCount       int
	stepMode         bool
	stepPaused       bool
	stepSignal       chan struct{}
	stepPending      bool
	currentAgent     *folder.AgentDef

	// Kitchen spinner state
	thinkingTicker    *time.Ticker
	thinkingFrame     int
	thinkingCardIndex int
	currentPhase      string // active cooking phase for spinner title
}

type assistantEnvelope struct {
	Message string                `json:"message"`
	Actions []tool.ActionProposal `json:"actions"`
	Tasks   []task.Update         `json:"tasks"`
}

type assistantResponseMsg struct {
	envelope    assistantEnvelope
	raw         string
	context     ctxpkg.Snapshot
	autoResults []tool.ActionResult
	showItem    bool
	err         error
}

type appliedActionsMsg struct {
	results []tool.ActionResult
	session *isolation.Session
	err     error
}

type streamOpenedMsg struct {
	stream  <-chan provider.StreamChunk
	context ctxpkg.Snapshot
	err     error
}

type streamChunkMsg struct {
	chunk   provider.StreamChunk
	stream  <-chan provider.StreamChunk
	context ctxpkg.Snapshot
}

type loopEventMsg struct {
	event tea.Msg
	ch    <-chan tea.Msg
}

type agentStatusMsg struct {
	body  string
	meta  string
	phase string // cooking phase for thinking card title
}

type agentStepMsg struct {
	kind  string
	title string
	body  string
	meta  string
}

type tickMsg struct{}

func (m Model) Init() tea.Cmd {
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
		case assistantResponseMsg:
			updatedModel, cmd := m.Update(event)
			if cmd != nil {
				return updatedModel, tea.Batch(cmd, waitForLoopEvent(msg.ch))
			}
			return updatedModel, waitForLoopEvent(msg.ch)
		case loopPausedMsg:
			// Step mode pause: agent loop has yielded, user controls resume via Space
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
		if msg.err != nil {
			m.status = "response failed"
			m.finishActivity("Provider Error", msg.err.Error(), "failed")
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
			m.addItem("action", preview.Summary, preview.Diff, proposal.Reason)
		}
		if len(m.pending) > 0 {
			m.status = fmt.Sprintf("%d action(s) pending approval", len(m.pending))
			m.addItem("system", "Approval Required", "Press `y` to apply pending actions or `n` to discard them.", "")
		} else {
			m.status = "ready"
		}
		m.persistSession()
		return m, nil
	case appliedActionsMsg:
		if msg.err != nil {
			m.addItem("error", "Apply Failed", msg.err.Error(), "")
			m.status = "apply failed"
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
		if updates := m.reconcileTODOTasks(resultsToPaths(msg.results)); len(updates) > 0 {
			var lines []string
			for _, t := range updates {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", t.Status, t.ID, t.Title))
			}
			m.addItem("system", "Task Reconciliation", strings.Join(lines, "\n"), "")
			m.refreshTaskBoard()
		}
		m.pending = nil
		m.status = "actions applied"
		m.addItem("system", "Apply Complete", "Pending actions were executed.", "")
		m.persistSession()
		if prompt := m.recoveryPrompt(msg.results); prompt != "" && m.retryCount < m.cfg.Autonomy.RetryBudget {
			m.busy = true
			m.status = "diagnose"
			m.retryCount++
			m.addItem("system", "Verification Failed", "Marcus is diagnosing the failed verification and preparing a retry.", "retry")
			return m, m.sendRecoveryLoop(prompt)
		}
		m.retryCount = 0
		return m, nil
	}

	if !m.inputFocused {
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd
	}
	ta, cmd := m.textarea.Update(msg)
	m.textarea = ta
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlQ:
		m.persistSession()
		return m, tea.Quit
	case tea.KeyTab:
		m.inputFocused = !m.inputFocused
		if m.inputFocused {
			m.textarea.Focus()
		} else {
			m.textarea.Blur()
		}
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
		for _, action := range m.pending {
			m.session.AppendAction(action.Proposal.Label(), "discarded")
		}
		m.pending = nil
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
				for _, action := range m.pending {
					m.session.AppendAction(action.Proposal.Label(), "discarded")
				}
				m.pending = nil
				m.addItem("system", "Pending Actions Cleared", "Discarded all pending proposals.", "")
				m.status = "pending actions discarded"
				m.persistSession()
				return m, nil
			}
		}
	case tea.KeySpace:
		// Toggle step mode or resume from pause
		if m.busy && m.stepPaused {
			// Resume from a pause point
			m.stepPaused = false
			m.status = "resuming"
			m.addItem("system", "Resuming", "Continuing agent loop...", "")
			if m.stepSignal != nil {
				close(m.stepSignal)
				m.stepSignal = nil
			}
			return m, nil
		}
		// Toggle step mode on/off when not busy
		m.stepMode = !m.stepMode
		if m.stepMode {
			m.addItem("system", "Step Mode", "Space pauses before each cooking phase.", "")
		} else {
			m.addItem("system", "Step Mode", "Step mode disabled.", "")
		}
		return m, nil
	case tea.KeyEnter:
		if !m.inputFocused || m.busy {
			return m, nil
		}
		content := strings.TrimSpace(m.textarea.Value())
		if content == "" {
			return m, nil
		}
		if content == "/newsession" || content == "/new" {
			return m.resetSession(), nil
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

	if m.inputFocused {
		ta, cmd := m.textarea.Update(msg)
		m.textarea = ta
		return m, cmd
	}
	vp, cmd := m.viewport.Update(msg)
	m.viewport = vp
	return m, cmd
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing Marcus..."
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.StatusBar.Render(m.renderStatusBar()),
		m.styles.TranscriptPane.Render(m.viewport.View()),
		m.styles.Composer.Render(m.textarea.View()),
		m.styles.Help.Render(m.renderHelp()),
	)
	return content
}

func (m Model) renderStatusBar() string {
	project := "no-project"
	if m.cfg.Project.Name != "" {
		project = m.cfg.Project.Name
	}
	branch := m.latestContext.Branch
	if branch == "" {
		branch = "-"
	}
	dirty := ""
	if m.latestContext.Dirty {
		dirty = "*"
	}
	// Decide what to show in the status slot
	statusText := m.status
	statusStyle := m.styles.StatusValue
	if m.thinkingTicker != nil {
		// Agent is running — show pulsing [*] + current phase
		statusText = m.cookingFrame()
		statusStyle = m.styles.StatusLabel // orange/purple color for active cook
	} else if m.status == "done" || m.status == "complete" {
		statusText = "done"
		statusStyle = m.styles.StatusMuted // grey when idle
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.styles.StatusLabel.Render("MARCUS "),
		m.styles.StatusValue.Render(project),
		m.styles.StatusMuted.Render("  "),
		m.styles.StatusKey.Render(m.cfg.Provider+"/"+m.cfg.Model),
		m.styles.StatusMuted.Render("  git:"),
		m.styles.StatusValue.Render(branch+dirty),
		m.styles.StatusMuted.Render("  "),
		statusStyle.Render(statusText),
	)
}

func (m Model) renderHelp() string {
	help := []string{
		m.styles.HelpKey.Render("Enter") + m.styles.HelpDesc.Render(" send"),
		m.styles.HelpKey.Render("Tab") + m.styles.HelpDesc.Render(" focus"),
		m.styles.HelpKey.Render("y") + m.styles.HelpDesc.Render(" apply"),
		m.styles.HelpKey.Render("n") + m.styles.HelpDesc.Render(" discard"),
		m.styles.HelpKey.Render("/newsession") + m.styles.HelpDesc.Render(" reset"),
		m.styles.HelpKey.Render("@file") + m.styles.HelpDesc.Render(" attach"),
		m.styles.HelpKey.Render("Space") + m.styles.HelpDesc.Render(" step/pause"),
	}
	return strings.Join(help, "  ")
}

func (m *Model) layout() {
	statusHeight := 1
	helpHeight := 1
	composerHeight := 5
	availableHeight := m.height - statusHeight - helpHeight - composerHeight
	if availableHeight < 8 {
		availableHeight = 8
	}
	width := m.width
	if width < 60 {
		width = 60
	}
	m.viewport.Width = width - 2
	m.viewport.Height = availableHeight
	m.textarea.SetWidth(width - 4)
	m.textarea.SetHeight(3)
}

func (m *Model) addItem(kind, title, body, meta string) {
	m.transcript = append(m.transcript, transcriptItem{
		Kind:  kind,
		Title: title,
		Body:  body,
		Meta:  meta,
	})
	m.renderTranscript()
}

func (m *Model) updateTranscriptItem(index int, kind, title, body, meta string) {
	if index < 0 || index >= len(m.transcript) {
		return
	}
	m.transcript[index] = transcriptItem{
		Kind:  kind,
		Title: title,
		Body:  body,
		Meta:  meta,
	}
	m.renderTranscript()
}

func (m *Model) renderTranscript() {
	var blocks []string
	for _, item := range m.transcript {
		blocks = append(blocks, m.styles.RenderItem(item))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, m.styles.RenderItem(transcriptItem{
			Kind:  "system",
			Title: "Marcus",
			Body:  "Single-pane terminal mode is ready. Ask for code changes, repo analysis, or attach files with @path.",
		}))
	}
	m.viewport.SetContent(strings.Join(blocks, "\n\n"))
	m.viewport.GotoBottom()
}

func (m *Model) sendToAI(content string) tea.Cmd {
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

	// Select the best-matching agent for this task
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
			BaseDir:   isoSession.Root,
			Config:    m.cfg,
			Folders:   m.flowEngine.FolderEngine(),
			CodeIndex: m.codeIndex,
			LSP:       m.lspBroker,
		})
		if err != nil {
			return appliedActionsMsg{err: err}
		}
		results := make([]tool.ActionResult, 0, len(pending))
		for _, action := range pending {
			result, err := runner.ApplyAction(ctx, action.Proposal)
			if err != nil {
				return appliedActionsMsg{err: fmt.Errorf("%s: %w", action.Proposal.Label(), err)}
			}
			results = append(results, result)
		}
		return appliedActionsMsg{results: results, session: isoSession}
	}
}

// Kitchen spinner frames — pulsing [*] + cook phase
var spinnerFrames = []string{
	"[* Frying   ]",
	"[* Frying.  ]",
	"[* Frying.. ]",
	"[* Frying...",
	"[* Sizzling ]",
	"[* Sizzling.]",
	"[* Sizzling..",
	"[* Sizzling..]",
	"[* Simmering]",
	"[* Simmering.]",
	"[* Simmering..",
	"[* Simmering...]",
	"[* Browning ]",
	"[* Browning.]",
	"[* Browning..",
}

// phaseStartIdx maps cooking phase names to their starting index in spinnerFrames
var phaseStartIdx = map[string]int{
	"Preheating": 0,
	"Frying":     0,
	"Sizzling":   4,
	"Simmering":  8,
	"Browning":   12,
	"Searing":    12,
	"Roasting":   12,
	"Grilling":   12,
	"Baking":     12,
	"Finishing":  12,
}

// cookingFrame returns the current spinner frame for the active cooking phase.
func (m *Model) cookingFrame() string {
	start := 0
	if phase, ok := phaseStartIdx[m.currentPhase]; ok {
		start = phase
	}
	// 4 frames per phase
	phaseFrames := spinnerFrames[start : start+4]
	return phaseFrames[m.thinkingFrame%len(phaseFrames)]
}

func (m *Model) finishActivity(title, body, meta string) {
	// Stop the spinner
	if m.thinkingTicker != nil {
		m.thinkingTicker.Stop()
		m.thinkingTicker = nil
	}

	// Turn the thinking card into a "Crunch" card (gray) if it exists
	if m.thinkingCardIndex >= 0 && m.thinkingCardIndex < len(m.transcript) {
		m.transcript[m.thinkingCardIndex].Kind = "crunched"
		m.transcript[m.thinkingCardIndex].Title = "Crunch"
		m.transcript[m.thinkingCardIndex].Meta = ""
		m.renderTranscript()
		m.thinkingCardIndex = -1
	}

	if m.activityIndex >= 0 && m.activityIndex < len(m.transcript) {
		m.updateTranscriptItem(m.activityIndex, "system", title, body, meta)
		m.activityIndex = -1
		return
	}
	m.addItem("system", title, body, meta)
}

func (m *Model) updateActivity(body, meta, phase string) {
	// Update cooking phase if provided
	if phase != "" {
		m.currentPhase = phase
	}
	// Ensure the spinner is running
	if m.thinkingTicker == nil {
		m.thinkingTicker = time.NewTicker(400 * time.Millisecond)
		m.thinkingFrame = 0
		// If no card exists yet, create one
		if m.thinkingCardIndex < 0 || m.thinkingCardIndex >= len(m.transcript) {
			m.thinkingCardIndex = len(m.transcript)
			m.addItem("thinking", m.cookingFrame(), body, meta)
		}
	}
	// Update the card title with current spinner frame
	frame := m.cookingFrame()
	if phase != "" {
		body = fmt.Sprintf("[%s] %s", phase, body)
	}
	if m.thinkingCardIndex >= 0 && m.thinkingCardIndex < len(m.transcript) {
		m.transcript[m.thinkingCardIndex].Title = frame
		m.transcript[m.thinkingCardIndex].Body = body
		m.transcript[m.thinkingCardIndex].Meta = meta
	}
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

func (m *Model) tickThinkingCard() {
	if m.thinkingTicker == nil {
		return
	}
	m.thinkingFrame++
	frame := m.cookingFrame()
	if m.thinkingCardIndex >= 0 && m.thinkingCardIndex < len(m.transcript) {
		m.transcript[m.thinkingCardIndex].Title = frame
		m.renderTranscript()
	}
}

func tickIfNeeded(m *Model) tea.Cmd {
	if m.thinkingTicker == nil {
		return nil
	}
	return func() tea.Msg {
		return tickMsg{}
	}
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

func (m *Model) reconcileTODOTasks(paths []string) []task.Task {
	if len(paths) == 0 {
		return nil
	}
	hints := m.contextAssembler.ScanTODOs(paths)
	updated, err := m.taskStore.ReconcileTODOHints(paths, hints)
	if err != nil {
		m.addItem("error", "Task Reconcile Failed", err.Error(), "")
		return nil
	}
	return updated
}

func (m *Model) resetSession() tea.Model {
	m.persistSession()
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
	m.textarea.SetValue("")
	m.bootstrapTranscript()
	m.persistSession()
	m.addItem("system", "New Session", "Started a fresh Marcus session.", "")
	return m
}

func (m *Model) refreshTaskBoard() {
	tasks, err := m.taskStore.List()
	if err != nil || len(tasks) == 0 {
		return
	}
	var lines []string
	for _, t := range tasks {
		marker := "[ ]"
		switch t.Status {
		case task.StatusDone:
			marker = "[x]"
		case task.StatusActive:
			marker = "[-]"
		case task.StatusBlocked:
			marker = "[!]"
		}
		lines = append(lines, fmt.Sprintf("%s %s", marker, t.Title))
	}
	if m.taskBoardIndex >= 0 && m.taskBoardIndex < len(m.transcript) {
		m.updateTranscriptItem(m.taskBoardIndex, "system", "Tasks", strings.Join(lines, "\n"), "")
		return
	}
	m.taskBoardIndex = len(m.transcript)
	m.addItem("system", "Tasks", strings.Join(lines, "\n"), "")
}

func resultsToPaths(results []tool.ActionResult) []string {
	var paths []string
	for _, result := range results {
		if result.Proposal.Type == "write_file" && strings.TrimSpace(result.Proposal.Path) != "" {
			paths = append(paths, result.Proposal.Path)
		}
	}
	return paths
}

func (m *Model) recoveryPrompt(results []tool.ActionResult) string {
	var failures []string
	for _, result := range results {
		if result.Proposal.Type == "run_command" && !result.Success {
			failures = append(failures, fmt.Sprintf("Verification failed for %s:\n%s", result.Proposal.Command, trimText(result.Output, 1200)))
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return "The last verification step failed after applying changes.\nDiagnose the failure, inspect the relevant files, propose the smallest corrective edit, and include a follow-up verification command.\n\n" + strings.Join(failures, "\n\n")
}

func startAgentLoopCmd(m *Model, content string, snapshot ctxpkg.Snapshot, agent *folder.AgentDef) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			m.runAgentLoopAsync(content, snapshot, agent, ch)
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

func (m *Model) runAgentLoopAsync(content string, snapshot ctxpkg.Snapshot, agent *folder.AgentDef, ch chan<- tea.Msg) {
	toolResults := []string{}
	currentSnapshot := snapshot
	var lastRaw string
	lastActionSignature := ""
	stagnationCount := 0

	// Use agent's iteration limit, or config default
	maxIterations := 10
	if agent != nil && agent.Autonomy.IterationLimit > 0 {
		maxIterations = agent.Autonomy.IterationLimit
	} else if m.cfg != nil {
		maxIterations = max(1, m.cfg.Autonomy.MaxIterations)
	}

	// Track live card index for thinking updates
	var thinkingCardIndex int = -1

	// Cooking metaphors for iteration display
	cookingPhases := []string{"Preheating", "Frying", "Sizzling", "Simmering", "Browning", "Searing", "Roasting", "Grilling", "Baking", "Finishing"}
	cookPhase := func(i int) string {
		if i-1 < len(cookingPhases) {
			return cookingPhases[i-1]
		}
		return cookingPhases[len(cookingPhases)%i]
	}

	for loop := 1; loop <= maxIterations; loop++ {
		iterStart := time.Now()
		phase := cookPhase(loop)
		m.addItem("iteration", phase, "Planning, execution, and verification", "")
		// Step mode pause: wait for user signal before starting iteration
		if m.stepMode {
			m.stepPaused = true
			m.status = "paused (step mode)"
			m.stepSignal = make(chan struct{})
			m.addItem("system", "Step Mode Active", fmt.Sprintf("%s: press Space to continue", phase), "")
			ch <- agentStatusMsg{body: fmt.Sprintf("paused before %s", phase), meta: "paused"}
			// Signal the main loop to return control
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

		// Build request with full conversation history
		prompt := buildPromptForAgent(m.toolRunner, currentSnapshot, m.session, content, toolResults, agent)
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
			// Real-time thinking text
			if event.Text != "" {
				streamBuffer.WriteString(event.Text)
				charCount += len(event.Text)
				// Update status every ~200 chars
				if charCount%200 < len(event.Text) {
					ch <- agentStatusMsg{
						body:  fmt.Sprintf("Thinking... (%d chars)", charCount),
						meta:  "thinking",
						phase: phase,
					}
				}
				// Keep the thinking card readable instead of streaming raw JSON into it.
				if thinkingCardIndex >= 0 {
					m.updateTranscriptItem(thinkingCardIndex, "thinking", "Marcus is thinking...", thinkingCardBody(charCount, toolCallsSeen), fmt.Sprintf("%d chars", charCount))
				} else {
					thinkingCardIndex = len(m.transcript)
					m.addItem("thinking", "Marcus is thinking...", thinkingCardBody(charCount, toolCallsSeen), fmt.Sprintf("%d chars", charCount))
				}
			}
			// Provider tool calls streamed during response
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

		// Clear the live thinking card now that response is complete
		if thinkingCardIndex >= 0 && thinkingCardIndex < len(m.transcript) {
			m.transcript[thinkingCardIndex].Kind = "thinking"
		}
		thinkingCardIndex = -1

		lastRaw = streamBuffer.String()
		envelope := parseAssistantEnvelope(lastRaw)

		// Show the parsed assistant message
		modelMessage := visibleAssistantMessage(envelope, lastRaw)
		m.addItem("assistant", "Marcus", modelMessage, "")

		elapsed := time.Since(iterStart).Round(time.Second)
		ch <- agentStatusMsg{
			body:  fmt.Sprintf("%s — done in %v: %d action(s) parsed", phase, elapsed, len(envelope.Actions)),
			meta:  "analyzing",
			phase: phase,
		}

		// No actions at all — done
		if len(envelope.Actions) == 0 {
			ch <- assistantResponseMsg{
				envelope: envelope,
				raw:      lastRaw,
				context:  currentSnapshot,
				showItem: false,
			}
			return
		}

		actionSignature := actionPlanSignature(envelope.Actions)
		if actionSignature != "" && actionSignature == lastActionSignature {
			stagnationCount++
		} else {
			stagnationCount = 0
		}
		lastActionSignature = actionSignature
		if stagnationCount >= 2 {
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

		// Show each proposed action as a pending card with human-readable details
		for i, action := range envelope.Actions {
			detail := formatActionHuman(action)
			reason := valueOr(action.Reason, "")
			if reason != "" {
				detail = detail + "\nReason: " + reason
			}
			m.addItem("action", fmt.Sprintf("Proposal #%d: %s", i+1, action.Label()), detail, "pending-review")
		}

		// Step mode: pause after proposing actions, before execution
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

		// Partition into safe and approval-required
		safeActions, pendingActions := m.partitionActions(envelope.Actions)

		// Apply safe actions with full visible output
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
				ch <- assistantResponseMsg{err: fmt.Errorf("%s: %w", action.Label(), err)}
				return
			}
			toolResults = append(toolResults, fmt.Sprintf("Tool: %s\n%s", action.Label(), result.Output))
			m.addItem("tool_result", "Result: "+action.Label(), trimText(result.Output, 1500), "auto")
			ch <- agentStatusMsg{
				body:  fmt.Sprintf("Completed: %s", action.Label()),
				meta:  "tool-done",
				phase: phase,
			}
			if action.Type == "write_file" {
				wroteFiles = true
			}
		}

		// Autonomous verification loop: if files were written, run build/test and feed result back
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
					// Feed build output back as conversation context for next iteration
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
						// Build/test failed — feed error back to model to self-correct
						failMsg := fmt.Sprintf("Build/test failed:\n%s\n\nFix the errors above and retry.", verifyResult.Output)
						m.addVerificationSummary(verifyCmd, verifyResult.ExitCode, false, "Automatic verification failed")
						m.addItem("system", "Build Failed", failMsg, "error")
						ch <- agentStatusMsg{
							body:  "Build failed — feeding error back to model for self-correction",
							meta:  "retry",
							phase: phase,
						}
						// Append failure context to toolResults so model sees it on next call
						toolResults = append(toolResults, failMsg)
					} else {
						m.addVerificationSummary(verifyCmd, verifyResult.ExitCode, true, "Checks passed")
						m.addItem("system", "Build Passed", "All checks passed.", "complete")
					}
				}
			}
		}

		// If there are pending (approval-required) actions, surface them
		if len(pendingActions) > 0 {
			for _, action := range pendingActions {
				preview, _ := m.toolRunner.PreviewAction(action)
				m.pending = append(m.pending, pendingAction{Proposal: action, Preview: preview})
			}
			m.addItem("system", "Approval Required", fmt.Sprintf("%d action(s) need your approval — press y to apply, n to discard", len(m.pending)), "")
			ch <- assistantResponseMsg{
				envelope: assistantEnvelope{Message: "Actions proposed — some need approval.", Actions: pendingActions},
				raw:      lastRaw,
				context:  currentSnapshot,
				showItem: false,
			}
			return
		}

		// Refresh context and loop
		currentSnapshot = m.contextAssembler.Assemble(content, m.session)
	}

	ch <- assistantResponseMsg{
		envelope: assistantEnvelope{
			Message: fmt.Sprintf("Reached iteration limit (%d). You can ask Marcus to continue.", maxIterations),
		},
		raw:      lastRaw,
		context:  currentSnapshot,
		showItem: false,
	}
}

// loopPausedMsg signals that the agent loop is waiting for step-mode resume.
type loopPausedMsg struct {
	iteration int
}

func (m *Model) partitionActions(actions []tool.ActionProposal) ([]tool.ActionProposal, []tool.ActionProposal) {
	var safe []tool.ActionProposal
	var pending []tool.ActionProposal

	agent := m.currentAgent

	// Count writes and commands
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

	// Build safe actions list from agent rules
	safeActions := map[string]bool{
		"list_files": true, "read_file": true,
		"search_code": true, "find_symbol": true, "list_symbols": true,
	}
	if agent != nil {
		for _, a := range agent.Rules.SafeActions {
			safeActions[a] = true
		}
	}

	// Build auto-run command prefixes from agent rules
	autoRunPrefixes := []string{
		"go build", "cargo build", "npm run", "ruff", "go test", "go vet",
		"golangci-lint", "python -m", "mvn", "gradle", "make",
	}
	if agent != nil {
		autoRunPrefixes = append(autoRunPrefixes, agent.Rules.AutoRunCommands...)
	}

	// Write policy: "always", "first_wave", "never"
	writeIf := "first_wave"
	if agent != nil {
		writeIf = agent.Rules.WriteIf
	}

	for _, action := range actions {
		switch {
		// Safe read-only tools auto-run
		case action.Type != "run_command" && safeActions[action.Type]:
			safe = append(safe, action)

		// write_file policy
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
			default: // "never"
				pending = append(pending, action)
			}

		// run_command: check against auto-run prefixes
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

func (m *Model) contextMeta(snapshot ctxpkg.Snapshot) string {
	var parts []string
	if snapshot.Branch != "" {
		parts = append(parts, "git:"+snapshot.Branch)
	}
	if len(snapshot.FileHints) > 0 {
		parts = append(parts, "@"+strings.Join(snapshot.FileHints, ", @"))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) persistSession() {
	if m.sessionStore == nil || !m.cfg.Session.AutoSave {
		return
	}
	m.session.LastContext = trimText(m.latestContext.Text, 2000)
	_ = m.sessionStore.Save(m.session)
}

func buildPromptForAgent(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, toolResults []string, agent *folder.AgentDef) string {
	var recent []string
	if sess != nil {
		for _, turn := range sess.RecentTurns(6) {
			recent = append(recent, fmt.Sprintf("%s: %s", turn.Role, turn.Content))
		}
	}
	toolContext := "No prior tool results."
	if len(toolResults) > 0 {
		toolContext = strings.Join(toolResults, "\n\n")
	}
	toolCatalog := toolCatalogForPrompt(runner)

	return strings.TrimSpace(fmt.Sprintf("## Output format\nAlways return JSON with this shape:\n{\n  \"message\": \"brief message to the user\",\n  \"actions\": [\n    {\n      \"type\": \"write_file | run_command | read_file | search_code | find_symbol | list_files\",\n      \"path\": \"relative/path/for/files\",\n      \"content\": \"full file content for write_file\",\n      \"command\": \"shell command for run_command\",\n      \"pattern\": \"regex pattern for search_code\",\n      \"symbol\": \"symbol name for find_symbol\",\n      \"reason\": \"why this action is needed\"\n    }\n  ],\n  \"tasks\": [\n    {\n      \"id\": \"optional-slug\",\n      \"title\": \"Task title\",\n      \"description\": \"optional detail\",\n      \"status\": \"active | done | blocked\"\n    }\n  ]\n}\n\nRepo Context:\n%s\n\nRecent Conversation:\n%s\n\nPrior Tool Results:\n%s\n\nAvailable Tool Catalog:\n%s\n\nUser Request:\n%s", snapshot.Text, strings.Join(recent, "\n"), toolContext, toolCatalog, input))
}

func buildPrompt(runner *tool.ToolRunner, snapshot ctxpkg.Snapshot, sess *session.Session, input string, toolResults []string) string {
	return buildPromptForAgent(runner, snapshot, sess, input, toolResults, nil)
}

func buildAgentSystemPrompt(agent *folder.AgentDef) string {
	base := "You are Marcus, a terminal-native coding agent."
	if agent == nil || strings.TrimSpace(agent.Autonomy.SystemPrompt) == "" {
		return base
	}
	return strings.TrimSpace(base + "\n\n" + agent.Autonomy.SystemPrompt)
}

func parseAssistantEnvelope(raw string) assistantEnvelope {
	text := stripMarkdownCodeFence(raw)
	if text == "" {
		return assistantEnvelope{}
	}
	var env assistantEnvelope
	if err := json.Unmarshal([]byte(text), &env); err == nil {
		return env
	}
	if start := strings.Index(text, "{"); start != -1 {
		if end := strings.LastIndex(text, "}"); end > start {
			if err := json.Unmarshal([]byte(text[start:end+1]), &env); err == nil {
				return env
			}
		}
	}
	env = recoverAssistantEnvelope(text)
	if strings.TrimSpace(env.Message) != "" || len(env.Actions) > 0 || len(env.Tasks) > 0 {
		return env
	}
	return assistantEnvelope{Message: text}
}

func recoverAssistantEnvelope(text string) assistantEnvelope {
	env := assistantEnvelope{
		Message: extractJSONStringField(text, "message"),
		Actions: extractJSONArrayObjects[tool.ActionProposal](text, "actions"),
		Tasks:   extractJSONArrayObjects[task.Update](text, "tasks"),
	}
	if strings.TrimSpace(env.Message) == "" && (len(env.Actions) > 0 || len(env.Tasks) > 0) {
		env.Message = "Recovered structured actions from a partial provider response."
	}
	return env
}

func visibleAssistantMessage(env assistantEnvelope, raw string) string {
	if message := strings.TrimSpace(env.Message); message != "" {
		return message
	}
	if len(env.Actions) > 0 || len(env.Tasks) > 0 {
		return summarizeStructuredResponse(env)
	}
	text := strings.TrimSpace(stripMarkdownCodeFence(raw))
	if text == "" || looksLikeStructuredPayload(text) {
		return "Prepared a structured response."
	}
	return trimText(text, 800)
}

func summarizeStructuredResponse(env assistantEnvelope) string {
	var parts []string
	if len(env.Actions) > 0 {
		parts = append(parts, fmt.Sprintf("Prepared %d action(s)", len(env.Actions)))
	}
	if len(env.Tasks) > 0 {
		parts = append(parts, fmt.Sprintf("%d task update(s)", len(env.Tasks)))
	}
	if len(parts) == 0 {
		return "Prepared a structured response."
	}
	return strings.Join(parts, " and ") + "."
}

func stripMarkdownCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	lines = lines[1:]
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func looksLikeStructuredPayload(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.Contains(trimmed, `"actions"`) ||
		strings.Contains(trimmed, `"tasks"`) ||
		strings.Contains(trimmed, `"message"`)
}

func extractJSONStringField(text, field string) string {
	key := `"` + field + `"`
	start := strings.Index(text, key)
	if start == -1 {
		return ""
	}
	colon := strings.Index(text[start+len(key):], ":")
	if colon == -1 {
		return ""
	}
	rest := strings.TrimSpace(text[start+len(key)+colon+1:])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	var out string
	if err := json.Unmarshal([]byte(readJSONObjectPrefix(rest)), &out); err != nil {
		return ""
	}
	return out
}

func extractJSONArrayObjects[T any](text, field string) []T {
	key := `"` + field + `"`
	start := strings.Index(text, key)
	if start == -1 {
		return nil
	}
	open := strings.Index(text[start:], "[")
	if open == -1 {
		return nil
	}
	arrayText := text[start+open:]
	var results []T
	for _, objectText := range splitJSONObjectCandidates(arrayText) {
		var item T
		if err := json.Unmarshal([]byte(objectText), &item); err == nil {
			results = append(results, item)
		}
	}
	return results
}

func splitJSONObjectCandidates(input string) []string {
	var objects []string
	inString := false
	escaped := false
	depth := 0
	start := -1
	for i, r := range input {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case !inString && r == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case !inString && r == '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objects = append(objects, input[start:i+1])
					start = -1
				}
			}
		}
	}
	return objects
}

func readJSONObjectPrefix(input string) string {
	inString := false
	escaped := false
	for i, r := range input {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			if inString {
				return input[:i+1]
			}
			inString = true
		}
	}
	return input
}

func trimText(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[:limit]) + "\n...[truncated]"
}

func thinkingCardBody(charCount, toolCallsSeen int) string {
	if toolCallsSeen > 0 {
		return fmt.Sprintf("Streaming a structured response.\nCaptured %d chars and %d provider tool call(s).", charCount, toolCallsSeen)
	}
	return fmt.Sprintf("Streaming a structured response.\nCaptured %d chars so far.", charCount)
}

// New creates the Marcus single-pane TUI model.
func New(cfg *config.Config) (*Model, error) {
	var prov provider.Provider
	var err error
	switch cfg.Provider {
	case "anthropic":
		prov, err = provider.NewAnthropicProvider()
	default:
		prov, err = provider.NewOllamaProvider(cfg.Model)
	}
	if err != nil {
		return nil, fmt.Errorf("provider: %w", err)
	}

	flowEngine, err := flow.NewEngine(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("flow engine: %w", err)
	}

	baseDir := cfg.ProjectRoot
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}
	codeIndex := codeintel.NewIndex(baseDir)
	_ = codeIndex.Build(context.Background())
	lspBroker := lsp.NewBroker(cfg.LSP, baseDir)
	toolRunner, err := tool.BuildRunner(tool.BuildOptions{
		BaseDir:   baseDir,
		Config:    cfg,
		Folders:   flowEngine.FolderEngine(),
		CodeIndex: codeIndex,
		LSP:       lspBroker,
	})
	if err != nil {
		return nil, fmt.Errorf("tool runner: %w", err)
	}

	taskStore := task.NewStore(baseDir)
	_ = taskStore.EnsureStructure()
	sessionStore := session.NewStore(baseDir)
	sess, err := sessionStore.LoadLatest()
	if err != nil {
		return nil, fmt.Errorf("session store: %w", err)
	}

	ta := textarea.New()
	ta.Placeholder = "Ask Marcus to inspect, plan, edit, or build. Use @path to attach files."
	ta.ShowLineNumbers = false
	ta.Prompt = "> "
	ta.Focus()
	ta.SetHeight(3)
	ta.CharLimit = 0
	ta.KeyMap.InsertNewline.SetEnabled(false)

	memoryManager := memory.NewManager(baseDir, cfg.Memory.RecallLimit)
	_ = memoryManager.EnsureStructure()

	model := &Model{
		provider:         prov,
		providerRuntime:  provider.NewRuntime(prov, baseDir, cfg.ProviderCfg.CacheEnabled),
		flowEngine:       flowEngine,
		toolRunner:       toolRunner,
		codeIndex:        codeIndex,
		lspBroker:        lspBroker,
		memoryManager:    memoryManager,
		isolationManager: isolation.NewManager(baseDir, cfg.Isolation),
		cfg:              cfg,
		styles:           DefaultStyles(),
		viewport:         viewport.New(100, 24),
		textarea:         ta,
		inputFocused:     true,
		ready:            true,
		status:           "ready",
		taskStore:        taskStore,
		session:          sess,
		sessionStore:     sessionStore,
		contextAssembler: ctxpkg.NewAssembler(cfg, flowEngine, taskStore, memoryManager),
		width:            100,
		height:           30,
		activityIndex:    -1,
		taskBoardIndex:   -1,
	}

	// Wrap contextAssembler so it returns flow.Snapshot for LoopEngine
	ctxAsm := model.contextAssembler
	loopCtxAsm := flowContextAssembler{
		delegate: ctxAsm,
		toFlowSnapshot: func(ctxSnap ctxpkg.Snapshot) flow.Snapshot {
			return flow.Snapshot{
				Text:      ctxSnap.Text,
				Branch:    ctxSnap.Branch,
				Dirty:     ctxSnap.Dirty,
				FileHints: ctxSnap.FileHints,
				TODOHints: ctxSnap.TODOHints,
			}
		},
	}

	model.loopEngine = flowEngine.LoopEngine(
		toolRunner,
		taskStore,
		memoryManager,
		loopCtxAsm,
		provider.NewRuntime(prov, baseDir, cfg.ProviderCfg.CacheEnabled),
	)
	model.layout()
	model.bootstrapTranscript()
	return model, nil
}

func (m *Model) bootstrapTranscript() {
	m.latestContext = m.contextAssembler.Assemble("", m.session)
	m.refreshTaskBoard()
	flows := m.flowEngine.ListFlows()
	sort.Strings(flows)
	tools := m.toolRunner.List()
	sort.Strings(tools)
	m.addItem(
		"system",
		"Marcus Ready",
		fmt.Sprintf(
			"Project root: %s\nFlows: %s\nTools: %s\nTasks: %s",
			valueOr(m.cfg.ProjectRoot, "(not detected)"),
			valueOr(strings.Join(flows, ", "), "none"),
			valueOr(strings.Join(tools, ", "), "none"),
			m.taskStore.Summary(),
		),
		m.contextMeta(m.latestContext),
	)
	if m.cfg.Session.AutoResume && len(m.session.Turns) > 0 {
		for _, turn := range m.session.RecentTurns(8) {
			title := strings.Title(turn.Role)
			kind := turn.Role
			if kind != "user" && kind != "assistant" {
				kind = "system"
			}
			m.addItem(kind, title, turn.Content, "restored")
		}
	}
}

// detectVerifyCommand looks at the project files to determine a suitable
// build/test verification command (go build, cargo build, npm build, etc.).
func (m *Model) detectVerifyCommand() string {
	baseDir := m.cfg.ProjectRoot
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}
	// Language-specific build detector
	projType := detectProjectType(baseDir)
	switch projType {
	case "go":
		if fileExists(filepath.Join(baseDir, "go.mod")) {
			return "go build ./..."
		}
		if fileExists(filepath.Join(baseDir, "main.go")) {
			return "go build ."
		}
	case "rust":
		if fileExists(filepath.Join(baseDir, "Cargo.toml")) {
			return "cargo build 2>&1"
		}
	case "node":
		if fileExists(filepath.Join(baseDir, "package.json")) {
			if fileExists(filepath.Join(baseDir, "Makefile")) {
				return "make build 2>&1 || npm run build 2>&1"
			}
			return "npm run build 2>&1"
		}
	case "python":
		if fileExists(filepath.Join(baseDir, "pyproject.toml")) || fileExists(filepath.Join(baseDir, "requirements.txt")) {
			return "python -m compileall -q . 2>&1 || ruff check . 2>&1"
		}
	case "java":
		if fileExists(filepath.Join(baseDir, "pom.xml")) {
			return "mvn compile 2>&1"
		}
		if fileExists(filepath.Join(baseDir, "build.gradle")) {
			return "gradle build 2>&1"
		}
	}
	// Generic fallback: try any present build files
	if fileExists(filepath.Join(baseDir, "Makefile")) {
		return "make 2>&1"
	}
	if fileExists(filepath.Join(baseDir, "CMakeLists.txt")) {
		return "cmake --build build 2>&1"
	}
	return ""
}

func (m *Model) detectDependencyInstallCommand(verifyOutput string) string {
	output := strings.TrimSpace(verifyOutput)
	if output == "" {
		return ""
	}

	// Go usually tells us exactly what to run.
	goGetRx := regexp.MustCompile(`(?m)^\s*(go get [^\r\n]+)\s*$`)
	if matches := goGetRx.FindStringSubmatch(output); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	// Python missing module.
	pyMissingRx := regexp.MustCompile(`No module named ['"]([^'"]+)['"]`)
	if matches := pyMissingRx.FindStringSubmatch(output); len(matches) == 2 && isLikelyPackageName(matches[1]) {
		return fmt.Sprintf("python -m pip install %s 2>&1", matches[1])
	}

	// Node missing module.
	nodeMissingRx := regexp.MustCompile(`Cannot find module ['"]([^'"]+)['"]`)
	if matches := nodeMissingRx.FindStringSubmatch(output); len(matches) == 2 && isLikelyPackageName(matches[1]) {
		return fmt.Sprintf("npm install %s 2>&1", matches[1])
	}

	// Missing ruff command (common on Windows shells).
	lower := strings.ToLower(output)
	if strings.Contains(lower, "ruff") &&
		(strings.Contains(lower, "not recognized") || strings.Contains(lower, "n'est pas reconnu")) {
		return "python -m pip install ruff 2>&1"
	}

	return ""
}

// selectAgent picks the best-matching agent for a user input.
// It matches against agent goals/role patterns; falls back to general_agent.
func (m *Model) selectAgent(input string) *folder.AgentDef {
	agents := m.flowEngine.FolderEngine().ListAgents()
	inputLower := strings.ToLower(input)

	// Goal pattern matching
	var best *folder.AgentDef
	bestScore := -1
	for _, name := range agents {
		agent, ok := m.flowEngine.FolderEngine().GetAgent(name)
		if !ok || agent == nil {
			continue
		}
		score := 0
		// Exact role match gets highest score
		if agent.Role == "coding" && (strings.Contains(inputLower, "build") || strings.Contains(inputLower, "implement") ||
			strings.Contains(inputLower, "create") || strings.Contains(inputLower, "fix") ||
			strings.Contains(inputLower, "add") || strings.Contains(inputLower, "write") ||
			strings.Contains(inputLower, "refactor") || strings.Contains(inputLower, "develop")) {
			score = 10
		} else if agent.Role == "general" {
			score = 1
		}
		// Goal pattern match
		for _, goal := range agent.Goals {
			if strings.Contains(inputLower, strings.ToLower(goal)) {
				score += 3
			}
		}
		if score > bestScore {
			bestScore = score
			best = agent
		}
	}

	if best != nil {
		return best
	}

	// Fallback: general_agent
	if general, ok := m.flowEngine.FolderEngine().GetAgent("general_agent"); ok {
		return general
	}
	// Last resort: return nil and caller uses hardcoded defaults
	return nil
}

func detectProjectType(baseDir string) string {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case name == "go.mod" || name == "go.sum":
			return "go"
		case name == "Cargo.toml" || name == "Cargo.lock":
			return "rust"
		case name == "package.json" || name == "package-lock.json":
			return "node"
		case name == "pyproject.toml" || name == "requirements.txt" || name == "setup.py":
			return "python"
		case name == "pom.xml" || name == "build.gradle":
			return "java"
		case name == "go.sum":
			return "go"
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// formatToolCallHuman formats a tool call into a human-readable summary.
func formatToolCallHuman(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return "(no input)"
	}
	var data map[string]any
	if err := json.Unmarshal(input, &data); err == nil {
		return formatMapHuman(data)
	}
	var generic any
	if err := json.Unmarshal(input, &generic); err == nil {
		pretty, err := json.MarshalIndent(generic, "", "  ")
		if err == nil {
			return strings.TrimSpace(string(pretty))
		}
	}
	return trimText(strings.TrimSpace(string(input)), 400)
}

// formatActionHuman formats an ActionProposal into a readable action summary.
func formatActionHuman(a tool.ActionProposal) string {
	var lines []string
	switch a.Type {
	case "write_file":
		lines = append(lines, fmt.Sprintf("Write file: %s", a.Path))
		if a.Content != "" {
			preview := a.Content
			if len(preview) > 200 {
				preview = preview[:200] + "\n  ..."
			}
			lines = append(lines, "Content:\n  "+preview)
		}
	case "run_command":
		lines = append(lines, fmt.Sprintf("Command: %s", a.Command))
		if a.Dir != "" {
			lines = append(lines, fmt.Sprintf("Dir: %s", a.Dir))
		}
	case "read_file":
		lines = append(lines, fmt.Sprintf("Read: %s", a.Path))
		if a.Symbol != "" {
			lines = append(lines, fmt.Sprintf("Symbol: %s", a.Symbol))
		}
	case "search_code":
		lines = append(lines, fmt.Sprintf("Search: %s", a.Pattern))
		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("In: %s", a.Path))
		}
	case "list_files":
		lines = append(lines, fmt.Sprintf("List files: %s", valueOr(a.Path, ".")))
	case "find_symbol":
		lines = append(lines, fmt.Sprintf("Find symbol: %s", a.Symbol))
		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("In: %s", a.Path))
		}
	default:
		// Fallback: dump the structured fields
		if a.Path != "" {
			lines = append(lines, fmt.Sprintf("Path: %s", a.Path))
		}
		if a.Command != "" {
			lines = append(lines, fmt.Sprintf("Command: %s", a.Command))
		}
		if a.Content != "" {
			lines = append(lines, "Content:\n  "+a.Content)
		}
		if len(lines) == 0 && len(a.Input) > 0 {
			return formatMapHuman(a.Input)
		}
	}
	return strings.Join(lines, "\n")
}

// formatMapHuman formats a flat-ish map into readable key: value lines.
func formatMapHuman(data map[string]any) string {
	var lines []string
	// Show most important keys first
	priority := []string{"path", "file", "command", "pattern", "content", "dir", "symbol", "regex"}
	for _, key := range priority {
		if v, ok := data[key]; ok {
			lines = append(lines, formatKeyValue(key, v))
		}
	}
	// Append remaining keys
	for key, v := range data {
		if isPriorityKey(key, priority) {
			continue
		}
		lines = append(lines, formatKeyValue(key, v))
	}
	if len(lines) == 0 {
		return "(empty)"
	}
	return strings.Join(lines, "\n")
}

func formatKeyValue(key string, v any) string {
	switch val := v.(type) {
	case string:
		if len(val) > 150 {
			return fmt.Sprintf("%s: %s\n  ...", key, val[:150])
		}
		if strings.Contains(val, "\n") {
			return fmt.Sprintf("%s:\n  %s", key, strings.Join(strings.Split(val, "\n"), "\n  "))
		}
		return fmt.Sprintf("%s: %s", key, val)
	case float64:
		return fmt.Sprintf("%s: %.0f", key, val)
	case bool:
		return fmt.Sprintf("%s: %v", key, val)
	case []any:
		if len(val) == 0 {
			return fmt.Sprintf("%s: (empty)", key)
		}
		items := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				items = append(items, s)
			} else {
				items = append(items, fmt.Sprintf("%v", item))
			}
		}
		joined := strings.Join(items, ", ")
		if len(joined) > 120 {
			joined = joined[:120] + " ..."
		}
		return fmt.Sprintf("%s: [%s]", key, joined)
	default:
		return fmt.Sprintf("%s: %v", key, val)
	}
}

func isPriorityKey(key string, priority []string) bool {
	for _, p := range priority {
		if key == p {
			return true
		}
	}
	return false
}

func isLikelyPackageName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "node:") {
		return false
	}
	return true
}

func hasAssistantTranscriptItem(items []transcriptItem, message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	for _, item := range items {
		if item.Kind == "assistant" && strings.TrimSpace(item.Body) == message {
			return true
		}
	}
	return false
}

func actionPlanSignature(actions []tool.ActionProposal) string {
	if len(actions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, fmt.Sprintf("%s|%s|%s|%s|%s", action.Type, action.Path, action.Command, action.Pattern, action.Symbol))
	}
	return strings.Join(parts, ";")
}

func providerToolSpecs(runner *tool.ToolRunner) []provider.ToolSpec {
	if runner == nil {
		return nil
	}
	defs := runner.Definitions()
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

func toolCatalogForPrompt(runner *tool.ToolRunner) string {
	if runner == nil {
		return "No tools available."
	}
	defs := runner.Definitions()
	if len(defs) == 0 {
		return "No tools available."
	}
	var lines []string
	for _, def := range defs {
		safety := "approval-required"
		if def.Safe {
			safety = "safe"
		}
		raw, _ := json.Marshal(def.Schema)
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s\n  schema: %s", def.Name, safety, def.Description, string(raw)))
	}
	return strings.Join(lines, "\n")
}

// Run starts the TUI application.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	model, err := New(cfg)
	if err != nil {
		return fmt.Errorf("create model: %w", err)
	}
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run program: %w", err)
	}
	return nil
}
