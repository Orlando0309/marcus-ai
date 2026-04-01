package safety

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// DangerousShellPatterns defines patterns that indicate potentially dangerous shell commands.
var DangerousShellPatterns = []string{
	// Pipe to shell
	"| sh", "| bash", "|sh ", "|bash ",
	// Semicolon followed by shell
	"; sh ", "; bash ", ";sh ", ";bash ",
	// Curl/wget piping (data exfiltration risk)
	"&& curl ", "| curl ", "; curl ", "curl |",
	"&& wget ", "| wget ", "; wget ", "wget |",
	// Destructive operations
	"rm -rf /", "rm -rf \\", "rm -rf *", "rm -rf .*",
	"del /f /s /q", "format ", "chkdsk /f",
	// Disk operations
	"mkfs.", "dd if=/dev/", "dd of=",
	// Permission changes
	"chmod 777 ", "chmod -R 777 ", "chmod a+rwx",
	// SUID binaries
	"chmod u+s ", "chmod +s ",
	// Password file access
	"/etc/passwd", "/etc/shadow", "SAM ", "SAM\\",
	// Process injection
	"ptrace", "LD_PRELOAD",
	// Base64/encoding (obfuscation)
	"base64 -d", "base64 --decode", "echo.*|.*base64",
	// Eval and code execution
	"eval ", "exec ", "$(", "`",
}

// SafeCommandPrefix defines command prefixes that are generally safe to execute.
var SafeCommandPrefix = []string{
	// Build commands
	"go build", "go test", "go vet", "go fmt", "go mod",
	"cargo build", "cargo test", "cargo check", "cargo fmt",
	"npm build", "npm test", "npm run build", "npm run test",
	"yarn build", "yarn test",
	"make ", "cmake ", "gradle ", "mvn ",
	// Python
	"python -m py_compile", "python -m pytest", "python -m unittest",
	"py -m py_compile", "py -m pytest",
	// Linters
	"golangci-lint ", "ruff ", "eslint ", "flake8 ",
	// Git (read-only)
	"git status", "git diff", "git log", "git show", "git branch",
	"git rev-parse", "git ls-files", "git describe",
	// File operations (safe)
	"ls ", "dir ", "cat ", "type ", "head ", "tail ",
	"find ", "grep ", "rg ",
	// Echo/printf with redirects (for logging/hooks)
	"echo ", "printf ",
	// Redirects (safe when combined with allowed commands)
	"> ", ">> ", "> ",
}

// ShellValidator validates shell commands against security policies.
type ShellValidator struct {
	// Allowlist - commands that are always allowed
	Allowlist []string
	// Blocklist - commands that are always blocked
	Blocklist []string
	// StrictMode - if true, only allowlisted commands are permitted
	StrictMode bool
	// AllowPrefixes - command prefixes that are allowed
	AllowPrefixes []string
}

// DefaultShellValidator returns a validator with conservative defaults.
func DefaultShellValidator() *ShellValidator {
	return &ShellValidator{
		Allowlist: []string{
			"go test ./...",
			"go build ./...",
			"go vet ./...",
			"cargo test",
			"cargo build",
			"npm test",
			"npm run build",
			"python -m pytest",
			"python -m py_compile",
		},
		Blocklist:       DangerousShellPatterns,
		StrictMode:      false,
		AllowPrefixes:   SafeCommandPrefix,
	}
}

// ValidateCommand checks if a command is safe to execute.
func (v *ShellValidator) ValidateCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}

	// Check length
	if len(cmd) > maxCommandLen {
		return fmt.Errorf("command exceeds maximum length of %d", maxCommandLen)
	}

	// Check for NUL bytes
	if strings.ContainsRune(cmd, 0) {
		return fmt.Errorf("command contains NUL byte")
	}

	// Check for multiline
	if strings.ContainsAny(cmd, "\r\n") {
		return fmt.Errorf("multiline commands are not allowed")
	}

	// Check blocklist first (always enforced)
	lowerCmd := strings.ToLower(cmd)
	for _, pattern := range v.Blocklist {
		pattern = strings.TrimSpace(strings.ToLower(pattern))
		if pattern == "" {
			continue
		}
		if strings.Contains(lowerCmd, pattern) {
			return fmt.Errorf("command blocked: contains dangerous pattern %q", pattern)
		}
	}

	// Check for dangerous regex patterns
	dangerousRegexes := []string{
		`echo\s+.*\|\s*(base64|bash|sh)`,
		`\$\([^)]+\)`,
		"`[^`]+`",
	}
	for _, pattern := range dangerousRegexes {
		if matched, _ := regexp.MatchString(pattern, cmd); matched {
			return fmt.Errorf("command blocked: matches dangerous pattern")
		}
	}

	// If strict mode, only allowlisted commands pass
	if v.StrictMode {
		// Check exact allowlist
		for _, allowed := range v.Allowlist {
			allowed = strings.TrimSpace(allowed)
			if cmd == allowed || strings.HasPrefix(cmd, allowed+" ") {
				return nil
			}
		}
		// Check prefixes
		for _, prefix := range v.AllowPrefixes {
			prefix = strings.TrimSpace(prefix)
			if strings.HasPrefix(cmd, prefix) {
				return nil
			}
		}
		return fmt.Errorf("command not on allowlist (strict mode enabled)")
	}

	// Non-strict mode: passed blocklist checks, command is allowed
	return nil
}

// ValidateFilePath validates a file path for safe operations.
func ValidateFilePath(path string, baseDir string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// Check for NUL bytes
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("path contains NUL byte")
	}

	// Check length
	if len(path) > maxPathLen {
		return fmt.Errorf("path exceeds maximum length")
	}

	// Check for absolute paths (relative only)
	// On Windows, filepath.IsAbs checks for drive letters like C:\
	// On Unix, it checks for leading /
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed")
	}

	// Also check for Windows-style absolute paths explicitly
	if len(path) >= 2 && path[1] == ':' {
		return fmt.Errorf("absolute paths (Windows drive letter) are not allowed")
	}

	// Check for path traversal
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path traversal not allowed")
	}

	return nil
}

// ValidateExecutablePath validates that a path points to a safe executable.
func ValidateExecutablePath(path string) error {
	if err := ValidateFilePath(path, ""); err != nil {
		return err
	}

	// Check for script interpreters that could be abused
	lowerPath := strings.ToLower(path)
	dangerousExecutables := []string{
		"cmd.exe", "cmd",
		"powershell", "pwsh",
		"bash", "sh", "zsh",
		"python", "python3",
		"perl", "ruby",
		"wget", "curl",
	}

	for _, dangerous := range dangerousExecutables {
		if strings.HasSuffix(lowerPath, dangerous) || strings.HasSuffix(lowerPath, dangerous+".exe") {
			return fmt.Errorf("direct execution of %s is not allowed", dangerous)
		}
	}

	return nil
}

// SanitizeShellArg escapes special shell characters in an argument.
// This is exported for use by other packages.
func SanitizeShellArg(arg string) string {
	// Escape single quotes by replacing ' with '\''
	// This is the safest way to escape shell arguments
	arg = strings.ReplaceAll(arg, "'", "'\\''")
	// Wrap in single quotes to prevent any shell interpretation
	return "'" + arg + "'"
}
