#!/usr/bin/env bash
# run.sh — test harness for the deterministic pre-scan scanner.
#
# WHAT IT DOES
#   For each fixture under scripts/prescan-test/fixtures/, it:
#     1. builds a throwaway git repo in a temp dir (a bare "origin" plus a work
#        clone),
#     2. commits the fixture's base files,
#     3. applies the fixture's head change and commits it,
#     4. runs scripts/pr-prescan.sh against that local state, stubbing the `gh`
#        metadata call so no network / real PR is needed,
#     5. normalizes the scanner's JSON (canonical key order, volatile fields
#        such as SHAs / temp paths masked) and diffs it against the fixture's
#        golden JSON.
#
#   All temp state lives under a single mktemp dir removed by an EXIT trap, so a
#   pass, a failure, or a Ctrl-C all leave nothing behind.
#
# USAGE
#   scripts/prescan-test/run.sh            # run every fixture
#   scripts/prescan-test/run.sh NAME ...   # run only the named fixture(s)
#
# A FIXTURE (scripts/prescan-test/fixtures/<name>/) CONTAINS
#   pr.json      what the stubbed `gh pr view` returns (number, title,
#                baseRefName, headRefName, headRefOid, body, statusCheckRollup).
#   _base/       files that exist at the base commit (optional). The leading
#                underscore keeps the Go toolchain from treating fixture source
#                files (e.g. *_test.go) as part of this repo's packages.
#   apply.sh     run inside the work tree to produce the head change.
#   golden.json  the expected, already-normalized scanner output.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
SCANNER="$REPO_ROOT/scripts/pr-prescan.sh"
FIXTURES_DIR="$HERE/fixtures"
STUB_DIR="$HERE/stub"
REPO_SLUG="owner/repo"

# Every temp dir we create is recorded here and removed by the EXIT trap, so a
# pass, a failure, a `die`, or a Ctrl-C all leave nothing behind.
TMPDIRS=""
cleanup() {
  local d
  for d in $TMPDIRS; do
    [ -n "$d" ] && rm -rf "$d"
  done
}
trap cleanup EXIT INT TERM

die() { echo "prescan-test: $*" >&2; exit 1; }

for bin in git jq; do
  command -v "$bin" >/dev/null 2>&1 || die "$bin is required and was not found"
done
[ -x "$SCANNER" ] || die "scanner not found or not executable: $SCANNER"
chmod +x "$STUB_DIR/gh"

# Mask fields that vary run to run: 40-char git SHAs, and the harness temp dir.
# The scanner's output does not currently embed either, but masking here keeps
# goldens stable if future signals start quoting them.
normalize() {
  local tmp="$1"
  jq -S . \
    | sed -E "s/[0-9a-f]{40}/<SHA>/g" \
    | sed "s#${tmp//#/\\#}#<TMP>#g"
}

run_fixture() {
  local name="$1"
  local fdir="$FIXTURES_DIR/$name"
  [ -d "$fdir" ] || die "no such fixture: $name"
  [ -f "$fdir/pr.json" ] || die "$name: missing pr.json"
  [ -f "$fdir/apply.sh" ] || die "$name: missing apply.sh"
  if [ "${PRESCAN_UPDATE_GOLDEN:-}" != "1" ]; then
    [ -f "$fdir/golden.json" ] || die "$name: missing golden.json"
  fi

  local pr_number base_ref
  pr_number="$(jq -r '.number' "$fdir/pr.json")"
  base_ref="$(jq -r '.baseRefName' "$fdir/pr.json")"
  [ -n "$pr_number" ] && [ "$pr_number" != "null" ] || die "$name: pr.json .number missing"
  [ -n "$base_ref" ] && [ "$base_ref" != "null" ] || die "$name: pr.json .baseRefName missing"

  local tmp
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/prescan-test.XXXXXX")"
  TMPDIRS="$TMPDIRS $tmp"

  local origin="$tmp/origin.git" work="$tmp/work"
  git init -q --bare "$origin"
  git init -q "$work"
  git -C "$work" config user.email "harness@example.com"
  git -C "$work" config user.name "Prescan Harness"
  git -C "$work" config commit.gpgsign false
  git -C "$work" remote add origin "$origin"

  # 1. base commit -> refs/heads/<base_ref> on origin.
  if [ -d "$fdir/_base" ]; then
    ( cd "$fdir/_base" && tar cf - . ) | ( cd "$work" && tar xf - )
  fi
  git -C "$work" add -A
  git -C "$work" commit -q --allow-empty -m "base"
  git -C "$work" push -q origin "HEAD:refs/heads/$base_ref"

  # 2. head commit -> refs/pull/<n>/head on origin.
  ( cd "$work" && bash "$fdir/apply.sh" )
  git -C "$work" add -A
  git -C "$work" commit -q --allow-empty -m "head"
  git -C "$work" push -q origin "HEAD:refs/pull/$pr_number/head"

  # 3. run the scanner against the local repo with a stubbed gh.
  local out="$tmp/out.json"
  (
    cd "$work"
    PATH="$STUB_DIR:$PATH" \
    PRESCAN_STUB_PR_JSON="$fdir/pr.json" \
    PRESCAN_STUB_REPO="$REPO_SLUG" \
      "$SCANNER" "$pr_number" --repo "$REPO_SLUG" --remote origin --out "$out"
  ) || die "$name: scanner exited non-zero"

  # 4. normalize and diff against the golden.
  local got="$tmp/got.norm.json" want="$tmp/want.norm.json"
  normalize "$tmp" <"$out" >"$got"

  # PRESCAN_UPDATE_GOLDEN=1 rewrites the golden from the current output. Use it
  # when adding a fixture; review the diff before committing.
  if [ "${PRESCAN_UPDATE_GOLDEN:-}" = "1" ]; then
    cp "$got" "$fdir/golden.json"
    echo "UPDATED  $name (golden.json rewritten)"
    return 0
  fi

  normalize "$tmp" <"$fdir/golden.json" >"$want"

  if diff -u "$want" "$got"; then
    echo "PASS  $name"
    return 0
  else
    echo "FAIL  $name (output differs from golden above; want < / got >)" >&2
    return 1
  fi
}

main() {
  local names="$*"
  if [ -z "$names" ]; then
    names="$(ls -1 "$FIXTURES_DIR")"
  fi
  [ -n "$names" ] || die "no fixtures found in $FIXTURES_DIR"

  local failures=0 total=0
  for name in $names; do
    total=$((total + 1))
    run_fixture "$name" || failures=$((failures + 1))
  done

  echo "----"
  echo "prescan-test: $((total - failures))/$total fixture(s) passed"
  [ "$failures" -eq 0 ] || exit 1
}

main "$@"
