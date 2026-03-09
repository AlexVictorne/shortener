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
		return s.buildShortURL(existingURL.ID), nil
	}

	const maxAttempts = 10
	for i := 0; i < maxAttempts; i++ {
		shortID, err := s.generator.GenerateID()
		if err != nil {
			return "", fmt.Errorf("failed to generate ID: %w", err)
		}

		urlStored := &model.ShortURL{
			ID:          shortID,
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

	return "", errors.New("failed to generate unique ID: max attempts reached")
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
