#!/usr/bin/env bash
# Negative: add t.Skip() inside a NON-_test.go file.
set -euo pipefail
cat > main.go <<'EOF'
package main

import "testing"

func main() {
	t := &testing.T{}
	t.Skip("skip in main")
	println("hello")
}
EOF
