-- Добавление поля user_id
ALTER TABLE short_urls ADD COLUMN user_id TEXT NOT NULL DEFAULT '';