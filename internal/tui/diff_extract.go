package tui

import "strings"

const maxStreamDiffRunes = 15000

// extractUnifiedDiffSnippet returns text from the last unified-diff hunk header onward, if any.
func extractUnifiedDiffSnippet(s string) string {
	idx := strings.LastIndex(s, "@@ -")
	if idx < 0 {
		return ""
	}
	out := s[idx:]
	r := []rune(out)
	if len(r) > maxStreamDiffRunes {
		return string(r[:maxStreamDiffRunes]) + "\n...[truncated]"
	}
	return out
}
