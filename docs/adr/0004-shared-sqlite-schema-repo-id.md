---
title: Shared SQLite schema with repo_id, not table-per-repo
status: accepted
date: 2026-08-23
tags: [architecture, sqlite, multi-repo]
---

## Context

The daemon needs to track PR/run state across multiple registered repos.
A table-per-repo design was considered as an alternative to a shared
schema.

## Decision

Use a shared schema: a `repos` table plus `prs`/`runs` tables with an
indexed `repo_id` foreign key. One daemon polls every registered repo;
`init` just inserts a row into `repos`, with no per-repo DDL.

## Consequences

A per-repo dynamic-table design would mean every schema migration runs N
times instead of once, and cross-repo status queries would be awkward.
At POC scale (dozens of PRs across a handful of repos), `repo_id` costs
nothing. See [[persistence-discipline]] for the migration/WAL rules this
schema is built under.
