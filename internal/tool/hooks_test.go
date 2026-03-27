package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcus-ai/marcus/internal/config"
)

func TestToolHooksRunForTools(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")
	tr := NewToolRunner()
	tr.baseDir = dir
	tr.hooks = config.HooksConfig{
		PreToolUse:  []config.HookRule{{Matcher: "read_file", Commands: []string{"echo pre>> hooks.log"}}},
		PostToolUse: []config.HookRule{{Matcher: "read_file", Commands: []string{"echo post>> hooks.log"}}},
	}
	tr.Register(NewReadFileTool(dir))
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"path": "a.txt"})
	if _, err := tr.Run(context.Background(), "read_file", payload); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "pre") || !strings.Contains(got, "post") {
		t.Fatalf("expected pre and post hooks, got %q", got)
	}
}
