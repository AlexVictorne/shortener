package auth_test

import (
	"testing"

	"shortener/pkg/auth"
)

const testSecret = "bench_secret_key"

func BenchmarkSign(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.Sign("abc123userID000000000000000000000000", testSecret)
	}
}

func BenchmarkVerify(b *testing.B) {
	cookie := auth.Sign("abc123userID000000000000000000000000", testSecret)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = auth.Verify(cookie, testSecret)
	}
}
