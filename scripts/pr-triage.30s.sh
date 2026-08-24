#!/usr/bin/env bash
#
# SwiftBar / xbar plugin for pr-triage
#
# Displays real-time triage daemon and PR agent execution status in the macOS menu bar.
# Reads ONLY ~/.pr-triage/status.json — never makes network or GitHub API calls.
#
# <swiftbar.title>pr-triage</swiftbar.title>
# <swiftbar.desc>Automated PR triage watcher and agent status</swiftbar.desc>
# <swiftbar.author>Dustin Mays</swiftbar.author>
# <swiftbar.version>1.0</swiftbar.version>
# <swiftbar.dependencies>jq</swiftbar.dependencies>

STATUS_FILE="${HOME}/.pr-triage/status.json"
PID_FILE="${HOME}/.pr-triage/pr-triage.pid"

if [[ ! -f "$STATUS_FILE" ]]; then
    echo "🔍 pr-triage: idle"
    echo "---"
    echo "Status: No active status file found | color=gray"
    echo "Expected location: ${STATUS_FILE}"
    echo "---"
    echo "Refresh | refresh=true"
    exit 0
fi

# Helper function to extract json fields via jq or python fallback
get_json() {
    if command -v jq &>/dev/null; then
        jq -r "$1" "$STATUS_FILE" 2>/dev/null
    else
        python3 -c "import json, sys; d=json.load(open('$STATUS_FILE')); print($2)" 2>/dev/null
    fi
}

# Check if daemon is active
DAEMON_PID=""
if [[ -f "$PID_FILE" ]]; then
    DAEMON_PID=$(cat "$PID_FILE" 2>/dev/null | tr -d '[:space:]')
fi

ACTIVE_COUNT=$(get_json '.active_runs | length' "len(d.get('active_runs', []))")

# Menu Bar Header
if [[ -n "$ACTIVE_COUNT" && "$ACTIVE_COUNT" -gt 0 ]]; then
    FIRST_PR=$(get_json '.active_runs[0].pr_number' "d['active_runs'][0]['pr_number'] if d.get('active_runs') else ''")
    FIRST_REPO=$(get_json '.active_runs[0].repo_name' "d['active_runs'][0]['repo_name'] if d.get('active_runs') else ''")
    echo "🤖 PR #${FIRST_PR} (${FIRST_REPO}) | color=yellow"
else
    if [[ -n "$DAEMON_PID" ]] && kill -0 "$DAEMON_PID" 2>/dev/null; then
        echo "🛡️ pr-triage: watching | color=green"
    else
        echo "⚪ pr-triage: idle | color=gray"
    fi
fi

echo "---"
echo "pr-triage Status"

if [[ -n "$DAEMON_PID" ]] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    echo "Daemon: Running (PID ${DAEMON_PID}) | color=green"
else
    echo "Daemon: Stopped | color=red"
fi

UPDATED_AT=$(get_json '.updated_at' "d.get('updated_at', '')")
if [[ -n "$UPDATED_AT" ]]; then
    echo "Last Updated: ${UPDATED_AT} | size=11 color=gray"
fi

echo "---"
echo "Active Agent Runs (${ACTIVE_COUNT:-0})"

if [[ "$ACTIVE_COUNT" -gt 0 ]]; then
    if command -v jq &>/dev/null; then
        jq -r '.active_runs[] | "-- \(.repo_owner)/\(.repo_name)#\(.pr_number) [\(.runtime)/\(.model)] | color=yellow"' "$STATUS_FILE" 2>/dev/null
    else
        echo "-- PR active in queue"
    fi
else
    echo "-- No active runs | color=gray"
fi

echo "---"
echo "Recent PRs"
if command -v jq &>/dev/null; then
    jq -r '.recent_prs[] | "-- \(.repo_owner)/\(.repo_name)#\(.pr_number): \(.state) (\(.head_sha[0:7]))"' "$STATUS_FILE" 2>/dev/null
else
    echo "-- See status.json for full history | color=gray"
fi

echo "---"
echo "Refresh | refresh=true"
