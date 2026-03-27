package tui

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

func TestExtractUnifiedDiffSnippet(t *testing.T) {
	s := `{"message":"x"} prefix text
@@ -1,3 +1,3 @@
-a
+b
`
	got := extractUnifiedDiffSnippet(s)
	if got == "" || got[:4] != "@@ -" {
		t.Fatalf("unexpected snippet: %q", got)
	}
}

func TestBuildDiffPaneSidePreview(t *testing.T) {
	cfg := config.DefaultConfig()
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.width = 120
	m.height = 40
	m.layout()
	// Diff is now shown inline in the transcript
}

func TestTUIProgramSmoke(t *testing.T) {
	cfg := config.DefaultConfig()
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	p := tea.NewProgram(m,
		tea.WithOutput(io.Discard),
		tea.WithInput(bytes.NewReader(nil)),
		tea.WithoutSignalHandler(),
		tea.WithMouseCellMotion(),
	)
	go func() {
		_, err := p.Run()
		errs <- err
	}()
	time.Sleep(40 * time.Millisecond)
	p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	p.Send(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	p.Quit()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for program")
	}
}

func TestFilterCompletionNoopActions(t *testing.T) {
	actions := []tool.ActionProposal{
		{Type: "run_command", Command: "echo Task done"},
		{Type: "read_file", Path: "main.go"},
	}
	filtered := filterCompletionNoopActions(actions, task.StatusDone)
	if len(filtered) != 1 {
		t.Fatalf("expected one action after filtering, got %d", len(filtered))
	}
	if filtered[0].Type != "read_file" {
		t.Fatalf("expected read_file to remain, got %s", filtered[0].Type)
	}
}
