package context

// EstimateTokens approximates token count (~4 chars per token).
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}
