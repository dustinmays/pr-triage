#!/usr/bin/env bash
set -euo pipefail
mkdir -p vendor/lib
printf '[submodule "vendor/lib"]\n\tpath = vendor/lib\n\turl = https://example.com/lib.git\n' > .gitmodules
git update-index --add --cacheinfo 160000,0000000000000000000000000000000000000001,vendor/lib
