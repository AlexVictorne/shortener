-- Откат миграции: удаление поля is_deleted из таблицы short_urls
ALTER TABLE short_urls DROP COLUMN is_deleted;