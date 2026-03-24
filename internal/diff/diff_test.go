package diff

import (
	"strings"
	"testing"
)

func TestParseAndApplySingleHunk(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	diffText := `--- a/x
+++ b/x
@@ -1,3 +1,3 @@
 alpha
-beta
+DELTA
 gamma
`
	patches, err := ParseUnifiedDiff(diffText)
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(patches))
	}
	got, err := ApplyPatch(original, patches)
	if err != nil {
		t.Fatal(err)
	}
	want := "alpha\nDELTA\ngamma\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTwoHunksForwardOrder(t *testing.T) {
	original := "A\nb\nc\nd\n"
	diffText := `@@ -1,1 +1,1 @@
-A
+a
@@ -3,1 +3,1 @@
-c
+C
`
	patches, err := ParseUnifiedDiff(diffText)
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(patches))
	}
	got, err := ApplyPatch(original, patches)
	if err != nil {
		t.Fatal(err)
	}
	want := "a\nb\nC\nd\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInsertAtBeginning(t *testing.T) {
	original := "mid\nend\n"
	diffText := `@@ -0,0 +1,2 @@
+start
+
`
	patches, err := ParseUnifiedDiff(diffText)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(original, patches)
	if err != nil {
		t.Fatal(err)
	}
	want := "start\n\nmid\nend\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestGenerateDiffRoundTrip(t *testing.T) {
	old := "one\ntwo\n"
	newContent := "one\nTWO\nthree\n"
	d, err := GenerateDiff(old, newContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "@@ -1,2 +1,3 @@") {
		t.Fatalf("expected unified header in %q", d)
	}
	patches, err := ParseUnifiedDiff(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(old, patches)
	if err != nil {
		t.Fatal(err)
	}
	if got != newContent {
		t.Fatalf("got %q want %q", got, newContent)
	}
}

func TestGenerateDiffEmptyToContent(t *testing.T) {
	d, err := GenerateDiff("", "hello\n")
	if err != nil {
		t.Fatal(err)
	}
	patches, err := ParseUnifiedDiff(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch("", patches)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello\n" {
		t.Fatalf("got %q", got)
	}
}

func TestContextMismatchError(t *testing.T) {
	original := "x\ny\n"
	diffText := `@@ -1,2 +1,2 @@
-a
-b
+a
+b
`
	patches, err := ParseUnifiedDiff(diffText)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyPatch(original, patches)
	if err == nil || !strings.Contains(err.Error(), "context mismatch") {
		t.Fatalf("expected context mismatch, got %v", err)
	}
}

func TestIdenticalGenerateDiff(t *testing.T) {
	d, err := GenerateDiff("same", "same")
	if err != nil {
		t.Fatal(err)
	}
	if d != "" {
		t.Fatalf("expected empty diff, got %q", d)
	}
}
