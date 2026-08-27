#!/usr/bin/env bash
# Positive: add a //go:generate directive to a .go file.
set -euo pipefail
cat > main.go <<'EOF'
package main

//go:generate stringer -type=Level
func main() {
	println("hello")
}
EOF
