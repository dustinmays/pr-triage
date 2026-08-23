# pr-triage

A Go CLI daemon that watches GitHub pull requests, waits for CI to finish,
ingests a pre-scan report, routes each PR by risk, runs a review agent in an
isolated git worktree, and escalates hard-fails to a human.
