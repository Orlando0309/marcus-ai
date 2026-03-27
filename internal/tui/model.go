package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	Kind     string
	Title    string
	Body     string
	Meta     string
	SubItems []transcriptItem // For nested content like task lists under thinking cards
}

type PlanStep struct {
	ID          string
	Title       string
	Status      string // "pending", "active", "complete", "error"
	Duration    string
	Tokens      int
	SubSteps    []PlanStep
	Expanded    bool
}

type Plan struct {
	ID          string
	Title       string
	Status      string // "planning", "running", "complete", "error"
	Duration    string
	Tokens      int
	Steps       []PlanStep
	StartTime   time.Time
	Expanded    bool
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

// focusComposer / focusTranscript — Tab cycles panes.
const (
	focusComposer = iota
	focusTranscript
)

// Model is the Marcus TUI model (transcript + optional diff pane + composer).
type Model struct {
	provider          provider.Provider
	providerRuntime   *provider.Runtime
	flowEngine        *flow.Engine
	loopEngine        *flow.LoopEngine
	toolRunner        *tool.ToolRunner
	codeIndex         *codeintel.Index
	lspBroker         *lsp.Broker
	memoryManager     *memory.Manager
	isolationManager  *isolation.Manager
	cfg               *config.Config
	styles            Styles
	viewport          viewport.Model
	textarea          textarea.Model

	// Claude Code-style state
	currentThinkingCard int // Index of the current thinking card in transcript
	thinkingSubItems    []transcriptItem // Subtasks/tool calls under current thinking card
	width             int
	height            int
	ready             bool
	focusPane         int
	projectRoot       string
	pendingDiffIndex  int
	undoMu            sync.Mutex
	undoStack         []tool.UndoBatch
	agentContMu       sync.Mutex
	agentContinuation *agentContinuation
	busy              bool
	status            string
	transcript        []transcriptItem
	pending           []pendingAction
	session           *session.Session
	sessionStore      *session.Store
	contextAssembler  *ctxpkg.Assembler
	taskStore         *task.Store
	latestContext     ctxpkg.Snapshot
	activeContext     ctxpkg.Snapshot
	streamBuffer      strings.Builder
	streaming         bool
	activityIndex     int
	taskBoardIndex    int
	retryCount        int
	stepMode          bool
	stepPaused        bool
	stepSignal        chan struct{}
	stepPending       bool
	currentAgent      *folder.AgentDef

	// Kitchen spinner state
	thinkingTicker    *time.Ticker
	thinkingFrame     int
	thinkingCardIndex int
	currentPhase      string // active cooking phase for spinner title

	// Side diff pane: live preview before pending queue; streaming snippet; pending wins when set.
	sideDiffLive      string
	sideDiffTitle     string
	streamDiffSnippet string

	// Plan display state
	activePlan        *Plan
	planDisplayIndex  int
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

type undoPopMsg struct {
	err      error
	restored int
	paths    []string
}

// sideDiffMsg updates the right diff pane before assistantResponseMsg fills pending.
type sideDiffMsg struct {
	text  string
	title string
}

// loopPausedMsg signals that the agent loop is waiting for step-mode resume.
type loopPausedMsg struct {
	iteration int
}

// agentContinuation captures loop state when pausing for user approval so we can
// resume the same goal after apply (otherwise the goroutine exits and the agent stops).
type agentContinuation struct {
	userContent         string
	startLoop           int
	maxIterations       int
	messages            []provider.Message
	toolResults         []string
	lastActionSignature string
	stagnationCount     int
}

// New creates the Marcus single-pane TUI model.
func New(cfg *config.Config) (*Model, error) {
	// Ensure API key is available in environment for providers that need it
	if config.ProviderNeedsAPIKey(cfg.Provider) {
		apiKey, err := config.GetAPIKey(cfg.Provider)
		if err == nil && apiKey != "" {
			// Set environment variable for the provider to pick up
			os.Setenv(config.ProviderAPIKeyEnvVar(cfg.Provider), apiKey)
		}
	}

	prov, err := provider.Stack(cfg.Provider, cfg.Model, cfg.ProviderFallbacks)
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
		BaseDir:        baseDir,
		Config:         cfg,
		Folders:        flowEngine.FolderEngine(),
		CodeIndex:      codeIndex,
		LSP:            lspBroker,
		SubagentRunner: flow.NewSubagentRunner(flowEngine.FolderEngine(), cfg, baseDir),
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

	mainVP := viewport.New(100, 24)
	mainVP.MouseWheelEnabled = true

	model := &Model{
		provider:            prov,
		providerRuntime:     provider.NewRuntime(prov, baseDir, cfg.ProviderCfg.CacheEnabled),
		flowEngine:          flowEngine,
		toolRunner:          toolRunner,
		codeIndex:           codeIndex,
		lspBroker:           lspBroker,
		memoryManager:       memoryManager,
		isolationManager:    isolation.NewManager(baseDir, cfg.Isolation),
		cfg:                 cfg,
		styles:              DefaultStyles(),
		viewport:            mainVP,
		textarea:            ta,
		focusPane:           focusComposer,
		projectRoot:         baseDir,
		ready:               true,
		status:              "ready",
		taskStore:           taskStore,
		session:             sess,
		sessionStore:        sessionStore,
		contextAssembler:    ctxpkg.NewAssembler(cfg, flowEngine, taskStore, memoryManager),
		width:               100,
		height:              30,
		activityIndex:       -1,
		taskBoardIndex:      -1,
		currentThinkingCard: -1,
	}

	ctxAsm := model.contextAssembler
	loopCtxAsm := flowContextAssembler{
		delegate: ctxAsm,
		toFlowSnapshot: func(ctxSnap ctxpkg.Snapshot) flow.Snapshot {
			return flow.Snapshot{
				Text:            ctxSnap.Text,
				Branch:          ctxSnap.Branch,
				Dirty:           ctxSnap.Dirty,
				FileHints:       ctxSnap.FileHints,
				TODOHints:       ctxSnap.TODOHints,
				EstimatedTokens: ctxSnap.EstimatedTokens,
				Truncated:       ctxSnap.Truncated,
				DroppedSections: ctxSnap.DroppedSections,
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
			"Project root: %s\nFlows: %s\nTools: %s\nTasks: %s\n\nWiden terminal (≥100 cols) for transcript + diff panes. Tab cycles focus; `/undo` reverts the last file batch.",
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

// Run starts the TUI application.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	return RunWithConfig(cfg, "")
}

// RunWithConfig starts the TUI application with a pre-configured config.
func RunWithConfig(cfg *config.Config, resumeSession string) error {
	model, err := New(cfg)
	if err != nil {
		return fmt.Errorf("create model: %w", err)
	}

	// TODO: Handle resumeSession if needed
	_ = resumeSession

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run program: %w", err)
	}
	return nil
}
