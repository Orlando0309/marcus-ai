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
