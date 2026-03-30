CREATE TABLE short_urls (
    uuid         SERIAL PRIMARY KEY,
    short_url    TEXT NOT NULL UNIQUE,
    original_url TEXT NOT NULL UNIQUE
);
