package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/memory"
)

// TriggerType represents the type of trigger
type TriggerType string

const (
	TriggerEvent    TriggerType = "event"     // Triggered by an event
	TriggerSchedule TriggerType = "schedule"  // Triggered on schedule (cron)
	TriggerCondition TriggerType = "condition" // Triggered when condition is met
	TriggerWebhook  TriggerType = "webhook"   // Triggered by webhook
)

// Trigger represents an event-to-action mapping
type Trigger struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        TriggerType       `json:"type"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`

	// Event triggers
	Event EventMatcher `json:"event,omitempty"`

	// Schedule triggers
	Schedule ScheduleConfig `json:"schedule,omitempty"`

	// Condition triggers
	Condition ConditionConfig `json:"condition,omitempty"`

	// Webhook triggers
	Webhook WebhookConfig `json:"webhook,omitempty"`

	// Action to execute
	Action TriggerAction `json:"action"`

	// Execution options
	Options ExecutionOptions `json:"options,omitempty"`

	// Statistics
	Executions int       `json:"executions"`
	LastRun    time.Time `json:"last_run,omitempty"`
	LastResult string    `json:"last_result,omitempty"`
}

// EventMatcher matches events
type EventMatcher struct {
	Source   string            `json:"source"` // e.g., "git", "file", "task"
	EventType string           `json:"event_type"` // e.g., "commit", "modify", "create"
	Pattern  string            `json:"pattern,omitempty"` // Regex pattern to match
	Metadata map[string]string `json:"metadata,omitempty"` // Additional metadata filters
}

// ScheduleConfig configures scheduled triggers
type ScheduleConfig struct {
	Cron     string `json:"cron"` // Cron expression
	Timezone string `json:"timezone,omitempty"`
}

// ConditionConfig configures condition-based triggers
type ConditionConfig struct {
	Type     string `json:"type"` // "file_exists", "file_changed", "task_status", "api_response"
	Path     string `json:"path,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Interval time.Duration `json:"interval"` // How often to check
}

// WebhookConfig configures webhook triggers
type WebhookConfig struct {
	Path       string            `json:"path"` // URL path for webhook
	Methods    []string          `json:"methods,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Secret     string            `json:"secret,omitempty"` // For HMAC verification
}

// TriggerAction is the action to execute
type TriggerAction struct {
	Type      string         `json:"type"` // "flow", "skill", "command", "webhook"
	Target    string         `json:"target"` // Flow name, skill name, command, or URL
	Arguments map[string]any `json:"arguments,omitempty"`
	Timeout   time.Duration  `json:"timeout,omitempty"`
}

// ExecutionOptions configures trigger execution
type ExecutionOptions struct {
	Debounce    time.Duration `json:"debounce,omitempty"` // Minimum time between executions
	Throttle    time.Duration `json:"throttle,omitempty"` // Maximum executions per period
	MaxRetries  int           `json:"max_retries,omitempty"`
	RunParallel bool          `json:"run_parallel,omitempty"`
}

// Engine manages triggers and their execution
type Engine struct {
	mu            sync.RWMutex
	triggers      map[string]*Trigger
	folder        *folder.FolderEngine
	config        *config.Config
	memory        *memory.Manager
	flowExecutor  *flow.FlowExecutor
	eventBus      *EventBus
	scheduler     *Scheduler
	dataDir       string
	activeRuns    map[string]time.Time // trigger ID -> start time
}

// EventBus handles event publishing/subscribing
type EventBus struct {
	mu        sync.RWMutex
	subscribers map[string][]func(Event)
}

