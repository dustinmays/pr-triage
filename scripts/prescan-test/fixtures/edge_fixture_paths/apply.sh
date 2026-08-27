#!/usr/bin/env bash
# Only ADDS scanner test data. Every path here is signal-excluded
# (scripts/prescan-test/fixtures/ or testdata/), yet unfiltered each would trip
# a real signal: the migration .sql files match migration_sql_added, the schema
# copy matches schema_changed_without_migration, and go.mod matches
# dependency_manifest_changed.
set -euo pipefail

mkdir -p scripts/prescan-test/fixtures/demo/_base/internal/db/migrations
printf 'CREATE TABLE t (id INTEGER);\n' > scripts/prescan-test/fixtures/demo/_base/internal/db/migrations/0002_add.sql
printf 'CREATE TABLE t (id INTEGER);\n' > scripts/prescan-test/fixtures/demo/_base/internal/db/schema.sql
printf 'module demo\n\ngo 1.25\n' > scripts/prescan-test/fixtures/demo/_base/go.mod

# A testdata dir outside the fixtures tree still counts as scanner test data.
# It sits under internal/db/ so it also matches MIGRATION_RE unfiltered.
mkdir -p internal/db/testdata
printf 'DROP TABLE users;\n' > internal/db/testdata/dangerous.sql
