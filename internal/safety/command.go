package safety

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxCommandLen = 100_000

// ValidateRunCommand applies conservative checks on shell commands proposed by the model.
func ValidateRunCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	if strings.ContainsRune(cmd, 0) {
		return fmt.Errorf("command contains NUL")
	}
	if !utf8.ValidString(cmd) {
		return fmt.Errorf("command is not valid UTF-8")
	}
	if len(cmd) > maxCommandLen {
		return fmt.Errorf("command exceeds max length")
	}
	if strings.ContainsAny(cmd, "\r\n") {
		return fmt.Errorf("multiline commands are not allowed")
	}
	return nil
}
