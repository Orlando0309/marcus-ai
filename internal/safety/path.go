package safety

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxPathLen = 4096

// CheckToolRelativePath rejects paths that are unsafe for LLM-supplied tool arguments.
func CheckToolRelativePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("path contains NUL")
	}
	if !utf8.ValidString(path) {
		return fmt.Errorf("path is not valid UTF-8")
	}
	if len(path) > maxPathLen {
		return fmt.Errorf("path exceeds max length")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed; use paths relative to the project root")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path must not escape the project directory")
	}
	return nil
}
