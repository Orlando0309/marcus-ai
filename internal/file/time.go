package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileTime tracks file modification times to prevent stale edits.
// It ensures that files are not overwritten if they've been modified
// since the agent last read them.
type FileTime struct {
	mu       sync.RWMutex
	mtimes   map[string]time.Time // path -> mtime when last read
	lockDir  string
}

// global is the singleton FileTime instance
var global = &FileTime{
	mtimes: make(map[string]time.Time),
}

// Global returns the global FileTime instance
func Global() *FileTime {
	return global
}

// Init initializes the FileTime tracker with a lock directory
func Init(lockDir string) error {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.lockDir = lockDir
	return os.MkdirAll(lockDir, 0755)
}

// Track records the current mtime of a file after reading it.
// This should be called after any file read operation.
func Track(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	global.mu.Lock()
	defer global.mu.Unlock()

	global.mtimes[path] = info.ModTime()
	return nil
}

// Assert checks that a file hasn't been modified since it was last tracked.
// Returns an error if the file has been modified externally.
func Assert(path string) error {
	global.mu.RLock()
	expected, ok := global.mtimes[path]
	global.mu.RUnlock()

	if !ok {
		// No tracking info - file wasn't read by us, allow write
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted - that's fine, we're not overwriting anything
			return nil
		}
		return err
	}

	if info.ModTime().After(expected) {
		return fmt.Errorf("file %q was modified externally (expected mtime %v, got %v)",
			path, expected, info.ModTime())
	}

	return nil
}

// WithLock performs an atomic read-modify-write operation.
// It asserts the file hasn't changed, runs the operation, and updates the tracking.
func WithLock(path string, op func([]byte) ([]byte, error)) error {
	global.mu.Lock()
	defer global.mu.Unlock()

	// Read current content
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Check mtime hasn't changed
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if expected, ok := global.mtimes[path]; ok && info.ModTime().After(expected) {
		return fmt.Errorf("file %q was modified externally during operation", path)
	}

	// Run the operation
	newContent, err := op(content)
	if err != nil {
		return err
	}

	// Write the result
	if err := os.WriteFile(path, newContent, 0644); err != nil {
		return err
	}

	// Update tracking
	info, err = os.Stat(path)
	if err != nil {
		return err
	}
	global.mtimes[path] = info.ModTime()

	return nil
}

// Forget removes tracking info for a file.
// Use this when you want to allow overwrites regardless of external changes.
func Forget(path string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	delete(global.mtimes, path)
}

// Reset clears all tracking info.
func Reset() {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.mtimes = make(map[string]time.Time)
}

// GetMtime returns the tracked mtime for a file (for testing).
func GetMtime(path string) (time.Time, bool) {
	global.mu.RLock()
	defer global.mu.RUnlock()
	mtime, ok := global.mtimes[path]
	return mtime, ok
}

// ResolvePath converts a relative path to an absolute path for consistent tracking.
func ResolvePath(path string, baseDir string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(filepath.Join(baseDir, path))
}
