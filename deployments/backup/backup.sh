#!/bin/sh
# Nightly pg_dump (compressed custom format) into /backups; old dumps pruned.
# Restore: pg_restore -h postgres -U $PGUSER -d $PGDATABASE --clean /backups/<file>
set -eu
while true; do
  file="/backups/movieapp-$(date -u +%Y%m%d-%H%M%S).dump"
  if pg_dump --format=custom --file="$file.tmp"; then
    mv "$file.tmp" "$file"
    echo "backup: wrote $file ($(du -h "$file" | cut -f1))"
  else
    rm -f "$file.tmp"
    echo "backup: pg_dump FAILED" >&2
  fi
  find /backups -name 'movieapp-*.dump' -mtime +"$BACKUP_KEEP_DAYS" -delete
  sleep 86400
done
