#!/usr/bin/env bash
# Positive: add a //nolint directive to a .go file.
set -euo pipefail
cat > main.go <<'EOF'
package main

//nolint:gomnd // magic number is intentional
var maxItems = 42

func main() {
	println("hello")
}
EOF
