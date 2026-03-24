package safety

import (
	"fmt"
	"strings"
)

// RunCommandPolicy is optional allow/block rules layered on top of ValidateRunCommand.
type RunCommandPolicy struct {
	BlockedSubstrings []string
	// AllowedPrefixes — if StrictAllowlist is true, the trimmed command must start with one of these or match AlwaysAllow.
	AllowedPrefixes []string
	AlwaysAllow     []string
	StrictAllowlist bool
}

// DefaultRunCommandPolicy returns conservative substring blocks (not strict allowlist).
func DefaultRunCommandPolicy() RunCommandPolicy {
	return RunCommandPolicy{
		BlockedSubstrings: []string{
			"| sh", "| bash", "|sh ", "; sh ", "; bash ",
			"&& curl ", "| curl ", "; curl ",
			"rm -rf /", "rm -rf \\", "mkfs.", "dd if=/dev/",
		},
	}
}

func commandMatchesAllow(cmd string, always []string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, a := range always {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if cmd == a || strings.HasPrefix(cmd, a+" ") {
			return true
		}
	}
	return false
}

func commandMatchesPrefix(cmd string, prefixes []string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	return false
}

// ValidateRunCommandPolicy applies blocklist and optional strict allowlist.
func ValidateRunCommandPolicy(cmd string, pol RunCommandPolicy) error {
	cmd = strings.TrimSpace(cmd)
	lower := strings.ToLower(cmd)
	for _, b := range pol.BlockedSubstrings {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(b)) {
			return fmt.Errorf("command blocked by policy (matches %q)", b)
		}
	}
	if pol.StrictAllowlist {
		if commandMatchesAllow(cmd, pol.AlwaysAllow) || commandMatchesPrefix(cmd, pol.AllowedPrefixes) {
			return nil
		}
		return fmt.Errorf("command not on allowlist (strict run_command policy)")
	}
	return nil
}
