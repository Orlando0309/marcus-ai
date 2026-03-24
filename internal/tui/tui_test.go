package tui

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/marcus-ai/marcus/internal/config"
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
	m.sideDiffTitle = "Test"
	m.sideDiffLive = "@@ -1 +1 @@\n-old\n+new\n"
	m.diffViewport.SetContent(m.buildDiffPaneContent())
	if m.diffViewport.TotalLineCount() == 0 {
		t.Fatal("expected diff viewport content")
	}
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
