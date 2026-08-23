# Running pr-triage as a Daemon

This guide describes how to run `pr-triage` as a persistent background daemon on macOS using `launchd`.

## Overview

The `pr-triage run` command starts the foreground daemon:
- Acquires a single-instance PID lock under `~/.pr-triage/pr-triage.pid`.
- Emits periodic heartbeats to `~/.pr-triage/pr-triage.heartbeat`.
- Automatically reconciles stranded runs (`agent_running`) from previous crashes on startup.
- Drains active agent worktrees and exits cleanly on `SIGTERM` / `SIGINT`.
- Refuses to start if another instance is actively running.

## macOS launchd Setup

1. Build and install `pr-triage` binary:
   ```bash
   make build
   cp bin/pr-triage /usr/local/bin/pr-triage
   ```

2. Copy the launchd service definition:
   ```bash
   mkdir -p ~/Library/LaunchAgents
   cp deploy/com.dustinmays.pr-triage.plist ~/Library/LaunchAgents/
   ```

3. Load the launchd agent:
   ```bash
   launchctl load ~/Library/LaunchAgents/com.dustinmays.pr-triage.plist
   ```

4. Check daemon status:
   ```bash
   launchctl list | grep com.dustinmays.pr-triage
   pr-triage status
   ```

5. View logs:
   ```bash
   tail -f ~/.pr-triage/daemon.stdout.log
   tail -f ~/.pr-triage/daemon.stderr.log
   ```

6. Stop and unload daemon:
   ```bash
   launchctl unload ~/Library/LaunchAgents/com.dustinmays.pr-triage.plist
   ```
