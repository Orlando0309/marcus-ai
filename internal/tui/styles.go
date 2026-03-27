package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds all the styles used in the TUI
type Styles struct {
	StatusBar         lipgloss.Style
	StatusLabel       lipgloss.Style
	StatusValue       lipgloss.Style
	StatusKey         lipgloss.Style
	StatusMuted       lipgloss.Style
	TranscriptPane    lipgloss.Style
	DiffPane          lipgloss.Style
	DiffTitle         lipgloss.Style
	Composer          lipgloss.Style
	Help              lipgloss.Style
	HelpKey           lipgloss.Style
	HelpDesc          lipgloss.Style
	UserCard          lipgloss.Style
	AssistantCard     lipgloss.Style
	SystemCard        lipgloss.Style
	ActionCard        lipgloss.Style
	ResultCard        lipgloss.Style
	ErrorCard         lipgloss.Style
	ThinkingCard      lipgloss.Style
	ToolCallCard      lipgloss.Style
	ToolResultCard    lipgloss.Style
	IterationCard     lipgloss.Style
	CrunchCard        lipgloss.Style
	Meta              lipgloss.Style
	Title             lipgloss.Style
	DiffHeader        lipgloss.Style
	DiffHunkHeader    lipgloss.Style
	DiffLineNumOld    lipgloss.Style
	DiffLineNumNew    lipgloss.Style
	DiffLineNumCtx    lipgloss.Style
	DiffAdded         lipgloss.Style
	DiffRemoved       lipgloss.Style
	DiffContext       lipgloss.Style
	DiffAddedLineNum  lipgloss.Style
	DiffRemovedLineNum lipgloss.Style
	// Plan/Task display styles
	PlanHeader        lipgloss.Style
	PlanTitle         lipgloss.Style
	PlanMeta          lipgloss.Style
	PlanStepPending   lipgloss.Style
	PlanStepActive    lipgloss.Style
	PlanStepComplete  lipgloss.Style
	PlanStepError     lipgloss.Style
	CheckboxPending   lipgloss.Style
	CheckboxActive    lipgloss.Style
	CheckboxComplete  lipgloss.Style
	CheckboxError     lipgloss.Style
	BottomBar         lipgloss.Style
	BottomBarKey      lipgloss.Style
	BottomBarDesc     lipgloss.Style
	// Claude Code-style card styles
	BulletGreen       lipgloss.Style // Green bullet for file operations
	BulletBlue        lipgloss.Style // Blue bullet for reads
	BulletYellow      lipgloss.Style // Yellow bullet for commands
	FilePath          lipgloss.Style // File path display
	LineCount         lipgloss.Style // Line count badge (green "428 lines")
	CodeBlock         lipgloss.Style // Code block container
	CodeBlockHeader   lipgloss.Style // Code block header with file path
	NestedTask        lipgloss.Style // Nested task item under thinking card
	SubtleText        lipgloss.Style // Subtle/muted text
}

