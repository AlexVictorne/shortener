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
			url:     &model.ShortURL{ShortURL: "abc", OriginalURL: "https://ya.ru"},
			wantErr: false,
		},
		{
			name:    "conflict by ID",
			url:     &model.ShortURL{ShortURL: "abc", OriginalURL: "https://ya.ru"},
			prefill: []*model.ShortURL{{ShortURL: "abc", OriginalURL: "https://yandex.ru"}},
			wantErr: true,
		},
		{
			name:    "conflict by OriginalURL",
			url:     &model.ShortURL{ShortURL: "def", OriginalURL: "https://ya.ru"},
			prefill: []*model.ShortURL{{ShortURL: "abc", OriginalURL: "https://ya.ru"}},
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
			prefill: []*model.ShortURL{{ShortURL: "abc", OriginalURL: "https://ya.ru"}},
			ID:      "abc",
			want:    &model.ShortURL{ShortURL: "abc", OriginalURL: "https://ya.ru"},
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
				if got.ShortURL != tt.want.ShortURL || got.OriginalURL != tt.want.OriginalURL {
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
			prefill:     []*model.ShortURL{{ShortURL: "abc", OriginalURL: "https://ya.ru"}},
			originalURL: "https://ya.ru",
			want:        &model.ShortURL{ShortURL: "abc", OriginalURL: "https://ya.ru"},
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
				if got.ShortURL != tt.want.ShortURL || got.OriginalURL != tt.want.OriginalURL {
					t.Errorf("GetByOriginal = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMemStorage_UUID_AutoIncrement(t *testing.T) {
	s := repository.NewMemStorage()
	url1 := &model.ShortURL{ShortURL: "a1", OriginalURL: "https://ya.ru"}
	url2 := &model.ShortURL{ShortURL: "a2", OriginalURL: "https://ya2.ru"}
	url3 := &model.ShortURL{ShortURL: "a3", OriginalURL: "https://ya3.ru"}

	if err := s.Create(context.Background(), url1); err != nil {
		t.Fatalf("Create url1 failed: %v", err)
	}
	if err := s.Create(context.Background(), url2); err != nil {
		t.Fatalf("Create url2 failed: %v", err)
	}
	if err := s.Create(context.Background(), url3); err != nil {
		t.Fatalf("Create url3 failed: %v", err)
	}

	if url1.UUID == 0 || url2.UUID == 0 || url3.UUID == 0 {
		t.Error("UUID should be set and non-zero")
	}
	if url1.UUID == url2.UUID || url2.UUID == url3.UUID || url1.UUID == url3.UUID {
		t.Error("UUIDs should be unique for each ShortURL")
	}
	if !(url1.UUID < url2.UUID && url2.UUID < url3.UUID) {
		t.Error("UUIDs should increment with each new ShortURL")
	}
}

// TestMemStorage_Stats проверяет корректный подсчет URL и уникальных пользователей.
func TestMemStorage_Stats(t *testing.T) {
	tests := []struct {
		name      string
		records   []*model.ShortURL
		wantURLs  int
		wantUsers int
	}{
		{
			name:      "empty storage",
			wantURLs:  0,
			wantUsers: 0,
		},
		{
			name: "one url one user",
			records: []*model.ShortURL{
				{ShortURL: "a1", OriginalURL: "https://example.com", UserID: "user1"},
			},
			wantURLs:  1,
			wantUsers: 1,
		},
		{
			name: "multiple urls same user",
			records: []*model.ShortURL{
				{ShortURL: "a1", OriginalURL: "https://example.com", UserID: "user1"},
				{ShortURL: "a2", OriginalURL: "https://example.org", UserID: "user1"},
				{ShortURL: "a3", OriginalURL: "https://example.net", UserID: "user1"},
			},
			wantURLs:  3,
			wantUsers: 1,
		},
		{
			name: "multiple urls different users",
			records: []*model.ShortURL{
				{ShortURL: "a1", OriginalURL: "https://example.com", UserID: "user1"},
				{ShortURL: "a2", OriginalURL: "https://example.org", UserID: "user2"},
				{ShortURL: "a3", OriginalURL: "https://example.net", UserID: "user1"},
			},
			wantURLs:  3,
			wantUsers: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := repository.NewMemStorage()
			ctx := context.Background()
			for _, r := range tt.records {
				if err := s.Create(ctx, r); err != nil {
					t.Fatalf("Create failed: %v", err)
				}
			}
			gotURLs, gotUsers, err := s.Stats(ctx)
			if err != nil {
				t.Fatalf("Stats returned error: %v", err)
			}
			if gotURLs != tt.wantURLs {
				t.Errorf("urlCount = %d, want %d", gotURLs, tt.wantURLs)
			}
			if gotUsers != tt.wantUsers {
				t.Errorf("userCount = %d, want %d", gotUsers, tt.wantUsers)
			}
		})
	}
}
