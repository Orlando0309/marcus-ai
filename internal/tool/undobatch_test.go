package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyProposalsInTransactionRollback(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p1, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := NewToolRunner()
	tr.baseDir = dir
	tr.Register(NewWriteFileTool(dir))
	tr.Register(NewListFilesTool(dir))

	proposals := []ActionProposal{
		{Type: "write_file", Path: "a.txt", Content: "changed"},
		{Type: "not_a_supported_action_type"},
	}
	_, _, err := tr.ApplyProposalsInTransaction(context.Background(), proposals)
	if err == nil {
		t.Fatal("expected error")
	}
	b, _ := os.ReadFile(p1)
	if string(b) != "orig" {
		t.Fatalf("rollback failed: %q", string(b))
	}
}
