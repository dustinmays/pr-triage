# pr-triage

A Go CLI daemon that watches GitHub pull requests, waits for CI to finish,
ingests a pre-scan report, routes each PR by risk, runs a review agent in an
isolated git worktree, and escalates hard-fails to a human.

## GitHub token setup

pr-triage reads its GitHub token via `internal/auth`, in this precedence
order: `GITHUB_TOKEN` env var, then `GH_TOKEN` env var, then the OS keyring.
Store one in the keyring with:

```bash
pr-triage setup --token <your-token>
# or, to enter it hidden instead of on the command line:
pr-triage setup
```

### Fine-grained personal access token permissions

Create the token at <https://github.com/settings/personal-access-tokens/new>,
scoped to the specific repo(s) pr-triage will watch, with these **repository
permissions**:

| Permission     | Access         | Why                                                              |
| -------------- | -------------- | ----------------------------------------------------------------- |
| Pull requests  | Read and write | List/get PRs being triaged                                       |
| Issues         | Read and write | PR comments and labels go through the Issues API under the hood  |
| Contents       | Read and write | The review agent commits and pushes fixes in its worktree        |
| Checks         | Read           | Polls check-run status to know when CI has finished              |
| Metadata       | Read           | Mandatory baseline permission; included automatically            |

`Commit statuses`, `Actions`, and `Workflows` aren't used by any pr-triage
code path and can be left at "No access".

**Gotcha:** `Commit statuses` and `Checks` are two different, unrelated
permissions — `Commit statuses` gates the legacy Status API, `Checks` gates
the check-runs API pr-triage actually polls
(`GET /repos/{owner}/{repo}/commits/{sha}/check-runs`). A token with
`Commit statuses` (or `Actions`) granted but not `Checks` authenticates fine,
lists PRs fine, and still gets a 403 on every check-run poll. Because that
403 was, until recently, swallowed silently, the PR just sits in `ci_running`
forever even though CI is green on GitHub — indistinguishable from a broken
token or a daemon that isn't polling at all. `pr-triage run`'s startup banner
prints the authenticated identity and rate limit for a quick sanity check,
but the only way to confirm `Checks` access specifically is to hit the
check-runs endpoint directly:

```bash
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $GITHUB_TOKEN" \
  https://api.github.com/repos/<owner>/<repo>/commits/<any-recent-sha>/check-runs
```

A `200` confirms `Checks` access; a `403` means it's missing from the token's
repository permissions (fine-grained tokens on an org repo may also need an
org owner to approve the permission change before it takes effect).

**Gotcha:** a read-only token causes `CreateComment` to fail with a 401 that
can go unnoticed if you're not watching the daemon's stderr — if review
comments or labels silently stop appearing, check that the token actually has
*write* access on Pull requests/Issues/Contents, not just read.

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

