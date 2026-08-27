#!/usr/bin/env bash
# Negative: edit a yaml that is NOT a safeguard config file.
set -euo pipefail
cat > config.yaml <<'EOF'
timeout: 60s
retries: 5
EOF
