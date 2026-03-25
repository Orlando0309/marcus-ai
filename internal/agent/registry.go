package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
)

// AgentRegistry manages agent lifecycle
type AgentRegistry interface {
	Register(def AgentDef) error
	Spawn(role string, goal string, parent string, opts SpawnOptions) (AgentInstance, error)
	Kill(id string) error
	Pause(id string) error
	Resume(id string) error
	List() []AgentInstance
	Get(id string) (AgentInstance, bool)
	Communicate(from, to string, msg Message) error
}

// AgentDef defines an agent profile
type AgentDef struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Role        string   `toml:"role"` // explorer, researcher, coder, reviewer, debugger, architect, planner
	SystemPrompt string  `toml:"system_prompt"`
	Tools       []string `toml:"tools,omitempty"`
	MaxIterations int   `toml:"max_iterations,omitempty"`
	AutoApprove bool   `toml:"auto_approve,omitempty"`
}

// SpawnOptions are options for spawning an agent
type SpawnOptions struct {
	MaxIterations  int
	AllowedTools   []string
	UseIsolation   bool
	Timeout        time.Duration
	Context        map[string]any
}

// AgentInstance represents a running agent
type AgentInstance struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Role      string                 `json:"role"`
	Goal      string                 `json:"goal"`
	ParentID  string                 `json:"parent_id,omitempty"`
	Status    string                 `json:"status"` // running, paused, completed, failed
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Result    *AgentResult            `json:"result,omitempty"`
	Context   map[string]any         `json:"context,omitempty"`
}

// AgentResult is the result of an agent execution
type AgentResult struct {
	Summary      string   `json:"summary"`
	Success      bool     `json:"success"`
	Output       string   `json:"output,omitempty"`
	Error        string   `json:"error,omitempty"`
	Iterations   int      `json:"iterations"`
	Actions      []string `json:"actions,omitempty"`
}

// Message is an inter-agent message
type Message struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Type      string    `json:"type"` // request, response, broadcast, status
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Specialized agent roles
const (
	RoleExplorer   = "explorer"
	RoleResearcher = "researcher"
	RoleCoder      = "coder"
	RoleReviewer   = "reviewer"
	RoleDebugger   = "debugger"
	RoleArchitect  = "architect"
	RolePlanner    = "planner"
)

// InMemoryAgentRegistry is an in-memory implementation of AgentRegistry
type InMemoryAgentRegistry struct {
	mu        sync.RWMutex
	defs      map[string]AgentDef        // role -> definition
	agents    map[string]*runningAgent   // id -> running agent
	messageCh chan Message
	folders   *folder.FolderEngine
	cfg       *config.Config
	baseDir   string
}

// runningAgent represents an actively running agent
type runningAgent struct {
	instance AgentInstance
	cancel   context.CancelFunc
	engine   *flow.LoopEngine
	messages []Message
}

// NewInMemoryAgentRegistry creates a new in-memory agent registry
func NewInMemoryAgentRegistry(folders *folder.FolderEngine, cfg *config.Config, baseDir string) *InMemoryAgentRegistry {
	reg := &InMemoryAgentRegistry{
		defs:      make(map[string]AgentDef),
		agents:    make(map[string]*runningAgent),
		messageCh: make(chan Message, 100),
		folders:   folders,
		cfg:       cfg,
		baseDir:   baseDir,
	}

	// Load predefined agents
	reg.loadPredefinedAgents()

	return reg
}

