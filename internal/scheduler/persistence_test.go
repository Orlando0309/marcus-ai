package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Test EnsureStructure
	if err := store.EnsureStructure(); err != nil {
		t.Fatalf("EnsureStructure failed: %v", err)
	}

	// Create a trigger
	trigger := &Trigger{
		Name:      "Test Trigger",
		Type:      TriggerCron,
		Enabled:   true,
		CreatedAt: time.Now(),
		Config: TriggerConfig{
			CronExpression: "0 9 * * *",
		},
		Action: ActionConfig{
			Type:   "flow",
			Target: "daily_report",
		},
	}

	// Test Save
	if err := store.Save(trigger); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if trigger.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify file exists
	path := filepath.Join(tmpDir, trigger.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("trigger file not created: %v", err)
	}

	// Test Get
	loaded, err := store.Get(trigger.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if loaded.Name != trigger.Name {
		t.Errorf("expected name %q, got %q", trigger.Name, loaded.Name)
	}

	if loaded.Config.CronExpression != trigger.Config.CronExpression {
		t.Errorf("expected cron %q, got %q", trigger.Config.CronExpression, loaded.Config.CronExpression)
	}

	// Test List
	triggers, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(triggers) != 1 {
		t.Errorf("expected 1 trigger, got %d", len(triggers))
	}

	// Test UpdateRun
	if err := store.UpdateRun(trigger.ID, true, ""); err != nil {
		t.Fatalf("UpdateRun failed: %v", err)
	}

	loaded, _ = store.Get(trigger.ID)
	if loaded.RunCount != 1 {
		t.Errorf("expected RunCount 1, got %d", loaded.RunCount)
	}
	if loaded.LastRun.IsZero() {
		t.Errorf("expected LastRun to be set")
	}

	// Test UpdateRun with error
	if err := store.UpdateRun(trigger.ID, false, "something went wrong"); err != nil {
		t.Fatalf("UpdateRun with error failed: %v", err)
	}

	loaded, _ = store.Get(trigger.ID)
	if loaded.RunCount != 2 {
		t.Errorf("expected RunCount 2, got %d", loaded.RunCount)
	}
	if loaded.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", loaded.ErrorCount)
	}
	if loaded.LastError != "something went wrong" {
		t.Errorf("expected LastError %q, got %q", "something went wrong", loaded.LastError)
	}

	// Test Delete
	if err := store.Delete(trigger.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}

	// Test Get after delete
	_, err = store.Get(trigger.ID)
	if err == nil {
		t.Error("expected error for deleted trigger")
	}
}

func TestDefaultStorePath(t *testing.T) {
	path := DefaultStorePath()
	if path == "" {
		t.Error("DefaultStorePath returned empty string")
	}

	// Should contain .marcus/triggers
	if !containsStr(path, ".marcus") {
		t.Error("expected path to contain .marcus")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
