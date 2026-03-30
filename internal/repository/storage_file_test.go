package repository_test

import (
	"context"
	"os"
	"shortener/internal/model"
	"shortener/internal/repository"
	"testing"
)

func TestMemStorage_SaveAndLoadToFile(t *testing.T) {
	filePath := "test_shortener_data.json"
	defer os.Remove(filePath)

	urls := []model.ShortURL{
		{UUID: 1, ShortURL: "abc", OriginalURL: "https://ya.ru"},
		{UUID: 2, ShortURL: "def", OriginalURL: "https://yandex.ru"},
	}

	s, err := repository.NewMemStorageWithFile(filePath)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	for i := range urls {
		u := urls[i]
		_ = s.Create(context.Background(), &u)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close/Save error: %v", err)
	}

	s2, err := repository.NewMemStorageWithFile(filePath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	defer s2.Close()
	loaded := s2.ExportAll()
	if len(loaded) != len(urls) {
		t.Fatalf("Loaded %d, want %d", len(loaded), len(urls))
	}
	wantMap := make(map[string]model.ShortURL)
	for _, u := range urls {
		wantMap[u.ShortURL] = u
	}
	for _, got := range loaded {
		want, ok := wantMap[got.ShortURL]
		if !ok {
			t.Errorf("Unexpected ShortURL: %+v", got)
			continue
		}
		if got != want {
			t.Errorf("Mismatch for %s: got %+v, want %+v", got.ShortURL, got, want)
		}
	}
}
