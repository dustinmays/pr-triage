#!/usr/bin/env bash
# pr-prescan.sh — deterministic pre-scan of a pull request (Go / SwiftBar tailored).
#
# WHAT THIS IS
#   The scan emits FACTS ONLY. No severity. No score. No verdict. It never says a
#   change is risky, destructive, or safe. A later triage agent reads this JSON and
#   assigns meaning. Keep it that way when you edit this file.
#
# USAGE
#   scripts/pr-prescan.sh <pr-number> [options]
#
#   Options:
#     --repo <owner/name>   Repository to scan. Default: the repo `gh` resolves here.
#     --remote <name>       Git remote to fetch from. Default: origin.
#     --out <file>          Write JSON to this file. Default: stdout.
#     -h, --help            Print this help.
#
#   Examples:
#     scripts/pr-prescan.sh 60
#     scripts/pr-prescan.sh 60 --repo dustinmays/pr-triage --out /tmp/scan.json
#     scripts/pr-prescan.sh 60 | jq '.signals[] | select(.present)'
#
# REQUIREMENTS
#   bash, git, gh (authenticated), jq. Run it from inside a clone of the repository.
#   The script fetches the PR head and the base branch from the remote. It creates no
#   refs, checks nothing out, and does not touch your working tree.
#
# HOW IT PICKS THE DIFF
#   BASE_SHA = git merge-base <base branch tip> <PR head>. The diff is BASE_SHA...HEAD.
#
# CONVENTIONS IN THE OUTPUT
#   - Every signal object has exactly three keys: id, present, evidence.
#   - All eleven signals always appear, so the agent sees what was checked.
#   - A "chunk" key appears only when pr.target_kind is "chunk_completion". It
#     names each pull request merged into the chunk branch, the issues it
#     closed, and whether it currently carries needs-owner-review.
#   - evidence[].line is null when the fact is about a whole file, not a line.
#   - A detail that starts with "+ " or "- " quotes an added or a removed diff line.
#     A detail with no marker is read from a file in the PR head, not from the diff.
#   - evidence[].detail quotes raw text from the diff or from a file in the diff.
#   - Tabs inside quoted text become single spaces. The scan is line-based.
#   - notes holds scanner-level facts only.

set -euo pipefail

readonly SCHEMA_VERSION=1

usage() {
  awk 'NR > 1 { if (/^#/) { sub(/^# ?/, ""); print } else { exit } }' "$0"
}

die() {
  echo "pr-prescan: $*" >&2
  exit 1
}

# Emit a minimal but valid schema_version:1 error document and exit 0.
# Used for runtime failures (can't read the PR, can't fetch a ref, no merge
# base): the scan reports facts and never fails the build, so a failure is
# reported AS a document, not as a non-zero exit. Requires jq (already checked).
emit_error() {
  local msg="$1"
  echo "pr-prescan: $msg (emitting error document)" >&2
  local doc
  doc="$(jq -n \
    --argjson number "${PR_NUMBER:-0}" \
    --arg title "${PR_TITLE:-unknown}" \
    --arg base "${BASE_REF:-unknown}" \
    --arg head "${HEAD_REF:-unknown}" \
    --arg error "$msg" \
    '{ schema_version: 1,
       pr: { number: $number, title: $title, base: $base, head: $head },
       ci: { status: "none", failing_checks: [] },
       stack: {},
       diff: {},
       signals: [],
       notes: [],
       error: $error }')"
  if [ -n "${OUT:-}" ]; then
    printf '%s\n' "$doc" >"$OUT"
    echo "pr-prescan: wrote $OUT" >&2
  else
    printf '%s\n' "$doc"
  fi
  exit 0
}

# ---------------------------------------------------------------- arguments ---

PR_NUMBER=""
REPO=""
REMOTE="origin"
OUT=""

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --repo) REPO="${2:-}"; shift 2 ;;
    --remote) REMOTE="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    -*) die "unknown option: $1" ;;
    *)
      [ -z "$PR_NUMBER" ] || die "give exactly one PR number"
      PR_NUMBER="$1"; shift ;;
  esac
