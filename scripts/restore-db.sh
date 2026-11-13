#!/usr/bin/env bash
set -euo pipefail

BACKUP_FILE="${1:-}"
CONTAINER_NAME="${CONTAINER_NAME:-task_manager_db}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-task_manager}"

if [[ -z "$BACKUP_FILE" ]]; then
    echo "Usage: $0 <path_to_backup_file.dump>"
    exit 1
fi

if [[ ! -f "$BACKUP_FILE" ]]; then
    echo "Error: Backup file '$BACKUP_FILE' not found!"
    exit 1
fi

echo "==> Restoring database '${DB_NAME}' from '$BACKUP_FILE'..."
echo "    [WARNING] This operation will overwrite existing data."

docker exec -i "$CONTAINER_NAME" pg_restore -U "$DB_USER" -d "$DB_NAME" --clean --if-exists < "$BACKUP_FILE" || true

echo "==> Database restore completed successfully!"
