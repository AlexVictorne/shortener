package repository_test

import (
	"context"
	"fmt"
	"testing"

	"shortener/internal/model"
	"shortener/internal/repository"
)

func newSeededStorage(n int) *repository.MemStorage {
	s := repository.NewMemStorage()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		_ = s.Create(ctx, &model.ShortURL{
			ShortURL:    fmt.Sprintf("short%07d", i),
			OriginalURL: fmt.Sprintf("https://example.com/page/%d", i),
			UserID:      "user1",
		})
	}
	return s
}

func BenchmarkMemStorage_Create(b *testing.B) {
	s := repository.NewMemStorage()
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_ = s.Create(ctx, &model.ShortURL{
			ShortURL:    fmt.Sprintf("s%d", i),
			OriginalURL: fmt.Sprintf("https://example.com/%d", i),
			UserID:      "u1",
		})
		i++
	}
}

func BenchmarkMemStorage_Get(b *testing.B) {
	s := newSeededStorage(10000)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_, _ = s.Get(context.Background(), fmt.Sprintf("short%07d", i%10000))
		i++
	}
}

func BenchmarkMemStorage_GetParallel(b *testing.B) {
	s := newSeededStorage(10000)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = s.Get(context.Background(), fmt.Sprintf("short%07d", i%10000))
			i++
		}
	})
}

func BenchmarkMemStorage_GetByOriginal(b *testing.B) {
	s := newSeededStorage(10000)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_, _ = s.GetByOriginal(context.Background(), fmt.Sprintf("https://example.com/page/%d", i%10000))
		i++
	}
}

func BenchmarkMemStorage_GetByOriginalParallel(b *testing.B) {
	s := newSeededStorage(10000)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = s.GetByOriginal(context.Background(), fmt.Sprintf("https://example.com/page/%d", i%10000))
			i++
		}
	})
}

func BenchmarkMemStorage_GetByUserID(b *testing.B) {
	// 1000 URL у одного пользователя — возврат слайса ощутимого размера
	s := repository.NewMemStorage()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = s.Create(ctx, &model.ShortURL{
			ShortURL:    fmt.Sprintf("u1short%05d", i),
			OriginalURL: fmt.Sprintf("https://example.com/u1/%d", i),
			UserID:      "user1",
		})
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.GetByUserID(context.Background(), "user1")
	}
}

func BenchmarkMemStorage_BatchCreate(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		s := repository.NewMemStorage()
		batch := make([]*model.ShortURL, 50)
		for j := range batch {
			batch[j] = &model.ShortURL{
				ShortURL:    fmt.Sprintf("b-%d", j),
				OriginalURL: fmt.Sprintf("https://example.com/batch/%d", j),
				UserID:      "u1",
			}
		}
		b.StartTimer()
		_ = s.BatchCreate(ctx, batch)
	}
}