// loadPredefinedAgents loads predefined agent definitions
func (r *InMemoryAgentRegistry) loadPredefinedAgents() {
	// Explorer agent - discovers and maps codebases
	r.defs[RoleExplorer] = AgentDef{
		Name:        "explorer",
		Description: "Discovers and maps project structure and code",
		Role:        RoleExplorer,
		SystemPrompt: `You are an explorer agent. Your job is to discover and map the project structure.
Focus on understanding:
- Overall project architecture
- Key files and directories
- Dependencies and imports
- Code patterns and conventions

Use tools to explore the codebase systematically. Be thorough but efficient.`,
		Tools:         []string{"list_files", "read_file", "glob_files", "search_code"},
		MaxIterations: 20,
		AutoApprove:   true,
	}

	// Researcher agent - finds solutions and best practices
	r.defs[RoleResearcher] = AgentDef{
		Name:        "researcher",
		Description: "Researches solutions, libraries, and best practices",
		Role:        RoleResearcher,
		SystemPrompt: `You are a researcher agent. Your job is to find the best solutions and practices.
Focus on:
- Researching appropriate libraries and tools
- Finding relevant documentation
- Identifying best practices
- Comparing different approaches

Be thorough in your research and cite sources.`,
		Tools:         []string{"fetch_url", "search_code", "read_file"},
		MaxIterations: 15,
		AutoApprove:   true,
	}

	// Coder agent - writes implementation code
	r.defs[RoleCoder] = AgentDef{
		Name:        "coder",
		Description: "Writes implementation code following project patterns",
		Role:        RoleCoder,
		SystemPrompt: `You are a coder agent. Your job is to write clean, working code.
Focus on:
- Following existing code patterns
- Writing idiomatic code
- Adding appropriate error handling
- Including necessary comments

Always verify your changes compile and tests pass.`,
		Tools:         []string{"read_file", "write_file", "edit_file", "patch_file", "run_command"},
		MaxIterations: 25,
		AutoApprove:   false,
	}

	// Reviewer agent - reviews code for quality and issues
	r.defs[RoleReviewer] = AgentDef{
		Name:        "reviewer",
		Description: "Reviews code for quality, bugs, and improvements",
		Role:        RoleReviewer,
		SystemPrompt: `You are a reviewer agent. Your job is to review code critically.
Focus on:
- Identifying bugs and potential issues
- Checking code style and conventions
- Suggesting improvements
- Verifying edge cases are handled

Be thorough but constructive in your feedback.`,
		Tools:         []string{"read_file", "search_code", "run_command"},
		MaxIterations: 15,
		AutoApprove:   true,
	}

	// Debugger agent - diagnoses and fixes issues
	r.defs[RoleDebugger] = AgentDef{
		Name:        "debugger",
		Description: "Diagnoses and fixes bugs and issues",
		Role:        RoleDebugger,
		SystemPrompt: `You are a debugger agent. Your job is to find and fix bugs.
Focus on:
- Reproducing the issue
- Finding root causes
- Implementing minimal fixes
- Verifying fixes work

Be methodical in your debugging approach.`,
		Tools:         []string{"read_file", "run_command", "search_code", "edit_file", "run_in_background"},
		MaxIterations: 30,
		AutoApprove:   false,
	}

	// Architect agent - designs system architecture
	r.defs[RoleArchitect] = AgentDef{
		Name:        "architect",
		Description: "Designs system architecture and interfaces",
		Role:        RoleArchitect,
		SystemPrompt: `You are an architect agent. Your job is to design clean architecture.
Focus on:
- System design and interfaces
- Component relationships
- API design
- Scalability and maintainability

Think carefully about trade-offs and document your decisions.`,
		Tools:         []string{"read_file", "write_file", "list_files", "search_code"},
		MaxIterations: 20,
		AutoApprove:   true,
	}

	// Planner agent - creates execution plans
	r.defs[RolePlanner] = AgentDef{
		Name:        "planner",
		Description: "Creates execution plans for complex tasks",
		Role:        RolePlanner,
		SystemPrompt: `You are a planner agent. Your job is to create effective execution plans.
Focus on:
- Breaking down complex tasks
- Identifying dependencies
- Estimating effort
- Creating actionable steps

Make plans concrete and actionable.`,
		Tools:         []string{"read_file", "list_files", "search_code"},
		MaxIterations: 15,
		AutoApprove:   true,
	}
}

// Register registers an agent definition
func (r *InMemoryAgentRegistry) Register(def AgentDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if def.Role == "" {
		return fmt.Errorf("agent role is required")
	}

	r.defs[def.Role] = def
	return nil
}

// Spawn spawns a new agent instance
func (r *InMemoryAgentRegistry) Spawn(role string, goal string, parent string, opts SpawnOptions) (AgentInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get agent definition
	def, ok := r.defs[role]
	if !ok {
		return AgentInstance{}, fmt.Errorf("unknown agent role: %s", role)
	}

	// Create agent instance
	id := generateAgentID(role)
	instance := AgentInstance{
		ID:        id,
		Name:      fmt.Sprintf("%s-%s", def.Name, id[:8]),
		Role:      role,
		Goal:      goal,
		ParentID:  parent,
		Status:    "running",
		StartTime: time.Now(),
		Context:   opts.Context,
	}

	// Create context for the agent
	ctx, cancel := context.WithCancel(context.Background())
	if opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	}

	// Store running agent
	r.agents[id] = &runningAgent{
		instance: instance,
		cancel:   cancel,
		messages: make([]Message, 0),
	}

	// Start agent execution in background
	go r.runAgent(ctx, id, def, goal, opts)

	return instance, nil
}

