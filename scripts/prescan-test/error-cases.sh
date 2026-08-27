#!/usr/bin/env bash
# error-cases.sh — verifies the scanner degrades gracefully: on a runtime
# failure it emits a valid schema_version:1 error document and exits 0, so a
# scanner failure can never fail the build (Layer 2 "reports facts, never
# blocks"). Complements run.sh, which covers successful scans.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
SCANNER="$REPO_ROOT/scripts/pr-prescan.sh"

TMPDIRS=""
cleanup() { local d; for d in $TMPDIRS; do [ -n "$d" ] && rm -rf "$d"; done; }
trap cleanup EXIT INT TERM
die() { echo "error-cases: $*" >&2; exit 1; }

fail=0
check() {
  # check <label> <json-file> <expected pr number>
  local label="$1" out="$2" want_num="$3"
  if ! jq -e . <"$out" >/dev/null 2>&1; then
    echo "FAIL  $label (output is not valid JSON)"; fail=1; return
  fi
  local sv err num
  sv="$(jq -r '.schema_version' "$out")"
  err="$(jq -r '.error // empty' "$out")"
  num="$(jq -r '.pr.number' "$out")"
  if [ "$sv" != "1" ]; then echo "FAIL  $label (schema_version=$sv, want 1)"; fail=1; return; fi
  if [ -z "$err" ]; then echo "FAIL  $label (missing .error)"; fail=1; return; fi
  if [ "$num" != "$want_num" ]; then echo "FAIL  $label (.pr.number=$num, want $want_num)"; fail=1; return; fi
  echo "PASS  $label"
}

# Case 1: `gh pr view` fails -> error document, exit 0.
tmp="$(mktemp -d "${TMPDIR:-/tmp}/prescan-error.XXXXXX")"; TMPDIRS="$TMPDIRS $tmp"
work="$tmp/work"; git init -q "$work"
git -C "$work" config user.email a@b.c; git -C "$work" config user.name t
git -C "$work" commit -q --allow-empty -m base
# A gh stub that always fails on `pr view` (repo view still succeeds).
stub="$tmp/bin"; mkdir -p "$stub"
cat >"$stub/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "repo" ]; then printf '%s\n' "owner/repo"; exit 0; fi
echo "gh stub: forced failure" >&2; exit 1
STUB
chmod +x "$stub/gh"
out="$tmp/out.json"
set +e
( cd "$work"; PATH="$stub:$PATH" "$SCANNER" 4242 --repo owner/repo --remote origin --out "$out" )
rc=$?
set -e
[ "$rc" -eq 0 ] || { echo "FAIL  gh-pr-view-fails (exit $rc, want 0)"; fail=1; }
check "gh-pr-view-fails" "$out" 4242

echo "----"
if [ "$fail" -eq 0 ]; then echo "error-cases: all checks passed"; else echo "error-cases: FAILURES" >&2; exit 1; fi
