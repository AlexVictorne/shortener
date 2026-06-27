package generator_test

import (
	"testing"

	"shortener/pkg/generator"
)

func BenchmarkGenerateID(b *testing.B) {
	g := generator.NewGenerator(8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.GenerateID()
	}
}
