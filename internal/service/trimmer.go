package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"shortener/internal/model"
	"shortener/internal/repository"
	"shortener/pkg/generator"
)

var ErrConflict = errors.New("conflict")

type TrimmerService struct {
	storage   repository.Storage
	generator generator.IDGenerator
	baseURL   string
}

func NewTrimmerService(
	storage repository.Storage,
	generator generator.IDGenerator,
	baseURL string,
) *TrimmerService {
	return &TrimmerService{
		storage:   storage,
		generator: generator,
		baseURL:   baseURL,
	}
}

func (s *TrimmerService) TrimURL(ctx context.Context, originalURL string) (string, error) {
	if err := s.validateURL(originalURL); err != nil {
		return "", err
	}

	normalizedURL, err := s.normalizeURL(originalURL)
	if err != nil {
		return "", fmt.Errorf("normalization failed %w", err)
	}

	existingURL, err := s.storage.GetByOriginal(ctx, normalizedURL)
	if err == nil && existingURL != nil {
		return s.buildShortURL(existingURL.ShortURL), ErrConflict
	}

	const maxAttempts = 10
	for i := 0; i < maxAttempts; i++ {
		shortID, err := s.generator.GenerateID()
		if err != nil {
			return "", fmt.Errorf("failed to generate ID: %w", err)
		}

		urlStored := &model.ShortURL{
			ShortURL:    shortID,
			OriginalURL: normalizedURL,
		}

		err = s.storage.Create(ctx, urlStored)
		if err == nil {
			return s.buildShortURL(shortID), nil
		}

		if errors.Is(err, repository.ErrConflict) {
			continue
		}
		return "", fmt.Errorf("storage error: %w", err)
	}

	return "", ErrConflict
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

func (s *TrimmerService) GetOriginalURL(ctx context.Context, shortURL string) (string, error) {
	shortID, err := extractID(shortURL)
	if err != nil {
		return "", err
	}

	urlStored, err := s.storage.Get(ctx, shortID)
	if err != nil {
		return "", fmt.Errorf("storage error: %w", err)
	}

	return urlStored.OriginalURL, nil
}

func (s *TrimmerService) buildShortURL(id string) string {
	u, err := url.JoinPath(s.baseURL, id)
	if err != nil {
		return strings.TrimSuffix(s.baseURL, "/") + "/" + id
	}

	return u
}

func (s *TrimmerService) validateURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("URL cannot be empty")
	}

	if len(rawURL) > 2048 {
		return errors.New("URL is too long")
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL must start with http:// or https://")
	}

	if parsed.Host == "" {
		return errors.New("URL must have host")
	}

	return nil
}

func (s *TrimmerService) normalizeURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	if (parsed.Scheme == "http" && parsed.Port() == "80") || (parsed.Scheme == "https" && parsed.Port() == "443") {
		parsed.Host = parsed.Hostname()
	}

	parsed.Fragment = ""

	return parsed.String(), nil
}

func extractID(shortURL string) (string, error) {
	parts := strings.Split(shortURL, "/")
	if len(parts) == 0 {
		return "", errors.New("invalid short URL format")
	}

	return parts[len(parts)-1], nil
}

func (s *TrimmerService) BatchShorten(ctx context.Context, req []model.BatchRequestItem) ([]model.BatchResponseItem, error) {
	urls := make([]*model.ShortURL, 0, len(req))
	resp := make([]model.BatchResponseItem, 0, len(req))
	for _, item := range req {
		if item.OriginalURL == "" || item.CorrelationID == "" {
			return nil, errors.New("empty original_url or correlation_id")
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

func (s *TrimmerService) BuildShortURL(id string) string {
	return s.buildShortURL(id)
}

func (s *TrimmerService) ValidateURL(rawURL string) error {
	return s.validateURL(rawURL)
}

func (s *TrimmerService) NormalizeURL(rawURL string) (string, error) {
	return s.normalizeURL(rawURL)
}

func (s *TrimmerService) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	return s.storage.GetByOriginal(ctx, originalURL)
}

func (s *TrimmerService) GenerateID() (string, error) {
	return s.generator.GenerateID()
}
