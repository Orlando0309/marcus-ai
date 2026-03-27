package agent

import (
	"context"
	"fmt"
	"time"
)

// WorkflowTemplate defines a reusable multi-agent workflow pattern
type WorkflowTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Roles       []string          `json:"roles"`
	Steps       []TemplateStep    `json:"steps"`
	Timeout     time.Duration     `json:"timeout"`
	MaxRetries  int               `json:"max_retries"`
}

// TemplateStep is a step in a workflow template
type TemplateStep struct {
	ID          string   `json:"id"`
	Role        string   `json:"role"`
	GoalPattern string   `json:"goal_pattern"` // Template for goal with {{placeholders}}
	Dependencies []string `json:"dependencies,omitempty"`
	Parallel    bool     `json:"parallel"` // Can run parallel with other steps at same level
}

// WorkflowInstance is a concrete instance of a workflow
type WorkflowInstance struct {
	TemplateID   string         `json:"template_id"`
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Steps        []WorkflowStep `json:"steps"`
	CreatedAt    time.Time      `json:"created_at"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
}

// Predefined workflow templates
var predefinedTemplates = map[string]WorkflowTemplate{
	"code_review": {
		ID:          "code_review",
		Name:        "Code Review",
		Description: "Multi-agent code review with architect, coder, and reviewer",
		Roles:       []string{RoleArchitect, RoleCoder, RoleReviewer},
		Timeout:     15 * time.Minute,
		MaxRetries:  2,
		Steps: []TemplateStep{
			{
				ID:       "analyze",
				Role:     RoleArchitect,
				GoalPattern: "Analyze the code changes and identify architectural concerns",
				Parallel: false,
			},
			{
				ID:       "implement_review",
				Role:     RoleCoder,
				GoalPattern: "Review code implementation for correctness and best practices",
				Dependencies: []string{"analyze"},
				Parallel: false,
			},
			{
				ID:       "quality_check",
				Role:     RoleReviewer,
				GoalPattern: "Perform final quality check and generate review summary",
				Dependencies: []string{"implement_review"},
				Parallel: false,
			},
		},
	},
	"bug_fix": {
		ID:          "bug_fix",
		Name:        "Bug Fix",
		Description: "Debug and fix issues with reproduction and verification",
		Roles:       []string{RoleDebugger, RoleCoder, RoleReviewer},
		Timeout:     20 * time.Minute,
		MaxRetries:  3,
		Steps: []TemplateStep{
			{
				ID:       "reproduce",
				Role:     RoleDebugger,
				GoalPattern: "Reproduce the bug and identify root cause",
				Parallel: false,
			},
			{
				ID:       "fix",
				Role:     RoleCoder,
				GoalPattern: "Implement a fix for the identified issue",
				Dependencies: []string{"reproduce"},
				Parallel: false,
			},
			{
				ID:       "verify",
				Role:     RoleReviewer,
				GoalPattern: "Verify the fix resolves the issue without regressions",
				Dependencies: []string{"fix"},
				Parallel: false,
			},
		},
	},
	"feature_impl": {
		ID:          "feature_impl",
		Name:        "Feature Implementation",
		Description: "Implement a new feature from specification",
		Roles:       []string{RoleArchitect, RolePlanner, RoleCoder, RoleReviewer},
		Timeout:     30 * time.Minute,
		MaxRetries:  2,
		Steps: []TemplateStep{
			{
				ID:       "design",
				Role:     RoleArchitect,
				GoalPattern: "Design the architecture for: {{feature}}",
				Parallel: false,
			},
			{
				ID:       "plan",
				Role:     RolePlanner,
				GoalPattern: "Create implementation plan based on architecture",
				Dependencies: []string{"design"},
				Parallel: false,
			},
			{
				ID:       "implement",
				Role:     RoleCoder,
				GoalPattern: "Implement the feature following the plan",
				Dependencies: []string{"plan"},
				Parallel: false,
			},
			{
				ID:       "test",
				Role:     RoleReviewer,
				GoalPattern: "Review implementation and suggest tests",
				Dependencies: []string{"implement"},
				Parallel: false,
			},
		},
	},
	"research": {
		ID:          "research",
		Name:        "Research Task",
		Description: "Research a topic with multiple agents and synthesize findings",
		Roles:       []string{RoleResearcher, RoleExplorer, RoleArchitect},
		Timeout:     10 * time.Minute,
		MaxRetries:  1,
		Steps: []TemplateStep{
			{
				ID:       "explore",
				Role:     RoleExplorer,
				GoalPattern: "Explore the codebase for relevant context on: {{topic}}",
				Parallel: true,
			},
			{
				ID:       "research",
				Role:     RoleResearcher,
				GoalPattern: "Research best practices and solutions for: {{topic}}",
				Parallel: true,
			},
			{
				ID:       "synthesize",
				Role:     RoleArchitect,
				GoalPattern: "Synthesize findings into actionable recommendations",
				Dependencies: []string{"explore", "research"},
				Parallel: false,
			},
		},
	},
	"refactor": {
		ID:          "refactor",
		Name:        "Code Refactoring",
		Description: "Safe code refactoring with analysis and verification",
		Roles:       []string{RoleArchitect, RoleCoder, RoleReviewer},
		Timeout:     20 * time.Minute,
		MaxRetries:  2,
		Steps: []TemplateStep{
			{
				ID:       "analyze_impact",
				Role:     RoleArchitect,
				GoalPattern: "Analyze refactoring impact and identify affected components",
				Parallel: false,
			},
			{
				ID:       "refactor",
				Role:     RoleCoder,
				GoalPattern: "Perform the refactoring changes",
				Dependencies: []string{"analyze_impact"},
				Parallel: false,
			},
			{
				ID:       "verify",
				Role:     RoleReviewer,
				GoalPattern: "Verify refactoring preserves behavior",
				Dependencies: []string{"refactor"},
				Parallel: false,
			},
		},
	},
}

// GetTemplate retrieves a workflow template by ID
func GetTemplate(id string) (WorkflowTemplate, bool) {
	tpl, ok := predefinedTemplates[id]
	return tpl, ok
}

// ListTemplates returns all available template IDs
func ListTemplates() []string {
	ids := make([]string, 0, len(predefinedTemplates))
	for id := range predefinedTemplates {
		ids = append(ids, id)
	}
	return ids
}

// RegisterTemplate registers a custom workflow template
func RegisterTemplate(tpl WorkflowTemplate) error {
	if tpl.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if len(tpl.Steps) == 0 {
		return fmt.Errorf("template must have at least one step")
	}
	predefinedTemplates[tpl.ID] = tpl
	return nil
}

// Instantiate creates a workflow instance from a template
func (t *WorkflowTemplate) Instantiate(variables map[string]string) *WorkflowInstance {
	instance := &WorkflowInstance{
		TemplateID: t.ID,
		ID:         fmt.Sprintf("%s-%d", t.ID, time.Now().UnixNano()),
		Status:     "pending",
		CreatedAt:  time.Now(),
		Steps:      make([]WorkflowStep, 0, len(t.Steps)),
	}

	for _, step := range t.Steps {
		goal := step.GoalPattern
		// Replace variables
		for k, v := range variables {
			goal = replacePlaceholder(goal, k, v)
		}

		instance.Steps = append(instance.Steps, WorkflowStep{
			ID:           step.ID,
			Role:         step.Role,
			Goal:         goal,
			Dependencies: step.Dependencies,
			Context:      make(map[string]any),
		})
	}

	return instance
}

// replacePlaceholder replaces {{key}} with value
func replacePlaceholder(s, key, value string) string {
	placeholder := "{{" + key + "}}"
	return replaceAll(s, placeholder, value)
}

// Simple string replacement
func replaceAll(s, old, new string) string {
	result := ""
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			result += s
			break
		}
		result += s[:idx] + new
		s = s[idx+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// AgentMessageBus handles inter-agent communication
type AgentMessageBus struct {
	messages   map[string][]Message
	subscribers map[string][]string // role -> agent IDs
	mutex      chan struct{}
}

// NewAgentMessageBus creates a new message bus
func NewAgentMessageBus() *AgentMessageBus {
	return &AgentMessageBus{
		messages:    make(map[string][]Message),
		subscribers: make(map[string][]string),
		mutex:       make(chan struct{}, 1),
	}
}

// Subscribe registers an agent to receive messages for a role
func (b *AgentMessageBus) Subscribe(agentID, role string) {
	select {
	case b.mutex <- struct{}{}:
		defer func() { <-b.mutex }()
		b.subscribers[role] = append(b.subscribers[role], agentID)
	case <-time.After(100 * time.Millisecond):
	}
}

// Unsubscribe removes an agent from a role subscription
func (b *AgentMessageBus) Unsubscribe(agentID, role string) {
	select {
	case b.mutex <- struct{}{}:
		defer func() { <-b.mutex }()
		subscribers := b.subscribers[role]
		for i, id := range subscribers {
			if id == agentID {
				b.subscribers[role] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// Send sends a message to a specific agent
func (b *AgentMessageBus) Send(msg Message) error {
	select {
	case b.mutex <- struct{}{}:
		defer func() { <-b.mutex }()
		b.messages[msg.To] = append(b.messages[msg.To], msg)
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("message bus busy")
	}
}

// Broadcast sends a message to all subscribers of a role
func (b *AgentMessageBus) Broadcast(role string, msg Message) error {
	select {
	case b.mutex <- struct{}{}:
		defer func() { <-b.mutex }()
		subscribers := b.subscribers[role]
		for _, agentID := range subscribers {
			msgCopy := msg
			msgCopy.To = agentID
			b.messages[agentID] = append(b.messages[agentID], msgCopy)
		}
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("message bus busy")
	}
}

// Receive retrieves pending messages for an agent
func (b *AgentMessageBus) Receive(agentID string) []Message {
	select {
	case b.mutex <- struct{}{}:
		defer func() { <-b.mutex }()
		msgs := b.messages[agentID]
		b.messages[agentID] = nil
		return msgs
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// RequestResponse sends a request and waits for response
func (b *AgentMessageBus) RequestResponse(ctx context.Context, from, to string, content string, timeout time.Duration) (string, error) {
	// Send request
	req := Message{
		From:      from,
		To:        to,
		Type:      "request",
		Content:   content,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"correlation_id": generateCorrelationID()},
	}

	if err := b.Send(req); err != nil {
		return "", err
	}

	// Wait for response
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("response timeout")
		case <-ticker.C:
			msgs := b.Receive(from)
			for _, msg := range msgs {
				if msg.From == to && msg.Type == "response" {
					if corrID, ok := msg.Metadata["correlation_id"]; ok {
						if corrID == req.Metadata["correlation_id"] {
							return msg.Content, nil
						}
					}
				}
			}
		}
	}
}

// generateCorrelationID generates a unique correlation ID
func generateCorrelationID() string {
	return fmt.Sprintf("corr-%d", time.Now().UnixNano())
}

// CollaborativeTask represents a task requiring agent collaboration
type CollaborativeTask struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Agents      []string          `json:"agents"` // Required agent roles
	ChatHistory []Message         `json:"chat_history"`
	Result      string            `json:"result,omitempty"`
	Status      string            `json:"status"`
}

// NewCollaborativeTask creates a new collaborative task
func NewCollaborativeTask(title, description string, agents []string) *CollaborativeTask {
	return &CollaborativeTask{
		ID:          fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Title:       title,
		Description: description,
		Agents:      agents,
		ChatHistory: make([]Message, 0),
		Status:      "pending",
	}
}

// AddMessage adds a message to the task chat history
func (t *CollaborativeTask) AddMessage(msg Message) {
	t.ChatHistory = append(t.ChatHistory, msg)
}

// GetLastMessageFromAgent returns the last message from a specific agent
func (t *CollaborativeTask) GetLastMessageFromAgent(agentID string) *Message {
	for i := len(t.ChatHistory) - 1; i >= 0; i-- {
		if t.ChatHistory[i].From == agentID {
			return &t.ChatHistory[i]
		}
	}
	return nil
}

// AgentCollaborationEngine manages collaborative multi-agent sessions
type AgentCollaborationEngine struct {
	bus       *AgentMessageBus
	registry  *InMemoryAgentRegistry
	tasks     map[string]*CollaborativeTask
	maxRounds int
}

// NewAgentCollaborationEngine creates a new collaboration engine
func NewAgentCollaborationEngine(registry *InMemoryAgentRegistry) *AgentCollaborationEngine {
	return &AgentCollaborationEngine{
		bus:       NewAgentMessageBus(),
		registry:  registry,
		tasks:     make(map[string]*CollaborativeTask),
		maxRounds: 10,
	}
}

// StartCollaboration starts a collaborative session
func (e *AgentCollaborationEngine) StartCollaboration(ctx context.Context, task *CollaborativeTask) (string, error) {
	e.tasks[task.ID] = task

	// Spawn agents for each required role
	agentIDs := make(map[string]string) // role -> agentID
	for _, role := range task.Agents {
		instance, err := e.registry.Spawn(role, task.Description, task.ID, SpawnOptions{})
		if err != nil {
			return "", fmt.Errorf("spawn %s: %w", role, err)
		}
		agentIDs[role] = instance.ID
		e.bus.Subscribe(instance.ID, role)
	}

	// Run collaboration rounds
	for round := 0; round < e.maxRounds; round++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Check if any agent has a conclusion
		conclusion := e.checkForConclusion(task)
		if conclusion != "" {
			task.Status = "completed"
			task.Result = conclusion
			return conclusion, nil
		}

		// Allow agents to communicate
		e.facilitateRound(ctx, task, agentIDs)
	}

	task.Status = "timeout"
	return "Collaboration timed out without conclusion", nil
}

// checkForConclusion checks if any agent has reached a conclusion
func (e *AgentCollaborationEngine) checkForConclusion(task *CollaborativeTask) string {
	// Check last messages for conclusion markers
	for _, msg := range task.ChatHistory {
		if msg.Metadata != nil {
			if conclusion, ok := msg.Metadata["conclusion"].(string); ok && conclusion != "" {
				return conclusion
			}
		}
	}
	return ""
}

// facilitateFacilitates one round of agent communication
func (e *AgentCollaborationEngine) facilitateRound(ctx context.Context, task *CollaborativeTask, agentIDs map[string]string) {
	// In a full implementation, this would:
	// 1. Gather each agent's current state
	// 2. Share relevant information between agents
	// 3. Allow agents to request information from each other
	// 4. Track progress toward resolution

	// Simplified version: just check for new messages
	for role, agentID := range agentIDs {
		messages := e.bus.Receive(agentID)
		for _, msg := range messages {
			task.AddMessage(msg)
		}
		_ = role
	}
}
