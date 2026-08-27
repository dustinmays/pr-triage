---
title: "Transfer-out checklist — promote to the template/seed repo"
kind: transfer-out-list
epic: 80
owner: dustinmays
updated: 2026-08-25
# A curated, owner-maintained checklist (single-writer, like STATE.md) of
# artifacts and conventions developed HERE that should be instantiated into the
# template/seed repository used to bootstrap future projects — so new projects
# start "chunk-ready" instead of re-deriving this each time. This is NOT the
# deferred/ backlog (that's "fix in pr-triage"); this is "graft into the template".
related:
  - ./STATE.md
  - ./deferred/README.md
---

# Transfer-out to the template/seed repo

Things built in pr-triage that belong in the template repository used to seed
future projects. Check an item off once it's been ported.

## Items

- [ ] **Chunk knowledge-base convention.** The whole `docs/<epic-or-chunk>/`
  structure:
  - `STATE.md` — single-writer build-state log (chunk owner writes; workers read).
  - `deferred/` — one-file-per-finding backlog (collision-free across parallel
    agents) + a generated `README.md` index. Frontmatter schema:
    `id, title, kind, severity, area, found_by, found_in, status, related`.
  - This `transfer-out.md` list itself.
  Porting this gives every new project chunk-management + institutional-memory
  scaffolding out of the box. When porting: keep the structure, README, and the
  "how to add one" instructions; strip the epic-80-specific content; generalize
  the path from `docs/epic-80/` to a template placeholder (e.g. `docs/<chunk>/`).

## Related future work (tracked in deferred/, not here)

The two skills that would operationalize this convention —
[skill-log-build-state](./deferred/skill-log-build-state.md) and
[skill-defer-finding](./deferred/skill-defer-finding.md) — and the
[chunk-setup-agent](./deferred/chunk-setup-agent.md) that scaffolds it at chunk
kickoff. Those are pr-triage/tooling work; this list is specifically about what
files/conventions to copy into the seed repo.