// DefaultStyles returns the default style configuration
func DefaultStyles() Styles {
	return Styles{
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Padding(0, 1),

		StatusLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8b5cf6")).
			Bold(true),

		StatusValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")),

		StatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")),

		StatusMuted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")),

		TranscriptPane: lipgloss.NewStyle().Padding(0, 1),

		DiffPane: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1),

		DiffTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Bold(true),

		Composer: lipgloss.NewStyle().
			BorderTop(true).
			BorderForeground(lipgloss.Color("#2d3748")).
			Padding(0, 1),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Padding(0, 1),

		HelpKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8b5cf6")).
			Bold(true),

		HelpDesc: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cbd5e1")),

		UserCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#93c5fd")),
		AssistantCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")),
		SystemCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cbd5e1")),
		ActionCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a78bfa")),
		ResultCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac")),
		ErrorCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")),
		ThinkingCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fde68a")).
			Italic(true),
		ToolCallCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#67e8f9")).
			Bold(false),
		ToolResultCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6ee7b7")),
		IterationCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4b5fd")).
			Bold(true),
		CrunchCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b")).
			Italic(true),

		Meta: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Italic(true),

		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Bold(true),

		DiffHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Bold(true),

		DiffHunkHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22d3ee")).
			Bold(true),

		DiffLineNumOld: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")).
			Background(lipgloss.Color("#7f1d1d")).
			Width(6).
			Align(lipgloss.Right),

		DiffLineNumNew: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac")).
			Background(lipgloss.Color("#14532d")).
			Width(6).
			Align(lipgloss.Right),

		DiffLineNumCtx: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b")).
			Width(6).
			Align(lipgloss.Right),

		DiffAdded: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac")).
			Background(lipgloss.Color("#14532d")),

		DiffRemoved: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")).
			Background(lipgloss.Color("#7f1d1d")),

		DiffContext: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cbd5e1")),

		DiffAddedLineNum: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac")).
			Background(lipgloss.Color("#14532d")).
			Width(6).
			Align(lipgloss.Right),

		DiffRemovedLineNum: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")).
			Background(lipgloss.Color("#7f1d1d")).
			Width(6).
			Align(lipgloss.Right),

		// Plan/Task display styles
		PlanHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Bold(true),

		PlanTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22d3ee")).
			Bold(true),

		PlanMeta: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")),

		PlanStepPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")),

		PlanStepActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fde68a")),

		PlanStepComplete: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac")),

		PlanStepError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")),

		CheckboxPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b")),

		CheckboxActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fbbf24")),

		CheckboxComplete: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")),

		CheckboxError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ef4444")),

		BottomBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Background(lipgloss.Color("#1e293b")).
			Padding(0, 1),

		BottomBarKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8b5cf6")).
			Bold(true),

		BottomBarDesc: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cbd5e1")),

		// Claude Code-style card styles
		BulletGreen: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")),

		BulletBlue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60a5fa")),

		BulletYellow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fbbf24")),

		FilePath: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")),

		LineCount: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")),

		CodeBlock: lipgloss.NewStyle().
			Background(lipgloss.Color("#0f172a")).
			Padding(0, 1),

		CodeBlockHeader: lipgloss.NewStyle().
			Background(lipgloss.Color("#1e293b")).
			Foreground(lipgloss.Color("#94a3b8")).
			Padding(0, 1),

		NestedTask: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cbd5e1")),

		SubtleText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b")),
	}
}

// RenderItem renders a transcript item as a card.
func (s Styles) RenderItem(item transcriptItem) string {
	switch item.Kind {
	case "user":
		return s.UserCard.Render("> " + item.Body)
	case "assistant":
		return s.AssistantCard.Render(item.Body + renderMetaInline(s, item.Meta))
	case "thinking":
		// Claude Code-style thinking card with nested tasks
		var b strings.Builder
		b.WriteString(s.ThinkingCard.Render("● " + item.Body))
		// Render nested subitems (tasks/tool calls)
		for _, sub := range item.SubItems {
			b.WriteString("\n")
			b.WriteString(renderNestedItem(s, sub, 1))
		}
		return b.String()
	case "iteration":
		return s.IterationCard.Render("◆ " + item.Title + renderMetaInline(s, item.Meta))
	case "crunched":
		return s.CrunchCard.Render("Crunch  " + item.Title + renderBody(item.Body) + renderMetaInline(s, item.Meta))
	case "tool_call":
		// Claude Code-style tool call (Write, Read, Run)
		return renderToolCallCard(s, item)
	case "tool_result":
		return s.ToolResultCard.Render("  " + item.Title + renderBodyLimited(item.Body, 10, 700) + renderMetaInline(s, item.Meta))
	case "action":
		// Claude Code-style action card with file path and diff
		return renderActionCard(s, item)
	case "result":
		return s.ResultCard.Render("  " + item.Title + renderBodyLimited(item.Body, 8, 500) + renderMetaInline(s, item.Meta))
	case "error":
		return s.ErrorCard.Render("! " + item.Title + renderBody(item.Body) + renderMetaInline(s, item.Meta))
	case "plan":
		// Plan items render their body directly (pre-formatted)
		return s.PlanHeader.Render(item.Body)
	default:
		if item.Title == "" {
			return s.SystemCard.Render(item.Body + renderMetaInline(s, item.Meta))
		}
		return s.SystemCard.Render("• " + item.Title + renderBody(item.Body) + renderMetaInline(s, item.Meta))
	}
}

