package generator_test

import (
	"testing"

	"shortener/pkg/generator"
)

func BenchmarkGenerateID(b *testing.B) {
	g := generator.NewGenerator(8)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = g.GenerateID()
	}
}
