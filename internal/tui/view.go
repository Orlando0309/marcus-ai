package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marcus-ai/marcus/internal/diff"
	"github.com/marcus-ai/marcus/internal/task"
)

func (m *Model) View() string {
	if !m.ready {
		return "Initializing Marcus..."
	}

	// Single-pane layout like Claude Code
	main := m.styles.TranscriptPane.Render(m.viewport.View())

	// Build the bottom section with help and optional bottom bar
	bottomSection := m.styles.Help.Render(m.renderHelp())
	if len(m.pending) > 0 {
		bottomBar := m.renderBottomBar()
		if bottomBar != "" {
			bottomSection = lipgloss.JoinVertical(lipgloss.Left, bottomBar, bottomSection)
		}
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.StatusBar.Render(m.renderStatusBar()),
		main,
		m.styles.Composer.Render(m.textarea.View()),
		bottomSection,
	)
	return content
}

func (m *Model) renderStatusBar() string {
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

	statusText := m.status
	statusStyle := m.styles.StatusValue
	if m.thinkingTicker != nil {

		statusText = m.cookingFrame()
		statusStyle = m.styles.StatusLabel
	} else if m.status == "done" || m.status == "complete" {
		statusText = "done"
		statusStyle = m.styles.StatusMuted
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
		m.styles.StatusMuted.Render(func() string {
			if d := m.undoDepth(); d > 0 {
				return fmt.Sprintf("  undo×%d", d)
			}
			return ""
		}()),
	)
}

func (m *Model) undoDepth() int {
	m.undoMu.Lock()
	n := len(m.undoStack)
	m.undoMu.Unlock()
	return n
}

func (m *Model) renderHelp() string {
	help := []string{
		m.styles.HelpKey.Render("Enter") + m.styles.HelpDesc.Render(" send"),
		m.styles.HelpKey.Render("Tab") + m.styles.HelpDesc.Render(" pane"),
		m.styles.HelpKey.Render("wheel") + m.styles.HelpDesc.Render(" scroll"),
		m.styles.HelpKey.Render("y/n") + m.styles.HelpDesc.Render(" apply/discard"),
		m.styles.HelpKey.Render("[/]") + m.styles.HelpDesc.Render(" diff"),
		m.styles.HelpKey.Render("/undo") + m.styles.HelpDesc.Render(" revert"),
		m.styles.HelpKey.Render("/help") + m.styles.HelpDesc.Render(" cmds"),
		m.styles.HelpKey.Render("Space") + m.styles.HelpDesc.Render(" step"),
	}
	return strings.Join(help, "  ")
}

// renderBottomBar renders a bottom bar similar to Claude Code's "accept edits on"
func (m *Model) renderBottomBar() string {
	if len(m.pending) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, m.styles.BottomBarKey.Render("y")+m.styles.BottomBarDesc.Render(" accept edits"))
	parts = append(parts, m.styles.BottomBarKey.Render("n")+m.styles.BottomBarDesc.Render(" discard"))
	if len(m.pending) > 1 {
		parts = append(parts, m.styles.BottomBarKey.Render("[/]")+m.styles.BottomBarDesc.Render(" cycle"))
	}
	parts = append(parts, m.styles.BottomBarKey.Render("shift+tab")+m.styles.BottomBarDesc.Render(" cycle"))
	parts = append(parts, m.styles.BottomBarKey.Render("esc")+m.styles.BottomBarDesc.Render(" interrupt"))
	parts = append(parts, m.styles.BottomBarKey.Render("ctrl+t")+m.styles.BottomBarDesc.Render(" hide tasks"))

	return m.styles.BottomBar.Render(strings.Join(parts, "  ·  "))
}

