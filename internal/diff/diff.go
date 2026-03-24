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
	// NoNewlineAtEOF is set when the diff contains "\ No newline at end of file" after this hunk.
	NoNewlineAtEOF bool
}

// PatchLine represents a single line in a diff
type PatchLine struct {
	Type    string // "-", "+", " " (context)
	Content string
}

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseUnifiedDiff parses unified diff text into hunks. File headers (---/+++),
// diff --git, and similar lines are skipped.
func ParseUnifiedDiff(diff string) ([]Patch, error) {
	var patches []Patch
	scanner := bufio.NewScanner(strings.NewReader(diff))

	var currentPatch *Patch
	var lastLine *PatchLine

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, `\`) {
			if currentPatch != nil && strings.Contains(line, "No newline at end of file") {
				currentPatch.NoNewlineAtEOF = true
			}
			continue
		}
		if strings.HasPrefix(line, "--- ") || line == "---" ||
			strings.HasPrefix(line, "+++ ") || line == "+++" ||
			strings.HasPrefix(line, "diff ") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "new file mode") ||
			strings.HasPrefix(line, "deleted file mode") ||
			strings.HasPrefix(line, "similarity index") ||
			strings.HasPrefix(line, "rename from") ||
			strings.HasPrefix(line, "rename to") ||
			strings.HasPrefix(line, "Binary files ") {
			continue
		}

		if matches := hunkHeaderRE.FindStringSubmatch(line); matches != nil {
			if currentPatch != nil {
				if err := validatePatchHeader(currentPatch); err != nil {
					return nil, err
				}
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

		if len(line) == 0 {
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: " ", Content: ""})
			lastLine = &currentPatch.Lines[len(currentPatch.Lines)-1]
			continue
		}

		switch line[0] {
		case '-':
			if strings.HasPrefix(line, "---") {
				continue
			}
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: "-", Content: line[1:]})
			lastLine = &currentPatch.Lines[len(currentPatch.Lines)-1]
		case '+':
			if strings.HasPrefix(line, "+++") {
				continue
			}
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: "+", Content: line[1:]})
			lastLine = &currentPatch.Lines[len(currentPatch.Lines)-1]
		case ' ':
			currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: " ", Content: line[1:]})
			lastLine = &currentPatch.Lines[len(currentPatch.Lines)-1]
		default:
			if lastLine != nil {
				lastLine.Content += "\n" + line
			} else {
				currentPatch.Lines = append(currentPatch.Lines, PatchLine{Type: " ", Content: line})
				lastLine = &currentPatch.Lines[len(currentPatch.Lines)-1]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if currentPatch != nil {
		if err := validatePatchHeader(currentPatch); err != nil {
			return nil, err
		}
		patches = append(patches, *currentPatch)
	}

	return patches, nil
}

func validatePatchHeader(p *Patch) error {
	var oldCount, newCount int
	for _, pl := range p.Lines {
		switch pl.Type {
		case "-":
			oldCount++
		case "+":
			newCount++
		case " ":
			oldCount++
			newCount++
		}
	}
	if oldCount != p.OriginalLength {
		return fmt.Errorf("hunk @@ -%d,%d: old side has %d lines, expected %d",
			p.OriginalLine, p.OriginalLength, oldCount, p.OriginalLength)
	}
	if newCount != p.NewLength {
		return fmt.Errorf("hunk @@ +%d,%d: new side has %d lines, expected %d",
			p.NewLine, p.NewLength, newCount, p.NewLength)
	}
	return nil
}

// fileToLines splits text into logical lines. endsWithNewline is true when the
// original string ended with '\n' (POSIX text file).
func fileToLines(s string) (lines []string, endsWithNewline bool) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	endsWithNewline = strings.HasSuffix(s, "\n")
	t := s
	if endsWithNewline {
		t = s[:len(s)-1]
	}
	if t == "" {
		if endsWithNewline {
			return []string{""}, true
		}
		return nil, false
	}
	return strings.Split(t, "\n"), endsWithNewline
}

func linesToFile(lines []string, endsWithNewline bool) string {
	if len(lines) == 0 {
		if endsWithNewline {
			return "\n"
		}
		return ""
	}
	out := strings.Join(lines, "\n")
	if endsWithNewline {
		return out + "\n"
	}
	return out
}

// ApplyPatch applies unified diff hunks in order. Hunk line numbers refer to the
// file before any hunk is applied; offsets from earlier hunks are tracked.
func ApplyPatch(original string, patches []Patch) (string, error) {
	if len(patches) == 0 {
		return original, nil
	}

	lines, endsNL := fileToLines(original)
	delta := 0

	for hi, patch := range patches {
		oldFromPatch := hunkOldLines(patch)
		newFromPatch := hunkNewLines(patch)

		insertAt, err := hunkInsertIndex(&patch, len(lines))
		if err != nil {
			return "", fmt.Errorf("hunk %d: %w", hi+1, err)
		}

		idx := insertAt + delta
		if patch.OriginalLength > 0 {
			if idx < 0 || idx > len(lines) {
				return "", fmt.Errorf("hunk %d: start index %d out of range (len=%d)", hi+1, idx, len(lines))
			}
			if idx+patch.OriginalLength > len(lines) {
				return "", fmt.Errorf("hunk %d: spans past EOF (need %d lines at %d, have %d)",
					hi+1, patch.OriginalLength, idx, len(lines))
			}
			got := lines[idx : idx+patch.OriginalLength]
			if !stringSliceEqual(got, oldFromPatch) {
				return "", fmt.Errorf("hunk %d: context mismatch at line %d", hi+1, patch.OriginalLine)
			}
			lines = append(lines[:idx], append(newFromPatch, lines[idx+patch.OriginalLength:]...)...)
		} else {
			if idx < 0 || idx > len(lines) {
				return "", fmt.Errorf("hunk %d: insertion index %d out of range (len=%d)", hi+1, idx, len(lines))
			}
			lines = append(lines[:idx], append(newFromPatch, lines[idx:]...)...)
		}

		delta += len(newFromPatch) - patch.OriginalLength
	}

	outEndsNL := endsNL
	if len(patches) > 0 {
		last := patches[len(patches)-1]
		if last.NoNewlineAtEOF {
			outEndsNL = false
		} else if original == "" {
			// Empty original → only '+' hunks; each line is a full record, POSIX newline at EOF unless marked above.
			outEndsNL = true
		}
	}

	return linesToFile(lines, outEndsNL), nil
}

func hunkInsertIndex(p *Patch, nLines int) (int, error) {
	if p.OriginalLength == 0 {
		if p.OriginalLine == 0 {
			return 0, nil
		}
		if p.OriginalLine < 1 {
			return 0, fmt.Errorf("invalid old start line %d", p.OriginalLine)
		}
		idx := p.OriginalLine - 1
		if idx > nLines {
			return 0, fmt.Errorf("insert before line %d but file has %d lines", p.OriginalLine, nLines)
		}
		return idx, nil
	}
	if p.OriginalLine < 1 {
		return 0, fmt.Errorf("invalid old start line %d", p.OriginalLine)
	}
	return p.OriginalLine - 1, nil
}

func hunkOldLines(p Patch) []string {
	var out []string
	for _, pl := range p.Lines {
		if pl.Type == "-" || pl.Type == " " {
			out = append(out, pl.Content)
		}
	}
	return out
}

func hunkNewLines(p Patch) []string {
	var out []string
	for _, pl := range p.Lines {
		if pl.Type == "+" || pl.Type == " " {
			out = append(out, pl.Content)
		}
	}
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GenerateDiff returns a valid unified diff between two versions of a file.
// Identical inputs yield an empty string.
func GenerateDiff(original, modified string) (string, error) {
	original = strings.ReplaceAll(original, "\r\n", "\n")
	modified = strings.ReplaceAll(modified, "\r\n", "\n")
	if original == modified {
		return "", nil
	}

	oLines, _ := fileToLines(original)
	nLines, _ := fileToLines(modified)

	oldCount := len(oLines)
	newCount := len(nLines)

	var b strings.Builder
	b.WriteString("--- original\n")
	b.WriteString("+++ modified\n")

	switch {
	case oldCount == 0 && newCount > 0:
		fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", newCount)
		for _, l := range nLines {
			b.WriteString("+" + l + "\n")
		}
	case oldCount > 0 && newCount == 0:
		fmt.Fprintf(&b, "@@ -1,%d +0,0 @@\n", oldCount)
		for _, l := range oLines {
			b.WriteString("-" + l + "\n")
		}
	default:
		fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", oldCount, newCount)
		for _, l := range oLines {
			b.WriteString("-" + l + "\n")
		}
		for _, l := range nLines {
			b.WriteString("+" + l + "\n")
		}
	}

	if modified != "" && !strings.HasSuffix(modified, "\n") {
		b.WriteString("\\ No newline at end of file\n")
	}

	return b.String(), nil
}

// RenderDiff renders a diff with ANSI colors for terminal display
func RenderDiff(diff string) string {
	var rendered strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(diff))

	green := "\x1b[32m"
	red := "\x1b[31m"
	cyan := "\x1b[36m"
	reset := "\x1b[0m"

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
	origBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	newContent, err := ApplyPatch(string(origBytes), patches)
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
