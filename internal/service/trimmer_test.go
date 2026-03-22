package service_test

import (
	"context"
	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/internal/service"
	"shortener/pkg/generator"
	"testing"
)

func TestTrimmerService_TrimURL(t *testing.T) {
	tests := []struct {
		name        string
		storage     repository.Storage
		generator   generator.IDGenerator
		baseURL     string
		originalURL string
		wantPrefix  string
		wantErr     bool
	}{
		{
			name:        "success",
			storage:     repository.NewMemStorage(),
			generator:   generator.NewGenerator(8),
			baseURL:     "http://localhost:8080/",
			originalURL: "https://ya.ru",
			wantPrefix:  "http://localhost:8080/",
			wantErr:     false,
		},
		{
			name:        "empty url",
			storage:     repository.NewMemStorage(),
			generator:   generator.NewGenerator(8),
			baseURL:     "http://localhost:8080/",
			originalURL: "",
			wantErr:     true,
		},
		{
			name:        "bad format",
			storage:     repository.NewMemStorage(),
			generator:   generator.NewGenerator(8),
			baseURL:     "http://localhost:8080/",
			originalURL: "ya.ru-123``",
			wantErr:     true,
		},
		{
			name:        "bad scheme",
			storage:     repository.NewMemStorage(),
			generator:   generator.NewGenerator(8),
			baseURL:     "http://localhost:8080/",
			originalURL: "ftp://ya.ru",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := service.NewTrimmerService(tt.storage, tt.generator, tt.baseURL)
			got, gotErr := s.TrimURL(context.Background(), tt.originalURL)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("TrimURL failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("TrimURL succeeded unexpectedly")
			}
			if tt.wantPrefix != "" && (len(got) < len(tt.wantPrefix) || got[:len(tt.wantPrefix)] != tt.wantPrefix) {
				t.Errorf("TrimURL = %v, want prefix %v", got, tt.wantPrefix)
			}
		})
	}
}

func TestTrimmerService_GetOriginalURL(t *testing.T) {
	tests := []struct {
		name      string
		storage   repository.Storage
		generator generator.IDGenerator
		baseURL   string
		prefill   *model.ShortURL
		shortURL  string
		want      string
		wantErr   bool
	}{
		{
			name:      "found",
			storage:   repository.NewMemStorage(),
			generator: generator.NewGenerator(8),
			baseURL:   "http://localhost:8080/",
			prefill:   &model.ShortURL{ShortURL: "abc12345", OriginalURL: "https://ya.ru"},
			shortURL:  "http://localhost:8080/abc12345",
			want:      "https://ya.ru",
			wantErr:   false,
		},
		{
			name:      "not found empty",
			storage:   repository.NewMemStorage(),
			generator: generator.NewGenerator(8),
			baseURL:   "http://localhost:8080/",
			shortURL:  "http://localhost:8080/notexist",
			wantErr:   true,
		},
		{
			name:      "not found non-empty",
			storage:   repository.NewMemStorage(),
			generator: generator.NewGenerator(8),
			baseURL:   "http://localhost:8080/",
			prefill:   &model.ShortURL{ShortURL: "abc12345", OriginalURL: "https://ya.ru"},
			shortURL:  "http://localhost:8080/notexist",
			wantErr:   true,
		},
		{
			name:      "bad id format",
			storage:   repository.NewMemStorage(),
			generator: generator.NewGenerator(8),
			baseURL:   "http://localhost:8080/",
			shortURL:  "http://localhost:8080/!badid!",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := service.NewTrimmerService(tt.storage, tt.generator, tt.baseURL)
			if tt.prefill != nil {
				_ = tt.storage.Create(context.Background(), tt.prefill)
			}
			got, gotErr := s.GetOriginalURL(context.Background(), tt.shortURL)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GetOriginalURL failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GetOriginalURL succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("GetOriginalURL = %v, want %v", got, tt.want)
			}
		})
	}
}
