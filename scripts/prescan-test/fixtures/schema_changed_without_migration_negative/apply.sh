#!/usr/bin/env bash
# Negative: edit schema.sql AND add a migration (migration suppresses the signal).
set -euo pipefail
cat > internal/db/schema.sql <<'SQL'
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
SQL
mkdir -p internal/db/migrations
cat > internal/db/migrations/0002_add_email.sql <<'SQL'
ALTER TABLE users ADD COLUMN email TEXT;
SQL
