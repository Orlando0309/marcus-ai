package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Executor runs trigger actions
type Executor struct {
	// Dependencies would be injected here
	// flowEngine  *flow.Engine
	// agentRunner AgentRunner

	// Semaphore for limiting concurrent executions
	sem chan struct{}

	// Track running executions
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewExecutor creates a new executor with the given concurrency limit
func NewExecutor(maxConcurrent int) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}

	return &Executor{
		sem:     make(chan struct{}, maxConcurrent),
		running: make(map[string]context.CancelFunc),
	}
}

// Execute runs a trigger's action
func (e *Executor) Execute(ctx context.Context, trigger *Trigger) error {
	// Acquire semaphore
	select {
	case e.sem <- struct{}{}:
		// Acquired
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-e.sem }()

	// Create a context for this execution
	execCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Track execution
	e.mu.Lock()
	e.running[trigger.ID] = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.running, trigger.ID)
		e.mu.Unlock()
	}()

	// Execute based on action type
	switch trigger.Action.Type {
	case "flow":
		return e.executeFlow(execCtx, trigger)
	case "agent":
		return e.executeAgent(execCtx, trigger)
	case "command":
		return e.executeCommand(execCtx, trigger)
	case "skill":
		return e.executeSkill(execCtx, trigger)
	default:
		return fmt.Errorf("unknown action type: %s", trigger.Action.Type)
	}
}

// executeFlow runs a flow by name
func (e *Executor) executeFlow(ctx context.Context, trigger *Trigger) error {
	// Placeholder: would integrate with flow engine
	// flowName := trigger.Action.Target
	// input := trigger.Action.Input

	// For now, just log
	fmt.Printf("[Trigger %s] Executing flow: %s\n", trigger.ID, trigger.Action.Target)

	// Simulate execution
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// executeAgent runs an agent by name
func (e *Executor) executeAgent(ctx context.Context, trigger *Trigger) error {
	// Placeholder: would integrate with agent runner
	// agentName := trigger.Action.Target

	fmt.Printf("[Trigger %s] Executing agent: %s\n", trigger.ID, trigger.Action.Target)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// executeCommand runs a shell command
func (e *Executor) executeCommand(ctx context.Context, trigger *Trigger) error {
	// command := trigger.Action.Target

	fmt.Printf("[Trigger %s] Executing command: %s\n", trigger.ID, trigger.Action.Target)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// executeSkill runs a skill
func (e *Executor) executeSkill(ctx context.Context, trigger *Trigger) error {
	// skillPattern := trigger.Action.Target
	// args := trigger.Action.Input

	fmt.Printf("[Trigger %s] Executing skill: %s\n", trigger.ID, trigger.Action.Target)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// IsRunning returns true if a trigger is currently executing
func (e *Executor) IsRunning(triggerID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.running[triggerID]
	return ok
}

// RunningCount returns the number of currently running executions
func (e *Executor) RunningCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.running)
}

// Cancel cancels a running execution
func (e *Executor) Cancel(triggerID string) bool {
	e.mu.Lock()
	cancel, ok := e.running[triggerID]
	e.mu.Unlock()

	if ok {
		cancel()
		return true
	}
	return false
}