// Event represents a triggerable event
type Event struct {
	Source    string            `json:"source"`
	EventType string            `json:"event_type"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   any               `json:"payload,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// TriggerResult is the result of trigger execution
type TriggerResult struct {
	TriggerID   string        `json:"trigger_id"`
	Success     bool          `json:"success"`
	Output      string        `json:"output,omitempty"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
}

// NewEngine creates a new trigger engine
func NewEngine(dataDir string, folder *folder.FolderEngine, cfg *config.Config, mem *memory.Manager, flowExec *flow.FlowExecutor) *Engine {
	e := &Engine{
		triggers:     make(map[string]*Trigger),
		folder:       folder,
		config:       cfg,
		memory:       mem,
		flowExecutor: flowExec,
		eventBus:     NewEventBus(),
		scheduler:    NewScheduler(),
		dataDir:      filepath.Join(dataDir, "triggers"),
		activeRuns:   make(map[string]time.Time),
	}
	e.loadTriggers()
	e.startScheduler()
	return e
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]func(Event)),
	}
}

// Publish publishes an event
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Publish to topic subscribers
	key := event.Source + ":" + event.EventType
	for _, handler := range b.subscribers[key] {
		go handler(event)
	}

	// Publish to wildcard subscribers
	for _, handler := range b.subscribers["*"] {
		go handler(event)
	}
}

// Subscribe subscribes to events
func (b *EventBus) Subscribe(source, eventType string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := source + ":" + eventType
	b.subscribers[key] = append(b.subscribers[key], handler)
}

// SubscribeAll subscribes to all events
func (b *EventBus) SubscribeAll(handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers["*"] = append(b.subscribers["*"], handler)
}

// Scheduler handles scheduled triggers
type Scheduler struct {
	mu      sync.RWMutex
	jobs    map[string]*ScheduledJob
	stopCh  chan struct{}
}

// ScheduledJob is a scheduled trigger job
type ScheduledJob struct {
	TriggerID string
	Cron      string
	NextRun   time.Time
	Enabled   bool
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	s := &Scheduler{
		jobs:   make(map[string]*ScheduledJob),
		stopCh: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkJobs()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) checkJobs() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	for _, job := range s.jobs {
		if job.Enabled && job.NextRun.Before(now) {
			// Trigger would fire - in a full implementation, this would execute
			// For now, just update next run
			_ = job
		}
	}
}

// AddJob adds a scheduled job
func (s *Scheduler) AddJob(triggerID, cron string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse cron and calculate next run
	nextRun, err := parseCron(cron)
	if err != nil {
		return err
	}

	s.jobs[triggerID] = &ScheduledJob{
		TriggerID: triggerID,
		Cron:      cron,
		NextRun:   nextRun,
		Enabled:   true,
	}

	return nil
}

// RemoveJob removes a scheduled job
func (s *Scheduler) RemoveJob(triggerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, triggerID)
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// RegisterTrigger registers a trigger
func (e *Engine) RegisterTrigger(trigger Trigger) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if trigger.ID == "" {
		trigger.ID = generateTriggerID()
	}

	trigger.CreatedAt = time.Now()
	trigger.UpdatedAt = time.Now()
	trigger.Enabled = true

	e.triggers[trigger.ID] = &trigger

	// Set up event subscription
	if trigger.Type == TriggerEvent {
		e.eventBus.Subscribe(
			trigger.Event.Source,
			trigger.Event.EventType,
			func(event Event) { e.handleEvent(event, trigger.ID) },
		)
	}

	// Set up schedule
	if trigger.Type == TriggerSchedule {
		if err := e.scheduler.AddJob(trigger.ID, trigger.Schedule.Cron); err != nil {
			return err
		}
	}

	e.saveTriggers()
	return nil
}

// UnregisterTrigger unregisters a trigger
func (e *Engine) UnregisterTrigger(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	trigger, ok := e.triggers[id]
	if !ok {
		return ErrTriggerNotFound
	}

	if trigger.Type == TriggerSchedule {
		e.scheduler.RemoveJob(id)
	}

	delete(e.triggers, id)
	e.saveTriggers()
	return nil
}

// EnableTrigger enables a trigger
func (e *Engine) EnableTrigger(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	trigger, ok := e.triggers[id]
	if !ok {
		return ErrTriggerNotFound
	}

	trigger.Enabled = true
	trigger.UpdatedAt = time.Now()
	e.saveTriggers()
	return nil
}

// DisableTrigger disables a trigger
func (e *Engine) DisableTrigger(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	trigger, ok := e.triggers[id]
	if !ok {
		return ErrTriggerNotFound
	}

	trigger.Enabled = false
	trigger.UpdatedAt = time.Now()
	e.saveTriggers()
	return nil
}

// ListTriggers returns all triggers
func (e *Engine) ListTriggers() []Trigger {
	e.mu.RLock()
	defer e.mu.RUnlock()

	triggers := make([]Trigger, 0, len(e.triggers))
	for _, t := range e.triggers {
		triggers = append(triggers, *t)
	}

	// Sort by name
	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].Name < triggers[j].Name
	})

	return triggers
}

// GetTrigger returns a trigger by ID
func (e *Engine) GetTrigger(id string) (*Trigger, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	trigger, ok := e.triggers[id]
	if !ok {
		return nil, false
	}

	copy := *trigger
	return &copy, true
}

// EmitEvent emits an event that may trigger actions
func (e *Engine) EmitEvent(source, eventType string, payload any, metadata map[string]string) {
	event := Event{
		Source:    source,
		EventType: eventType,
		Timestamp: time.Now(),
		Payload:   payload,
		Metadata:  metadata,
	}

	e.eventBus.Publish(event)
}

func (e *Engine) handleEvent(event Event, triggerID string) {
	e.mu.RLock()
	trigger, ok := e.triggers[triggerID]
	e.mu.RUnlock()

	if !ok || !trigger.Enabled {
		return
	}

	// Check if event matches trigger
	if !e.eventMatches(event, trigger.Event) {
		return
	}

	// Check debounce
	if trigger.Options.Debounce > 0 {
		e.mu.RLock()
		lastRun, hasRun := e.activeRuns[triggerID]
		e.mu.RUnlock()

		if hasRun && time.Since(lastRun) < trigger.Options.Debounce {
			return
		}
	}

	// Execute action
	go e.executeTrigger(trigger)
}

func (e *Engine) eventMatches(event Event, matcher EventMatcher) bool {
	if matcher.Source != "" && event.Source != matcher.Source {
		return false
	}
	if matcher.EventType != "" && event.EventType != matcher.EventType {
		return false
	}
	if matcher.Pattern != "" {
		// Simple pattern matching
		payloadStr := fmt.Sprintf("%v", event.Payload)
		if !strings.Contains(payloadStr, matcher.Pattern) {
			return false
		}
	}
	for key, value := range matcher.Metadata {
		if event.Metadata == nil || event.Metadata[key] != value {
			return false
		}
	}
	return true
}

func (e *Engine) executeTrigger(trigger *Trigger) {
	e.mu.Lock()
	e.activeRuns[trigger.ID] = time.Now()
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.activeRuns, trigger.ID)
		e.mu.Unlock()
	}()

	result := TriggerResult{
		TriggerID: trigger.ID,
		Timestamp: time.Now(),
	}

	start := time.Now()

	switch trigger.Action.Type {
	case "flow":
		output, err := e.executeFlow(trigger.Action)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case "skill":
		output, err := e.executeSkill(trigger.Action)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case "command":
		output, err := e.executeCommand(trigger.Action)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	case "webhook":
		output, err := e.executeWebhook(trigger.Action)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
		}
	default:
		result.Error = fmt.Sprintf("unknown action type: %s", trigger.Action.Type)
	}

	result.Duration = time.Since(start)
	result.Success = result.Error == ""

	// Update trigger stats
	e.mu.Lock()
	trigger.Executions++
	trigger.LastRun = time.Now()
	trigger.LastResult = result.Output
	if result.Error != "" {
		trigger.LastResult = result.Error
	}
	e.mu.Unlock()

	// Record to memory
	e.memory.Remember(
		"project",
		"trigger-execution",
		fmt.Sprintf("Trigger %s executed", trigger.Name),
		fmt.Sprintf("Result: %s, Duration: %v", result.Output, result.Duration),
		"trigger-engine",
		"trigger",
		trigger.ID,
	)
}

func (e *Engine) executeFlow(action TriggerAction) (string, error) {
	if e.flowExecutor == nil {
		return "", fmt.Errorf("flow executor not configured")
	}

	// Execute flow with arguments
	ctx := context.Background()
	if action.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	}

	// In a full implementation, this would call the flow executor
	// For now, return a placeholder
	return fmt.Sprintf("Flow %s executed with args: %v", action.Target, action.Arguments), nil
}

func (e *Engine) executeSkill(action TriggerAction) (string, error) {
	// Execute skill - would integrate with skill system
	return fmt.Sprintf("Skill %s executed", action.Target), nil
}

func (e *Engine) executeCommand(action TriggerAction) (string, error) {
	// Execute command - would use tool system
	return fmt.Sprintf("Command executed: %s", action.Target), nil
}

func (e *Engine) executeWebhook(action TriggerAction) (string, error) {
	// Execute webhook - would make HTTP request
	return fmt.Sprintf("Webhook called: %s", action.Target), nil
}

func (e *Engine) saveTriggers() error {
	if e.dataDir == "" {
		return nil
	}

	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(e.dataDir, "triggers.json")
	data, _ := json.MarshalIndent(e.triggers, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func (e *Engine) loadTriggers() error {
	if e.dataDir == "" {
		return nil
	}

	path := filepath.Join(e.dataDir, "triggers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &e.triggers)
}

func (e *Engine) startScheduler() {
	// Scheduler is started in NewEngine
}

// GetStats returns trigger statistics
func (e *Engine) GetStats() TriggerStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := TriggerStats{
		TotalTriggers: len(e.triggers),
		ActiveRuns:    len(e.activeRuns),
	}

	for _, t := range e.triggers {
		if t.Enabled {
			stats.Enabled++
		} else {
			stats.Disabled++
		}
		stats.TotalExecutions += t.Executions
	}

	return stats
}

// TriggerStats holds trigger statistics
type TriggerStats struct {
	TotalTriggers  int `json:"total_triggers"`
	Enabled        int `json:"enabled"`
	Disabled       int `json:"disabled"`
	TotalExecutions int `json:"total_executions"`
	ActiveRuns     int `json:"active_runs"`
}

// Common event sources
const (
	EventSourceGit      = "git"
	EventSourceFile     = "file"
	EventSourceTask     = "task"
	EventSourceSession  = "session"
)

// Common event types
const (
	EventTypeCommit     = "commit"
	EventTypePush       = "push"
	EventTypePull       = "pull"
	EventTypeFileCreate = "create"
	EventTypeFileModify = "modify"
	EventTypeFileDelete = "delete"
	EventTypeTaskCreate = "create"
	EventTypeTaskComplete = "complete"
)

func generateTriggerID() string {
	return "trigger_" + time.Now().Format("20060102150405")
}

// parseCron parses a cron expression and returns next run time
func parseCron(cron string) (time.Time, error) {
	// Simplified cron parsing - in production use a proper cron library
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression: %s", cron)
	}

	// For now, just return next minute
	return time.Now().Add(1 * time.Minute), nil
}

// Errors
var (
	ErrTriggerNotFound = fmt.Errorf("trigger not found")
)
