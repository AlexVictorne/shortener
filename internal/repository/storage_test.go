package repository_test

import (
	"context"
	"shortener/internal/model"
	"shortener/internal/repository"
	"testing"
)

func TestMemStorage_Create(t *testing.T) {
	tests := []struct {
		name    string
		url     *model.ShortURL
		prefill []*model.ShortURL
		wantErr bool
	}{
		{
			name:    "success",
			url:     &model.ShortURL{ID: "abc", OriginalURL: "https://ya.ru"},
			wantErr: false,
		},
		{
			name:    "conflict by ID",
			url:     &model.ShortURL{ID: "abc", OriginalURL: "https://ya.ru"},
			prefill: []*model.ShortURL{{ID: "abc", OriginalURL: "https://yandex.ru"}},
			wantErr: true,
		},
		{
			name:    "conflict by OriginalURL",
			url:     &model.ShortURL{ID: "def", OriginalURL: "https://ya.ru"},
			prefill: []*model.ShortURL{{ID: "abc", OriginalURL: "https://ya.ru"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := repository.NewMemStorage()
			for _, u := range tt.prefill {
				_ = s.Create(context.Background(), u)
			}
			gotErr := s.Create(context.Background(), tt.url)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Create failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Create succeeded unexpectedly")
			}
		})
	}
}

func TestMemStorage_Get(t *testing.T) {
	tests := []struct {
		name    string
		prefill []*model.ShortURL
		ID      string
		want    *model.ShortURL
		wantErr bool
	}{
		{
			name:    "found",
			prefill: []*model.ShortURL{{ID: "abc", OriginalURL: "https://ya.ru"}},
			ID:      "abc",
			want:    &model.ShortURL{ID: "abc", OriginalURL: "https://ya.ru"},
			wantErr: false,
		},
		{
			name:    "not found",
			ID:      "notexist",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := repository.NewMemStorage()
			for _, u := range tt.prefill {
				_ = s.Create(context.Background(), u)
			}
			got, gotErr := s.Get(context.Background(), tt.ID)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Get failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Get succeeded unexpectedly")
			}
			if got == nil && tt.want != nil || got != nil && tt.want == nil {
				t.Errorf("Get = %v, want %v", got, tt.want)
			} else if got != nil && tt.want != nil {
				if got.ID != tt.want.ID || got.OriginalURL != tt.want.OriginalURL {
					t.Errorf("Get = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMemStorage_GetByOriginal(t *testing.T) {
	tests := []struct {
		name        string
		prefill     []*model.ShortURL
		originalURL string
		want        *model.ShortURL
		wantErr     bool
	}{
		{
			name:        "found",
			prefill:     []*model.ShortURL{{ID: "abc", OriginalURL: "https://ya.ru"}},
			originalURL: "https://ya.ru",
			want:        &model.ShortURL{ID: "abc", OriginalURL: "https://ya.ru"},
			wantErr:     false,
		},
		{
			name:        "not found",
			originalURL: "https://ya.ru",
			want:        nil,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := repository.NewMemStorage()
			for _, u := range tt.prefill {
				_ = s.Create(context.Background(), u)
			}
			got, gotErr := s.GetByOriginal(context.Background(), tt.originalURL)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GetByOriginal failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GetByOriginal succeeded unexpectedly")
			}
			if got == nil && tt.want != nil || got != nil && tt.want == nil {
				t.Errorf("GetByOriginal = %v, want %v", got, tt.want)
			} else if got != nil && tt.want != nil {
				if got.ID != tt.want.ID || got.OriginalURL != tt.want.OriginalURL {
					t.Errorf("GetByOriginal = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
