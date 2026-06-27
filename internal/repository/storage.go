// Package repository предоставляет интерфейс Storage и его реализации:
// MemStorage (in-memory с опциональной файловой персистентностью) и PgStorage (PostgreSQL).
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
	// ErrNotFound возвращается, когда запись с указанным идентификатором не найдена в хранилище.
	ErrNotFound = errors.New("not found")
	// ErrConflict возвращается при попытке создать запись с уже существующим коротким или оригинальным URL.
	ErrConflict = errors.New("conflict")
)

// Storage — общий интерфейс хранилища коротких URL.
// Реализуется MemStorage (in-memory) и PgStorage (PostgreSQL).
type Storage interface {
	// Create сохраняет новую запись. Возвращает ErrConflict при дублировании.
	Create(ctx context.Context, url *model.ShortURL) error
	// BatchCreate атомарно сохраняет несколько записей.
	BatchCreate(ctx context.Context, urls []*model.ShortURL) error
	// Get возвращает запись по короткому идентификатору. Возвращает ErrNotFound, если не найдена.
	Get(ctx context.Context, shortURL string) (*model.ShortURL, error)
	// GetByOriginal возвращает запись по нормализованному оригинальному URL.
	GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error)
	// GetByUserID возвращает все записи, принадлежащие пользователю.
	GetByUserID(ctx context.Context, userID string) ([]*model.ShortURL, error)
	// Close освобождает ресурсы и при необходимости персистирует данные на диск.
	Close() error
	// BatchMarkDeleted помечает указанные короткие URL как удалённые для заданного пользователя.
	BatchMarkDeleted(ctx context.Context, userID string, shortURLs []string) error
	// Stats возвращает общее количество коротких URL и уникальных пользователей в хранилище.
	Stats(ctx context.Context) (urlCount int, userCount int, err error)
}

// MemStorage — потокобезопасное in-memory хранилище с тремя индексами:
// по короткому URL, по оригинальному URL и по идентификатору пользователя.
// При наличии filePath автоматически персистирует данные на диск при закрытии.
type MemStorage struct {
	mu        sync.RWMutex
	urls      map[string]*model.ShortURL // key: shortURL
	index     map[string]string          // key: originalURL, value: shortURL
	userIndex map[string][]string        // key: userID, value: []shortURL

	nextUUID int
	filePath string
}

// ExportAll возвращает снимок всех записей хранилища для сериализации.
func (s *MemStorage) ExportAll() []model.ShortURL {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.ShortURL, 0, len(s.urls))
	for _, v := range s.urls {
		result = append(result, *v)
	}
	return result
}

// ImportAll полностью заменяет содержимое хранилища переданными записями и перестраивает индексы.
func (s *MemStorage) ImportAll(urls []model.ShortURL) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = make(map[string]*model.ShortURL)
	s.index = make(map[string]string)
	s.userIndex = make(map[string][]string)
	s.nextUUID = 1
	for i := range urls {
		u := urls[i]
		s.urls[u.ShortURL] = &u
		s.index[u.OriginalURL] = u.ShortURL
		s.userIndex[u.UserID] = append(s.userIndex[u.UserID], u.ShortURL)
		if u.UUID >= s.nextUUID {
			s.nextUUID = u.UUID + 1
		}
	}
}

// SaveToFile сериализует все записи хранилища в JSON и записывает в указанный файл.
func (s *MemStorage) SaveToFile(filePath string) error {
	return filestorage.SaveToFile(s.ExportAll(), filePath)
}

// LoadFromFile читает JSON-файл и загружает записи в хранилище через ImportAll.
func (s *MemStorage) LoadFromFile(filePath string) error {
	var urls []model.ShortURL
	err := filestorage.LoadFromFile(filePath, &urls)
	if err != nil {
		return err
	}
	s.ImportAll(urls)
	return nil
}

// Stats возвращает количество коротких URL и уникальных пользователей в MemStorage.
func (s *MemStorage) Stats(_ context.Context) (int, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.urls), len(s.userIndex), nil
}

func (s *MemStorage) BatchMarkDeleted(ctx context.Context, userID string, shortURLs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, short := range shortURLs {
		url, ok := s.urls[short]
		if ok && url.UserID == userID {
			url.DeletedFlag = true
		}
	}
	return nil
}

// NewMemStorage создаёт пустое in-memory хранилище без файловой персистентности.
func NewMemStorage() *MemStorage {
	return &MemStorage{
		urls:      make(map[string]*model.ShortURL),
		index:     make(map[string]string),
		userIndex: make(map[string][]string),
		nextUUID:  1,
		filePath:  "",
	}
}

// NewMemStorageWithFile создаёт хранилище с файловой персистентностью.
// Если файл уже существует, данные из него загружаются при инициализации.
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
	s.userIndex[url.UserID] = append(s.userIndex[url.UserID], url.ShortURL)

	return nil
}

func (s *MemStorage) Get(ctx context.Context, shortURL string) (*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *MemStorage) GetByUserID(ctx context.Context, userID string) ([]*model.ShortURL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.userIndex[userID]
	result := make([]*model.ShortURL, 0, len(ids))
	for _, id := range ids {
		if url, ok := s.urls[id]; ok {
			copy := *url
			result = append(result, &copy)
		}
	}
	return result, nil
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

func (s *MemStorage) BatchCreate(ctx context.Context, urls []*model.ShortURL) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, url := range urls {
		if _, exists := s.urls[url.ShortURL]; exists {
			return fmt.Errorf("ShortURL conflict: %w", ErrConflict)
		}
		if _, exists := s.index[url.OriginalURL]; exists {
			return fmt.Errorf("OriginalURL conflict: %w", ErrConflict)
		}
	}

	for _, url := range urls {
		if url.UUID == 0 {
			url.UUID = s.nextUUID
			s.nextUUID++
		}
		s.urls[url.ShortURL] = url
		s.index[url.OriginalURL] = url.ShortURL
		s.userIndex[url.UserID] = append(s.userIndex[url.UserID], url.ShortURL)
	}
	return nil
}
