---
name: migration
description: >-
  DEV-KIT Migration workflow for database schema changes, data backfills, runtime upgrades, or third-party provider transitions. Use this skill when the user asks to migrate schemas, backfill or transform persisted data, upgrade runtimes/frameworks, or switch external providers.
---

# DEV-KIT Migration Workflow

Canonical references: [workflows/migration.md](../../kits/DEV-KIT/workflows/migration.md) · [contract-delta.md](../../kits/DEV-KIT/templates/contract-delta.md)

## Procedure

1. **Discovery & Compatibility Baseline (Read-Only):**
   - Inspect `AGENTS.md`, `docs/03-database-schema.md`, `docs/08-operations.md`, current schema/migrations, read/write code paths, data volume assumptions, and active consumers.
2. **Owner-Approved Change Contract:**
   - Document and obtain explicit owner approval for: dual-read/dual-write or transition behavior, compatibility with old/new clients or workers, consistency guarantees, and downtime tolerance.
3. **Design Safe Transition Sequence:**
   - Default to **expand → backfill → verify → switch → contract** (or explicitly justify an alternative strategy for the project's scale/deployment mode).
   - Specify idempotence, batching, retry/checkpointing, verification counts/checksums, observability metrics/logs, and failure recovery.
4. **Explicit Rollback & Recovery Evaluation:**
   - Distinguish truly reversible steps from destructive steps that require snapshot restore or forward-fix. Never claim automatic rollback if a destructive drop/overwrite is involved.
5. **Verification & Authorization Gate:**
   - Integrate tasks into `docs/plans/` (`scripts/plan.mjs`) when present.
   - Test forward migration, backfill idempotence, and rollback/recovery on local or anonymized representative data.
   - Never run migrations against production or shared environments without explicit owner authorization.
6. **Documentation Sync:**
   - Reconcile `docs/03-database-schema.md`, `docs/08-operations.md`, `docs/CHANGELOG.md`, and canonical schema files; report commands, checksums/test results, and exit codes.
