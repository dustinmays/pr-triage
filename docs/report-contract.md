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
- `pr` (`object`, required): Pull request metadata (`number`, `title`, `base`, `head`, `target_kind`, `issue_refs`).
- `ci` (`object`, required): CI pipeline status and failing check names (`status`, `failing_checks`).
- `stack` (`object`, required): Technology stack information (`language`, `framework`, `runtime`, etc.).
- `diff` (`object`, required): Diff statistics and summary (`files_changed`, `additions`, `deletions`, `summary`).
- `signals` (`array of objects`, required): Detected architectural/risk signals.
- `notes` (`array of strings`, optional): Human/CI diagnostic notes.

### Signal Item Structure

Each element in `signals` contains:
- `id` (`string`, required): Unique signal identifier.
- `present` (`boolean`, required): Whether the signal condition was detected.
- `evidence` (`array of strings`, optional): File paths, line numbers, or snippets supporting the detection.

## Known Signal IDs

The following signal IDs are recognized in the v1 contract:

| Signal ID | Description | Typical Risk Severity |
|-----------|-------------|-----------------------|
| `schema_changed_without_migration` | Database schema files were modified without a matching migration file | Critical / High |
| `migration_history_rewritten` | Existing migration files were edited or deleted | Critical / High |
| `destructive_db_operation` | Destructive SQL statements (DROP TABLE, DROP COLUMN) detected | High |
| `auth_logic_changed` | Authentication, authorization, or security-sensitive code modified | High |
| `api_contract_break` | Public API endpoints or schemas altered in a breaking manner | Medium / High |
| `adr_modified` | Architecture Decision Records (ADRs) added, changed, or deleted | Medium |
| `config_schema_changed` | Application or infrastructure configuration schemas modified | Medium |
| `dependency_major_bump` | Major version upgrade of third-party dependencies | Medium |
