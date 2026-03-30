package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"shortener/internal/model"
)

type PgStorage struct {
	db *sql.DB
}

func NewPgStorage(ctx context.Context, dsn string) (*PgStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &PgStorage{db: db}, nil
}

func (s *PgStorage) Create(ctx context.Context, url *model.ShortURL) error {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO short_urls (short_url, original_url)
		VALUES ($1, $2)
		RETURNING uuid
	`, url.ShortURL, url.OriginalURL)

	if err := row.Scan(&url.UUID); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("conflict: %w", ErrConflict)
		}
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (s *PgStorage) Get(ctx context.Context, shortURL string) (*model.ShortURL, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT uuid, short_url, original_url
		FROM short_urls
		WHERE short_url = $1
	`, shortURL)

	var u model.ShortURL
	if err := row.Scan(&u.UUID, &u.ShortURL, &u.OriginalURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	return &u, nil
}

func (s *PgStorage) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT uuid, short_url, original_url
		FROM short_urls
		WHERE original_url = $1
	`, originalURL)

	var u model.ShortURL
	if err := row.Scan(&u.UUID, &u.ShortURL, &u.OriginalURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get by original: %w", err)
	}

	return &u, nil
}

func (s *PgStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PgStorage) Close() error {
	return s.db.Close()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
