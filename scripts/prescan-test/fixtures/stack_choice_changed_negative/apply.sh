#!/usr/bin/env bash
# Negative: change only a require dependency line (go version and module name unchanged).
set -euo pipefail
cat > go.mod <<'EOF'
module pr-triage

go 1.24

require github.com/spf13/cobra v1.9.0
EOF
