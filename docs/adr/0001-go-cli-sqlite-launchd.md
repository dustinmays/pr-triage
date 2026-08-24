---
title: Delivery shape - Go CLI + SQLite + launchd
status: accepted
date: 2026-08-23
tags: [architecture, delivery, go, sqlite]
---

## Context

The tool needs a long-running background watcher (poll loop, backoff,
agent invocation tracking) plus at-a-glance status visibility, with low
operational overhead for a solo/POC deployment on macOS.

## Decision

Ship as a Go CLI tool, scheduled via cron/launchd, backed by a local
SQLite store, with a SwiftBar plugin for menu-bar status. The Go binary
owns all GitHub polling and writes a small local status file; SwiftBar
only reads that file on its own refresh cycle and never makes its own
GitHub calls.

## Consequences

Go's goroutines suit the backoff/poll loop and process tracking; a single
static binary is easy to deploy via launchd. SwiftBar gives cheap status
visibility without building a full UI, but the file must remain the only
communication channel from daemon to plugin — two independent poll loops
hitting the API would go out of sync.
