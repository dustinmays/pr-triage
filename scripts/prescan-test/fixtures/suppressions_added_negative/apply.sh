#!/usr/bin/env bash
# Negative: add a comment that mentions "lint" but is NOT //nolint.
set -euo pipefail
cat > main.go <<'EOF'
package main

// no lint issues here — already reviewed
func main() {
	println("hello")
}
EOF
