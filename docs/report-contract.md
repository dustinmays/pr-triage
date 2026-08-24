# Report Contract & Signal Registry

This document defines the schema contract and recognized signal IDs for CI/CD pre-scan JSON reports consumed by `pr-triage`.

## Overview

The pre-scan report is generated upstream in CI/CD (where full build context and toolchains are available) and ingested by the `pr-triage` daemon upon receiving a `report_ready` trigger.

Per [[adr/0002-report-stays-in-cicd]] and [[hard-fail-philosophy]]:
- Reports declare an explicit `schema_version`.
- An unknown/unsupported `schema_version` or a malformed report is a **hard fail** that escalates immediately to a human.
- Reports do **not** contain a `risk_tier` field; risk tiers are computed downstream by matching present signals against daemon configuration.

## Schema Specification (v1)

Schema definition is located at `schema/report.schema.json`.

### Top-Level Fields

- `schema_version` (`integer`, required): Must be `1` for v1 reports.
- `pr` (`object`, required): Pull request metadata:
  - `number` (`integer`, required): Pull request number.
  - `title` (`string`, required): Pull request title.
  - `base` (`string`, required): Base branch name (e.g. `main`, `chunk/feature-xyz`).
  - `head` (`string`, required): Head branch name.
  - `target_kind` (`string`, optional): Classification of PR target (`implementation` or `chunk_completion`).
  - `issue_refs` (`array of integers`, optional): Linked issue numbers closed or referenced by the PR (e.g. `[123, 456]`).
- `ci` (`object`, required): CI pipeline status and failing check names:
  - `status` (`string`, required): One of `"none"`, `"failing"`, `"pending"`, or `"passing"`.
  - `failing_checks` (`array of strings`, optional): Names of failing CI checks.
- `stack` (`object`, required): Technology stack information (`framework`, `orm`, `package_manager`, `linter`, etc.; values may be `null` or strings).
- `diff` (`object`, required): Diff statistics:
  - `files_changed` (`integer`): Total number of files changed.
  - `insertions` (`integer`): Total line insertions across all changed files.
  - `deletions` (`integer`): Total line deletions across all changed files.
  - `source_files` (`integer`): Number of non-generated source files changed.
  - `generated_files` (`integer`): Number of generated/lock files changed.
  - `source_insertions` (`integer`): Line insertions in source files.
  - `top_level_dirs` (`array of strings`): Top-level directories touched.
  - `largest_file` (`object` or `null`): `{ path: string, changed: integer }` representing largest modified file.
  - `summary` (`string`, optional): Narrative summary if provided.
- `signals` (`array of objects`, required): Detected architectural/risk signals (all 11 signals always appear).
- `notes` (`array of strings`, optional): Diagnostic or scanner notes.
- `chunk` (`object` or `null`, optional): Present on `chunk_completion` PRs, containing:
  - `branch` (`string`): The chunk branch name.
  - `merged_prs` (`array of objects`): List of PRs merged into the chunk, each containing `number`, `title`, `issue_refs` (integers), and `needs_owner_review` (`boolean`).

### Signal Item Structure

Each element in `signals` contains:
- `id` (`string`, required): Unique signal identifier.
- `present` (`boolean`, required): Whether the signal condition was detected.
- `evidence` (`array of objects`, optional): Evidence details supporting detection:
  - `file` (`string`): File path where evidence was found.
  - `line` (`integer` or `null`): Line number where evidence was found, or null for file-level facts.
  - `detail` (`string`): Diff snippet or explanation.

## Known Signal IDs

The following 11 signal IDs form the standard deterministic pre-scan contract:

| Signal ID | Description | Typical Action / Risk |
|-----------|-------------|-----------------------|
| `migration_sql_added` | Database migration SQL file was added | Audit / Escalate |
| `migration_history_rewritten` | Existing migration files or journals were edited or deleted | Escalate / Hard gate |
| `schema_changed_without_migration` | Database schema files were modified without a matching migration file | Escalate / Hard gate |
| `dependency_manifest_changed` | Dependency manifests (`package.json`, `go.mod`, etc.) added, removed, or dependencies changed | Audit / Escalate |
| `install_execution_allowed` | Package manager install-time script execution or build allowlists modified | Escalate |
| `test_files_deleted` | Test specification files were deleted | Escalate |
| `tests_skipped_added` | Test skips or skip directives (`t.Skip`, `.skip`, etc.) introduced | Escalate |
| `safeguard_config_changed` | Linter, compiler, or test runner configuration files modified | Escalate |
| `suppressions_added` | Linter/type suppression comments (`//nolint`, `@ts-ignore`, etc.) added | Escalate |
| `workflow_changed` | CI/CD workflow files (`.github/workflows/*`) added, removed, or modified | Escalate |
| `stack_choice_changed` | Fundamental framework, ORM, package manager, or linter stack choice altered | Escalate |
