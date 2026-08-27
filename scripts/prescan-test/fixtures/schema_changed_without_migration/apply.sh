#!/usr/bin/env bash
# Positive: edit schema.sql without adding any migration.
set -euo pipefail
cat > internal/db/schema.sql <<'SQL'
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
SQL
