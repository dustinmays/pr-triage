#!/usr/bin/env bash
# Negative: add a brand-new migration (status A, not M/D) — history not rewritten.
set -euo pipefail
mkdir -p internal/db/migrations
cat > internal/db/migrations/0001_init.sql <<'SQL'
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
SQL
