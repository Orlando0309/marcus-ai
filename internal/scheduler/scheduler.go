package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Scheduler manages and executes triggers
type Scheduler struct {
	store     *Store
	executor  *Executor
	triggers  map[string]*Trigger
	ticker    *time.Ticker
	stopCh    chan struct{}
	mu        sync.RWMutex
	wg        sync.WaitGroup
}

// NewScheduler creates a new scheduler
func NewScheduler(store *Store, executor *Executor) *Scheduler {
	return &Scheduler{
		store:    store,
		executor: executor,
		triggers: make(map[string]*Trigger),
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler loop
func (s *Scheduler) Start(ctx context.Context) error {
	// Load existing triggers
	if err := s.loadTriggers(); err != nil {
		return fmt.Errorf("load triggers: %w", err)
	}

	// Start ticker for checking triggers (every minute)
	s.ticker = time.NewTicker(1 * time.Minute)

	s.wg.Add(1)
	go s.run(ctx)

	return nil
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
	s.wg.Wait()
}

// run is the main scheduler loop
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	// Check triggers immediately on start
	s.checkTriggers(ctx)

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-s.ticker.C:
			s.checkTriggers(ctx)
		}
	}
}

// checkTriggers evaluates all triggers and executes due ones
func (s *Scheduler) checkTriggers(ctx context.Context) {
	now := time.Now()

	s.mu.RLock()
	triggers := make([]*Trigger, 0, len(s.triggers))
	for _, t := range s.triggers {
		triggers = append(triggers, t)
	}
	s.mu.RUnlock()

	for _, trigger := range triggers {
		if trigger.ShouldRun(now) {
			s.executeTrigger(ctx, trigger)
		}
	}
}

// executeTrigger runs a trigger and updates its state
func (s *Scheduler) executeTrigger(ctx context.Context, trigger *Trigger) {
	// Skip if already running
	if s.executor.IsRunning(trigger.ID) {
		return
	}

	// Execute in background
	s.wg.Add(1)
	go func(t *Trigger) {
		defer s.wg.Done()

		err := s.executor.Execute(ctx, t)

		// Update next run time for cron triggers
		if t.Type == TriggerCron && t.Config.CronExpression != "" {
			if schedule, err := ParseCron(t.Config.CronExpression); err == nil {
				next := schedule.Next(time.Now())
				t.NextRun = &next
			}
		}

		// Update run status
		if err != nil {
			_ = s.store.UpdateRun(t.ID, false, err.Error())
		} else {
			_ = s.store.UpdateRun(t.ID, true, "")
		}
	}(trigger)
}

// loadTriggers loads triggers from the store
func (s *Scheduler) loadTriggers() error {
	triggers, err := s.store.List()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range triggers {
		s.triggers[triggers[i].ID] = &triggers[i]
	}

	return nil
}

// Register adds a new trigger to the scheduler
func (s *Scheduler) Register(trigger *Trigger) error {
	// Calculate next run for cron triggers
	if trigger.Type == TriggerCron && trigger.Config.CronExpression != "" {
		schedule, err := ParseCron(trigger.Config.CronExpression)
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		next := schedule.Next(time.Now())
		trigger.NextRun = &next
	}

	// Save to store
	if err := s.store.Save(trigger); err != nil {
		return fmt.Errorf("save trigger: %w", err)
	}

	// Add to in-memory map
	s.mu.Lock()
	s.triggers[trigger.ID] = trigger
	s.mu.Unlock()

	return nil
}

// Unregister removes a trigger
func (s *Scheduler) Unregister(id string) error {
	s.mu.Lock()
	delete(s.triggers, id)
	s.mu.Unlock()

	return s.store.Delete(id)
}

// Get returns a trigger by ID
func (s *Scheduler) Get(id string) (*Trigger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	trigger, ok := s.triggers[id]
	return trigger, ok
}

// List returns all triggers
func (s *Scheduler) List() []Trigger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	triggers := make([]Trigger, 0, len(s.triggers))
	for _, t := range s.triggers {
		triggers = append(triggers, *t)
	}

	return triggers
}

// Enable enables a trigger
func (s *Scheduler) Enable(id string) error {
	trigger, ok := s.Get(id)
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	trigger.Enabled = true

	// Recalculate next run for cron triggers
	if trigger.Type == TriggerCron && trigger.Config.CronExpression != "" {
		if schedule, err := ParseCron(trigger.Config.CronExpression); err == nil {
			next := schedule.Next(time.Now())
			trigger.NextRun = &next
		}
	}

	return s.store.Save(trigger)
}

// Disable disables a trigger
func (s *Scheduler) Disable(id string) error {
	trigger, ok := s.Get(id)
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	trigger.Enabled = false
	trigger.NextRun = nil

	return s.store.Save(trigger)
}

// TriggerNow manually triggers a trigger
func (s *Scheduler) TriggerNow(ctx context.Context, id string) error {
	trigger, ok := s.Get(id)
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	s.executeTrigger(ctx, trigger)
	return nil
}

// Stats returns scheduler statistics
func (s *Scheduler) Stats() map[string]interface{} {
	s.mu.RLock()
	triggerCount := len(s.triggers)
	s.mu.RUnlock()

	return map[string]interface{}{
		"triggers":       triggerCount,
		"running":        s.executor.RunningCount(),
		"max_concurrent": cap(s.executor.sem),
	}
}
