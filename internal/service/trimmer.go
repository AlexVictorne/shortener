// Package service содержит бизнес-логику сервиса сокращения ссылок.
// Основной тип — TrimmerService, который валидирует, нормализует и сохраняет URL,
// а также предоставляет операции поиска и пакетного удаления.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/pkg/generator"
	"shortener/pkg/middleware"
)

var (
	// ErrNoContent возвращается, когда для пользователя не найдено ни одного URL.
	ErrNoContent = errors.New("no content")
	// ErrConflict возвращается, когда оригинальный URL уже существует в хранилище.
	ErrConflict = errors.New("conflict")
	// ErrURLEmpty возвращается, когда переданный URL пуст или состоит только из пробелов.
	ErrURLEmpty = errors.New("url is empty")
	// ErrURLTooLong возвращается, когда длина URL превышает 2048 символов.
	ErrURLTooLong = errors.New("url is too long")
	// ErrURLInvalidFormat возвращается, когда URL не поддаётся разбору.
	ErrURLInvalidFormat = errors.New("invalid url format")
	// ErrURLInvalidScheme возвращается, когда схема URL отличается от http или https.
	ErrURLInvalidScheme = errors.New("url must start with http or https")
	// ErrURLNoHost возвращается, когда в URL отсутствует хост.
	ErrURLNoHost = errors.New("url must have host")
	// ErrBatchItemEmpty возвращается, когда элемент пакета содержит пустой original_url или correlation_id.
	ErrBatchItemEmpty = errors.New("empty original_url or correlation_id")
	// ErrInvalidShortURLFormat возвращается, когда короткий URL имеет неверный формат.
	ErrInvalidShortURLFormat = errors.New("invalid short url format")
)

// TrimmerService реализует бизнес-логику: валидацию, нормализацию, генерацию и хранение коротких URL.
type TrimmerService struct {
	storage    repository.Storage
	generator  generator.IDGenerator
	baseURL    string
	basePrefix string
}

// NewTrimmerService создаёт TrimmerService с переданными хранилищем, генератором ID и базовым URL.
// baseURL используется как префикс при построении коротких ссылок (например, "http://localhost:8080/").
func NewTrimmerService(
	storage repository.Storage,
	generator generator.IDGenerator,
	baseURL string,
) *TrimmerService {
	return &TrimmerService{
		storage:    storage,
		generator:  generator,
		baseURL:    baseURL,
		basePrefix: strings.TrimSuffix(baseURL, "/") + "/",
	}
}

// TrimURL валидирует, нормализует originalURL, генерирует короткий идентификатор и сохраняет запись.
// Возвращает полный короткий URL (с baseURL-префиксом) или ошибку.
// При конфликте (URL уже существует) дополнительно возвращает ErrConflict вместе с существующим коротким URL.
func (s *TrimmerService) TrimURL(ctx context.Context, originalURL string) (string, error) {
	normalizedURL, err := s.validateAndNormalize(originalURL)
	if err != nil {
		return "", err
	}

	shortID, err := s.generator.GenerateID()
	if err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}

	var userID string
	if v := ctx.Value(middleware.UserIDKeyFunc()); v != nil {
		if s, ok := v.(string); ok {
			userID = s
		}
	}

	urlStored := &model.ShortURL{
		ShortURL:    shortID,
		OriginalURL: normalizedURL,
		UserID:      userID,
	}

	err = s.storage.Create(ctx, urlStored)
	if err == nil {
		return s.buildShortURL(urlStored.ShortURL), nil
	}
	if errors.Is(err, repository.ErrConflict) {
		return s.buildShortURL(urlStored.ShortURL), ErrConflict
	}
	return "", fmt.Errorf("storage error: %w", err)
}

