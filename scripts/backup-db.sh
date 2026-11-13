#!/usr/bin/env bash
set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-./backups}"
CONTAINER_NAME="${CONTAINER_NAME:-task_manager_db}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-task_manager}"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_backup_${TIMESTAMP}.dump"

mkdir -p "$BACKUP_DIR"

echo "==> Creating database backup for '${DB_NAME}'..."
docker exec -i "$CONTAINER_NAME" pg_dump -U "$DB_USER" -Fc -d "$DB_NAME" > "$BACKUP_FILE"

FILE_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
echo "==> Backup completed successfully!"
echo "    File: $BACKUP_FILE ($FILE_SIZE)"
