#!/usr/bin/env bash
# Positive: bump go directive version in go.mod.
set -euo pipefail
cat > go.mod <<'EOF'
module pr-triage

go 1.25

require github.com/spf13/cobra v1.8.0
EOF
