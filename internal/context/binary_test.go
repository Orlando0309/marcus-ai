package context

import "testing"

func TestProbablyBinary(t *testing.T) {
	if !ProbablyBinary([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Fatal("expected PNG signature to be binary")
	}
	if ProbablyBinary([]byte("package main\n\nfunc main() {}\n")) {
		t.Fatal("expected Go source not binary")
	}
	if !ProbablyBinary([]byte("text\x00more")) {
		t.Fatal("expected NUL to imply binary")
	}
}
