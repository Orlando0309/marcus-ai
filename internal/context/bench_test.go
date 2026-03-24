package context

import "testing"

func BenchmarkProbablyBinary_text(b *testing.B) {
	data := []byte("package main\n\nfunc BenchmarkFoo(b *testing.B) {\n\tfor i := 0; i < b.N; i++ {}\n}\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ProbablyBinary(data)
	}
}

func BenchmarkProbablyBinary_png(b *testing.B) {
	data := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ProbablyBinary(data)
	}
}
