package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTodoWriteToolPersistsTasks(t *testing.T) {
	dir := t.TempDir()
	tool := NewTodoWriteTool(dir)
	payload, err := json.Marshal(map[string]any{
		"todos": []map[string]any{
			{"id": "one", "content": "First task", "status": "active"},
			{"id": "two", "content": "Second task", "status": "queue", "depends_on": []string{"one"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Run(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".marcus", "tasks", "active", "one.json")); err != nil {
		t.Fatalf("expected active task file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".marcus", "tasks", "workflow.json")); err != nil {
		t.Fatalf("expected workflow file: %v", err)
	}
}

func TestTodoWriteAutoGeneratesID(t *testing.T) {
	dir := t.TempDir()
	tool := NewTodoWriteTool(dir)
	payload, err := json.Marshal(map[string]any{
		"todos": []map[string]any{
			{"content": "Task without ID", "status": "queue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Run(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}

	// Check result contains the task with generated ID
	var response map[string]any
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}

	count, ok := response["count"].(float64)
	if !ok || count != 1 {
		t.Fatalf("expected count=1, got %v", response["count"])
	}

	tasks, ok := response["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %v", response["tasks"])
	}

	// Verify the task file was created with auto-generated ID
	queueDir := filepath.Join(dir, ".marcus", "tasks", "queue")
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		t.Fatalf("expected queue directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 task file, got %d", len(entries))
	}
}
