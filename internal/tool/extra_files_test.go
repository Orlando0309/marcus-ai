package tool

import "testing"

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		pat  string
		path string
		want bool
	}{
		{"*.go", "foo.go", true},
		{"*.go", "internal/foo.go", false},
		{"**/*.go", "internal/diff/diff.go", true},
		{"**/*.go", "main.go", true},
		{"cmd/*", "cmd/marcus", true},
		{"cmd/*", "internal/cmd/x", false},
		{"**", "anything", true},
	}
	for _, tc := range tests {
		if got := matchGlobPattern(tc.pat, tc.path); got != tc.want {
			t.Errorf("%q vs %q: got %v want %v", tc.pat, tc.path, got, tc.want)
		}
	}
}
