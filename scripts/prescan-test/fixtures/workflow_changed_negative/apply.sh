#!/usr/bin/env bash
# Negative: edit a .github/ file NOT under workflows/.
set -euo pipefail
cat > .github/CODEOWNERS <<'EOF'
@ALL /dustin
@team /internal/
EOF