// renderNestedItem renders a nested item with proper indentation
func renderNestedItem(s Styles, item transcriptItem, depth int) string {
	indent := strings.Repeat("  ", depth)
	switch item.Kind {
	case "task":
		// Task checkbox with status
		checkbox := "☐ "
		style := s.NestedTask
		if item.Meta == "done" || item.Meta == "complete" {
			checkbox = "✓ "
			style = s.PlanStepComplete
		}
		return indent + style.Render(checkbox+item.Body)
	case "tool_call":
		return indent + renderToolCallCard(s, item)
	case "action":
		return indent + renderActionCard(s, item)
	default:
		return indent + s.NestedTask.Render(item.Body)
	}
}

// renderToolCallCard renders a Claude Code-style tool call (Write, Read, Run)
func renderToolCallCard(s Styles, item transcriptItem) string {
	var b strings.Builder

	// Determine bullet color based on action type
	bullet := "●"
	bulletStyle := s.BulletGreen

	if strings.Contains(item.Title, "read") || strings.Contains(item.Title, "Read") {
		bulletStyle = s.BulletBlue
	} else if strings.Contains(item.Title, "run") || strings.Contains(item.Title, "Run") || strings.Contains(item.Title, "command") {
		bulletStyle = s.BulletYellow
	}

	b.WriteString(bulletStyle.Render(bullet))
	b.WriteString(" ")
	b.WriteString(s.FilePath.Render(item.Title))

	if item.Meta != "" {
		b.WriteString(" ")
		b.WriteString(s.SubtleText.Render(item.Meta))
	}

	return b.String()
}

// renderActionCard renders a Claude Code-style action card with file path and line count
func renderActionCard(s Styles, item transcriptItem) string {
	var b strings.Builder

	// Extract file path and line count from the title
	parts := strings.SplitN(item.Title, " — ", 2)
	filePath := parts[0]
	lineCount := ""
	if len(parts) > 1 {
		lineCount = parts[1]
	}

	// Green bullet + file path
	b.WriteString(s.BulletGreen.Render("●"))
	b.WriteString(" ")
	b.WriteString(s.FilePath.Render(filePath))

	// Line count badge (green)
	if lineCount != "" {
		b.WriteString(" ")
		b.WriteString(s.LineCount.Render(lineCount))
	}

	// Add the diff/code block if present
	if item.Body != "" {
		b.WriteString("\n")
		b.WriteString(renderCodeBlock(s, item.Body))
	}

	return b.String()
}

// renderCodeBlock renders a code block with Claude Code-style formatting
func renderCodeBlock(s Styles, code string) string {
	if strings.TrimSpace(code) == "" {
		return ""
	}

	// Indent the code block
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}

	return s.CodeBlock.Render(strings.Join(lines, "\n"))
}

func renderBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 1 {
		return "\n  " + lines[0]
	}
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return "\n" + strings.Join(lines, "\n")
}

func renderBodyLimited(body string, maxLines, maxChars int) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	if maxChars > 0 && len(trimmed) > maxChars {
		trimmed = trimmed[:maxChars] + "\n...[collapsed]"
	}
	lines := strings.Split(trimmed, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines], "...[collapsed]")
	}
	if len(lines) == 1 {
		return "\n  " + lines[0]
	}
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return "\n" + strings.Join(lines, "\n")
}

func renderMetaInline(s Styles, meta string) string {
	if strings.TrimSpace(meta) == "" {
		return ""
	}
	return "\n" + s.Meta.Render("  "+meta)
}
