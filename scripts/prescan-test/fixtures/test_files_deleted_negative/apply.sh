#!/usr/bin/env bash
# Negative: delete a NON-test file (not *_test.go).
set -euo pipefail
git rm -q widget.go
