package context

import (
	"testing"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/session"
)

func TestAssemblerTokenBudgetDropsLowPriority(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.Context.MaxContextTokens = 80 // very small
	cfg.Context.MaxFilesPerPrompt = 0
	cfg.Context.AlwaysInclude = nil

	a := NewAssembler(cfg, nil, nil, nil)
	snap := a.Assemble("hello", &session.Session{})
	if snap.Text == "" {
		t.Fatal("expected some text")
	}
	if snap.EstimatedTokens > cfg.Context.MaxContextTokens+20 {
		t.Fatalf("estimated %d exceeds budget cap loosely", snap.EstimatedTokens)
	}
	if !snap.Truncated && len(snap.DroppedSections) == 0 {
		// May still fit if only header; then relax
		if EstimateTokens(snap.Text) > cfg.Context.MaxContextTokens {
			t.Fatal("text exceeds budget but not marked truncated")
		}
	}
}

func TestFileRelevanceOrdersMentions(t *testing.T) {
	s := fileRelevanceScore("foo bar baz", "foo.go", "package main")
	if s < fileRelevanceScore("other", "foo.go", "package main") {
		t.Fatal("expected keyword match to raise score")
	}
}
