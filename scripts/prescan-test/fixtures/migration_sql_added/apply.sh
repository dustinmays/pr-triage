#!/usr/bin/env bash
# Positive: add a new migration file to trigger migration_sql_added.
set -euo pipefail
mkdir -p internal/db/migrations
cat > internal/db/migrations/0002_add_table.sql <<'SQL'
CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT);
SQL
