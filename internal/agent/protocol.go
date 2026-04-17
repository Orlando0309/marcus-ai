package agent

import (
	"sync"
	"time"
)

// MessageType represents the type of inter-agent message
type MessageType string

const (
	MsgRequest       MessageType = "request"
	MsgResponse      MessageType = "response"
	MsgBroadcast     MessageType = "broadcast"
	MsgStatus        MessageType = "status"
	MsgHandoff       MessageType = "handoff"       // Pass work to another agent
	MsgCollaboration MessageType = "collaboration" // Request joint work
)

// Priority represents message priority
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
	PriorityUrgent Priority = 3
)

// Message is a structured inter-agent communication
type Message struct {
	ID          string            `json:"id"`
	From        string            `json:"from"`         // Sender agent ID
	To          string            `json:"to"`           // Recipient agent ID (empty for broadcast)
	Type        MessageType       `json:"type"`
	Priority    Priority          `json:"priority"`
	Timestamp   time.Time         `json:"timestamp"`
	Subject     string            `json:"subject"`      // Brief subject line
	Content     string            `json:"content"`      // Main message content
	Context     map[string]any    `json:"context"`      // Shared context data
	RequiresAck bool              `json:"requires_ack"` // Whether sender expects acknowledgment
	InReplyTo   string            `json:"in_reply_to"`  // ID of message being replied to
	Attachments []Attachment      `json:"attachments,omitempty"`
}

// Attachment is a file or data attachment to a message
type Attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // mime type or content type
	Data    []byte `json:"data,omitempty"`
	Content string `json:"content,omitempty"` // For text attachments
}

// MessageHandler handles incoming messages
type MessageHandler func(Message) error

// Protocol manages inter-agent communication
type Protocol struct {
	mu            sync.RWMutex
	agents        map[string]*AgentState
	messageQueues map[string][]Message
	handlers      map[string][]MessageHandler
	broadcastSubs []MessageHandler
	maxQueueSize  int
}

// AgentState tracks an agent's communication state
type AgentState struct {
	ID            string        `json:"id"`
	Role          string        `json:"role"`
	Status        string        `json:"status"` // active, busy, paused, offline
	LastSeen      time.Time     `json:"last_seen"`
	Capabilities  []string      `json:"capabilities"`
	CurrentGoal   string        `json:"current_goal,omitempty"`
	MessageCount  int           `json:"message_count"`
}

// NewProtocol creates a new agent communication protocol
func NewProtocol(maxQueueSize int) *Protocol {
	return &Protocol{
		agents:        make(map[string]*AgentState),
		messageQueues: make(map[string][]Message),
		handlers:      make(map[string][]MessageHandler),
		broadcastSubs: make([]MessageHandler, 0),
		maxQueueSize:  maxQueueSize,
	}
}

// RegisterAgent registers an agent with the protocol
func (p *Protocol) RegisterAgent(id, role string, capabilities []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.agents[id] = &AgentState{
		ID:           id,
		Role:         role,
		Status:       "active",
		LastSeen:     time.Now(),
		Capabilities: capabilities,
	}
}

// UnregisterAgent removes an agent from the protocol
func (p *Protocol) UnregisterAgent(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.agents, id)
	delete(p.messageQueues, id)
}

// Send sends a message to another agent
func (p *Protocol) Send(msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Validate sender
	if _, ok := p.agents[msg.From]; !ok {
		return ErrAgentNotRegistered
	}

	// Set metadata
	msg.ID = generateMessageID()
	msg.Timestamp = time.Now()

	// Handle broadcast
	if msg.Type == MsgBroadcast {
		p.handleBroadcast(msg)
		return nil
	}

	// Validate recipient
	if _, ok := p.agents[msg.To]; !ok {
		return ErrAgentNotFound
	}

	// Add to recipient's queue
	queue := p.messageQueues[msg.To]
	if len(queue) >= p.maxQueueSize {
		// Drop oldest message
		queue = queue[1:]
	}
	queue = append(queue, msg)
	p.messageQueues[msg.To] = queue

	// Update sender stats
	p.agents[msg.From].MessageCount++
	p.agents[msg.From].LastSeen = time.Now()

	// Invoke handlers
	p.invokeHandlers(msg)

	return nil
}

// Receive retrieves pending messages for an agent
func (p *Protocol) Receive(agentID string, limit int) []Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	queue := p.messageQueues[agentID]
	if len(queue) == 0 {
		return nil
	}

	if limit <= 0 || limit > len(queue) {
		limit = len(queue)
	}

	messages := make([]Message, limit)
	copy(messages, queue[:limit])

	// Remove delivered messages
	p.messageQueues[agentID] = queue[limit:]

	return messages
}

// Subscribe registers a handler for messages matching a pattern
func (p *Protocol) Subscribe(agentID string, handler MessageHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.handlers[agentID] = append(p.handlers[agentID], handler)
}

// SubscribeBroadcast registers a handler for broadcast messages
func (p *Protocol) SubscribeBroadcast(handler MessageHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.broadcastSubs = append(p.broadcastSubs, handler)
}

// GetAgent returns an agent's state
func (p *Protocol) GetAgent(agentID string) (*AgentState, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	agent, ok := p.agents[agentID]
	if !ok {
		return nil, false
	}

	// Return copy
	copy := *agent
	return &copy, true
}

// ListAgents returns all registered agents
func (p *Protocol) ListAgents() []AgentState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	agents := make([]AgentState, 0, len(p.agents))
	for _, a := range p.agents {
		agents = append(agents, *a)
	}
	return agents
}

// UpdateAgentStatus updates an agent's status
func (p *Protocol) UpdateAgentStatus(agentID, status string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	agent, ok := p.agents[agentID]
	if !ok {
		return ErrAgentNotFound
	}

	agent.Status = status
	agent.LastSeen = time.Now()
	return nil
}

// FindAgentsByCapability finds agents with specific capabilities
func (p *Protocol) FindAgentsByCapability(capability string) []AgentState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var matching []AgentState
	for _, a := range p.agents {
		for _, c := range a.Capabilities {
			if c == capability {
				matching = append(matching, *a)
				break
			}
		}
	}
	return matching
}

// GetPendingMessageCount returns the number of pending messages for an agent
func (p *Protocol) GetPendingMessageCount(agentID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.messageQueues[agentID])
}

// ClearQueue clears an agent's message queue
func (p *Protocol) ClearQueue(agentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageQueues[agentID] = nil
}

func (p *Protocol) handleBroadcast(msg Message) {
	for _, handler := range p.broadcastSubs {
		go handler(msg)
	}
	for agentID, handlers := range p.handlers {
		if agentID != msg.From {
			for _, handler := range handlers {
				go handler(msg)
			}
		}
	}
}

func (p *Protocol) invokeHandlers(msg Message) {
	handlers := p.handlers[msg.To]
	for _, handler := range handlers {
		go handler(msg)
	}
}

// Errors
var (
	ErrAgentNotRegistered = &ProtocolError{"agent not registered"}
	ErrAgentNotFound      = &ProtocolError{"agent not found"}
	ErrQueueFull          = &ProtocolError{"message queue full"}
)

// ProtocolError represents a protocol error
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string {
	return e.Message
}

func generateMessageID() string {
	return time.Now().Format("msg_20060102150405.000000")
}
