package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"shortener/pkg/filestorage"

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
	mu       sync.Mutex
	urls     map[string]*model.ShortURL // key: shortURL
	index    map[string]string          // key: originalURL, value: shortURL
	nextUUID int
	filePath string
}

func (s *MemStorage) ExportAll() []model.ShortURL {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]model.ShortURL, 0, len(s.urls))
	for _, v := range s.urls {
		result = append(result, *v)
	}
	return result
}

func (s *MemStorage) ImportAll(urls []model.ShortURL) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = make(map[string]*model.ShortURL)
	s.index = make(map[string]string)
	s.nextUUID = 1
	for i := range urls {
		u := urls[i]
		s.urls[u.ShortURL] = &u
		s.index[u.OriginalURL] = u.ShortURL
		if u.UUID >= s.nextUUID {
			s.nextUUID = u.UUID + 1
		}
	}
}

func (s *MemStorage) SaveToFile(filePath string) error {
	return filestorage.SaveToFile(s.ExportAll(), filePath)
}

func (s *MemStorage) LoadFromFile(filePath string) error {
	var urls []model.ShortURL
	err := filestorage.LoadFromFile(filePath, &urls)
	if err != nil {
		return err
	}
	s.ImportAll(urls)
	return nil
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		urls:     make(map[string]*model.ShortURL),
		index:    make(map[string]string),
		nextUUID: 1,
		filePath: "",
	}
}

func NewMemStorageWithFile(filePath string) (*MemStorage, error) {
	s := NewMemStorage()
	s.filePath = filePath
	if _, err := os.Stat(filePath); err == nil {
		err := s.LoadFromFile(filePath)
		if err != nil {
			return nil, err
		}

		fmt.Printf("[storage] Loaded %d rows from %s\n", len(s.ExportAll()), filePath)
	}
	return s, nil
}

func (s *MemStorage) Create(ctx context.Context, url *model.ShortURL) error {
	// Присваиваем UUID, если он не задан
	if url.UUID == 0 {
		url.UUID = s.nextUUID
		s.nextUUID++
	}
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
	var count int
	var saveErr error
	if s.filePath != "" {
		s.mu.Lock()
		export := make([]model.ShortURL, 0, len(s.urls))
		for _, v := range s.urls {
			export = append(export, *v)
		}
		count = len(export)
		s.mu.Unlock()
		saveErr = filestorage.SaveToFile(export, s.filePath)
		if saveErr != nil {
			fmt.Printf("[storage] Error saving %d rows to %s: %v\n", count, s.filePath, saveErr)
		} else {
			fmt.Printf("[storage] Saved %d rows to %s\n", count, s.filePath)
		}
	}

	s.mu.Lock()
	s.urls = nil
	s.index = nil
	s.mu.Unlock()

	if saveErr != nil {
		return saveErr
	}
	return nil
}
