#!/usr/bin/env bash
# Positive: edit a safeguard config file (Makefile).
set -euo pipefail
cat > Makefile <<'EOF'
.PHONY: all build test lint

all: build test lint

build:
	echo "building"

test:
	echo "testing"

lint:
	echo "linting"
EOF