func (s *TrimmerService) checkIDExists(ctx context.Context, shortID string) (bool, error) {
	_, err := s.storage.Get(ctx, shortID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ErrURLDeleted возвращается, когда короткий URL был мягко удалён.
var ErrURLDeleted = errors.New("url deleted")

// GetOriginalURL разрешает короткий идентификатор или полный короткий URL в оригинальный URL.
// Возвращает ErrURLDeleted, если запись помечена удалённой, или ErrNotFound, если id не найден.
func (s *TrimmerService) GetOriginalURL(ctx context.Context, shortURL string) (string, error) {
	shortID, err := extractID(shortURL)
	if err != nil {
		return "", err
	}

	urlStored, err := s.storage.Get(ctx, shortID)
	if err != nil {
		return "", fmt.Errorf("storage error: %w", err)
	}
	if urlStored.DeletedFlag {
		return "", ErrURLDeleted
	}
	return urlStored.OriginalURL, nil
}

func (s *TrimmerService) buildShortURL(id string) string {
	return s.basePrefix + id
}

// validateAndNormalize парсит URL один раз: проверяет корректность и возвращает нормализованную форму.
func (s *TrimmerService) validateAndNormalize(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", ErrURLEmpty
	}
	if len(rawURL) > 2048 {
		return "", ErrURLTooLong
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ErrURLInvalidFormat
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrURLInvalidScheme
	}
	if parsed.Host == "" {
		return "", ErrURLNoHost
	}

	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	if (scheme == "http" && parsed.Port() == "80") || (scheme == "https" && parsed.Port() == "443") {
		parsed.Host = parsed.Hostname()
	}
	parsed.Fragment = ""

	return parsed.String(), nil
}

func (s *TrimmerService) validateURL(rawURL string) error {
	_, err := s.validateAndNormalize(rawURL)
	return err
}

func (s *TrimmerService) normalizeURL(rawURL string) (string, error) {
	return s.validateAndNormalize(rawURL)
}

func extractID(shortURL string) (string, error) {
	parts := strings.Split(shortURL, "/")
	if len(parts) == 0 {
		return "", ErrInvalidShortURLFormat
	}
	return parts[len(parts)-1], nil
}

// BatchShorten принимает список элементов BatchRequestItem, сокращает каждый URL
// (повторно использует существующий короткий ID при конфликте) и возвращает
// список BatchResponseItem с теми же CorrelationID.
func (s *TrimmerService) BatchShorten(ctx context.Context, req []model.BatchRequestItem) ([]model.BatchResponseItem, error) {
	var userID string
	if v := ctx.Value("userID"); v != nil {
		if s, ok := v.(string); ok {
			userID = s
		}
	}

	urls := make([]*model.ShortURL, 0, len(req))
	resp := make([]model.BatchResponseItem, 0, len(req))
	for _, item := range req {
		if item.OriginalURL == "" || item.CorrelationID == "" {
			return nil, ErrBatchItemEmpty
		}
		if err := s.ValidateURL(item.OriginalURL); err != nil {
			return nil, err
		}
		normalized, err := s.NormalizeURL(item.OriginalURL)
		if err != nil {
			return nil, err
		}
		existing, err := s.GetByOriginal(ctx, normalized)
		if err == nil && existing != nil {
			resp = append(resp, model.BatchResponseItem{
				CorrelationID: item.CorrelationID,
				ShortURL:      s.BuildShortURL(existing.ShortURL),
			})
			continue
		}
		shortID, err := s.GenerateID()
		if err != nil {
			return nil, err
		}
		urls = append(urls, &model.ShortURL{
			ShortURL:    shortID,
			OriginalURL: normalized,
			UserID:      userID,
		})
		resp = append(resp, model.BatchResponseItem{
			CorrelationID: item.CorrelationID,
			ShortURL:      s.BuildShortURL(shortID),
		})
	}
	if len(urls) > 0 {
		if err := s.storage.BatchCreate(ctx, urls); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// BuildShortURL формирует полный короткий URL из короткого идентификатора, добавляя baseURL-префикс.
func (s *TrimmerService) BuildShortURL(id string) string {
	return s.buildShortURL(id)
}

// ValidateURL проверяет, что rawURL является валидным http/https URL с непустым хостом и длиной ≤ 2048.
func (s *TrimmerService) ValidateURL(rawURL string) error {
	return s.validateURL(rawURL)
}

// NormalizeURL приводит rawURL к нормализованной форме: схема и хост в нижнем регистре,
// стандартные порты (80/443) убираются, фрагмент удаляется.
func (s *TrimmerService) NormalizeURL(rawURL string) (string, error) {
	return s.normalizeURL(rawURL)
}

// GetByOriginal ищет запись по нормализованному оригинальному URL.
// Возвращает repository.ErrNotFound, если запись не найдена.
func (s *TrimmerService) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	return s.storage.GetByOriginal(ctx, originalURL)
}

// GenerateID делегирует генерацию нового уникального короткого идентификатора генератору.
func (s *TrimmerService) GenerateID() (string, error) {
	return s.generator.GenerateID()
}

// GetURLsByUser возвращает все ненулевые URL, принадлежащие пользователю userID.
// Возвращает ErrNoContent, если у пользователя нет сохранённых записей.
func (s *TrimmerService) GetURLsByUser(ctx context.Context, userID string) ([]model.UserURLResponse, error) {
	urls, err := s.storage.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, ErrNoContent
	}
	resp := make([]model.UserURLResponse, 0, len(urls))
	for _, u := range urls {
		resp = append(resp, model.UserURLResponse{
			ShortURL:    s.buildShortURL(u.ShortURL),
			OriginalURL: u.OriginalURL,
		})
	}
	return resp, nil
}

// MarkURLsDeleted асинхронно помечает переданные короткие идентификаторы как удалённые для userID.
// Метод возвращается немедленно; фактическое обновление хранилища происходит в фоновых горутинах
// с использованием паттерна fan-out/fan-in с размером чанка 100.
func (s *TrimmerService) MarkURLsDeleted(ctx context.Context, userID string, shortURLs []string) error {
	const chunkSize = 100
	if len(shortURLs) == 0 {
		return nil
	}
	go func() {
		// Channel: для передачи чанков shortURLs между FanOut и FanIn
		chunkCh := make(chan []string)

		// FanOut: делим на чанки и отправляем в канал
		go func() {
			for i := 0; i < len(shortURLs); i += chunkSize {
				end := i + chunkSize
				if end > len(shortURLs) {
					end = len(shortURLs)
				}
				chunk := shortURLs[i:end]
				chunkCh <- chunk
			}
			close(chunkCh)
		}()

		// FanIn: читаем чанки и параллельно обновляем
		var wg sync.WaitGroup
		for chunk := range chunkCh {
			wg.Add(1)
			go func(c []string) {
				defer wg.Done()
				_ = s.storage.BatchMarkDeleted(context.Background(), userID, c)
			}(chunk)
		}
		wg.Wait()
	}()
	return nil
}

// Stats возвращает количество сокращенных URL и уникальных пользователей из хранилища.
func (s *TrimmerService) Stats(ctx context.Context) (int, int, error) {
	return s.storage.Stats(ctx)
}
