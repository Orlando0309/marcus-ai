package skill

import (
	"strings"
)

// ParseSlashCommand extracts the command name and arguments from a slash command input.
// Returns the command (without the leading "/"), arguments, and true if valid.
// Returns "", nil, false if the input is not a valid slash command.
func ParseSlashCommand(input string) (string, []string, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", nil, false
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil, false
	}

	command := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	return command, args, true
}

// IsSlashCommand returns true if the input looks like a slash command
func IsSlashCommand(input string) bool {
	input = strings.TrimSpace(input)
	return strings.HasPrefix(input, "/")
}

// ExtractCommandName returns just the command name without leading slash
// Returns empty string if not a valid command
func ExtractCommandName(input string) string {
	cmd, _, ok := ParseSlashCommand(input)
	if !ok {
		return ""
	}
	return cmd
}
