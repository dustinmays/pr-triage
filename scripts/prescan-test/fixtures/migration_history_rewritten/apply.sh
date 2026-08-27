#!/usr/bin/env bash
# Positive: edit an existing migration file (M status → migration_history_rewritten).
set -euo pipefail
cat > internal/db/migrations/0001_init.sql <<'SQL'
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
SQL
