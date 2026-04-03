package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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
	if err := ApplyMigrations(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return &PgStorage{db: db}, nil
}

func (s *PgStorage) Create(ctx context.Context, url *model.ShortURL) error {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO short_urls (short_url, original_url, user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (original_url) DO UPDATE SET original_url = EXCLUDED.original_url
		RETURNING uuid, short_url
	`, url.ShortURL, url.OriginalURL, url.UserID)

	var existing model.ShortURL
	if err := row.Scan(&existing.UUID, &existing.ShortURL); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	if existing.ShortURL != url.ShortURL {
		url.UUID = existing.UUID
		url.ShortURL = existing.ShortURL
		return fmt.Errorf("conflict: %w", ErrConflict)
	}

	url.UUID = existing.UUID
	return nil
}

func (s *PgStorage) Get(ctx context.Context, shortURL string) (*model.ShortURL, error) {
	row := s.db.QueryRowContext(ctx, `
	       SELECT uuid, short_url, original_url, user_id, is_deleted
	       FROM short_urls
	       WHERE short_url = $1
       `, shortURL)

	var u model.ShortURL
	if err := row.Scan(&u.UUID, &u.ShortURL, &u.OriginalURL, &u.UserID, &u.DeletedFlag); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	return &u, nil
}

func (s *PgStorage) GetByOriginal(ctx context.Context, originalURL string) (*model.ShortURL, error) {
	row := s.db.QueryRowContext(ctx, `
	       SELECT uuid, short_url, original_url, user_id, is_deleted
	       FROM short_urls
	       WHERE original_url = $1
       `, originalURL)

	var u model.ShortURL
	if err := row.Scan(&u.UUID, &u.ShortURL, &u.OriginalURL, &u.UserID, &u.DeletedFlag); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("get by original: %w", err)
	}

	return &u, nil
}

func (s *PgStorage) GetByUserID(ctx context.Context, userID string) ([]*model.ShortURL, error) {
	rows, err := s.db.QueryContext(ctx, `
	       SELECT uuid, short_url, original_url, user_id, is_deleted
	       FROM short_urls
	       WHERE user_id = $1
       `, userID)
	if err != nil {
		return nil, fmt.Errorf("get by user_id: %w", err)
	}
	defer rows.Close()
	var result []*model.ShortURL
	for rows.Next() {
		var u model.ShortURL
		if err := rows.Scan(&u.UUID, &u.ShortURL, &u.OriginalURL, &u.UserID, &u.DeletedFlag); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return result, nil
}

func (s *PgStorage) BatchMarkDeleted(ctx context.Context, userID string, shortURLs []string) error {
	if len(shortURLs) == 0 {
		return nil
	}

	placeholders := make([]string, len(shortURLs))
	args := make([]interface{}, 0, len(shortURLs)+1)
	args = append(args, userID)
	for i, url := range shortURLs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, url)
	}
	query := fmt.Sprintf(`UPDATE short_urls SET is_deleted = TRUE WHERE user_id = $1 AND short_url IN (%s)`, strings.Join(placeholders, ","))
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch mark deleted: %w", err)
	}
	return nil
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

func ApplyMigrations(db *sql.DB, migrationsDir string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsDir,
		"postgres", driver,
	)
	if err != nil {
		return err
	}

	// Применяем миграции
	err = m.Up()
	if err == nil {
		log.Println("[migrate] DB up successfully")
		return nil
	}
	if errors.Is(err, migrate.ErrNoChange) {
		ver, dirty, verr := m.Version()
		if verr != nil {
			log.Printf("[migrate] DB actual, but version incorrect %v", verr)
		} else {
			log.Printf("[migrate] DB actual, current version: %d (dirty=%v)", ver, dirty)
		}
		return nil
	}
	return err
}

func (s *PgStorage) BatchCreate(ctx context.Context, urls []*model.ShortURL) error {
	if len(urls) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	valueStrings := make([]string, 0, len(urls))
	valueArgs := make([]interface{}, 0, len(urls)*3)
	for i, url := range urls {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d)", i*3+1, i*3+2, i*3+3))
		valueArgs = append(valueArgs, url.ShortURL, url.OriginalURL, url.UserID)
	}
	query := "INSERT INTO short_urls (short_url, original_url, user_id) VALUES " +
		strings.Join(valueStrings, ",") +
		" RETURNING uuid, short_url"
	rows, err := tx.QueryContext(ctx, query, valueArgs...)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("conflict: %w", ErrConflict)
		}
		return fmt.Errorf("batch create: %w", err)
	}
	defer rows.Close()
	uuidMap := make(map[string]int)
	for rows.Next() {
		var uuid int
		var short string
		if err := rows.Scan(&uuid, &short); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		uuidMap[short] = uuid
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}
	for _, url := range urls {
		if id, ok := uuidMap[url.ShortURL]; ok {
			url.UUID = id
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
