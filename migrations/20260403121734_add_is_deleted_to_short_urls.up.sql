-- Добавление поля is_deleted в таблицу short_urls для поддержки soft delete
ALTER TABLE short_urls ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT FALSE;