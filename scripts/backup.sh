#!/usr/bin/env bash
# Saves the whole online database to backups/bosho-<date-time>.dump and checks the file reads back.
# Needs pg_dump and pg_restore version 17 or newer (Supabase runs PostgreSQL 17).
# Run it every night, for example from cron on an always-on computer:
#   30 3 * * *  cd /path/to/restaurant-menu && make backup >> backups/backup.log 2>&1
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; . ./.env; set +a
mkdir -p backups
file="backups/bosho-$(date +%Y-%m-%d-%H%M).dump"
pg_dump --format=custom --no-owner --no-privileges --file="$file" "$DATABASE_URL"
pg_restore --list "$file" > /dev/null
echo "saved and readable: $file"
# Keep the newest 30 backups.
ls -1t backups/bosho-*.dump | tail -n +31 | while read -r old; do rm -- "$old"; done
