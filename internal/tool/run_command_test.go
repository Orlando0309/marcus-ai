package tool

import "testing"

func TestSplitCommandLinePreservesQuotedGitCommitMessage(t *testing.T) {
	args, err := splitCommandLine(`git commit -m "Update project state and session files"`)
	if err != nil {
		t.Fatalf("splitCommandLine returned error: %v", err)
	}
	want := []string{"git", "commit", "-m", "Update project state and session files"}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d: %#v", len(want), len(args), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg %d: want %q, got %q", i, want[i], args[i])
		}
	}
}

func TestNormalizeCommandForShellUnwrapsWrappedWindowsCommand(t *testing.T) {
	got := normalizeCommandForShell(`"git commit -m \"Update project state and session files\""`)
	want := `git commit -m "Update project state and session files"`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
