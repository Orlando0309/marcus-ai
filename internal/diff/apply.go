package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Hunk represents a single hunk in a unified diff.
type Hunk struct {
	OriginalStart int
	OriginalLines int
	NewStart      int
	NewLines      int
	Lines         []HunkLine
}

// HunkLine represents a single line in a hunk.
type HunkLine struct {
	Kind    string // "-", "+", " " (context), or "\\" (no newline)
	Content string
}

// ApplyUnifiedDiff applies a unified diff to a file.
// This is a secure implementation that validates the diff before applying.
func ApplyUnifiedDiff(filePath, diff string, baseDir string) error {
	// Validate file path
	cleanPath := filePath
	if !filepath.IsAbs(filePath) && baseDir != "" {
		cleanPath = filepath.Join(baseDir, filePath)
	}

	// Ensure the path is within baseDir if provided
	if baseDir != "" {
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		absBase, err := filepath.Abs(baseDir)
		if err != nil {
			return fmt.Errorf("invalid base dir: %w", err)
		}
		if !strings.HasPrefix(absPath, absBase) {
			return fmt.Errorf("path %q is outside base directory", filePath)
		}
	}

	// Read original file
	originalContent, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Parse the diff
	hunks, err := parseUnifiedDiff(diff)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	if len(hunks) == 0 {
		return nil // No hunks to apply
	}

	// Apply hunks
	result, err := applyHunks(originalContent, hunks)
	if err != nil {
		return fmt.Errorf("apply hunks: %w", err)
	}

	// Create parent directory if needed
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// Write with restricted permissions
	return os.WriteFile(cleanPath, result, 0644)
}

// parseUnifiedDiff parses a unified diff into hunks.
func parseUnifiedDiff(diff string) ([]Hunk, error) {
	var hunks []Hunk
	scanner := bufio.NewScanner(strings.NewReader(diff))

	var currentHunk *Hunk
	for scanner.Scan() {
		line := scanner.Text()

		// Check for hunk header
		if strings.HasPrefix(line, "@@") {
			// Save previous hunk
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			// Parse new hunk header: @@ -start,count +start,count @@
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			currentHunk = &hunk
			continue
		}

		// Skip file headers and other metadata
		if currentHunk == nil {
			continue
		}

		// Parse hunk lines
		if len(line) == 0 {
			// Empty line in diff is a context line with empty content
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: " ", Content: ""})
			continue
		}

		switch line[0] {
		case '-':
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: "-", Content: line[1:]})
		case '+':
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: "+", Content: line[1:]})
		case ' ':
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: " ", Content: line[1:]})
		case '\\':
			// "\ No newline at end of file"
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: "\\", Content: line})
		default:
			// Unknown line, treat as context
			currentHunk.Lines = append(currentHunk.Lines, HunkLine{Kind: " ", Content: line})
		}
	}

	// Save last hunk
	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, scanner.Err()
}

// parseHunkHeader parses a hunk header like "@@ -1,5 +1,6 @@".
func parseHunkHeader(header string) (Hunk, error) {
	// Remove leading/trailing @@ and spaces
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "@@") {
		return Hunk{}, fmt.Errorf("invalid hunk header: %s", header)
	}

	header = strings.TrimPrefix(header, "@@")
	header = strings.TrimSpace(header)

	// Find the closing @@
	parts := strings.SplitN(header, "@@", 2)
	if len(parts) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header: %s", header)
	}

	rangePart := strings.TrimSpace(parts[0])

	// Parse "-start,count +start,count"
	fields := strings.Fields(rangePart)
	if len(fields) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header ranges: %s", rangePart)
	}

	origStart, origCount, err := parseRange(fields[0])
	if err != nil {
		return Hunk{}, fmt.Errorf("invalid original range: %w", err)
	}

	newStart, newCount, err := parseRange(fields[1])
	if err != nil {
		return Hunk{}, fmt.Errorf("invalid new range: %w", err)
	}

	return Hunk{
		OriginalStart: origStart,
		OriginalLines: origCount,
		NewStart:      newStart,
		NewLines:      newCount,
	}, nil
}

