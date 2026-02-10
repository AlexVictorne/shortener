package repository

import (
	"context"

	"shortener/internal/model"
)

type Storage interface {
	Create(ctx context.Context, url *model.ShortURL) error
	Get(ctx context.Context, ID string) (*model.ShortURL, error)
	GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error)

	Ping(ctx context.Context) error
	Close() error
}
