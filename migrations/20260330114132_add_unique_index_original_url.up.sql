-- Добавление уникального индекса на original_url
CREATE UNIQUE INDEX IF NOT EXISTS idx_short_urls_original_url ON short_urls (original_url);
