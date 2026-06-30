package service_test

import (
	"context"
	"fmt"
	"testing"

	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
)

func newTestService() *service.TrimmerService {
	store := repository.NewMemStorage()
	gen := generator.NewGenerator(8)
	return service.NewTrimmerService(store, gen, "http://localhost:8080")
}

func BenchmarkTrimURL(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Уникальный URL на каждую итерацию — иначе вернётся ErrConflict
		_, _ = svc.TrimURL(ctx, fmt.Sprintf("https://example.com/bench/%d", i))
		i++
	}
}

func BenchmarkBatchShorten(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		b.StopTimer()
		svc := newTestService()
		items := make([]model.BatchRequestItem, 10)
		for j := range items {
			items[j] = model.BatchRequestItem{
				CorrelationID: fmt.Sprintf("corr-%d-%d", i, j),
				OriginalURL:   fmt.Sprintf("https://example.com/batch/%d/%d", i, j),
			}
		}
		b.StartTimer()
		_, _ = svc.BatchShorten(ctx, items)
		i++
	}
}
