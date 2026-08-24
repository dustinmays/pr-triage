# SwiftBar Menu Bar Plugin

The `pr-triage.30s.sh` script is a [SwiftBar](https://github.com/swiftbar/SwiftBar) / xbar plugin providing menu bar visibility into daemon execution and active PR reviews.

## Design Split & Architectural Constraint

Per [[adr/0001-go-cli-sqlite-launchd]] and [[persistence-discipline]]:
- The Go binary (`pr-triage run`) owns all GitHub polling and writes `~/.pr-triage/status.json`.
- The SwiftBar plugin **only reads that file** on its 30-second refresh cycle.
- The plugin **never makes GitHub API calls or network requests**. This prevents redundant polling and ensures menu bar status never drifts out of sync with the daemon.

## Installation

1. Install SwiftBar (or xbar):
   ```bash
   brew install --cask swiftbar
   ```

2. Symlink the plugin script into your SwiftBar plugins directory:
   ```bash
   ln -s "$(pwd)/scripts/pr-triage.30s.sh" ~/Library/Application\ Support/SwiftBar/Plugins/
   ```

3. Refresh SwiftBar.

## Menu Bar Indicators

- `🛡️ pr-triage: watching` (Green): Daemon is actively polling registered repositories.
- `🤖 PR #<num> (<repo>)` (Yellow): Agent invocation is currently running on the specified PR.
- `⚪ pr-triage: idle / stopped` (Gray/Red): Daemon is not running or no status file is present.
