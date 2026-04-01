package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/flow"
)

// SwarmCoordinator orchestrates multi-agent teams for complex goals
type SwarmCoordinator struct {
	mu           sync.RWMutex
	registry     *InMemoryAgentRegistry
	protocol     *Protocol
	blackboard   *Blackboard
	activeSwarms map[string]*Swarm
	maxSwarms    int
}

// Swarm represents an active agent team
type Swarm struct {
	ID          string            `json:"id"`
	Goal        string            `json:"goal"`
	Agents      []string          `json:"agents"`
	RoleMap     map[string]string `json:"role_map"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Plan        *SwarmPlan        `json:"plan,omitempty"`
}

// SwarmPlan represents the swarm's execution plan
type SwarmPlan struct {
	Phases       []SwarmPhase `json:"phases"`
	CurrentPhase int          `json:"current_phase"`
}

// SwarmPhase is a single phase in swarm execution
type SwarmPhase struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	LeadRole     string   `json:"lead_role"`
	SupportRoles []string `json:"support_roles"`
	Status       string   `json:"status"`
	Output       string   `json:"output,omitempty"`
}

// NewSwarmCoordinator creates a new swarm coordinator
func NewSwarmCoordinator(reg *InMemoryAgentRegistry, protocol *Protocol, blackboard *Blackboard, maxSwarms int) *SwarmCoordinator {
	return &SwarmCoordinator{
		registry:     reg,
		protocol:     protocol,
		blackboard:   blackboard,
		activeSwarms: make(map[string]*Swarm),
		maxSwarms:    maxSwarms,
	}
}

// SpawnSwarm creates a new agent swarm for a complex goal
func (c *SwarmCoordinator) SpawnSwarm(ctx context.Context, goal string, requiredRoles []string) (*Swarm, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.activeSwarms) >= c.maxSwarms {
		return nil, fmt.Errorf("maximum swarm limit reached (%d)", c.maxSwarms)
	}

	swarm := &Swarm{
		ID:        generateSwarmID(),
		Goal:      goal,
		Agents:    make([]string, 0),
		RoleMap:   make(map[string]string),
		Status:    "forming",
		CreatedAt: time.Now(),
	}

	for _, role := range requiredRoles {
		agent, err := c.registry.Spawn(role, goal, swarm.ID, SpawnOptions{})
		if err != nil {
			return nil, fmt.Errorf("spawn agent %s: %w", role, err)
		}

		swarm.Agents = append(swarm.Agents, agent.ID)
		swarm.RoleMap[role] = agent.ID
		c.protocol.RegisterAgent(agent.ID, role, getRoleCapabilities(role))
	}

	swarm.Plan = c.generatePlan(requiredRoles)
	swarm.Status = "active"
	c.activeSwarms[swarm.ID] = swarm

	return swarm, nil
}

func (c *SwarmCoordinator) generatePlan(roles []string) *SwarmPlan {
	phases := make([]SwarmPhase, 0)

	hasArchitect := containsRole(roles, RoleArchitect)
	hasPlanner := containsRole(roles, RolePlanner)
	hasCoder := containsRole(roles, RoleCoder)
	hasReviewer := containsRole(roles, RoleReviewer)
	hasDebugger := containsRole(roles, RoleDebugger)

	if hasArchitect || hasPlanner {
		lead := RoleArchitect
		if !hasArchitect && hasPlanner {
			lead = RolePlanner
		}
		phases = append(phases, SwarmPhase{
			ID:       "phase-1",
			Name:     "Planning & Design",
			LeadRole: lead,
			Status:   "pending",
		})
	}

	if hasCoder {
		phases = append(phases, SwarmPhase{
			ID:       "phase-2",
			Name:     "Implementation",
			LeadRole: RoleCoder,
			Status:   "pending",
		})
	}

	if hasReviewer {
		phases = append(phases, SwarmPhase{
			ID:       "phase-3",
			Name:     "Code Review",
			LeadRole: RoleReviewer,
			Status:   "pending",
		})
	}

	if hasDebugger {
		phases = append(phases, SwarmPhase{
			ID:       "phase-4",
			Name:     "Testing & Debugging",
			LeadRole: RoleDebugger,
			Status:   "pending",
		})
	}

	if len(phases) == 0 {
		phases = append(phases, SwarmPhase{
			ID:       "phase-1",
			Name:     "Execution",
			LeadRole: roles[0],
			Status:   "pending",
		})
	}

	return &SwarmPlan{
		Phases:       phases,
		CurrentPhase: 0,
	}
}

// GetSwarm returns a swarm by ID
func (c *SwarmCoordinator) GetSwarm(id string) (*Swarm, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	swarm, ok := c.activeSwarms[id]
	if !ok {
		return nil, false
	}

	copy := *swarm
	return &copy, true
}

// ListSwarms returns all active swarms
func (c *SwarmCoordinator) ListSwarms() []Swarm {
	c.mu.RLock()
	defer c.mu.RUnlock()

	swarms := make([]Swarm, 0, len(c.activeSwarms))
	for _, s := range c.activeSwarms {
		swarms = append(swarms, *s)
	}
	return swarms
}

// AdvancePhase advances a swarm to the next phase
func (c *SwarmCoordinator) AdvancePhase(swarmID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	swarm, ok := c.activeSwarms[swarmID]
	if !ok {
		return ErrSwarmNotFound
	}

	if swarm.Plan.CurrentPhase >= len(swarm.Plan.Phases)-1 {
		swarm.Status = "completed"
		now := time.Now()
		swarm.CompletedAt = &now
		return nil
	}

	swarm.Plan.Phases[swarm.Plan.CurrentPhase].Status = "completed"
	swarm.Plan.CurrentPhase++
	swarm.Plan.Phases[swarm.Plan.CurrentPhase].Status = "active"
	swarm.UpdatedAt = time.Now()

	return nil
}

// UpdateRegistryEngine updates the loop engine in the registry
func (c *SwarmCoordinator) UpdateRegistryEngine(eng *flow.LoopEngine) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registry.SetLoopEngine(eng)
}

// TerminateSwarm terminates a swarm
func (c *SwarmCoordinator) TerminateSwarm(id string, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	swarm, ok := c.activeSwarms[id]
	if !ok {
		return ErrSwarmNotFound
	}

	swarm.Status = "failed"
	swarm.UpdatedAt = time.Now()
	now := time.Now()
	swarm.CompletedAt = &now

	for _, agentID := range swarm.Agents {
		_ = c.registry.Kill(agentID)
		c.protocol.UnregisterAgent(agentID)
	}

	delete(c.activeSwarms, id)
	return nil
}

// RecommendSwarmComposition analyzes a goal and recommends agent roles
func (c *SwarmCoordinator) RecommendSwarmComposition(goal string) []string {
	requiredRoles := []string{}
	goalLower := strings.ToLower(goal)

	// Complex projects need full team
	if containsStr(goalLower, "api") || containsStr(goalLower, "service") || containsStr(goalLower, "platform") || containsStr(goalLower, "system") {
		requiredRoles = append(requiredRoles, RoleArchitect, RoleCoder, RoleReviewer)
	}

	if containsStr(goal, "design") || containsStr(goal, "architecture") || containsStr(goal, "interface") || containsStr(goal, "structure") {
		if !hasRole(requiredRoles, RoleArchitect) {
			requiredRoles = append(requiredRoles, RoleArchitect)
		}
	}
	if containsStr(goal, "plan") || containsStr(goal, "strategy") || containsStr(goal, "steps") {
		if !hasRole(requiredRoles, RolePlanner) {
			requiredRoles = append(requiredRoles, RolePlanner)
		}
	}
	if containsStr(goal, "implement") || containsStr(goal, "write") || containsStr(goal, "create") || containsStr(goal, "add") || containsStr(goal, "build") || containsStr(goal, "develop") {
		if !hasRole(requiredRoles, RoleCoder) {
			requiredRoles = append(requiredRoles, RoleCoder)
		}
	}
	if containsStr(goal, "review") || containsStr(goal, "check") || containsStr(goal, "audit") || containsStr(goal, "quality") || containsStr(goal, "test") {
		if !hasRole(requiredRoles, RoleReviewer) {
			requiredRoles = append(requiredRoles, RoleReviewer)
		}
	}
	if containsStr(goal, "debug") || containsStr(goal, "fix") || containsStr(goal, "error") || containsStr(goal, "bug") {
		if !hasRole(requiredRoles, RoleDebugger) {
			requiredRoles = append(requiredRoles, RoleDebugger)
		}
	}
	if containsStr(goal, "explore") || containsStr(goal, "discover") || containsStr(goal, "understand") || containsStr(goal, "analyze") {
		if !hasRole(requiredRoles, RoleExplorer) {
			requiredRoles = append(requiredRoles, RoleExplorer)
		}
	}

	// Default team for complex tasks
	if len(requiredRoles) == 0 {
		requiredRoles = []string{RoleExplorer, RoleCoder, RoleReviewer}
	}

	sort.Slice(requiredRoles, func(i, j int) bool {
		rolePriority := map[string]int{
			RoleArchitect: 0,
			RolePlanner:   1,
			RoleExplorer:  2,
			RoleCoder:     3,
			RoleReviewer:  4,
			RoleDebugger:  5,
		}
		return rolePriority[requiredRoles[i]] < rolePriority[requiredRoles[j]]
	})

	return requiredRoles
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func getRoleCapabilities(role string) []string {
	switch role {
	case RoleExplorer:
		return []string{"explore", "discover", "map"}
	case RoleResearcher:
		return []string{"research", "analyze", "compare"}
	case RoleCoder:
		return []string{"implement", "write", "refactor"}
	case RoleReviewer:
		return []string{"review", "audit", "validate"}
	case RoleDebugger:
		return []string{"debug", "fix", "diagnose"}
	case RoleArchitect:
		return []string{"design", "architect", "plan"}
	case RolePlanner:
		return []string{"plan", "decompose", "estimate"}
	default:
		return []string{"general"}
	}
}

func generateSwarmID() string {
	return "swarm_" + time.Now().Format("20060102150405")
}

func containsStr(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// Errors
var (
	ErrSwarmNotFound = fmt.Errorf("swarm not found")
)
