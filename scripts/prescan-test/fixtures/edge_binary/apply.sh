#!/usr/bin/env bash
set -euo pipefail
mkdir -p assets
printf '\0211PNG\r\n\032\n\000\000\000\015IHDR\000\000\000\001' > assets/logo.png
