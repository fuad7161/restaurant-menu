#!/usr/bin/env bash
# Restores the newest backup into a throwaway local PostgreSQL (Docker) and counts what came back,
# to prove the backups can really be restored. Nothing online is touched.
set -euo pipefail
cd "$(dirname "$0")/.."
file=$(ls -1t backups/bosho-*.dump | head -1)
echo "checking $file"
name=bosho-restore-check
docker run -d --rm --name "$name" -e POSTGRES_PASSWORD=check postgres:17-alpine > /dev/null
trap 'docker stop "$name" > /dev/null' EXIT
until docker exec "$name" pg_isready -U postgres > /dev/null 2>&1; do sleep 1; done
sleep 2
docker exec -i "$name" pg_restore -U postgres -d postgres --no-owner < "$file"
docker exec "$name" psql -U postgres -tAc \
  "SELECT 'restaurants: '||(SELECT COUNT(*) FROM restaurants)||', orders: '||(SELECT COUNT(*) FROM orders)||', payments: '||(SELECT COUNT(*) FROM payments)"
