package diff

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Patch represents a unified diff hunk
type Patch struct {
	OriginalLine   int
	OriginalLength int
	NewLine        int
	NewLength      int
	Lines          []PatchLine
}

// PatchLine represents a single line in a diff
type PatchLine struct {
	Type    string // "-", "+", " " (context)
	Content string
}

// ParseUnifiedDiff parses a unified diff format string
func ParseUnifiedDiff(diff string) ([]Patch, error) {
	var patches []Patch
	scanner := bufio.NewScanner(strings.NewReader(diff))

	var currentPatch *Patch
	hunkRegex := regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

	// Track the last diff line so we can merge continuation lines
	var lastLine *PatchLine

	for scanner.Scan() {
		line := scanner.Text()

		// Check for hunk header
		if matches := hunkRegex.FindStringSubmatch(line); matches != nil {
			if currentPatch != nil {
				patches = append(patches, *currentPatch)
			}

			origLine, _ := strconv.Atoi(matches[1])
			origLen := 1
			if matches[2] != "" {
				origLen, _ = strconv.Atoi(matches[2])
			}
			newLine, _ := strconv.Atoi(matches[3])
			newLen := 1
			if matches[4] != "" {
				newLen, _ = strconv.Atoi(matches[4])
			}

			currentPatch = &Patch{
				OriginalLine:   origLine,
				OriginalLength: origLen,
				NewLine:        newLine,
				NewLength:      newLen,
			}
			lastLine = nil
			continue
		}

		if currentPatch == nil {
			continue
		}

		if strings.HasPrefix(line, "-") {
			content := strings.TrimPrefix(line, "-")
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: "-", Content: content})
			lastLine = nil
		} else if strings.HasPrefix(line, "+") {
			content := strings.TrimPrefix(line, "+")
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: "+", Content: content})
			lastLine = nil
		} else if strings.HasPrefix(line, " ") || line == "" {
			// Empty context line or explicit space-prefixed line
			content := strings.TrimPrefix(line, " ")
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: " ", Content: content})
			lastLine = nil
		} else if lastLine != nil {
			// Continuation line — append to previous diff line's content
			// This handles cases where the LLM returns "+func" as separate lines
			lastLine.Content += "\n" + line
		} else {
			// Orphan context line (no previous diff line to attach to)
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: " ", Content: line})
		}
	}

	if currentPatch != nil {
		patches = append(patches, *currentPatch)
	}

	return patches, nil
}

// ApplyPatch applies a patch to the original content
func ApplyPatch(original string, patches []Patch) (string, error) {
	lines := strings.SplitAfter(original, "\n")
	if len(original) > 0 && !strings.HasSuffix(original, "\n") {
		lines = append(lines, "")
	}

	var result []string
	lineOffset := 0 // Track how many lines we've added/removed

	for _, patch := range patches {
		// Adjust patch position based on offset
		applyLine := patch.OriginalLine - 1 + lineOffset

		if applyLine < 0 || applyLine >= len(lines) {
			return "", fmt.Errorf("patch line %d out of range (file has %d lines)", patch.OriginalLine, len(lines))
		}

		// Extract lines to replace
		replaceCount := patch.OriginalLength
		if replaceCount > len(lines)-applyLine {
			replaceCount = len(lines) - applyLine
		}

		// Build new lines from patch
		var newLines []string
		for _, pl := range patch.Lines {
			if pl.Type != "-" {
				newLines = append(newLines, pl.Content+"\n")
			}
		}

		// Replace old lines with new
		result = append(result, lines[:applyLine]...)
		result = append(result, newLines...)
		result = append(result, lines[applyLine+replaceCount:]...)

		// Update offset
		lineOffset += len(newLines) - int(patch.OriginalLength)
	}

	if len(result) == 0 {
		return "", nil
	}

	return strings.Join(result, ""), nil
}

// GenerateDiff generates a unified diff between two files
func GenerateDiff(original, new string) (string, error) {
	return simpleDiff(original, new), nil
}

// simpleDiff creates a simple line-by-line diff
func simpleDiff(original, new string) string {
	origLines := strings.Split(original, "\n")
	newLines := strings.Split(new, "\n")

	var diff strings.Builder
	diff.WriteString("--- original\n")
	diff.WriteString("+++ modified\n")

	maxOrig := len(origLines)
	maxNew := len(newLines)

	for i := 0; i < maxOrig || i < maxNew; i++ {
		if i < maxOrig && i < maxNew {
			if origLines[i] != newLines[i] {
				diff.WriteString(fmt.Sprintf("@@ line %d @@\n", i+1))
				diff.WriteString("-" + origLines[i] + "\n")
				diff.WriteString("+" + newLines[i] + "\n")
			}
		} else if i < maxOrig {
			diff.WriteString("-" + origLines[i] + "\n")
		} else {
			diff.WriteString("+" + newLines[i] + "\n")
		}
	}

	return diff.String()
}

// RenderDiff renders a diff with ANSI colors for terminal display
func RenderDiff(diff string) string {
	var rendered strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(diff))

	// ANSI color codes (ESC is \x1b in Go)
	green   := "\x1b[32m"
	red     := "\x1b[31m"
	cyan    := "\x1b[36m"
	reset   := "\x1b[0m"

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+"):
			rendered.WriteString(green)
			rendered.WriteString(line)
			rendered.WriteString(reset)
			rendered.WriteString("\n")
		case strings.HasPrefix(line, "-"):
			rendered.WriteString(red)
			rendered.WriteString(line)
			rendered.WriteString(reset)
			rendered.WriteString("\n")
		case strings.HasPrefix(line, "@"):
			rendered.WriteString(cyan)
			rendered.WriteString(line)
			rendered.WriteString(reset)
			rendered.WriteString("\n")
		default:
			rendered.WriteString(line)
			rendered.WriteString("\n")
		}
	}

	return rendered.String()
}

// ConfirmApply prompts the user to confirm applying a diff
func ConfirmApply(diff string) bool {
	fmt.Println(RenderDiff(diff))
	fmt.Print("\nApply this diff? [y/N]: ")

	var answer string
	fmt.Scanln(&answer)

	return answer == "y" || answer == "Y"
}

// ApplyPatchToFile reads a file, applies a patch, and writes the result
func ApplyPatchToFile(path string, patches []Patch) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	newContent, err := ApplyPatch(string(original), patches)
	if err != nil {
		return fmt.Errorf("apply patch: %w", err)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// WriteFileWithPatch reads a file, applies a patch, and writes the result
func WriteFileWithPatch(path string, patches []Patch) error {
	return ApplyPatchToFile(path, patches)
}
