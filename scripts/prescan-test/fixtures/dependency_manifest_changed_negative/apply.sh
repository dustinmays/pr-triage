#!/usr/bin/env bash
# Negative: only edit a Go source file, no go.mod/go.sum changes.
set -euo pipefail
cat > main.go <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("hello world")
}
EOF
