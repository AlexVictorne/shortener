package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"shortener/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Storage interface {
	Create(ctx context.Context, url *model.ShortURL) error
	Get(ctx context.Context, shortURL string) (*model.ShortURL, error)
	GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error)

	Close() error
}

type MemStorage struct {
	mu    sync.Mutex
	urls  map[string]*model.ShortURL // key: shortURL
	index map[string]string          // key: originalURL, value: shortURL
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		urls:  make(map[string]*model.ShortURL),
		index: make(map[string]string),
	}
}

func (s *MemStorage) Create(ctx context.Context, url *model.ShortURL) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context error in Create: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[url.ShortURL]; exists {
		return fmt.Errorf("ShortURL conflict: %w", ErrConflict)
	}

	if _, exists := s.index[url.OriginalURL]; exists {
		return fmt.Errorf("OriginalURL conflict: %w", ErrConflict)
	}

	s.urls[url.ShortURL] = url
	s.index[url.OriginalURL] = url.ShortURL

	return nil
}

func (s *MemStorage) Get(ctx context.Context, shortURL string) (*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	url, exists := s.urls[shortURL]
	if !exists {
		return nil, fmt.Errorf("ShortURL not found: %w", ErrNotFound)
	}

	urlCopy := *url

	return &urlCopy, nil
}

func (s *MemStorage) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	shortURL, exists := s.index[originalURL]
	if !exists {
		return nil, fmt.Errorf("originalURL not found: %w", ErrNotFound)
	}

	url, exists := s.urls[shortURL]
	if !exists {
		return nil, fmt.Errorf("shortURL not found: %w", ErrNotFound)
	}

	urlCopy := *url
	return &urlCopy, nil
}

func (s *MemStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = nil
	s.index = nil

	return nil
}
