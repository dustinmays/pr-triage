#!/usr/bin/env bash
# Negative: add a .sql file outside any migration directory.
set -euo pipefail
mkdir -p scripts
cat > scripts/seed.sql <<'SQL'
INSERT INTO users (name) VALUES ('seed');
SQL
