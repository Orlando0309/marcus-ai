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
	main := m.styles.TranscriptPane.Render(m.viewport.View())
	if m.width >= 100 && m.diffViewport.Width > 0 {
		diffCol := m.styles.DiffPane.Render(m.diffViewport.View())
		main = lipgloss.JoinHorizontal(lipgloss.Top, main, diffCol)
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.StatusBar.Render(m.renderStatusBar()),
		main,
		m.styles.Composer.Render(m.textarea.View()),
		m.styles.Help.Render(m.renderHelp()),
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
	split := width >= 100
	if split {
		gap := 1
		leftW := (width - gap) * 58 / 100
		rightW := width - gap - leftW
		borderPad := 4
		if rightW < borderPad+12 {
			rightW = borderPad + 12
			leftW = width - gap - rightW
		}
		m.viewport.Width = leftW - 2
		m.viewport.Height = availableHeight
		m.diffViewport.Width = rightW - borderPad
		if m.diffViewport.Width < 12 {
			m.diffViewport.Width = 12
		}
		m.diffViewport.Height = availableHeight
	} else {
		m.viewport.Width = width - 2
		m.viewport.Height = availableHeight
		m.diffViewport.Width = 0
		m.diffViewport.Height = 0
	}
	m.textarea.SetWidth(width - 4)
	m.textarea.SetHeight(3)
	m.refreshDiffPane()
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
	writeColored := func(title, body string) {
		b.WriteString(m.styles.DiffTitle.Render(title))
		b.WriteString("\n\n")
		b.WriteString(diff.RenderDiff(body))
		b.WriteString("\n\n")
	}

	if len(m.pending) > 0 {
		m.clampPendingDiffIndex()
		idx := m.pendingDiffIndex
		p := m.pending[idx]
		title := fmt.Sprintf("Pending [%d/%d] — %s", idx+1, len(m.pending), strings.TrimSpace(p.Preview.Summary))
		if title == "" {
			title = fmt.Sprintf("Pending [%d/%d]", idx+1, len(m.pending))
		}
		raw := strings.TrimSpace(p.Preview.Diff)
		if raw == "" {
			b.WriteString(m.styles.DiffTitle.Render(title))
			b.WriteString("\n\n")
			b.WriteString("(No unified diff text for this action — ")
			b.WriteString(p.Proposal.Label())
			b.WriteString(")\n")
			return b.String()
		}
		writeColored(title, raw)
		return strings.TrimRight(b.String(), "\n")
	}

	if strings.TrimSpace(m.sideDiffLive) != "" {
		title := strings.TrimSpace(m.sideDiffTitle)
		if title == "" {
			title = "Live preview"
		}
		writeColored(title, strings.TrimSpace(m.sideDiffLive))
		return strings.TrimRight(b.String(), "\n")
	}

	if m.streaming && strings.TrimSpace(m.streamDiffSnippet) != "" {
		writeColored("Streaming (partial diff)", strings.TrimSpace(m.streamDiffSnippet))
		return strings.TrimRight(b.String(), "\n")
	}

	b.WriteString(m.styles.DiffTitle.Render("Diff viewer"))
	b.WriteString("\n\n")
	b.WriteString("(No preview yet. During agent runs, proposed diffs appear here before approval. With streaming, partial @@ hunks show when detected.)\n")
	return b.String()
}

func (m *Model) refreshDiffPane() {
	m.diffViewport.SetContent(m.buildDiffPaneContent())
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
