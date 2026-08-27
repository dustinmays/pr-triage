#!/usr/bin/env bash
# Positive: edit a workflow file under .github/workflows/.
set -euo pipefail
cat > .github/workflows/ci.yml <<'YAML'
name: CI
on: [push]
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
YAML
