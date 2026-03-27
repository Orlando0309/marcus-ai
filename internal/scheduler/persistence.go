package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store persists triggers to disk
type Store struct {
	root string // e.g., ~/.marcus/triggers/
}

// NewStore creates a new trigger store
func NewStore(root string) *Store {
	return &Store{root: root}
}

// DefaultStorePath returns the default path for trigger storage
func DefaultStorePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".marcus", "triggers")
}

// EnsureStructure creates the storage directory if it doesn't exist
func (s *Store) EnsureStructure() error {
	if s.root == "" {
		return fmt.Errorf("store root not set")
	}
	return os.MkdirAll(s.root, 0755)
}

// List returns all stored triggers
func (s *Store) List() ([]Trigger, error) {
	if s.root == "" {
		return nil, fmt.Errorf("store root not set")
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Trigger{}, nil
		}
		return nil, fmt.Errorf("read triggers dir: %w", err)
	}

	var triggers []Trigger
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}

		// Parse JSON files
		data, err := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if err != nil {
			continue
		}

		var trigger Trigger
		if err := json.Unmarshal(data, &trigger); err != nil {
			continue
		}

		triggers = append(triggers, trigger)
	}

	// Sort by created time (newest first)
	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].CreatedAt.After(triggers[j].CreatedAt)
	})

	return triggers, nil
}

// Get retrieves a specific trigger by ID
func (s *Store) Get(id string) (*Trigger, error) {
	if s.root == "" {
		return nil, fmt.Errorf("store root not set")
	}

	path := filepath.Join(s.root, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("trigger not found: %s", id)
		}
		return nil, fmt.Errorf("read trigger: %w", err)
	}

	var trigger Trigger
	if err := json.Unmarshal(data, &trigger); err != nil {
		return nil, fmt.Errorf("parse trigger: %w", err)
	}

	return &trigger, nil
}

// Save persists a trigger to disk
func (s *Store) Save(trigger *Trigger) error {
	if s.root == "" {
		return fmt.Errorf("store root not set")
	}

	// Set creation time if not set
	if trigger.CreatedAt.IsZero() {
		trigger.CreatedAt = time.Now()
	}

	// Generate ID if not set
	if trigger.ID == "" {
		trigger.ID = generateTriggerID()
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(trigger, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trigger: %w", err)
	}

	// Write to file
	path := filepath.Join(s.root, trigger.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write trigger: %w", err)
	}

	return nil
}

// Delete removes a trigger by ID
func (s *Store) Delete(id string) error {
	if s.root == "" {
		return fmt.Errorf("store root not set")
	}

	path := filepath.Join(s.root, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("trigger not found: %s", id)
		}
		return fmt.Errorf("delete trigger: %w", err)
	}

	return nil
}

// UpdateRun updates the trigger after a run
func (s *Store) UpdateRun(id string, success bool, errMsg string) error {
	trigger, err := s.Get(id)
	if err != nil {
		return err
	}

	trigger.LastRun = time.Now()
	trigger.RunCount++

	if success {
		trigger.LastError = ""
	} else {
		trigger.ErrorCount++
		trigger.LastError = errMsg
	}

	return s.Save(trigger)
}

var triggerIDCounter int64

func generateTriggerID() string {
	triggerIDCounter++
	return fmt.Sprintf("trig_%d_%d", time.Now().Unix(), triggerIDCounter)
}
