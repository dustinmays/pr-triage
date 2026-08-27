#!/usr/bin/env bash
# Positive: add a require line to go.mod.
set -euo pipefail
cat > go.mod <<'EOF'
module pr-triage

go 1.24

require (
	github.com/spf13/cobra v1.8.0
	github.com/mattn/go-sqlite3 v1.14.22
)
EOF
