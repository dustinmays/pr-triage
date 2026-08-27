#!/usr/bin/env bash
# Positive: add t.Skip() inside a _test.go file.
set -euo pipefail
cat > widget_test.go <<'EOF'
package widget

import "testing"

func TestWidget(t *testing.T) {
	t.Skip("flaky")
	if 1+1 != 2 {
		t.Fatal("math is broken")
	}
}
EOF