// runAgent executes an agent's work
func (r *InMemoryAgentRegistry) runAgent(ctx context.Context, id string, def AgentDef, goal string, opts SpawnOptions) {
	// Create loop engine for this agent
	// This is simplified - real implementation would set up proper dependencies
	_ = def.MaxIterations
	if opts.MaxIterations > 0 {
		_ = opts.MaxIterations
	}

	result := AgentResult{
		Actions: make([]string, 0),
	}

	// Simulate agent work (real implementation would use LoopEngine)
	// For now, just mark as complete after a delay
	select {
	case <-ctx.Done():
		result.Success = false
		result.Error = ctx.Err().Error()
	case <-time.After(100 * time.Millisecond):
		result.Success = true
		result.Summary = fmt.Sprintf("Agent %s completed goal: %s", def.Name, goal)
	}

	// Update agent status
	r.mu.Lock()
	if agent, ok := r.agents[id]; ok {
		now := time.Now()
		agent.instance.Status = "completed"
		if result.Error != "" {
			agent.instance.Status = "failed"
		}
		agent.instance.EndTime = &now
		agent.instance.Result = &result
	}
	r.mu.Unlock()
}

// Kill kills an agent
func (r *InMemoryAgentRegistry) Kill(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	if agent.cancel != nil {
		agent.cancel()
	}

	now := time.Now()
	agent.instance.Status = "killed"
	agent.instance.EndTime = &now

	return nil
}

// Pause pauses an agent
func (r *InMemoryAgentRegistry) Pause(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	if agent.instance.Status != "running" {
		return fmt.Errorf("agent is not running: %s", id)
	}

	agent.instance.Status = "paused"
	return nil
}

// Resume resumes a paused agent
func (r *InMemoryAgentRegistry) Resume(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	if agent.instance.Status != "paused" {
		return fmt.Errorf("agent is not paused: %s", id)
	}

	agent.instance.Status = "running"
	return nil
}

// List lists all agents
func (r *InMemoryAgentRegistry) List() []AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]AgentInstance, 0, len(r.agents))
	for _, agent := range r.agents {
		instances = append(instances, agent.instance)
	}

	return instances
}

// Get gets an agent by ID
func (r *InMemoryAgentRegistry) Get(id string) (AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return AgentInstance{}, false
	}

	return agent.instance, true
}

// Communicate sends a message between agents
func (r *InMemoryAgentRegistry) Communicate(from, to string, msg Message) error {
	msg.From = from
	msg.To = to
	msg.Timestamp = time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Store message for recipient
	if agent, ok := r.agents[to]; ok {
		agent.messages = append(agent.messages, msg)
	}

	// Also broadcast to message channel
	select {
	case r.messageCh <- msg:
	default:
		// Channel full, drop message
	}

	return nil
}

// GetMessages gets messages for an agent
func (r *InMemoryAgentRegistry) GetMessages(agentID string) []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[agentID]
	if !ok {
		return nil
	}

	// Return copy of messages
	msgs := make([]Message, len(agent.messages))
	copy(msgs, agent.messages)

	// Clear messages
	agent.messages = agent.messages[:0]

	return msgs
}

// GetMessageChannel returns the message channel
func (r *InMemoryAgentRegistry) GetMessageChannel() <-chan Message {
	return r.messageCh
}

// generateAgentID generates a unique agent ID
func generateAgentID(role string) string {
	return fmt.Sprintf("%s-%d", role, time.Now().UnixNano())
}

// GetRegisteredRoles returns all registered agent roles
func (r *InMemoryAgentRegistry) GetRegisteredRoles() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]string, 0, len(r.defs))
	for role := range r.defs {
		roles = append(roles, role)
	}

	return roles
}

// GetAgentDef gets an agent definition
func (r *InMemoryAgentRegistry) GetAgentDef(role string) (AgentDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.defs[role]
	return def, ok
}
