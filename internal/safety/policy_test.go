package safety

import "testing"

func TestValidateRunCommandPolicyBlocked(t *testing.T) {
	pol := DefaultRunCommandPolicy()
	if err := ValidateRunCommandPolicy("echo ok | sh", pol); err == nil {
		t.Fatal("expected block")
	}
	if err := ValidateRunCommandPolicy("go build ./...", pol); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRunCommandPolicyStrict(t *testing.T) {
	pol := RunCommandPolicy{
		StrictAllowlist:   true,
		AllowedPrefixes:   []string{"go ", "npm "},
		AlwaysAllow:       []string{"make"},
		BlockedSubstrings: nil,
	}
	if err := ValidateRunCommandPolicy("go test ./...", pol); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRunCommandPolicy("rm -rf x", pol); err == nil {
		t.Fatal("expected strict rejection")
	}
}