// renderPlan renders an active plan with its steps in a hierarchical display
func (m *Model) renderPlan(plan *Plan) string {
	if plan == nil {
		return ""
	}

	var b strings.Builder

	// Plan header with title and metadata
	title := m.styles.PlanTitle.Render(plan.Title)
	meta := ""
	if plan.Duration != "" {
		meta += "  " + m.styles.PlanMeta.Render("("+plan.Duration)
		if plan.Tokens > 0 {
			meta += "  ·  " + m.styles.PlanMeta.Render(fmt.Sprintf("↓ %s", formatTokens(plan.Tokens)))
		}
		if plan.Status == "running" || plan.Status == "planning" {
			meta += "  ·  thinking)"
		} else {
			meta += ")"
		}
	}
	b.WriteString(title + meta)
	b.WriteString("\n")

	// Render steps
	for _, step := range plan.Steps {
		b.WriteString(m.renderPlanStep(step, 0))
		b.WriteString("\n")
	}

	return b.String()
}

// renderPlanStep renders a single plan step with checkbox and optional substeps
func (m *Model) renderPlanStep(step PlanStep, depth int) string {
	var b strings.Builder

	indent := strings.Repeat("  ", depth)

	// Checkbox based on status
	checkbox := "☐ "
	style := m.styles.PlanStepPending
	checkStyle := m.styles.CheckboxPending

	switch step.Status {
	case "active", "running":
		checkbox = "☐ "
		style = m.styles.PlanStepActive
		checkStyle = m.styles.CheckboxActive
	case "complete", "done":
		checkbox = "✓ "
		style = m.styles.PlanStepComplete
		checkStyle = m.styles.CheckboxComplete
	case "error", "failed":
		checkbox = "☒ "
		style = m.styles.PlanStepError
		checkStyle = m.styles.CheckboxError
	}

	// Build the line
	line := indent + checkStyle.Render(checkbox) + style.Render(step.Title)

	// Add metadata if present
	meta := ""
	if step.Duration != "" {
		meta = "  " + m.styles.PlanMeta.Render(step.Duration)
	}
	if step.Tokens > 0 {
		if meta != "" {
			meta += "  "
		} else {
			meta = "  "
		}
		meta += m.styles.PlanMeta.Render(fmt.Sprintf("· %s", formatTokens(step.Tokens)))
	}
	if step.Status == "running" || step.Status == "active" {
		meta += "  " + m.styles.PlanMeta.Render("· thinking")
	}

	b.WriteString(line + meta)

	// Render substeps if expanded
	if step.Expanded && len(step.SubSteps) > 0 {
		for _, sub := range step.SubSteps {
			b.WriteString("\n")
			b.WriteString(m.renderPlanStep(sub, depth+1))
		}
	}

	return b.String()
}

