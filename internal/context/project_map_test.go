package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectMapManagerBuildsFromRepoScan(t *testing.T) {
	root := t.TempDir()
	mustWriteProjectMapFixture(t, root, ".marcus/marcus.toml", "project = true\n")
	mustWriteProjectMapFixture(t, root, "go.mod", "module example.com/test\n")
	mustWriteProjectMapFixture(t, root, "cmd/marcus/main.go", "package main\nfunc main() {}\n")
	mustWriteProjectMapFixture(t, root, "internal/tui/tui.go", "package tui\n")

	manager := NewProjectMapManager(root)
	text := manager.Ensure()

	for _, want := range []string{
		"# Project Map",
		"## Stack",
		"Go module (`go.mod`)",
		"`cmd/` -> executable entrypoints: marcus",
		"`go.mod` -> Go module definition",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected project map to contain %q\n\n%s", want, text)
		}
	}

	data, err := os.ReadFile(filepath.Join(root, ".marcus", "context", "PROJECT_MAP.md"))
	if err != nil {
		t.Fatalf("expected persisted project map: %v", err)
	}
	if string(data) != text {
		t.Fatal("persisted project map does not match generated text")
	}
}

func TestProjectMapManagerRebuildsPlaceholderWithoutState(t *testing.T) {
	root := t.TempDir()
	mustWriteProjectMapFixture(t, root, ".marcus/marcus.toml", "project = true\n")
	mustWriteProjectMapFixture(t, root, "go.mod", "module example.com/test\n")
	mustWriteProjectMapFixture(t, root, ".marcus/context/PROJECT_MAP.md", "# Placeholder\n")

	manager := NewProjectMapManager(root)
	text := manager.Ensure()

	if strings.Contains(text, "# Placeholder") {
		t.Fatalf("expected placeholder content to be replaced\n\n%s", text)
	}
	if !strings.Contains(text, "# Project Map") {
		t.Fatalf("expected generated project map header\n\n%s", text)
	}
}

func mustWriteProjectMapFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
