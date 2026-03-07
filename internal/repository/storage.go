package repository

import (
	"context"
	"errors"
	"sync"

	"shortener/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type MemStorage struct {
	mu    sync.RWMutex
	urls  map[string]*model.ShortURL
	index map[string]string
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		urls:  make(map[string]*model.ShortURL),
		index: make(map[string]string),
	}
}

func (s *MemStorage) Create(ctx context.Context, url *model.ShortURL) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[url.ID]; exists {
		return ErrConflict
	}

	if _, exists := s.index[url.OriginalURL]; exists {
		return ErrConflict
	}

	s.urls[url.ID] = url
	s.index[url.OriginalURL] = url.ID

	return nil
}

func (s *MemStorage) Get(ctx context.Context, ID string) (*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	url, exists := s.urls[ID]
	if !exists {
		return nil, ErrNotFound
	}

	urlCopy := *url

	return &urlCopy, nil
}

func (s *MemStorage) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	shortID, exists := s.index[originalURL]
	if !exists {
		return nil, ErrNotFound
	}

	return s.Get(ctx, shortID)
}

func (s *MemStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = nil
	s.index = nil

	return nil
}