// parseRange parses a range like "1,5" or "1" (count defaults to 1).
func parseRange(s string) (start, count int, err error) {
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "-")

	parts := strings.SplitN(s, ",", 2)
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("empty range")
	}

	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
	} else {
		count = 1
	}

	return start, count, nil
}

// applyHunks applies hunks to original content.
func applyHunks(original []byte, hunks []Hunk) ([]byte, error) {
	// Split original into lines
	originalLines := splitLines(original)

	// Apply hunks in reverse order to avoid line number shifts
	for i := len(hunks) - 1; i >= 0; i-- {
		var err error
		originalLines, err = applyHunk(originalLines, hunks[i])
		if err != nil {
			return nil, err
		}
	}

	// Reconstruct file
	var result bytes.Buffer
	for _, line := range originalLines {
		result.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			result.WriteString("\n")
		}
	}

	return result.Bytes(), nil
}

// splitLines splits content into lines, preserving line endings.
func splitLines(content []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		lines = append(lines, scanner.Text()+"\n")
	}
	// Handle last line without newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if len(lines) > 0 {
			lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "\n")
		}
	}
	return lines
}

// applyHunk applies a single hunk to lines.
func applyHunk(lines []string, hunk Hunk) ([]string, error) {
	if hunk.OriginalStart < 1 {
		return nil, fmt.Errorf("invalid hunk start line: %d", hunk.OriginalStart)
	}

	startIdx := hunk.OriginalStart - 1
	if startIdx > len(lines) {
		return nil, fmt.Errorf("hunk start %d exceeds file length %d", hunk.OriginalStart, len(lines))
	}

	// Count expected original lines (deletions + context)
	expectedOrigLines := 0
	for _, hl := range hunk.Lines {
		if hl.Kind == "-" || hl.Kind == " " {
			expectedOrigLines++
		}
	}

	// Verify we have enough lines
	availableLines := len(lines) - startIdx
	if availableLines < expectedOrigLines {
		return nil, fmt.Errorf("hunk expects %d original lines but only %d available", expectedOrigLines, availableLines)
	}

	// Verify context lines match
	if err := verifyHunkContext(lines[startIdx:], hunk); err != nil {
		return nil, err
	}

	// Build new lines
	var newLines []string
	lineIdx := 0

	for _, hl := range hunk.Lines {
		switch hl.Kind {
		case "-":
			// Skip this original line
			lineIdx++
		case "+":
			// Add new line
			newLines = append(newLines, hl.Content+"\n")
		case " ":
			// Keep original line
			if lineIdx < len(lines) {
				newLines = append(newLines, lines[startIdx+lineIdx])
				lineIdx++
			}
		case "\\":
			// No newline marker - skip
		}
	}

	// Replace original lines with new lines
	result := make([]string, 0, len(lines)-expectedOrigLines+len(newLines))
	result = append(result, lines[:startIdx]...)
	result = append(result, newLines...)
	result = append(result, lines[startIdx+expectedOrigLines:]...)

	return result, nil
}

// verifyHunkContext verifies that context lines in the hunk match the file.
func verifyHunkContext(lines []string, hunk Hunk) error {
	lineIdx := 0
	for _, hl := range hunk.Lines {
		if hl.Kind == "-" || hl.Kind == " " {
			if lineIdx >= len(lines) {
				return fmt.Errorf("hunk context extends past file")
			}
			if hl.Kind == " " {
				// Compare content (trim for comparison)
				expected := strings.TrimSuffix(hl.Content, "\n")
				actual := strings.TrimSuffix(lines[lineIdx], "\n")
				if expected != actual {
					return fmt.Errorf("hunk context mismatch at line: expected %q, got %q", expected, actual)
				}
			}
			lineIdx++
		}
	}
	return nil
}
