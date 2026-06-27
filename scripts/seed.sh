#!/usr/bin/env bash
# Предзаполнить хранилище 10 000 уникальными URL для реалистичной нагрузки на хэш-таблицы MemStorage.
set -e

BASE_URL="${1:-http://localhost:8080}"
COUNT="${2:-10000}"

echo "Seeding $COUNT URLs to $BASE_URL..."
for i in $(seq 1 "$COUNT"); do
  curl -s -X POST \
    -H "Content-Type: text/plain" \
    -d "https://example.com/page/$i" \
    "$BASE_URL/" > /dev/null
done
echo "Done."
