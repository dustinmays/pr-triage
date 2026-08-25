#!/usr/bin/env bash
# Head change for the test_files_deleted fixture: delete a *_test.go file.
# Run by the harness inside the work tree (cwd == repo root).
set -euo pipefail
git rm -q widget_test.go
