#!/usr/bin/env bash
set -euo pipefail
mkdir -p internal/big
for i in $(seq 1 60); do
  printf 'package big\n\nfunc F%d() int { return %d }\n' "$i" "$i" > "internal/big/f$i.go"
done
