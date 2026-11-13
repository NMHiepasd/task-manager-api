#!/usr/bin/env bash
set -euo pipefail

# Default database connection string if not provided
DB_URL="${1:-${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/task_manager?sslmode=disable}}"

echo "==> Testing database schema migrations against: $DB_URL"

# 1. Apply all UP migrations in alphanumeric order
echo "==> [1/3] Applying all UP migrations in sequence..."
for file in $(ls -1 migrations/*.up.sql | sort); do
    echo "    Applying: $file"
    psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$file" > /dev/null
done
echo "    All UP migrations completed successfully."

# 2. Roll back all DOWN migrations in reverse alphanumeric order
echo "==> [2/3] Rolling back all DOWN migrations in reverse sequence..."
for file in $(ls -1 migrations/*.down.sql | sort -r); do
    echo "    Rolling back: $file"
    psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$file" > /dev/null
done
echo "    All DOWN migrations completed successfully."

# 3. Re-apply all UP migrations to verify clean re-execution
echo "==> [3/3] Re-applying all UP migrations..."
for file in $(ls -1 migrations/*.up.sql | sort); do
    echo "    Re-applying: $file"
    psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$file" > /dev/null
done
echo "==> Schema migration verification completed successfully! Schema is clean and consistent."
