package safety

import (
	"path/filepath"
	"testing"
)

func TestCheckToolRelativePath(t *testing.T) {
	if err := CheckToolRelativePath("internal/foo.go"); err != nil {
		t.Fatal(err)
	}
	if err := CheckToolRelativePath("../etc/passwd"); err == nil {
		t.Fatal("expected error for ..")
	}
	abs := filepath.Join(t.TempDir(), "absfile.txt")
	if err := CheckToolRelativePath(abs); err == nil {
		t.Fatal("expected error for absolute")
	}
	if err := CheckToolRelativePath("ok\x00bad"); err == nil {
		t.Fatal("expected error for NUL")
	}
}

func TestValidateRunCommand(t *testing.T) {
	if err := ValidateRunCommand("go build ./..."); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRunCommand(""); err == nil {
		t.Fatal("expected empty error")
	}
	if err := ValidateRunCommand("echo hi\nrm -rf /"); err == nil {
		t.Fatal("expected multiline rejection")
	}
}