done

case "$PR_NUMBER" in
  ''|*[!0-9]*) usage >&2; die "a PR number is required" ;;
esac

for bin in git gh jq; do
  command -v "$bin" >/dev/null 2>&1 || die "$bin is required and was not found"
done

git rev-parse --git-dir >/dev/null 2>&1 || die "run this from inside a git clone"

# ------------------------------------------------------------------ scratch ---

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/ev"

NOTES="$TMP/notes.txt"
: >"$NOTES"

note() { printf '%s\n' "$*" >>"$NOTES"; }

# grep exits 1 when nothing matches. Under `set -o pipefail` that would end the run.
filter() { grep -E "$1" || true; }

# Evidence rows are TSV: file, line, detail. An empty line field becomes null.
ev() {
  local signal="$1" file="$2" line="$3" detail="$4"
  detail="$(printf '%s' "$detail" | tr '\t' ' ')"
  printf '%s\t%s\t%s\n' "$file" "$line" "$detail" >>"$TMP/ev/$signal"
}

evidence_json() {
  local f="$TMP/ev/$1"
  if [ -s "$f" ]; then
    awk '!seen[$0]++' "$f" | jq -R -s '
      split("\n") | map(select(length > 0)) | map(split("\t")) |
      map({ file: .[0],
            line: (if (.[1] // "") == "" then null else (.[1] | tonumber) end),
            detail: (.[2] // "") })'
  else
    echo '[]'
  fi
}

# ------------------------------------------------------------- PR metadata ----

if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)" ||
    emit_error "could not resolve the repository; pass --repo owner/name"
fi

gh pr view "$PR_NUMBER" --repo "$REPO" \
  --json number,title,baseRefName,headRefName,headRefOid,body,statusCheckRollup \
  >"$TMP/pr.json" || emit_error "could not read PR #$PR_NUMBER in $REPO"

PR_TITLE="$(jq -r '.title' "$TMP/pr.json")"
BASE_REF="$(jq -r '.baseRefName' "$TMP/pr.json")"
HEAD_REF="$(jq -r '.headRefName' "$TMP/pr.json")"

case "$BASE_REF" in
  chunk/*) TARGET_KIND="implementation" ;;
  main)    TARGET_KIND="chunk_completion" ;;
  *)
    TARGET_KIND="implementation"
    note "base ref \"$BASE_REF\" is neither chunk/** nor main; target_kind set to implementation"
    ;;
esac

# Collect referenced issue numbers as integers
jq -r '.body // ""' "$TMP/pr.json" \
  | grep -oE '#[0-9]+' \
  | tr -d '#' \
  | sort -n -u >"$TMP/issue_refs.txt" || true
ISSUE_REFS="$(jq -R -s 'split("\n") | map(select(length > 0) | tonumber)' "$TMP/issue_refs.txt")"

# ------------------------------------------------------------------- CI ------

CI_JSON="$(jq -c '
  (.statusCheckRollup // []) as $c
  | [ $c[] | {
        name: (.name // .context // "unknown"),
        state: ((.conclusion // .state // .status // "") | ascii_downcase),
        status: ((.status // "") | ascii_downcase)
      } ] as $checks
  | ($checks | map(select(.state | IN("failure","timed_out","cancelled","action_required","error","startup_failure"))) | map(.name)) as $failing
  | ($checks | map(select((.status | IN("queued","in_progress","waiting","pending"))
                          or (.state | IN("pending","expected")))) | length) as $running
  | {
      status: (
        if ($checks | length) == 0 then "none"
        elif ($failing | length) > 0 then "failing"
        elif $running > 0 then "pending"
        else "passing" end),
      failing_checks: $failing
    }' "$TMP/pr.json")"

# -------------------------------------------------------------- chunk block --
CHUNK_JSON=null
if [ "$TARGET_KIND" = "chunk_completion" ]; then
  gh pr list --repo "$REPO" --base "$HEAD_REF" --state merged --limit 500 \
    --json number,title,body,labels >"$TMP/chunk_prs.json" ||
    emit_error "could not list pull requests merged into $HEAD_REF"

  CHUNK_JSON="$(jq -c --arg branch "$HEAD_REF" '
    {
      branch: $branch,
      merged_prs: [ .[] | {
        number: .number,
        title: .title,
        issue_refs: ( (.body // "") | [scan("#[0-9]+")] | map(ltrimstr("#") | tonumber) | unique ),
        needs_owner_review: ( [ .labels[].name ] | index("needs-owner-review") != null )
      } ]
    }' "$TMP/chunk_prs.json")"
fi

# -------------------------------------------------------------- fetch diff ---

git fetch --quiet --no-tags "$REMOTE" "refs/pull/$PR_NUMBER/head" ||
  emit_error "could not fetch refs/pull/$PR_NUMBER/head from $REMOTE"
HEAD_SHA="$(git rev-parse FETCH_HEAD)"

git fetch --quiet --no-tags "$REMOTE" "refs/heads/$BASE_REF" ||
  emit_error "could not fetch base branch $BASE_REF from $REMOTE"
BASE_TIP="$(git rev-parse FETCH_HEAD)"

BASE_SHA="$(git merge-base "$BASE_TIP" "$HEAD_SHA")" ||
  emit_error "no merge base between $BASE_REF and the PR head"

# Paths that are scanner TEST DATA, not real repo changes: exclude them from
# signal evaluation so a PR that merely adds fixtures/testdata does not trip
# signals. The diff/numstat block is intentionally NOT filtered — these are
# still real file changes and should show in the diff counts.
readonly SIGNAL_EXCLUDE_RE='^scripts/prescan-test/fixtures/|(^|/)testdata/'

git diff --name-status --find-renames "$BASE_SHA" "$HEAD_SHA" >"$TMP/status.tsv"
awk -F'\t' -v re="$SIGNAL_EXCLUDE_RE" '
  { p = ($3 != "" ? $3 : $2) } p !~ re' "$TMP/status.tsv" >"$TMP/status.tsv.f" && mv "$TMP/status.tsv.f" "$TMP/status.tsv"
git diff --numstat "$BASE_SHA" "$HEAD_SHA" >"$TMP/numstat.tsv"
git diff -U0 --no-color --find-renames "$BASE_SHA" "$HEAD_SHA" >"$TMP/diff.txt"

awk -F'\n' '
  /^\+\+\+ / { p = substr($0, 5); sub(/^b\//, "", p); newpath = p; next }
  /^--- /    { p = substr($0, 5); sub(/^a\//, "", p); oldpath = p; next }
  /^@@ / {
    if (match($0, /-[0-9]+(,[0-9]+)?/)) {
      s = substr($0, RSTART + 1, RLENGTH - 1); split(s, a, ","); oldln = a[1] + 0
    }
    rest = substr($0, RSTART + RLENGTH)
    if (match(rest, /\+[0-9]+(,[0-9]+)?/)) {
      s = substr(rest, RSTART + 1, RLENGTH - 1); split(s, b, ","); newln = b[1] + 0
    }
    next
  }
  /^\\/ { next }
  /^\+/ { gsub(/\t/, " "); print "+\t" newpath "\t" newln "\t" substr($0, 2); newln++; next }
  /^-/  { gsub(/\t/, " "); print "-\t" oldpath "\t" oldln "\t" substr($0, 2); oldln++; next }
' "$TMP/diff.txt" >"$TMP/lines.tsv"

grep -E '^\+' "$TMP/lines.tsv" | cut -f2- >"$TMP/added.tsv" || true
grep -E '^-' "$TMP/lines.tsv" | cut -f2- >"$TMP/removed.tsv" || true
touch "$TMP/added.tsv" "$TMP/removed.tsv"

for tsv in added removed; do
  awk -F'\t' -v re="$SIGNAL_EXCLUDE_RE" '$1 !~ re' "$TMP/$tsv.tsv" >"$TMP/$tsv.tsv.f" && mv "$TMP/$tsv.tsv.f" "$TMP/$tsv.tsv"
done

added_matching() {
  awk -F'\t' -v pre="$1" -v cre="$2" '$1 ~ pre && $3 ~ cre { print $1 "\t" $2 "\t" $3 }' "$TMP/added.tsv"
}

removed_matching() {
  awk -F'\t' -v pre="$1" -v cre="$2" '$1 ~ pre && $3 ~ cre { print $1 "\t" $2 "\t" $3 }' "$TMP/removed.tsv"
}

first_change() {
  awk -F'\t' -v want="$1" '
    $2 == want { c = $4; sub(/^[[:space:]]+/, "", c); print $2 "\t" $3 "\t" $1 " " c; exit }
  ' "$TMP/lines.tsv"
}

files_with_status() {
  awk -F'\t' -v want="$1" '$1 ~ want { print ($3 != "" ? $3 : $2) }' "$TMP/status.tsv"
}

deleted_files() {
  awk -F'\t' '$1 == "D" { print $2 }' "$TMP/status.tsv"
}

changed_files() {
  awk -F'\t' '{ print ($3 != "" ? $3 : $2) }' "$TMP/status.tsv"
}

added_files() {
  awk -F'\t' '$1 == "A" { print $2 }' "$TMP/status.tsv"
}

# ---------------------------------------------------------- stack detection ---

git ls-tree -r --name-only "$HEAD_SHA" >"$TMP/tree.txt"
git ls-tree -r --name-only "$BASE_SHA" >"$TMP/tree_base.txt"

shallowest() {
  awk '
    { line = $0; depth = gsub(/\//, "/")
      if (best == "" || depth < best_depth) { best_depth = depth; best = line } }
    END { if (best != "") print best }'
}

find_file_in() {
  filter "$2" <"$1" | shallowest
}

find_file() {
  find_file_in "$TMP/tree.txt" "$1"
}

show_head() { git show "$HEAD_SHA:$1" 2>/dev/null || true; }
show_base() { git show "$BASE_SHA:$1" 2>/dev/null || true; }

# Inspect Go stack
GO_MOD_PATH="$(find_file '(^|/)go\.mod$')"

FRAMEWORK=null
ORM=null
PACKAGE_MANAGER=null
LINTER=null

detect_go_stack() {
  local tree="$1" show="$2"
  local mod_path linter_path mod_content go_ver mod_name framework=null orm=null pm=null linter=null

  mod_path="$(find_file_in "$tree" '(^|/)go[.]mod$')"
  linter_path="$(find_file_in "$tree" '(^|/)[.]golangci[.]ya?ml$')"

  if [ -n "$mod_path" ]; then
    pm='"go"'
    mod_content="$("$show" "$mod_path")"
    go_ver="$(printf '%s\n' "$mod_content" | grep -E '^[[:space:]]*go[[:space:]]+' | head -1 | awk '{print $2}')"
    mod_name="$(printf '%s\n' "$mod_content" | grep -E '^[[:space:]]*module[[:space:]]+' | head -1 | awk '{print $2}')"
    if printf '%s\n' "$mod_content" | grep -q 'github.com/go-chi/chi'; then
      framework='"chi"'
    elif printf '%s\n' "$mod_content" | grep -q 'github.com/gin-gonic/gin'; then
      framework='"gin"'
    elif printf '%s\n' "$mod_content" | grep -q 'github.com/charmbracelet/bubbletea'; then
      framework='"bubbletea"'
    fi
  fi

  if [ -n "$linter_path" ]; then
    linter='"golangci-lint"'
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$framework" "$orm" "$pm" "$linter" "${go_ver:-}" "${mod_name:-}"
}

IFS=$'\t' read -r FRAMEWORK ORM PACKAGE_MANAGER LINTER HEAD_GO_VER HEAD_MOD_NAME \
  <<<"$(detect_go_stack "$TMP/tree.txt" show_head)"
# BASE_ORM and BASE_PACKAGE_MANAGER are positional placeholders needed to
# consume the tab-separated columns in order; only the other base fields are
# compared against HEAD below (orm/package_manager aren't diffed for the base).
# shellcheck disable=SC2034
IFS=$'\t' read -r BASE_FRAMEWORK BASE_ORM BASE_PACKAGE_MANAGER BASE_LINTER BASE_GO_VER BASE_MOD_NAME \
  <<<"$(detect_go_stack "$TMP/tree_base.txt" show_base)"

# ============================================================== the signals ===

SIGNAL_IDS="migration_sql_added migration_history_rewritten schema_changed_without_migration \
dependency_manifest_changed install_execution_allowed test_files_deleted tests_skipped_added \
safeguard_config_changed suppressions_added workflow_changed stack_choice_changed"

for s in $SIGNAL_IDS; do : >"$TMP/ev/$s"; done

DESTRUCTIVE_SQL='DROP TABLE|DROP COLUMN|DROP CONSTRAINT|TRUNCATE|DELETE FROM|ALTER COLUMN .* SET NOT NULL|ALTER COLUMN .* TYPE|RENAME'

# --- 1. migration_sql_added ---------------------------------------------------
MIGRATION_RE='(^|/)(internal/db/migrations/|internal/db/|migrations/|db/migrations/).*[.]sql$'
ADDED_MIGRATIONS="$TMP/added_migrations.txt"
: >"$ADDED_MIGRATIONS"

added_files | filter "$MIGRATION_RE" >"$ADDED_MIGRATIONS"
while IFS= read -r f; do
  [ -n "$f" ] || continue
  first="$(show_head "$f" | { grep -n -m1 -E '[^[:space:]]' || true; })"
  if [ -n "$first" ]; then
    ev migration_sql_added "$f" "${first%%:*}" "${first#*:}"
  fi
  show_head "$f" | { grep -inE "$DESTRUCTIVE_SQL" || true; } | while IFS= read -r hit; do
    ev migration_sql_added "$f" "${hit%%:*}" "${hit#*:}"
  done
done <"$ADDED_MIGRATIONS"

# --- 2. migration_history_rewritten -------------------------------------------
files_with_status '^(M|D)$' | filter "$MIGRATION_RE" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  row="$(first_change "$f")"
  if [ -n "$row" ]; then
    ev migration_history_rewritten "$f" "$(printf '%s' "$row" | cut -f2)" "$(printf '%s' "$row" | cut -f3)"
  else
    ev migration_history_rewritten "$f" "" "$(awk -F'\t' -v w="$f" '$2 == w { print $1; exit }' "$TMP/status.tsv")"
  fi
done

# --- 3. schema_changed_without_migration --------------------------------------
SCHEMA_FILE_RE='(^|/)(internal/db/schema[.]sql|db/schema[.]sql)$'
if [ ! -s "$ADDED_MIGRATIONS" ]; then
  changed_files | filter "$SCHEMA_FILE_RE" | while IFS= read -r f; do
    [ -n "$f" ] || continue
    row="$(first_change "$f")"
    ev schema_changed_without_migration "$f" \
      "$(printf '%s' "$row" | cut -f2)" "$(printf '%s' "$row" | cut -f3)"
  done
fi

# --- 4. dependency_manifest_changed -------------------------------------------
if changed_files | grep -qE '(^|/)go\.(mod|sum)$'; then
  if changed_files | grep -qE '(^|/)go\.mod$'; then
    added_matching '(^|/)go[.]mod$' '^[[:space:]]*(require|replace)[[:space:]]' | while IFS= read -r row; do
      ev dependency_manifest_changed "$(printf '%s' "$row" | cut -f1)" \
        "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
    done
    removed_matching '(^|/)go[.]mod$' '^[[:space:]]*(require|replace)[[:space:]]' | while IFS= read -r row; do
      ev dependency_manifest_changed "$(printf '%s' "$row" | cut -f1)" \
        "$(printf '%s' "$row" | cut -f2)" "- $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
    done
    if [ ! -s "$TMP/ev/dependency_manifest_changed" ]; then
      row="$(first_change "go.mod")"
      if [ -n "$row" ]; then
        ev dependency_manifest_changed "go.mod" "$(printf '%s' "$row" | cut -f2)" "$(printf '%s' "$row" | cut -f3)"
      fi
    fi
  fi
  if changed_files | grep -qE '(^|/)go\.sum$'; then
    ev dependency_manifest_changed "go.sum" "" "changed in the same diff"
  fi
fi

# --- 5. install_execution_allowed ---------------------------------------------
# Go analogs: go:generate directives, local module replace paths
added_matching '.' '//go:generate' | while IFS= read -r row; do
  ev install_execution_allowed "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done
added_matching '(^|/)go[.]mod$' 'replace[[:space:]]+.*=>[[:space:]]+(\./|\.\./)' | while IFS= read -r row; do
  ev install_execution_allowed "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

# --- 6. test_files_deleted ----------------------------------------------------
deleted_files | while IFS= read -r f; do
  [ -n "$f" ] || continue
  case "$f" in
    *_test.go) ev test_files_deleted "$f" "" "D go-test" ;;
  esac
done

# --- 7. tests_skipped_added ---------------------------------------------------
SKIP_RE='(^|[^A-Za-z0-9_])t[.](Skip|SkipNow|Skipf)[(]'
added_matching '_test[.]go$' "$SKIP_RE" | while IFS= read -r row; do
  ev tests_skipped_added "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

# --- 8. safeguard_config_changed ----------------------------------------------
SAFEGUARD_FILES='(^|/)(\.golangci\.ya?ml|Makefile|lefthook\.ya?ml)$'
changed_files | filter "$SAFEGUARD_FILES" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  row="$(first_change "$f")"
  [ -n "$row" ] || continue
  ev safeguard_config_changed "$f" "$(printf '%s' "$row" | cut -f2)" "$(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

# --- 9. suppressions_added ----------------------------------------------------
SUPPRESS_RE='//[[:space:]]*nolint'
added_matching '[.]go$' "$SUPPRESS_RE" | while IFS= read -r row; do
  ev suppressions_added "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

# --- 10. workflow_changed -----------------------------------------------------
WORKFLOW_RE='^[.]github/workflows/'
changed_files | filter "$WORKFLOW_RE" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  st="$(awk -F'\t' -v w="$f" '$2 == w || $3 == w { print $1; exit }' "$TMP/status.tsv")"
  ev workflow_changed "$f" "" "git status $st"
done

WORKFLOW_ADDED_RE='pull_request_target|permissions:|secrets[.]|continue-on-error:[[:space:]]*true|if:[[:space:]]*false|self-hosted|github[.]event[.]'
added_matching "$WORKFLOW_RE" "$WORKFLOW_ADDED_RE" | while IFS= read -r row; do
  ev workflow_changed "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

added_matching "$WORKFLOW_RE" '(^|[[:space:]])uses:' | while IFS= read -r row; do
  content="$(printf '%s' "$row" | cut -f3)"
  case "$content" in
    *uses:*\./*) continue ;;
  esac
  if ! printf '%s' "$content" | grep -qE '@[0-9a-f]{40}([[:space:]]|$|#)'; then
    ev workflow_changed "$(printf '%s' "$row" | cut -f1)" \
      "$(printf '%s' "$row" | cut -f2)" "+ $(printf '%s' "$content" | sed 's/^[[:space:]]*//')"
  fi
done

removed_matching "$WORKFLOW_RE" 'test|vet|lint|build' | while IFS= read -r row; do
  ev workflow_changed "$(printf '%s' "$row" | cut -f1)" \
    "$(printf '%s' "$row" | cut -f2)" "- $(printf '%s' "$row" | cut -f3 | sed 's/^[[:space:]]*//')"
done

# --- 11. stack_choice_changed --------------------------------------------------
compare_stack_field() {
  [ "$2" = "$3" ] && return
  ev stack_choice_changed "${GO_MOD_PATH:-go.mod}" "" "$1: $2 -> $3"
}
compare_stack_field go_version "$BASE_GO_VER" "$HEAD_GO_VER"
compare_stack_field module_name "$BASE_MOD_NAME" "$HEAD_MOD_NAME"
compare_stack_field framework "$BASE_FRAMEWORK" "$FRAMEWORK"
compare_stack_field linter "$BASE_LINTER" "$LINTER"

# ================================================================ diff block ===

GENERATED_RE='(^|/)(go\.sum)$|(^|/)(bin|dist|vendor)/|[.]pb[.]go$|[.]generated[.]'

DIFF_JSON="$(awk -F'\t' -v gre="$GENERATED_RE" '
  {
    ins = ($1 == "-" ? 0 : $1 + 0)
    del = ($2 == "-" ? 0 : $2 + 0)
    path = $3
    files++
    insertions += ins
    deletions += del
    generated = (path ~ gre)
    if (generated) { gen_files++ } else { src_files++; src_ins += ins }
    changed = ins + del
    if (changed > max_changed) { max_changed = changed; max_path = path }
    n = split(path, parts, "/")
    top = (n > 1 ? parts[1] : ".")
    if (!(top in seen)) { seen[top] = 1; dirs[++ndirs] = top }
  }
  END {
    printf "{\"files_changed\":%d,\"insertions\":%d,\"deletions\":%d,", files, insertions, deletions
    printf "\"source_files\":%d,\"generated_files\":%d,\"source_insertions\":%d,", src_files, gen_files, src_ins
    printf "\"top_level_dirs\":["
    for (i = 1; i <= ndirs; i++) { printf "%s\"%s\"", (i > 1 ? "," : ""), dirs[i] }
    printf "],"
    if (files > 0) {
      printf "\"largest_file\":{\"path\":\"%s\",\"changed\":%d}}", max_path, max_changed
    } else {
      printf "\"largest_file\":null}"
    }
  }' "$TMP/numstat.tsv")"

# =============================================================== assemble ======

SIGNALS_JSON="$(
  {
    echo '['
    first=1
    for s in $SIGNAL_IDS; do
      [ "$first" -eq 1 ] || echo ','
      first=0
      e="$(evidence_json "$s")"
      present=false
      [ -s "$TMP/ev/$s" ] && present=true
      printf '{"id":"%s","present":%s,"evidence":%s}' "$s" "$present" "$e"
    done
    echo ']'
  } | jq -c .
)"

NOTES_JSON="$(jq -R -s 'split("\n") | map(select(length > 0))' "$NOTES")"

jq -n \
  --argjson schema_version "$SCHEMA_VERSION" \
  --argjson number "$PR_NUMBER" \
  --arg title "$PR_TITLE" \
  --arg base "$BASE_REF" \
  --arg head "$HEAD_REF" \
  --arg target_kind "$TARGET_KIND" \
  --argjson issue_refs "$ISSUE_REFS" \
  --argjson ci "$CI_JSON" \
  --argjson framework "$FRAMEWORK" \
  --argjson orm "$ORM" \
  --argjson package_manager "$PACKAGE_MANAGER" \
  --argjson linter "$LINTER" \
  --argjson diff "$DIFF_JSON" \
  --argjson signals "$SIGNALS_JSON" \
  --argjson notes "$NOTES_JSON" \
  --argjson chunk "$CHUNK_JSON" \
  '{
    schema_version: $schema_version,
    pr: { number: $number, title: $title, base: $base, head: $head,
          target_kind: $target_kind, issue_refs: $issue_refs },
    ci: $ci,
    stack: { framework: $framework, orm: $orm,
             package_manager: $package_manager, linter: $linter },
    diff: $diff,
    signals: $signals,
    notes: $notes,
    chunk: $chunk
  }' >"$TMP/out.json"

if [ -n "$OUT" ]; then
  cp "$TMP/out.json" "$OUT"
  echo "pr-prescan: wrote $OUT" >&2
else
  cat "$TMP/out.json"
fi