// formatTokens formats token count in a human-readable way (e.g., "31.3k")
func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func renderHelpText() string {
	return strings.TrimSpace(`
Commands
  /help       Show this help in the transcript
  /undo       Revert the last batch of file writes (see undo stack)
  /newsession Reset session (also: /new)

Panes (terminal width ≥ 100)
  Tab         Cycle: composer → transcript → diff viewer
  In diff pane: ↑ ↓ PgUp/PgHome or mouse wheel scroll (mouse reporting must be on)
  Live diffs appear in the side pane as soon as proposals are parsed, before y/n approval.

Pending proposals
  y / Ctrl+Y  Apply pending actions
  n / Ctrl+N  Discard pending
  [ / ]       When multiple proposals, cycle which diff is shown in the side pane
`)
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

func (m *Model) clampPendingDiffIndex() {
	if len(m.pending) == 0 {
		m.pendingDiffIndex = 0
		return
	}
	m.pendingDiffIndex %= len(m.pending)
	if m.pendingDiffIndex < 0 {
		m.pendingDiffIndex += len(m.pending)
	}
}

func (m *Model) buildDiffPaneContent() string {
	var b strings.Builder

	if len(m.pending) > 0 {
		m.clampPendingDiffIndex()
		idx := m.pendingDiffIndex
		p := m.pending[idx]
		raw := strings.TrimSpace(p.Preview.Diff)
		if raw == "" {
			title := fmt.Sprintf("Pending [%d/%d]", idx+1, len(m.pending))
			b.WriteString(m.styles.DiffTitle.Render(title))
			b.WriteString("\n\n")
			b.WriteString("(No unified diff text for this action — ")
			b.WriteString(p.Proposal.Label())
			b.WriteString(")\n")
			return b.String()
		}
		return m.renderStyledDiff(p.Preview.Summary, raw, idx+1, len(m.pending))
	}

	if strings.TrimSpace(m.sideDiffLive) != "" {
		title := strings.TrimSpace(m.sideDiffTitle)
		if title == "" {
			title = "Live preview"
		}
		return m.renderStyledDiff(title, strings.TrimSpace(m.sideDiffLive), 0, 0)
	}

	if m.streaming && strings.TrimSpace(m.streamDiffSnippet) != "" {
		return m.renderStyledDiff("Streaming (partial diff)", strings.TrimSpace(m.streamDiffSnippet), 0, 0)
	}

	b.WriteString(m.styles.DiffTitle.Render("Diff viewer"))
	b.WriteString("\n\n")
	b.WriteString("(No preview yet. During agent runs, proposed diffs appear here before approval. With streaming, partial @@ hunks show when detected.)\n")
	return b.String()
}

func (m *Model) renderStyledDiff(summary, diffText string, current, total int) string {
	var b strings.Builder

	// Parse statistics and render header
	stats := diff.ParseDiffStats(diffText)
	var title string
	if total > 0 {
		title = fmt.Sprintf("Pending [%d/%d] — ", current, total)
	}
	if stats.FilePath != "" {
		title += stats.FilePath
	} else if summary != "" {
		title += summary
	}

	b.WriteString(m.styles.DiffHeader.Render(title))
	b.WriteString("\n")

	if stats.Added > 0 || stats.Removed > 0 {
		statsText := fmt.Sprintf("Added %d line", stats.Added)
		if stats.Added != 1 {
			statsText += "s"
		}
		statsText += fmt.Sprintf(", removed %d line", stats.Removed)
		if stats.Removed != 1 {
			statsText += "s"
		}
		b.WriteString(m.styles.DiffTitle.Render(statsText))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Parse and render the diff with line numbers
	lines, _ := diff.StyledDiff(diffText)

	for _, line := range lines {
		switch line.Type {
		case "@":
			// Hunk header
			b.WriteString(m.styles.DiffHunkHeader.Render(line.Content))
			b.WriteString("\n")

		case "+":
			// Added line
			lineNum := ""
			if line.NewLineNum > 0 {
				lineNum = m.styles.DiffAddedLineNum.Render(fmt.Sprintf("%d", line.NewLineNum))
			}
			content := m.styles.DiffAdded.Render(" + " + line.Content)
			b.WriteString(lineNum + content)
			b.WriteString("\n")

		case "-":
			// Removed line
			lineNum := ""
			if line.OldLineNum > 0 {
				lineNum = m.styles.DiffRemovedLineNum.Render(fmt.Sprintf("%d", line.OldLineNum))
			}
			content := m.styles.DiffRemoved.Render(" - " + line.Content)
			b.WriteString(lineNum + content)
			b.WriteString("\n")

		case " ":
			// Context line
			oldNum := ""
			newNum := ""
			if line.OldLineNum > 0 {
				oldNum = m.styles.DiffLineNumCtx.Render(fmt.Sprintf("%d", line.OldLineNum))
			}
			if line.NewLineNum > 0 {
				newNum = m.styles.DiffLineNumCtx.Render(fmt.Sprintf("%d", line.NewLineNum))
			}
			// For context lines, show old line number on left, new on right (but we only have space for one)
			// Show old line number since it's the reference
			if oldNum != "" {
				b.WriteString(oldNum)
			} else if newNum != "" {
				b.WriteString(newNum)
			}
			b.WriteString(m.styles.DiffContext.Render("   " + line.Content))
			b.WriteString("\n")

		case "h":
			// File header (---/+++ lines) - skip or show minimally
			// We'll skip these as we already show the file path in the header
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) addItem(kind, title, body, meta string) {
	m.addItemWithBadges(kind, title, body, meta, nil)
}

func (m *Model) addItemWithBadges(kind, title, body, meta string, badges []Badge) {
	item := transcriptItem{
		Kind:   kind,
		Title:  title,
		Body:   body,
		Meta:   meta,
		Badges: badges,
	}

	// If we're in a thinking card, add as subitem instead
	if kind != "thinking" && m.currentThinkingCard >= 0 && m.currentThinkingCard < len(m.transcript) {
		m.transcript[m.currentThinkingCard].SubItems = append(
			m.transcript[m.currentThinkingCard].SubItems,
			item,
		)
		m.renderTranscript()
		return
	}

	m.transcript = append(m.transcript, item)
	m.renderTranscript()
}

// addSubItem adds a sub-item under the current thinking card
func (m *Model) addSubItem(kind, title, body, meta string) {
	if m.currentThinkingCard < 0 || m.currentThinkingCard >= len(m.transcript) {
		// No active thinking card, add as top-level
		m.addItem(kind, title, body, meta)
		return
	}

	m.transcript[m.currentThinkingCard].SubItems = append(
		m.transcript[m.currentThinkingCard].SubItems,
		transcriptItem{
			Kind:  kind,
			Title: title,
			Body:  body,
			Meta:  meta,
		},
	)
	m.renderTranscript()
}

// startThinkingCard starts a new thinking card for grouping sub-items
func (m *Model) startThinkingCard(body, meta string) {
	m.currentThinkingCard = len(m.transcript)
	m.transcript = append(m.transcript, transcriptItem{
		Kind:     "thinking",
		Body:     body,
		Meta:     meta,
		SubItems: []transcriptItem{},
	})
	m.renderTranscript()
}

// endThinkingCard ends the current thinking card
func (m *Model) endThinkingCard() {
	m.currentThinkingCard = -1
}

func (m *Model) updateTranscriptItem(index int, kind, title, body, meta string) {
	if index < 0 || index >= len(m.transcript) {
		return
	}
	// Preserve existing SubItems when updating
	existingSubItems := m.transcript[index].SubItems
	m.transcript[index] = transcriptItem{
		Kind:     kind,
		Title:    title,
		Body:     body,
		Meta:     meta,
		SubItems: existingSubItems,
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
			Body:  "Terminal UI ready. Ask for code changes or use @path. Wide terminal: transcript + colored diff pane; `/help` for commands.",
		}))
	}
	m.viewport.SetContent(strings.Join(blocks, "\n\n"))
	m.viewport.GotoBottom()
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
	if start < 0 {
		start = 0
	}
	if start >= len(spinnerFrames) {
		start = len(spinnerFrames) - 1
	}
	end := start + 4
	if end > len(spinnerFrames) {
		end = len(spinnerFrames)
	}
	phaseFrames := spinnerFrames[start:end]
	if len(phaseFrames) == 0 {
		return spinnerFrames[0]
	}
	return phaseFrames[m.thinkingFrame%len(phaseFrames)]
}

func (m *Model) finishActivity(title, body, meta string) {

	if m.thinkingTicker != nil {
		m.thinkingTicker.Stop()
		m.thinkingTicker = nil
	}

	if m.thinkingCardIndex >= 0 && m.thinkingCardIndex < len(m.transcript) {
		m.transcript[m.thinkingCardIndex].Kind = "crunched"
		m.transcript[m.thinkingCardIndex].Title = "Crunch"
		m.transcript[m.thinkingCardIndex].Meta = ""
		m.renderTranscript()
		m.thinkingCardIndex = -1
		m.currentThinkingCard = -1
	}

	if m.activityIndex >= 0 && m.activityIndex < len(m.transcript) {
		m.updateTranscriptItem(m.activityIndex, "system", title, body, meta)
		m.activityIndex = -1
		return
	}
	m.addItem("system", title, body, meta)
}

func (m *Model) updateActivity(body, meta, phase string) {

	if phase != "" {
		m.currentPhase = phase
	}

	if m.thinkingTicker == nil {
		m.thinkingTicker = time.NewTicker(400 * time.Millisecond)
		m.thinkingFrame = 0

		if m.thinkingCardIndex < 0 || m.thinkingCardIndex >= len(m.transcript) {
			m.thinkingCardIndex = len(m.transcript)
			m.currentThinkingCard = m.thinkingCardIndex
			m.addItem("thinking", m.cookingFrame(), body, meta)
		}
	}

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

// CreatePlan creates a new plan and displays it in the transcript
func (m *Model) CreatePlan(title string) *Plan {
	plan := &Plan{
		ID:        fmt.Sprintf("plan-%d", time.Now().Unix()),
		Title:     title,
		Status:    "planning",
		StartTime: time.Now(),
		Expanded:  true,
		Steps:     []PlanStep{},
	}
	m.activePlan = plan
	m.planDisplayIndex = len(m.transcript)
	m.addItem("plan", title, m.renderPlan(plan), "")
	return plan
}

// UpdatePlan updates the active plan display
func (m *Model) UpdatePlan() {
	if m.activePlan == nil {
		return
	}
	// Update duration
	if m.activePlan.StartTime.IsZero() {
		m.activePlan.Duration = ""
	} else {
		elapsed := time.Since(m.activePlan.StartTime)
		m.activePlan.Duration = formatDuration(elapsed)
	}

	if m.planDisplayIndex >= 0 && m.planDisplayIndex < len(m.transcript) {
		m.transcript[m.planDisplayIndex].Body = m.renderPlan(m.activePlan)
		m.renderTranscript()
	}
}

// CompletePlan marks the active plan as complete
func (m *Model) CompletePlan() {
	if m.activePlan == nil {
		return
	}
	m.activePlan.Status = "complete"
	m.UpdatePlan()
	m.activePlan = nil
	m.planDisplayIndex = -1
}

// AddPlanStep adds a step to the active plan
func (m *Model) AddPlanStep(title, status string) *PlanStep {
	if m.activePlan == nil {
		m.CreatePlan("Working...")
	}
	step := PlanStep{
		ID:     fmt.Sprintf("step-%d", len(m.activePlan.Steps)),
		Title:  title,
		Status: status,
	}
	m.activePlan.Steps = append(m.activePlan.Steps, step)
	m.UpdatePlan()
	return &m.activePlan.Steps[len(m.activePlan.Steps)-1]
}

// UpdatePlanStep updates the status of a plan step
func (m *Model) UpdatePlanStep(stepID, status string) {
	if m.activePlan == nil {
		return
	}
	for i := range m.activePlan.Steps {
		if m.activePlan.Steps[i].ID == stepID {
			m.activePlan.Steps[i].Status = status
			m.UpdatePlan()
			return
		}
	}
}

// SetPlanTokens updates the token count for the active plan
func (m *Model) SetPlanTokens(tokens int) {
	if m.activePlan == nil {
		return
	}
	m.activePlan.Tokens = tokens
	m.UpdatePlan()
}

// SetPlanStepTokens updates the token count for a specific step
func (m *Model) SetPlanStepTokens(stepID string, tokens int) {
	if m.activePlan == nil {
		return
	}
	for i := range m.activePlan.Steps {
		if m.activePlan.Steps[i].ID == stepID {
			m.activePlan.Steps[i].Tokens = tokens
			m.UpdatePlan()
			return
		}
	}
}

// formatDuration formats a duration in a human-readable way (e.g., "8m 54s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs > 0 {
			return fmt.Sprintf("%dm %ds", mins, secs)
		}
		return fmt.Sprintf("%dm", mins)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
