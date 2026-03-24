package integration

import (
	"testing"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/tool"
)

func TestBuildToolRunner(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	r, err := tool.BuildRunner(tool.BuildOptions{
		BaseDir: cfg.ProjectRoot,
		Config:  cfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.List()) < 5 {
		t.Fatalf("expected several tools, got %v", r.List())
	}
}
