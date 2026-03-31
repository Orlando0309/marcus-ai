package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTime_TrackAndAssert(t *testing.T) {
	// Create temp file
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track the file
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// Assert should pass (no changes)
	if err := Assert(path); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Modify the file
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	if err := os.WriteFile(path, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Assert should fail (file changed)
	if err := Assert(path); err == nil {
		t.Fatal("Expected error for modified file, got nil")
	}
}

func TestFileTime_WithLock(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track first
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// WithLock should work
	err := WithLock(path, func(content []byte) ([]byte, error) {
		return append(content, []byte(" world")...), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify content changed
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Fatalf("Expected 'hello world', got %q", string(content))
	}
}

func TestFileTime_WithLock_ExternallyModified(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track first
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// Modify file BEFORE WithLock (simulating external modification after track)
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}

	// WithLock should detect the modification and fail
	err := WithLock(path, func(content []byte) ([]byte, error) {
		return append(content, []byte(" world")...), nil
	})
	if err == nil {
		t.Fatal("Expected error for externally modified file, got nil")
	}
}

func TestFileTime_Forget(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track the file
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// Forget tracking
	Forget(path)

	// Modify the file
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Assert should pass (tracking was forgotten)
	if err := Assert(path); err != nil {
		t.Fatalf("Expected no error after Forget, got %v", err)
	}
}

func TestFileTime_NotTracked(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Assert on untracked file should pass (no tracking info)
	if err := Assert(path); err != nil {
		t.Fatalf("Expected no error for untracked file, got %v", err)
	}
}

func TestFileTime_FileDeleted(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track the file
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// Delete the file
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Assert should pass (file doesn't exist, no overwrite risk)
	if err := Assert(path); err != nil {
		t.Fatalf("Expected no error for deleted file, got %v", err)
	}
}

func TestFileTime_ResolvePath(t *testing.T) {
	// Test relative path resolution
	abs, err := ResolvePath("test.txt", "/home/user")
	if err != nil {
		t.Fatal(err)
	}
	// filepath.Abs uses OS-specific separators, so just check it's absolute
	if !filepath.IsAbs(abs) {
		t.Fatalf("Expected absolute path, got %q", abs)
	}

	// Absolute path should be returned as-is
	// Use current OS native absolute path format
	cwd, _ := os.Getwd()
	input := filepath.Join(cwd, "absolute", "path", "test.txt")
	abs2, err := ResolvePath(input, "/home/user")
	if err != nil {
		t.Fatal(err)
	}
	if abs2 != input {
		t.Fatalf("Expected absolute path unchanged, got %q", abs2)
	}
}

func TestFileTime_Reset(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Track the file
	if err := Track(path); err != nil {
		t.Fatal(err)
	}

	// Verify tracking
	if _, ok := GetMtime(path); !ok {
		t.Fatal("Expected file to be tracked")
	}

	// Reset
	Reset()

	// Verify tracking cleared
	if _, ok := GetMtime(path); ok {
		t.Fatal("Expected tracking to be cleared after Reset")
	}
}
