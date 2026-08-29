# pr-triage

A Go CLI daemon that watches GitHub pull requests, waits for CI to finish,
ingests a pre-scan report, routes each PR by risk, runs a review agent in an
isolated git worktree, and escalates hard-fails to a human.

## Using the OpenCode runtime

The daemon selects the runtime from `routing.<tier>.runtime` in
`.pr-triage/config.yaml`. The top-level `runtime:`/`model:` config fields are
display-only — they are **not** consulted at agent invocation.

To run the routine reviewer on OpenCode, either run `init` with
`--runtime opencode --model openrouter/z-ai/glm-5.3-flash`, or hand-edit
`.pr-triage/config.yaml` to set:

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
unmapped tier hard-fails to escalate.

Verify the effective config with `pr-triage config show`.

Model rules for OpenCode:

- The model MUST be in `provider/model` form (the adapter rejects a slash-less
  model loudly).
- OpenCode must be authenticated for that provider. Credentials live in
  OpenCode's own config via `opencode auth`, NOT the daemon's env. Rule of
  thumb: if `opencode run -m <model> "hi"` works in your shell, the daemon can
  use it.

**Model precedence:** if `routing.<tier>.model` is set it is passed to OpenCode
and overrides everything; if it is left EMPTY, the adapter omits `-m` and
OpenCode uses the model defined by the agent (or its own default). This lets an
OpenCode agent definition carry its own model while still allowing a config
override.

