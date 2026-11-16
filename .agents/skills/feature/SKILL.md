---
name: feature
description: >-
  Full F0-F8 DEV-KIT Feature Development workflow for designing, reviewing, planning, and implementing new capabilities, modules, or integrations on an existing repository. Use this skill when the user asks to add a new feature, build a new user-facing flow or API capability, or run a feature readiness review.
---

# DEV-KIT Feature Workflow (F0–F8)

Canonical references:
- Guide & Workflow: [DEV-KIT.md](../../kits/DEV-KIT/DEV-KIT.md) · [workflows/feature.md](../../kits/DEV-KIT/workflows/feature.md)
- Prompts: [feature-start.md](../../kits/DEV-KIT/prompts/feature-start.md) · [feature-review.md](../../kits/DEV-KIT/prompts/feature-review.md)
- Templates: [impact-matrix.md](../../kits/DEV-KIT/templates/impact-matrix.md) · [feature-spec.md](../../kits/DEV-KIT/templates/feature-spec.md) · [contract-delta.md](../../kits/DEV-KIT/templates/contract-delta.md) · [readiness-review.md](../../kits/DEV-KIT/templates/readiness-review.md)

## Pre-Coding Phases (Read-Only by Default)

### F0 — Baseline Snapshot
- Inspect `git status`, branch, `AGENTS.md`, canonical `docs/`, `docs/plans/README.md`, and `scripts/plan.mjs` availability/help.
- Locate relevant routes, domain services, schemas, migrations, UI entrypoints, and tests. Cite file paths and symbols/lines. Flag any doc-vs-code mismatches.
- If `docs/plans/` or `scripts/plan.mjs` is missing, report the blocker and propose a compatible plan setup instead of inventing a shadow tracker.

### F1 — Feature Intake
- Clarify actor/role, user outcome, entry points, success signal, V1 scope, explicit exclusions, security/privacy/tenancy boundaries, and target environments.
- Confirm the request is truly a feature (route to `bugfix` if fixing broken existing behavior).

### F2 — Impact Analysis
- Complete the structure in [impact-matrix.md](../../kits/DEV-KIT/templates/impact-matrix.md) across domain/invariants, auth/multi-tenancy, API/events, DB/migrations, UI states, integrations/workers, observability, and tests/ops (`none | additive | modifying | breaking`).
- Identify existing modules/schemas to reuse without violating bounded-context boundaries.

### F3 — Decisions & Adversarial Scenarios
- Check locked `D*` and `ADR-*` records first; never re-ask answered decisions.
- For genuinely unresolved business decisions, ask in Vietnamese (when conversing in Vietnamese) with 2–3 concrete options, trade-offs, and a recommendation. Wait for owner selection.
- Stress-test duplicates, retries, interrupted flows, revoked permissions, concurrency, bad inputs, external failures, and rollback.

### F4 — Specification & Contract Delta
- Produce tailored [feature-spec.md](../../kits/DEV-KIT/templates/feature-spec.md) and [contract-delta.md](../../kits/DEV-KIT/templates/contract-delta.md) referencing canonical schema/contract files.
- Define rollout order, backwards compatibility for existing clients, and rollback/recovery strategy. Do not modify protected canonical docs before authorization.

### F5 — DOCS-KIT Plan Creation
- After inspecting actual `scripts/plan.mjs` usage, create the plan (`node scripts/plan.mjs new <slug> "<Plan name>"`) and populate `plan.md` with goal, scope, decision IDs, affected docs, small reviewable tasks (`Read first`, allowed files, steps, tests, rollback, observable DoD), and dependencies.
- Follow the project's `sync` flow and pause for explicit owner approval before `approve` or coding.

### F6 — Pre-Implementation Readiness Review
- Audit requirement → design → contract → task → test traceability using [readiness-review.md](../../kits/DEV-KIT/templates/readiness-review.md) (and [feature-review.md](../../kits/DEV-KIT/prompts/feature-review.md) when performing an independent review).
- Classify findings (`P0`–`P3`; confirmed defect / risk / missing decision / assumption) and issue `READY`, `CONDITIONALLY READY`, or `NOT READY`.
- **Hard stop:** Do not write implementation code until the owner approves the plan and readiness is satisfied.

## Execution & Closure Phases (Authorized Only)

### F7 — Controlled Implementation
- Execute tasks in approved dependency order: start task → implement minimal cohesive code + positive/negative/boundary/regression tests → run checks → record actual commands and exit codes.
- Never weaken/skip tests, hand-edit generated plan state (`progress.md`, `INDEX.md`, `log.jsonl`), expand scope silently, or commit/push/deploy without authorization.

### F8 — Docs, Regression & Close
- Synchronize affected canonical docs (`docs/01`..`docs/10`) and `docs/CHANGELOG.md` via the repository's authorized workflow.
- Run `node scripts/check-docs.mjs --strict`, `node scripts/plan.mjs check`, and project test suites when present (explicitly report `not available` if absent).
- Verify all items in the Feature DoD checklist in [workflows/feature.md](../../kits/DEV-KIT/workflows/feature.md) and close the plan via the native CLI.
