---
title: OpenCode runtime
tags: [adapters, runtime, config, opencode]
related: [[runtime-capability-table]], [[config-resolve-once]]
source: plan.md
---

How to point the daemon's review agent at the OpenCode runtime.

## Runtime selection happens in routing, not the top-level fields

The daemon selects the runtime from `routing.<tier>.runtime` in
`.pr-triage/config.yaml`. The top-level `runtime:`/`model:` config fields are
display-only — they are NOT consulted at agent invocation.

To run the routine reviewer on OpenCode, either run `init` with
`--runtime opencode --model openrouter/z-ai/glm-5.3-flash` (both flags are
pinned into `routing.routine`), or hand-edit `.pr-triage/config.yaml`:

```yaml
routing:
  routine:
    runtime: opencode
    model: openrouter/z-ai/glm-5.3-flash
    agent_def: review-agent
  high:
    runtime: escalate
    model: none
    agent_def: escalate
  critical:
    runtime: escalate
    model: none
    agent_def: human-review
```

Once you set `routing:`, include the `high`/`critical` escalate routes — an
unmapped tier hard-fails to escalate (see [[hard-fail-philosophy]]).

Verify the effective config with `pr-triage config show`.

## Model rules

- The model MUST be in `provider/model` form (e.g.
  `openrouter/z-ai/glm-5.3-flash`). The adapter rejects a slash-less model
  loudly at invocation time.
- OpenCode must be authenticated for that provider. Credentials live in
  OpenCode's own config via `opencode auth`, NOT the daemon's env. Rule of
  thumb: if `opencode run -m <model> "hi"` works in your shell, the daemon can
  use it.

## Model precedence

If `routing.<tier>.model` is set it is passed to OpenCode (via `-m`) and
overrides everything. If it is left EMPTY, the adapter omits `-m` and OpenCode
uses the model defined by the agent (or its own default). This lets an OpenCode
agent definition carry its own model while still allowing a config override.

See [[runtime-capability-table]] for OpenCode's cost/turn/budget capabilities
relative to the other adapters.
