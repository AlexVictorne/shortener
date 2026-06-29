package auth_test

import (
	"testing"

	"shortener/pkg/auth"
)

const testSecret = "bench_secret_key"

func BenchmarkSign(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = auth.Sign("abc123userID000000000000000000000000", testSecret)
	}
}

func BenchmarkVerify(b *testing.B) {
	cookie := auth.Sign("abc123userID000000000000000000000000", testSecret)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = auth.Verify(cookie, testSecret)
	}
}
