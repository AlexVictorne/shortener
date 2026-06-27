package audit_test

import (
	"context"
	"path/filepath"
	"testing"

	"shortener/pkg/audit"
)

func BenchmarkFileAuditor_Emit(b *testing.B) {
	path := filepath.Join(b.TempDir(), "audit.log")
	fa, err := audit.NewFileAuditor(path)
	if err != nil {
		b.Fatal(err)
	}
	defer fa.Close()

	e := audit.Event{TS: 1700000000, Action: "follow", UserID: "u1", URL: "https://example.com/page/123"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fa.Emit(context.Background(), e)
	}
}

func BenchmarkFileAuditor_EmitParallel(b *testing.B) {
	path := filepath.Join(b.TempDir(), "audit.log")
	fa, err := audit.NewFileAuditor(path)
	if err != nil {
		b.Fatal(err)
	}
	defer fa.Close()

	e := audit.Event{TS: 1700000000, Action: "follow", UserID: "u1", URL: "https://example.com/page/123"}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = fa.Emit(context.Background(), e)
		}
	})
}
